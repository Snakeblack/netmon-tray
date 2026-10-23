// Command netmon-tray monitors the network by layers and shows its state in
// the system tray.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/adapter/export"
	"github.com/Snakeblack/netmon-tray/internal/adapter/logging"
	"github.com/Snakeblack/netmon-tray/internal/adapter/probe"
	"github.com/Snakeblack/netmon-tray/internal/adapter/storage"
	"github.com/Snakeblack/netmon-tray/internal/adapter/tray"
	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

const (
	probeInterval = 2 * time.Second
	failThreshold = 3
	mergeGap      = 10 * time.Second
	retention     = 60 * 24 * time.Hour
)

func main() {
	log := logging.New(os.Stderr)

	historyPath, err := historyPath()
	if err != nil {
		log.Error("resolve history path", "err", err)
		os.Exit(1)
	}
	store, err := storage.Open(historyPath)
	if err != nil {
		log.Error("open history", "path", historyPath, "err", err)
		os.Exit(1)
	}
	defer store.Close()

	prober, err := probe.New(probe.DefaultConfig())
	if err != nil {
		log.Error("start probes", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tracker := domain.NewTracker(domain.TrackerConfig{FailThreshold: failThreshold, MergeGap: mergeGap})
	exporter := export.New(filepath.Join(filepath.Dir(historyPath), "exports"))
	// The engine needs the tray as its sink and the tray needs the engine to
	// export, so the tray calls through a closure bound once the engine exists.
	var engine *app.Engine
	t := tray.New(func(ctx context.Context) ([]string, error) {
		paths, err := engine.Export(ctx)
		if err != nil {
			log.Error("export history", "err", err)
			return nil, err
		}
		log.Info("history exported", "files", paths)
		return paths, nil
	})
	engine = app.NewEngine(prober, store, t, tracker, probeInterval, log,
		app.WithRetention(retention), app.WithExporter(exporter))

	log.Info("netmon-tray started", "history", historyPath, "interval", probeInterval, "retention", retention)
	t.Run(ctx, cancel, func(ctx context.Context) {
		if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("engine stopped", "err", err)
			cancel()
		}
	})
	log.Info("netmon-tray stopped")
}

// historyPath is $XDG_DATA_HOME/netmon-tray/history.db, or under ~/.local/share.
func historyPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "netmon-tray", "history.db"), nil
}
