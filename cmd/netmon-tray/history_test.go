package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/adapter/export"
	"github.com/Snakeblack/netmon-tray/internal/adapter/storage"
	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

type stopAfterProbe struct{ cancel context.CancelFunc }

func (p stopAfterProbe) Probe(context.Context) domain.Sample {
	p.cancel()
	return domain.Sample{At: time.Now(), Link: true, Router: true, Internet: true, DNS: true}
}

type discard struct{}

func (discard) Publish(app.Status) {}

func outageEndedAgo(ago time.Duration, cause domain.Cause) domain.Outage {
	end := time.Now().Add(-ago)
	start := end.Add(-8 * time.Second)
	return domain.Outage{ID: start.UnixNano(), Start: start, End: end, LastSeen: end, Cause: cause}
}

// ADR 0003 fitness: the history survives a restart, rotates at 60 days and
// is exported from the store.
func TestHistorySurvivesRestartRotatesAndExports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	ctx := context.Background()

	first, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []domain.Outage{
		outageEndedAgo(61*24*time.Hour, domain.CauseLink),
		outageEndedAgo(59*24*time.Hour, domain.CauseRouter),
		outageEndedAgo(time.Hour, domain.CauseLine),
	} {
		if err := first.Save(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	first.Close() // restart

	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tracker := domain.NewTracker(domain.TrackerConfig{FailThreshold: failThreshold, MergeGap: mergeGap})
	engine := app.NewEngine(stopAfterProbe{cancel}, store, discard{}, tracker, time.Hour, log,
		app.WithRetention(retention), app.WithExporter(export.New(filepath.Join(dir, "exports"))))
	engine.Run(runCtx) // purges on start, then stops after its first probe

	paths, err := engine.Export(ctx)
	if err != nil || len(paths) != 2 {
		t.Fatalf("Export() = (%v, %v), want the csv and json files", paths, err)
	}
	csv, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(csv)), "\n")
	if len(lines) != 3 { // header + the two outages inside the retention
		t.Fatalf("csv has %d lines, want header and 2 outages:\n%s", len(lines), csv)
	}
	if strings.Contains(string(csv), ",link,") {
		t.Fatalf("the outage past the 60 days was exported:\n%s", csv)
	}
}
