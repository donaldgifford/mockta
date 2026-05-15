package contract

import (
	"net/http"
	"testing"
)

// TestContract_GroupMembership covers the okta_group_membership
// resource: add (idempotent) → list members → remove.
func TestContract_GroupMembership(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	// Setup: create one group and one user.
	uResp := h.Do(ctx, t, http.MethodPost, "/api/v1/users", map[string]any{
		"profile": map[string]string{
			"login": "member@contract.example",
			"email": "member@contract.example",
		},
	})
	ExpectStatus(t, uResp, http.StatusOK)
	var user struct {
		ID string `json:"id"`
	}
	DecodeJSON(t, uResp, &user)

	gResp := h.Do(ctx, t, http.MethodPost, "/api/v1/groups", map[string]any{
		"profile": map[string]any{"name": "members-grp"},
	})
	ExpectStatus(t, gResp, http.StatusOK)
	var group struct {
		ID string `json:"id"`
	}
	DecodeJSON(t, gResp, &group)

	mPath := "/api/v1/groups/" + group.ID + "/users/" + user.ID

	// Add (idempotent — call twice).
	for i := 0; i < 2; i++ {
		r := h.Do(ctx, t, http.MethodPut, mPath, nil)
		ExpectStatus(t, r, http.StatusNoContent)
		_ = r.Body.Close()
	}

	// List members of the group.
	resp := h.Do(ctx, t, http.MethodGet,
		"/api/v1/groups/"+group.ID+"/users", nil)
	ExpectStatus(t, resp, http.StatusOK)
	var members []struct {
		ID string `json:"id"`
	}
	DecodeJSON(t, resp, &members)
	if len(members) != 1 || members[0].ID != user.ID {
		t.Errorf("members = %+v, want [%s]", members, user.ID)
	}

	// Provider always expects the Link header on list responses.
	if resp.Header.Get("Link") == "" {
		t.Error("missing Link header on members list")
	}

	// Remove.
	rmResp := h.Do(ctx, t, http.MethodDelete, mPath, nil)
	ExpectStatus(t, rmResp, http.StatusNoContent)
	_ = rmResp.Body.Close()
}

func TestContract_GroupMembership_UnknownGroup(t *testing.T) {
	h := Start(t)
	resp := h.Do(t.Context(), t, http.MethodPut,
		"/api/v1/groups/nope/users/nope", nil)
	ExpectStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}
