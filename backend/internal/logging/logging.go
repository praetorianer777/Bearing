// Package logging makes the JSON logger every process writes with.
package logging

import (
	"log/slog"
	"os"
)

// New logs JSON lines to stdout at level, one of debug, info, warn and error.
func New(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
