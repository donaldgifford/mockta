package handlers

import (
	"log/slog"
	"net/http"

	"github.com/donaldgifford/mockta/internal/gaps"
	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/oktaerr"
)

// NewNotImplemented returns a catch-all handler for every API path
// the v0 implementation doesn't cover. It resolves the gap ID via
// the registry, surfaces it both in the response body (as the error
// code) and as a response header so the audit middleware can record
// it. Uncatalogued hits log a warning so the triage process can
// promote them to a real registry entry.
func NewNotImplemented(registry gaps.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gapID, known := registry.Lookup(r.Method, r.URL.Path)
		if !known {
			slog.Default().Warn("uncatalogued gap",
				"method", r.Method, "path", r.URL.Path)
		}
		// Surface the gap ID to the audit middleware via header.
		w.Header().Set(middleware.GapHeader, gapID)

		oktaerr.Write(w, http.StatusNotImplemented,
			gapID,
			"This endpoint is not implemented by mockta. See the gap list for status.",
			oktaerr.Cause{
				Summary: r.Method + " " + r.URL.Path + " is not yet implemented",
			})
	})
}
