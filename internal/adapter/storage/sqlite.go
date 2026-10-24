// Package storage keeps the outage history in a local SQLite file. Only the
// engine process opens it (ADR 0003).
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/adapter/filelock"
	"github.com/Snakeblack/netmon-tray/internal/domain"

	_ "modernc.org/sqlite" // pure Go driver, registers "sqlite"
)

// migrations[i] upgrades the schema from version i to i+1; the file records
// its version in PRAGMA user_version.
var migrations = []string{
	`CREATE TABLE outages (
		id         INTEGER PRIMARY KEY,
		started_at INTEGER NOT NULL,
		ended_at   INTEGER,
		last_seen  INTEGER NOT NULL,
		cause      TEXT    NOT NULL
	);
	CREATE INDEX idx_outages_started ON outages(started_at);`,
	// Retention and export filter on the end of the outage.
	`CREATE INDEX idx_outages_ended ON outages(ended_at);`,
}

// ErrInUse means another engine holds the history.
var ErrInUse = errors.New("ya hay un netmon-tray en marcha con este historial")

// Store is the outage history.
type Store struct {
	db   *sql.DB
	lock *os.File // held while the store is open (ADR 0003)
}

// Open opens or creates the history at path, migrates its schema and closes
// outages left open by a previous process at the last instant it saw them.
// While another engine holds the history it fails with ErrInUse and touches
// nothing.
func Open(path string) (s *Store, err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	// MkdirAll leaves an existing directory as it was.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("restrict history directory: %w", err)
	}
	// Taken before the file is touched: a second engine would otherwise close
	// the outage the first one still has open.
	lock, err := filelock.TryLock(path + ".lock")
	if errors.Is(err, filelock.ErrLocked) {
		return nil, ErrInUse
	}
	if err != nil {
		return nil, fmt.Errorf("lock history: %w", err)
	}
	defer func() {
		if err != nil {
			lock.Close()
		}
	}()
	// SQLite gives the -wal and -shm files the mode of the main file, so the
	// file must be private before the driver first touches it.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create history file: %w", err)
	}
	f.Close()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	db.SetMaxOpenConns(1)

	s = &Store{db: db, lock: lock}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`UPDATE outages SET ended_at = last_seen WHERE ended_at IS NULL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("close dangling outages: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("history schema version %d is newer than this binary supports (%d)", version, len(migrations))
	}
	for ; version < len(migrations); version++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version+1, err)
		}
		if _, err := tx.Exec(migrations[version]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to schema %d: %w", version+1, err)
		}
		// PRAGMA does not accept bound parameters.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record schema %d: %w", version+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version+1, err)
		}
	}
	return nil
}

// Close releases the history file and its lock.
func (s *Store) Close() error {
	err := s.db.Close()
	s.lock.Close()
	return err
}

// Save inserts the outage or updates it when its ID is already stored.
func (s *Store) Save(ctx context.Context, o domain.Outage) error {
	var ended any
	if !o.Open() {
		ended = o.End.UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO outages (id, started_at, ended_at, last_seen, cause)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at  = excluded.ended_at,
			last_seen = excluded.last_seen,
			cause     = excluded.cause`,
		o.ID, o.Start.UnixMilli(), ended, o.LastSeen.UnixMilli(), string(o.Cause))
	if err != nil {
		return fmt.Errorf("save outage: %w", err)
	}
	return nil
}

// Last returns the most recent outage, or nil when there is none.
func (s *Store) Last(ctx context.Context) (*domain.Outage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, started_at, ended_at, last_seen, cause
		FROM outages ORDER BY started_at DESC LIMIT 1`)
	o, err := scanOutage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read last outage: %w", err)
	}
	return &o, nil
}

// List returns every stored outage, oldest first.
func (s *Store) List(ctx context.Context) ([]domain.Outage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, started_at, ended_at, last_seen, cause
		FROM outages ORDER BY started_at`)
	if err != nil {
		return nil, fmt.Errorf("list outages: %w", err)
	}
	defer rows.Close()

	var outages []domain.Outage
	for rows.Next() {
		o, err := scanOutage(rows)
		if err != nil {
			return nil, fmt.Errorf("list outages: %w", err)
		}
		outages = append(outages, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list outages: %w", err)
	}
	return outages, nil
}

// Purge deletes the outages that ended before the given instant and returns
// how many it removed. Open outages are never deleted.
func (s *Store) Purge(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM outages WHERE ended_at < ?`, before.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("purge outages: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge outages: %w", err)
	}
	return n, nil
}

// scanner is the part of *sql.Row and *sql.Rows that scanOutage needs.
type scanner interface {
	Scan(dest ...any) error
}

func scanOutage(r scanner) (domain.Outage, error) {
	var (
		id, started, lastSeen int64
		ended                 sql.NullInt64
		cause                 string
	)
	if err := r.Scan(&id, &started, &ended, &lastSeen, &cause); err != nil {
		return domain.Outage{}, err
	}
	o := domain.Outage{
		ID:       id,
		Start:    time.UnixMilli(started),
		LastSeen: time.UnixMilli(lastSeen),
		Cause:    domain.Cause(cause),
	}
	if ended.Valid {
		o.End = time.UnixMilli(ended.Int64)
	}
	return o, nil
}
