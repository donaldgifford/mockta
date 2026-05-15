package cli

import (
	"io"
	"log/slog"
)

// newLogger returns a structured slog logger writing JSON to w at the
// given level. Stderr is the conventional destination so stdout stays
// free for command output.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
