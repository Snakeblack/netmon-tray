// Package logging builds the structured local logger (no external sinks).
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger; systemd --user captures it from stderr.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil))
}
