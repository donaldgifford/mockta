package contract

import (
	"net/http"
	"testing"
)

// TestContract_User walks the full okta_user resource lifecycle:
// create → read by ID → read by login → update → activate →
// deactivate → delete. Mirrors the provider's plan/apply/destroy
// path without the Terraform overhead.
func TestContract_User(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	// Create.
	resp := h.Do(ctx, t, http.MethodPost, "/api/v1/users", map[string]any{
		"profile": map[string]string{
			"login":     "alice@contract.example",
			"email":     "alice@contract.example",
			"firstName": "Alice",
			"lastName":  "Contract",
		},
	})
	ExpectStatus(t, resp, http.StatusOK)
	var created struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Profile struct {
			Login string `json:"login"`
			Email string `json:"email"`
		} `json:"profile"`
	}
	DecodeJSON(t, resp, &created)
	if created.ID == "" {
		t.Fatal("create response missing id")
	}
	if created.Status != "PROVISIONED" {
		t.Errorf("status = %q, want PROVISIONED", created.Status)
	}
	if created.Profile.Login != "alice@contract.example" {
		t.Errorf("profile.login = %q", created.Profile.Login)
	}

	// Read by ID — used by the provider's Read implementation.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/users/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Read by login — the provider uses this for data-source lookups.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/users/alice@contract.example", nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Update — full replace, status must not flip.
	resp = h.Do(ctx, t, http.MethodPut, "/api/v1/users/"+created.ID, map[string]any{
		"profile": map[string]string{
			"login":     "alice@contract.example",
			"email":     "alice2@contract.example",
			"firstName": "Alice",
			"lastName":  "Renamed",
		},
	})
	ExpectStatus(t, resp, http.StatusOK)
	var updated struct {
		Status string `json:"status"`
	}
	DecodeJSON(t, resp, &updated)
	if updated.Status != "PROVISIONED" {
		t.Errorf("PUT flipped status to %q", updated.Status)
	}

	// Activate.
	resp = h.Do(ctx, t, http.MethodPost,
		"/api/v1/users/"+created.ID+"/lifecycle/activate", nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Deactivate.
	resp = h.Do(ctx, t, http.MethodPost,
		"/api/v1/users/"+created.ID+"/lifecycle/deactivate", nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Delete.
	resp = h.Do(ctx, t, http.MethodDelete, "/api/v1/users/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// Read after delete → 404 with the Okta envelope shape.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/users/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusNotFound)
	var envelope struct {
		Code    string `json:"errorCode"`
		Summary string `json:"errorSummary"`
		ID      string `json:"errorId"`
	}
	DecodeJSON(t, resp, &envelope)
	if envelope.Code == "" || envelope.ID == "" {
		t.Errorf("404 body missing envelope fields: %+v", envelope)
	}
}

func TestContract_User_DuplicateLoginConflict(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	body := map[string]any{
		"profile": map[string]string{
			"login": "dup@contract.example",
			"email": "dup@contract.example",
		},
	}
	r1 := h.Do(ctx, t, http.MethodPost, "/api/v1/users", body)
	ExpectStatus(t, r1, http.StatusOK)
	_ = r1.Body.Close()

	r2 := h.Do(ctx, t, http.MethodPost, "/api/v1/users", body)
	ExpectStatus(t, r2, http.StatusConflict)
	_ = r2.Body.Close()
}
