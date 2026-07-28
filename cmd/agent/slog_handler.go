package main

import (
	"io"
	"log/slog"
)

// newAgentTextHandler builds the process-wide text slog handler.
// Kept in one place so the win2012 Go 1.20 builder can rewrite the call shape
// for golang.org/x/exp/slog (opts.NewTextHandler(w) vs NewTextHandler(w, opts)).
func newAgentTextHandler(w io.Writer) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
}
