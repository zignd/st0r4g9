// Package logging builds the application's structured logger (log/slog).
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger writing to stdout. format is "json" or "text"
// (anything else falls back to text); level is debug|info|warn|error
// (anything else falls back to info).
func New(format, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
