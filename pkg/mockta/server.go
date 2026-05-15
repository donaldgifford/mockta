// Package mockta is the in-process embeddable form of the Okta mock.
//
// v0 ships this package because the binary uses it internally — the
// public Go API is not yet considered stable; downstream consumers
// should depend on the container image, not on this package.
package mockta

import (
	"context"
	"log/slog"

	"github.com/donaldgifford/mockta/internal/config"
)

// Server holds the resolved configuration and the dependencies a mockta
// instance needs to run. Phase 3 will add HTTP listeners and a store.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
}

// New constructs a Server from a Config. A nil logger falls back to
// slog.Default so callers can pass nil for quick scripts; production
// callers should pass a configured logger.
func New(cfg config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, logger: logger}
}

// Start runs the server until ctx is canceled. Phase 1 returns
// immediately so the serve subcommand can be exercised before the HTTP
// layer exists; Phase 3 will block on the listeners.
func (s *Server) Start(_ context.Context) error {
	s.logger.Info("server start (stub — no HTTP listeners until Phase 3)",
		"org", s.cfg.OrgName,
		"strict_mode", s.cfg.StrictMode,
	)
	return nil
}

// Stop drains and closes the server. No-op in Phase 1; the log line
// exists so the receiver isn't dead and so Phase 3's wiring has a clear
// hook to extend.
func (s *Server) Stop() error {
	s.logger.Debug("server stop (stub)")
	return nil
}
