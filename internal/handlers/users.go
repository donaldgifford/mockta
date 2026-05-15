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

// Okta user lifecycle states relevant to v0. Provider creates start as
// STAGED (activate=false) or PROVISIONED (default). The lifecycle
// endpoints flip to ACTIVE / DEPROVISIONED synchronously.
const (
	userStatusStaged        = "STAGED"
	userStatusProvisioned   = "PROVISIONED"
	userStatusActive        = "ACTIVE"
	userStatusDeprovisioned = "DEPROVISIONED"
)

// userRequest is the inbound shape for create/update. Profile is held
// as RawMessage so unknown fields round-trip back unchanged — the
// provider sometimes sends computed/legacy keys we don't validate.
type userRequest struct {
	Profile json.RawMessage `json:"profile"`
	// Credentials, GroupIds, Type, etc. are accepted by the lenient
	// decoder and ignored.
}

// userProfile is the minimal subset of the inbound profile we
// validate. The full inbound JSON is preserved verbatim in the store.
type userProfile struct {
	Login     string `json:"login"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// userResponse is the Okta user payload returned for create/get/update.
type userResponse struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Created     time.Time       `json:"created"`
	Activated   *time.Time      `json:"activated,omitempty"`
	LastUpdated time.Time       `json:"lastUpdated"`
	Profile     json.RawMessage `json:"profile"`
}

// NewUsersCreate handles POST /api/v1/users.
//
// The strict flag enables required-field + format validation. The
// ?activate=true|false query parameter controls the initial status;
// per Okta, default is activate=true (PROVISIONED).
func NewUsersCreate(s *store.Store, strict bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req userRequest
		if !decodeJSONBodyLenient(w, r, &req) {
			return
		}

		profile, ok := parseAndValidateProfile(w, req.Profile, strict)
		if !ok {
			return
		}

		status := userStatusProvisioned
		if r.URL.Query().Get("activate") == "false" {
			status = userStatusStaged
		}

		now := time.Now().UTC()
		u := &store.User{
			ID:        store.NewID(store.KindUser, profile.Login),
			Login:     profile.Login,
			Email:     profile.Email,
			Status:    status,
			Profile:   req.Profile,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.CreateUser(u); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toUserResponse(u))
	})
}

// NewUsersGet handles GET /api/v1/users/{id_or_login}.
//
// Okta accepts either the user ID or the login as the path segment;
// we try ID first, then fall back to login. ServeMux gives us the raw
// segment via PathValue.
func NewUsersGet(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("idOrLogin")
		u, err := s.GetUser(key)
		if err != nil && errors.Is(err, store.ErrNotFound) {
			u, err = s.GetUserByLogin(key)
		}
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toUserResponse(u))
	})
}

// NewUsersUpdate handles PUT /api/v1/users/{id}.
//
// Full replace per Okta semantics. The status is preserved from the
// existing row — lifecycle flips happen via the dedicated endpoints,
// not through PUT.
func NewUsersUpdate(s *store.Store, strict bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, err := s.GetUser(id)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		var req userRequest
		if !decodeJSONBodyLenient(w, r, &req) {
			return
		}

		profile, ok := parseAndValidateProfile(w, req.Profile, strict)
		if !ok {
			return
		}

		updated := &store.User{
			ID:        existing.ID,
			Login:     profile.Login,
			Email:     profile.Email,
			Status:    existing.Status,
			Profile:   req.Profile,
			CreatedAt: existing.CreatedAt,
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.UpdateUser(updated); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toUserResponse(updated))
	})
}

// NewUsersActivate handles POST /api/v1/users/{id}/lifecycle/activate.
// Synchronous in v0 — no async activation token flow.
func NewUsersActivate(s *store.Store) http.Handler {
	return newUserLifecycle(s, userStatusActive)
}

// NewUsersDeactivate handles POST /api/v1/users/{id}/lifecycle/deactivate.
func NewUsersDeactivate(s *store.Store) http.Handler {
	return newUserLifecycle(s, userStatusDeprovisioned)
}

func newUserLifecycle(s *store.Store, target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		u, err := s.GetUser(id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		u.Status = target
		u.UpdatedAt = time.Now().UTC()
		if err := s.UpdateUser(u); err != nil {
			writeStoreError(w, err)
			return
		}
		// Okta lifecycle endpoints return 200 with an empty object.
		writeJSON(w, struct{}{})
	})
}

// NewUsersDelete handles DELETE /api/v1/users/{id}.
//
// Okta requires two deletes to actually destroy: the first sets the
// user to DEPROVISIONED, the second removes the row. v0 collapses
// this — DELETE always removes the row regardless of current status,
// because the provider's `okta_user` resource calls DELETE once and
// expects it to stick.
func NewUsersDelete(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := s.DeleteUser(id); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NewUsersList handles GET /api/v1/users with optional ?filter= and
// ?limit=.
//
// v0 supports SCIM-filter `eq` and `sw` against id, login (or
// profile.login), email (or profile.email), and status. Anything else
// 400s with a gap pointer. The pagination Link header is always
// emitted with an empty cursor — exercises the provider's paging code
// without actually paginating.
func NewUsersList(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		limit := parseLimit(r.URL.Query().Get("limit"))

		var pred userPredicate
		if filter != "" {
			p, err := parseUserFilter(filter)
			if err != nil {
				w.Header().Set(middleware.GapHeader, gaps.IDUserFilter)
				oktaerr.Write(w, http.StatusBadRequest,
					oktaerr.CodeAPIValidationFailed,
					"Api validation failed: filter — "+err.Error())
				return
			}
			pred = p
		}

		all, err := s.ListUsers(0)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		out := make([]userResponse, 0, len(all))
		for _, u := range all {
			if pred != nil && !pred(u) {
				continue
			}
			out = append(out, toUserResponse(u))
			if limit > 0 && len(out) >= limit {
				break
			}
		}

		middleware.EmitNextLink(w, r, "")
		writeJSON(w, out)
	})
}

// parseAndValidateProfile pulls login/email out of the inbound profile
// JSON. In strict mode it enforces required fields and a light format
// check on login. On failure it writes the 400 envelope and returns
// (zero, false); callers should return immediately.
func parseAndValidateProfile(w http.ResponseWriter, raw json.RawMessage, strict bool) (userProfile, bool) {
	var p userProfile
	if len(raw) == 0 {
		if strict {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: profile is required",
				oktaerr.Cause{Summary: "profile: is required"})
			return p, false
		}
		return p, true
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: profile — "+err.Error())
		return p, false
	}
	if !strict {
		return p, true
	}
	if p.Login == "" {
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: login is required",
			oktaerr.Cause{Summary: "login: is required"})
		return p, false
	}
	if p.Email == "" {
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: email is required",
			oktaerr.Cause{Summary: "email: is required"})
		return p, false
	}
	if !loginEmailLooksValid(p.Login) {
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: login is not a well-formed address",
			oktaerr.Cause{Summary: "login: format invalid"})
		return p, false
	}
	return p, true
}

// toUserResponse builds the Okta wire shape from the stored row.
// Profile may be nil for permissive-mode rows; emit `{}` in that case
// so the provider sees a valid JSON object.
func toUserResponse(u *store.User) userResponse {
	prof := u.Profile
	if len(prof) == 0 {
		prof = json.RawMessage("{}")
	}
	resp := userResponse{
		ID:          u.ID,
		Status:      u.Status,
		Created:     u.CreatedAt,
		LastUpdated: u.UpdatedAt,
		Profile:     prof,
	}
	if u.Status == userStatusActive ||
		u.Status == userStatusProvisioned {
		// Activated mirrors UpdatedAt at lifecycle flip time — close
		// enough for the provider, which only checks presence.
		t := u.UpdatedAt
		resp.Activated = &t
	}
	return resp
}

// userPredicate is the in-memory filter callback.
type userPredicate func(*store.User) bool

// parseUserFilter parses a small SCIM-filter subset: `<attr> eq "<v>"`
// or `<attr> sw "<v>"`. Returns an error with a human-readable hint
// for any other shape.
func parseUserFilter(s string) (userPredicate, error) {
	attr, op, value, err := splitFilter(s)
	if err != nil {
		return nil, err
	}
	get, err := userAttrGetter(attr)
	if err != nil {
		return nil, err
	}
	switch op {
	case "eq":
		return func(u *store.User) bool { return get(u) == value }, nil
	case "sw":
		return func(u *store.User) bool { return strings.HasPrefix(get(u), value) }, nil
	default:
		return nil, errors.New("unsupported operator (want eq|sw)")
	}
}

func userAttrGetter(attr string) (func(*store.User) string, error) {
	switch attr {
	case "id":
		return func(u *store.User) string { return u.ID }, nil
	case "login", "profile.login":
		return func(u *store.User) string { return u.Login }, nil
	case "email", "profile.email":
		return func(u *store.User) string { return u.Email }, nil
	case "status":
		return func(u *store.User) string { return u.Status }, nil
	default:
		return nil, errors.New("unsupported attribute (want id|login|email|status)")
	}
}

// splitFilter pulls (attribute, operator, value) from a SCIM-ish
// expression. The grammar is strict: three space-separated tokens, the
// third one quoted. Anything else is rejected so the provider gets
// fast, predictable feedback when it sends an unsupported expression.
func splitFilter(s string) (attr, op, value string, err error) {
	parts := strings.SplitN(s, " ", 3)
	if len(parts) != 3 {
		return "", "", "", errors.New("expected <attr> <op> \"<value>\"")
	}
	attr = parts[0]
	op = parts[1]
	value = parts[2]
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", "", "", errors.New("value must be double-quoted")
	}
	value = value[1 : len(value)-1]
	return attr, op, value, nil
}

// parseLimit parses ?limit=. Negative or unparseable values fall back
// to 0 (no limit). Okta's docs cap at 200; v0 doesn't enforce a max —
// the working set is small enough that it doesn't matter.
func parseLimit(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
