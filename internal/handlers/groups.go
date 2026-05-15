package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/donaldgifford/mockta/internal/gaps"
	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/oktaerr"
	"github.com/donaldgifford/mockta/internal/store"
)

// Group type values per Okta. v0 only implements OKTA_GROUP — the
// other two land in the gap registry and get a 501.
const (
	groupTypeOkta    = "OKTA_GROUP"
	groupTypeApp     = "APP_GROUP"
	groupTypeBuiltin = "BUILT_IN"
)

type groupRequest struct {
	Profile groupProfile `json:"profile"`
	Type    string       `json:"type"`
	// Raw lets us round-trip the entire profile back to the caller
	// without dropping fields the provider sends but we don't model.
	Raw json.RawMessage `json:"-"`
}

type groupProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type groupResponse struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Created     time.Time       `json:"created"`
	LastUpdated time.Time       `json:"lastUpdated"`
	Profile     json.RawMessage `json:"profile"`
}

// decodeGroupRequest parses the inbound payload twice — once into the
// typed struct, once as RawMessage on the profile field — so we keep
// the original bytes for round-tripping.
func decodeGroupRequest(w http.ResponseWriter, r *http.Request) (groupRequest, bool) {
	var raw struct {
		Profile json.RawMessage `json:"profile"`
		Type    string          `json:"type"`
	}
	if !decodeJSONBodyLenient(w, r, &raw) {
		return groupRequest{}, false
	}
	out := groupRequest{Type: raw.Type, Raw: raw.Profile}
	if len(raw.Profile) > 0 {
		if err := json.Unmarshal(raw.Profile, &out.Profile); err != nil {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: profile — "+err.Error())
			return groupRequest{}, false
		}
	}
	return out, true
}

// validateGroupType emits a 501 with the right gap ID for the
// out-of-scope types and a 400 for an unrecognized value. Empty
// defaults to OKTA_GROUP for compatibility with the provider's
// minimal payloads.
func validateGroupType(w http.ResponseWriter, t string) (string, bool) {
	switch t {
	case "", groupTypeOkta:
		return groupTypeOkta, true
	case groupTypeApp:
		w.Header().Set(middleware.GapHeader, gaps.IDGroupTypeAppGroup)
		oktaerr.Write(w, http.StatusNotImplemented,
			gaps.IDGroupTypeAppGroup,
			"APP_GROUP is not implemented by mockta")
		return "", false
	case groupTypeBuiltin:
		w.Header().Set(middleware.GapHeader, gaps.IDGroupTypeBuiltIn)
		oktaerr.Write(w, http.StatusNotImplemented,
			gaps.IDGroupTypeBuiltIn,
			"BUILT_IN groups are not implemented by mockta")
		return "", false
	default:
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: type — unsupported value")
		return "", false
	}
}

// NewGroupsCreate handles POST /api/v1/groups. Name is required (the
// provider always sends one); we use it as both the unique key and
// the seed for the deterministic ID.
func NewGroupsCreate(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeGroupRequest(w, r)
		if !ok {
			return
		}
		groupType, ok := validateGroupType(w, req.Type)
		if !ok {
			return
		}
		if req.Profile.Name == "" {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: profile.name is required",
				oktaerr.Cause{Summary: "profile.name: is required"})
			return
		}

		now := time.Now().UTC()
		g := &store.Group{
			ID:          store.NewID(store.KindGroup, req.Profile.Name),
			Name:        req.Profile.Name,
			Type:        groupType,
			Description: req.Profile.Description,
			Profile:     req.Raw,
			CreatedAt:   now,
		}
		if err := s.CreateGroup(g); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toGroupResponse(g))
	})
}

// NewGroupsGet handles GET /api/v1/groups/{id}.
func NewGroupsGet(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g, err := s.GetGroup(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toGroupResponse(g))
	})
}

// NewGroupsUpdate handles PUT /api/v1/groups/{id}.
func NewGroupsUpdate(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		existing, err := s.GetGroup(r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		req, ok := decodeGroupRequest(w, r)
		if !ok {
			return
		}
		// Type is immutable in Okta; reject mismatch attempts.
		if req.Type != "" && req.Type != existing.Type {
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: type is immutable")
			return
		}
		if req.Profile.Name == "" {
			req.Profile.Name = existing.Name
		}
		updated := &store.Group{
			ID:          existing.ID,
			Name:        req.Profile.Name,
			Type:        existing.Type,
			Description: req.Profile.Description,
			Profile:     req.Raw,
			CreatedAt:   existing.CreatedAt,
		}
		if err := s.UpdateGroup(updated); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, toGroupResponse(updated))
	})
}

// NewGroupsDelete handles DELETE /api/v1/groups/{id}.
func NewGroupsDelete(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.DeleteGroup(r.PathValue("id")); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NewGroupsList handles GET /api/v1/groups[?q=...&limit=...].
// Okta's `q` parameter does case-insensitive prefix search on name;
// v0 matches that semantic. Filter (the SCIM-filter parameter) is not
// implemented for groups and routes to the gap registry.
func NewGroupsList(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f := r.URL.Query().Get("filter"); f != "" {
			w.Header().Set(middleware.GapHeader, gaps.IDGroupFilter)
			oktaerr.Write(w, http.StatusBadRequest,
				oktaerr.CodeAPIValidationFailed,
				"Api validation failed: filter on groups is not implemented")
			return
		}
		q := strings.ToLower(r.URL.Query().Get("q"))
		limit := parseLimit(r.URL.Query().Get("limit"))

		all, err := s.ListGroups(0)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		out := make([]groupResponse, 0, len(all))
		for _, g := range all {
			if q != "" && !strings.HasPrefix(strings.ToLower(g.Name), q) {
				continue
			}
			out = append(out, toGroupResponse(g))
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		middleware.EmitNextLink(w, r, "")
		writeJSON(w, out)
	})
}

func toGroupResponse(g *store.Group) groupResponse {
	prof := g.Profile
	if len(prof) == 0 {
		// Construct a minimal profile so the provider always sees
		// the documented shape, even for permissive-mode rows.
		// Marshal of two static strings cannot fail; fall back to
		// an empty object if it ever does so the response stays
		// well-formed.
		b, err := json.Marshal(groupProfile{
			Name:        g.Name,
			Description: g.Description,
		})
		if err != nil {
			b = []byte(`{}`)
		}
		prof = b
	}
	return groupResponse{
		ID:          g.ID,
		Type:        g.Type,
		Created:     g.CreatedAt,
		LastUpdated: g.CreatedAt,
		Profile:     prof,
	}
}

// NewGroupMembershipAdd handles PUT /api/v1/groups/{gid}/users/{uid}.
// Idempotent — adding an existing membership succeeds silently.
func NewGroupMembershipAdd(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gid := r.PathValue("gid")
		uid := r.PathValue("uid")
		err := s.AddGroupMembership(&store.GroupMembership{
			GroupID:   gid,
			UserID:    uid,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NewGroupMembershipRemove handles DELETE /api/v1/groups/{gid}/users/{uid}.
func NewGroupMembershipRemove(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gid := r.PathValue("gid")
		uid := r.PathValue("uid")
		if err := s.RemoveGroupMembership(gid, uid); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// NewGroupMembershipList handles GET /api/v1/groups/{gid}/users.
// Returns full user payloads, not just memberships, mirroring real
// Okta. Always emits the Link header.
func NewGroupMembershipList(s *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gid := r.PathValue("gid")
		// Confirm the group exists so missing groups 404 rather
		// than silently returning [].
		if _, err := s.GetGroup(gid); err != nil {
			writeStoreError(w, err)
			return
		}
		members, err := s.ListGroupMembers(gid)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		out := make([]userResponse, 0, len(members))
		for _, m := range members {
			u, err := s.GetUser(m.UserID)
			if err != nil {
				// Membership refers to a deleted user — should be
				// impossible because DeleteUser cascades, but skip
				// rather than 500.
				continue
			}
			out = append(out, toUserResponse(u))
		}
		middleware.EmitNextLink(w, r, "")
		writeJSON(w, out)
	})
}
