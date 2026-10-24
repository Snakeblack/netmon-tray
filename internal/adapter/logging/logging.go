// Package logging builds the structured local logger (no external sinks).
package logging

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// MaxFileSize is the size past which OpenFile starts a new log.
const MaxFileSize = 1 << 20

// New returns a JSON logger; systemd --user captures it from stderr.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil))
}

// OpenFile opens the log at path for appending, for systems without a journal
// to read stderr. A log past max moves to path+".old", replacing the previous
// one, so the logs take about twice max. When the move fails, as on Windows
// while another engine holds the log open, it appends to the long log rather
// than losing it.
func OpenFile(path string, max int64) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil && info.Size() > max {
		os.Rename(path, path+".old")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
