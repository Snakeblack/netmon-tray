package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

var base = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func outage(startSec int, endSec int, cause domain.Cause) domain.Outage {
	start := base.Add(time.Duration(startSec) * time.Second)
	o := domain.Outage{ID: start.UnixNano(), Start: start, Cause: cause, LastSeen: start}
	if endSec > 0 {
		o.End = base.Add(time.Duration(endSec) * time.Second)
		o.LastSeen = o.End
	}
	return o
}

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLastIsNilOnEmptyHistory(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "history.db"))

	got, err := s.Last(context.Background())
	if err != nil || got != nil {
		t.Fatalf("Last() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestHistorySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := outage(0, 8, domain.CauseRouter)
	if err := s.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	s.Close()

	got, err := open(t, path).Last(context.Background())
	if err != nil || got == nil {
		t.Fatalf("Last() = (%v, %v), want the saved outage", got, err)
	}
	if got.ID != want.ID || got.Cause != want.Cause || !got.Start.Equal(want.Start) || !got.End.Equal(want.End) {
		t.Fatalf("Last() = %+v, want %+v", *got, want)
	}
}

func TestSaveUpdatesTheSameOutage(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "history.db"))

	if err := s.Save(ctx, outage(0, 0, domain.CauseLine)); err != nil {
		t.Fatal(err)
	}
	closed := outage(0, 10, domain.CauseLine)
	if err := s.Save(ctx, closed); err != nil {
		t.Fatal(err)
	}

	got, err := s.Last(ctx)
	if err != nil || got == nil || got.Open() || got.Duration() != 10*time.Second {
		t.Fatalf("Last() = (%+v, %v), want one closed 10s outage", got, err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM outages`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stored outages = %d (%v), want 1", n, err)
	}
}

func TestLastReturnsMostRecentByStart(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "history.db"))
	s.Save(ctx, outage(100, 110, domain.CauseLink))
	s.Save(ctx, outage(0, 10, domain.CauseLine))

	got, _ := s.Last(ctx)
	if got == nil || got.Cause != domain.CauseLink {
		t.Fatalf("Last() = %+v, want the outage that started at 100s", got)
	}
}

func TestOpenClosesDanglingOutageAtLastSeen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, _ := Open(path)
	dangling := outage(0, 0, domain.CauseLine)
	dangling.LastSeen = base.Add(30 * time.Second)
	if err := s.Save(context.Background(), dangling); err != nil {
		t.Fatal(err)
	}
	s.Close() // the process died while the outage was open

	got, err := open(t, path).Last(context.Background())
	if err != nil || got == nil || got.Open() {
		t.Fatalf("Last() = (%+v, %v), want the outage closed", got, err)
	}
	if !got.End.Equal(dangling.LastSeen) {
		t.Fatalf("End = %v, want last seen %v", got.End, dangling.LastSeen)
	}
}

func TestOpenMigratesAnUnversionedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s := open(t, path)
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations) {
		t.Fatalf("schema version = %d (%v), want %d", version, err, len(migrations))
	}
	if err := s.Save(context.Background(), outage(0, 5, domain.CauseLink)); err != nil {
		t.Fatalf("Save() after migration: %v", err)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("Open() error = %v, want a newer-schema error", err)
	}
}

func TestHistoryIsPrivateEvenInALooseExistingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits: the ACL of %LOCALAPPDATA% keeps the history private")
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil { // independent of the umask
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	s := open(t, path)
	if err := s.Save(context.Background(), outage(0, 5, domain.CauseLink)); err != nil {
		t.Fatal(err)
	}

	// the sidecars exist while the store is open
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", filepath.Base(p), err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s permissions = %o, want no group or world access", filepath.Base(p), perm)
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory permissions = %o, want 700", perm)
	}
}

func TestOpenMigratesV1HistoryKeepingItsOutages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// the schema and one outage exactly as version 1 left them
	for _, stmt := range []string{
		migrations[0],
		`INSERT INTO outages (id, started_at, ended_at, last_seen, cause) VALUES (7, 1000, 9000, 9000, 'router')`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	s := open(t, path)

	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version = %d (%v), want 2", version, err)
	}
	var indexes int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_outages_ended'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("idx_outages_ended count = %d (%v), want 1", indexes, err)
	}
	got, err := s.List(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != 7 || got[0].Cause != domain.CauseRouter {
		t.Fatalf("List() = (%+v, %v), want the v1 outage intact", got, err)
	}
}

func TestListReturnsEveryOutageOldestFirst(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "history.db"))

	got, err := s.List(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("List() on empty history = (%+v, %v), want none", got, err)
	}

	s.Save(ctx, outage(200, 210, domain.CauseLine))
	s.Save(ctx, outage(0, 10, domain.CauseLink))
	s.Save(ctx, outage(100, 0, domain.CauseRouter)) // still open

	got, err = s.List(ctx)
	if err != nil || len(got) != 3 {
		t.Fatalf("List() = (%+v, %v), want 3 outages", got, err)
	}
	wantCauses := []domain.Cause{domain.CauseLink, domain.CauseRouter, domain.CauseLine}
	for i, want := range wantCauses {
		if got[i].Cause != want {
			t.Fatalf("List()[%d].Cause = %q, want %q (oldest first)", i, got[i].Cause, want)
		}
	}
	if !got[1].Open() {
		t.Fatalf("List()[1] = %+v, want the open outage to stay open", got[1])
	}
}

func TestPurgeDropsOnlyOutagesThatEndedBeforeTheCutoff(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "history.db"))
	s.Save(ctx, outage(0, 10, domain.CauseLink))      // ended long before the cutoff
	s.Save(ctx, outage(1000, 1100, domain.CauseLine)) // ended exactly at the cutoff
	s.Save(ctx, outage(2000, 2010, domain.CauseRouter))
	s.Save(ctx, outage(-5000, 0, domain.CauseLine)) // started before the cutoff, still open

	removed, err := s.Purge(ctx, base.Add(1100*time.Second))
	if err != nil || removed != 1 {
		t.Fatalf("Purge() = (%d, %v), want 1 removed", removed, err)
	}

	got, _ := s.List(ctx)
	if len(got) != 3 {
		t.Fatalf("List() after Purge = %+v, want 3 outages", got)
	}
	for _, o := range got {
		if o.Cause == domain.CauseLink {
			t.Fatalf("outage that ended before the cutoff survived: %+v", o)
		}
	}
}

// ADR 0003: one owner. A second engine must not open the history, and above
// all must not close the outage the first engine still has open.
func TestSecondOpenIsRefusedWhileTheFirstHoldsTheHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ongoing := outage(0, 0, domain.CauseRouter)
	if err := first.Save(context.Background(), ongoing); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if !errors.Is(err, ErrInUse) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("second Open() error = %v, want ErrInUse", err)
	}
	got, err := first.Last(context.Background())
	if err != nil || got == nil || !got.Open() {
		t.Fatalf("Last() = (%+v, %v), want the first engine's outage still open", got, err)
	}

	first.Close()
	open(t, path) // the history is free again once its owner closes it
}
