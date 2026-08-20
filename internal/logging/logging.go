// Package logging configures structured logging for all processes.
// Logs must never contain passwords, session secrets, API keys, full email
// bodies or attachment content.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a slog.Logger writing to stdout. format is "json" (default) or
// "text"; level is one of debug/info/warn/error. Timestamps are UTC.
func New(component, level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	}

	var handler slog.Handler
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler).With(slog.String("component", component))
}
