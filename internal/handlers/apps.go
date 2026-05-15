package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/donaldgifford/mockta/internal/gaps"
	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/oktaerr"
	"github.com/donaldgifford/mockta/internal/store"
)

// v0 only implements SAML 2.0; the other sign-on modes route through
// the gap registry.
const (
	appSignOnSAML     = "SAML_2_0"
	appStatusActive   = "ACTIVE"
	appStatusInactive = "INACTIVE"
)

// appRequest is the minimal inbound shape. Settings is held as
// RawMessage so we round-trip the entire blob.
type appRequest struct {
	Name       string          `json:"name"`
	Label      string          `json:"label"`
	SignOnMode string          `json:"signOnMode"`
	Settings   json.RawMessage `json:"settings"`
}

type appResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Status      string          `json:"status"`
	SignOnMode  string          `json:"signOnMode"`
	Created     time.Time       `json:"created"`
	LastUpdated time.Time       `json:"lastUpdated"`
	Settings    json.RawMessage `json:"settings"`
}

// NewAppsCreate handles POST /api/v1/apps. Only SAML_2_0 is accepted;
// other sign-on modes return 501 with a gap pointer.
func NewAppsCreate(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req appRequest
		if !decodeJSONBodyLenient(w, r, &req) {
			return
		}
		if req.SignOnMode != appSignOnSAML {
			w.Header().Set(middleware.GapHeader, gaps.IDAppSignOnNonSAML)
			oktaerr.Write(w, http.StatusNotImplemented,
				gaps.IDAppSignOnNonSAML,
				"signOnMode "+req.SignOnMode+" is not implemented; v0 only supports SAML_2_0")
			return
		}
		if req.Label == "" {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: label is required",
				oktaerr.Cause{Summary: "label: is required"})
			return
		}

		now := time.Now().UTC()
		a := &store.App{
			ID:         store.NewID(store.KindApp, req.Label),
			Name:       firstNonEmpty(req.Name, req.Label),
			Label:      req.Label,
			Status:     appStatusActive,
			SignOnMode: req.SignOnMode,
			Settings:   req.Settings,
			CreatedAt:  now,
		}
		if err := s.CreateApp(a); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toAppResponse(a, now))
	})
}

// NewAppsGet handles GET /api/v1/apps/{id}.
func NewAppsGet(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := s.GetApp(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toAppResponse(a, a.CreatedAt))
	})
}

// NewAppsUpdate handles PUT /api/v1/apps/{id}.
func NewAppsUpdate(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		existing, err := s.GetApp(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		var req appRequest
		if !decodeJSONBodyLenient(w, r, &req) {
			return
		}
		// SignOnMode is immutable per Okta; reject mismatches.
		if req.SignOnMode != "" && req.SignOnMode != existing.SignOnMode {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: signOnMode is immutable")
			return
		}
		updated := &store.App{
			ID:         existing.ID,
			Name:       firstNonEmpty(req.Name, existing.Name),
			Label:      firstNonEmpty(req.Label, existing.Label),
			Status:     existing.Status,
			SignOnMode: existing.SignOnMode,
			Settings:   req.Settings,
			CreatedAt:  existing.CreatedAt,
		}
		if err := s.UpdateApp(updated); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toAppResponse(updated, time.Now().UTC()))
	})
}

// NewAppsActivate handles POST /api/v1/apps/{id}/lifecycle/activate.
func NewAppsActivate(s *store.Store) http.Handler {
	return newAppLifecycle(s, appStatusActive)
}

// NewAppsDeactivate handles POST /api/v1/apps/{id}/lifecycle/deactivate.
func NewAppsDeactivate(s *store.Store) http.Handler {
	return newAppLifecycle(s, appStatusInactive)
}

func newAppLifecycle(s *store.Store, target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := s.GetApp(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		a.Status = target
		if err := s.UpdateApp(a); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, struct{}{})
	})
}

// NewAppsDelete handles DELETE /api/v1/apps/{id}.
//
// Okta requires apps to be deactivated before delete; v0 enforces the
// same precondition so the provider's two-step destroy flow exercises
// both code paths.
func NewAppsDelete(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := s.GetApp(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if a.Status != appStatusInactive {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: app must be deactivated before delete")
			return
		}
		if err := s.DeleteApp(a.ID); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NewAppsList handles GET /api/v1/apps[?filter=...&limit=...].
// Filter supports `status eq` and `label sw`; other expressions
// route through the gap registry.
func NewAppsList(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		limit := parseLimit(r.URL.Query().Get("limit"))

		var pred appPredicate
		if filter != "" {
			p, err := parseAppFilter(filter)
			if err != nil {
				w.Header().Set(middleware.GapHeader, gaps.IDAppFilter)
				oktaerr.Write(w, http.StatusBadRequest,
					oktaerr.CodeAPIValidationFailed,
					"Api validation failed: filter — "+err.Error())
				return
			}
			pred = p
		}

		all, err := s.ListApps(0)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		out := make([]appResponse, 0, len(all))
		for _, a := range all {
			if pred != nil && !pred(a) {
				continue
			}
			out = append(out, toAppResponse(a, a.CreatedAt))
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		middleware.EmitNextLink(w, r, "")
		writeJSON(w, out)
	})
}

func toAppResponse(a *store.App, lastUpdated time.Time) appResponse {
	settings := a.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	return appResponse{
		ID:          a.ID,
		Name:        a.Name,
		Label:       a.Label,
		Status:      a.Status,
		SignOnMode:  a.SignOnMode,
		Created:     a.CreatedAt,
		LastUpdated: lastUpdated,
		Settings:    settings,
	}
}

type appPredicate func(*store.App) bool

// parseAppFilter is the apps-specific variant of the SCIM-ish filter
// grammar. Supports `status eq "<v>"` and `label sw "<v>"` — the two
// expressions the provider's list-apps data source actually emits.
func parseAppFilter(s string) (appPredicate, error) {
	attr, op, value, err := splitFilter(s)
	if err != nil {
		return nil, err
	}
	get, err := appAttrGetter(attr)
	if err != nil {
		return nil, err
	}
	switch op {
	case "eq":
		return func(a *store.App) bool { return get(a) == value }, nil
	case "sw":
		return func(a *store.App) bool { return strings.HasPrefix(get(a), value) }, nil
	default:
		return nil, errors.New("unsupported operator (want eq|sw)")
	}
}

func appAttrGetter(attr string) (func(*store.App) string, error) {
	switch attr {
	case "id":
		return func(a *store.App) string { return a.ID }, nil
	case "label":
		return func(a *store.App) string { return a.Label }, nil
	case "name":
		return func(a *store.App) string { return a.Name }, nil
	case "status":
		return func(a *store.App) string { return a.Status }, nil
	case "signOnMode":
		return func(a *store.App) string { return a.SignOnMode }, nil
	default:
		return nil, errors.New("unsupported attribute (want id|label|name|status|signOnMode)")
	}
}

// firstNonEmpty returns the first non-empty argument, or "" if both
// are empty. Used to default Name from Label when the caller omits
// Name (Okta accepts either).
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
