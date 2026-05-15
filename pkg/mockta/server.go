// Package mockta is the in-process embeddable form of the Okta mock.
//
// v0 ships this package because the binary uses it internally — the
// public Go API is not yet considered stable; downstream consumers
// should depend on the container image, not on this package.
package mockta

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/donaldgifford/mockta/internal/config"
	"github.com/donaldgifford/mockta/internal/gaps"
	"github.com/donaldgifford/mockta/internal/handlers"
	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/store"
)

// Default listen addresses. Exposed as constants so tests and the
// healthcheck subcommand can reference them by symbol.
const (
	APIAddr   = ":8080"
	AdminAddr = ":9090"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Server holds the resolved configuration and the dependencies a
// mockta instance needs to run. Construct via New; do not zero-value.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
	store  *store.Store
	gaps   gaps.Registry

	apiSrv   *http.Server
	adminSrv *http.Server
}

// New constructs a Server from a Config. A nil logger falls back to
// slog.Default so callers can pass nil for quick scripts; production
// callers should pass a configured logger.
func New(cfg config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	st, err := store.New()
	if err != nil {
		// store.New only fails on a malformed schema, which is
		// hard-coded — a failure here is a programmer error and
		// the process is unrecoverable. Log loudly and fall
		// through with a nil store; Start will detect and error.
		logger.Error("construct store", "err", err)
	}
	return &Server{
		cfg:    cfg,
		logger: logger,
		store:  st,
		gaps:   gaps.StubRegistry{},
	}
}

// Start opens the API (:8080) and admin (:9090) listeners and serves
// until ctx is canceled or either listener errors. On shutdown both
// servers get a grace period to drain in-flight requests.
func (s *Server) Start(ctx context.Context) error {
	if s.store == nil {
		return errors.New("store not initialized; see prior log for cause")
	}

	s.apiSrv = &http.Server{
		Addr:              APIAddr,
		Handler:           s.apiHandler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	s.adminSrv = &http.Server{
		Addr:              AdminAddr,
		Handler:           s.adminHandler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errs := make(chan error, 2)
	go func() {
		s.logger.Info("api listener up", "addr", APIAddr)
		errs <- s.apiSrv.ListenAndServe()
	}()
	go func() {
		s.logger.Info("admin listener up", "addr", AdminAddr)
		errs <- s.adminSrv.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		if stopErr := s.shutdownDetached(ctx); stopErr != nil && err == nil {
			err = stopErr
		}
		return err
	case <-ctx.Done():
		return s.shutdownDetached(ctx)
	}
}

// shutdownDetached drains the listeners under a fresh deadline that
// inherits ctx's values but ignores its cancellation. The parent ctx
// is typically canceled by the time we reach shutdown (that's what
// triggered it), so we need a context that stays live long enough
// for in-flight requests to drain.
func (s *Server) shutdownDetached(ctx context.Context) error {
	shutCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return s.shutdown(shutCtx)
}

// Stop drains both listeners. Provided as a convenience for callers
// that own the Server without driving it via Start's lifecycle (e.g.,
// in-process tests). Uses a bounded grace period internally.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return s.shutdown(ctx)
}

// shutdown runs the actual server.Shutdown calls under the supplied
// context. Safe to call multiple times; subsequent calls are no-ops
// because http.Server.Shutdown returns ErrServerClosed on a
// previously-shutdown server, which we swallow.
func (s *Server) shutdown(ctx context.Context) error {
	if s.apiSrv == nil && s.adminSrv == nil {
		return nil
	}
	var firstErr error
	if s.apiSrv != nil {
		if err := s.apiSrv.Shutdown(ctx); err != nil &&
			!errors.Is(err, http.ErrServerClosed) && firstErr == nil {
			firstErr = fmt.Errorf("api shutdown: %w", err)
		}
	}
	if s.adminSrv != nil {
		if err := s.adminSrv.Shutdown(ctx); err != nil &&
			!errors.Is(err, http.ErrServerClosed) && firstErr == nil {
			firstErr = fmt.Errorf("admin shutdown: %w", err)
		}
	}
	s.logger.Info("server stopped")
	return firstErr
}

// apiHandler builds the :8080 router and wraps it in auth + audit
// middleware. The catch-all 501 handler is registered last so any
// path not claimed by a real handler lands there.
func (s *Server) apiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/org", handlers.NewOrg(s.cfg.OrgName))

	// Catch-all 501. ServeMux's "/" pattern matches any path not
	// already claimed.
	mux.Handle("/", handlers.NewNotImplemented(s.gaps))

	return middleware.Chain(mux,
		middleware.Audit(s.store),
		middleware.Auth(s.cfg.AdminToken, s.cfg.StrictMode),
	)
}

// adminHandler builds the :9090 router. /health is unauthenticated;
// /admin/reset requires the admin token. Audit middleware wraps
// everything so admin traffic shows up in GapsHit / AuditByGap too.
func (s *Server) adminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", handlers.NewHealth())
	mux.Handle("POST /admin/reset",
		middleware.Auth(s.cfg.AdminToken, s.cfg.StrictMode)(
			handlers.NewAdminReset(s.store),
		),
	)
	// Anything else on the admin port is a 404 with the standard
	// envelope — not a gap, just a missing path.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return middleware.Audit(s.store)(mux)
}
