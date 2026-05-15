package contract

import (
	"net/http"
	"testing"
)

// TestContract_AppSAML walks the okta_app_saml lifecycle: create →
// read → update → deactivate (Okta precondition for delete) → delete.
func TestContract_AppSAML(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	resp := h.Do(ctx, t, http.MethodPost, "/api/v1/apps", map[string]any{
		"name":       "saml-contract",
		"label":      "Contract SAML",
		"signOnMode": "SAML_2_0",
		"settings": map[string]any{
			"signOn": map[string]string{"ssoUrl": "https://x.example/sso"},
		},
	})
	ExpectStatus(t, resp, http.StatusOK)
	var created struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		SignOnMode string `json:"signOnMode"`
	}
	DecodeJSON(t, resp, &created)
	if created.ID == "" {
		t.Fatal("missing id")
	}
	if created.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", created.Status)
	}
	if created.SignOnMode != "SAML_2_0" {
		t.Errorf("signOnMode = %q", created.SignOnMode)
	}

	// Read.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/apps/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Update.
	resp = h.Do(ctx, t, http.MethodPut, "/api/v1/apps/"+created.ID, map[string]any{
		"label": "Contract SAML — renamed",
	})
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Delete while active must 400 — provider expects the two-step.
	resp = h.Do(ctx, t, http.MethodDelete, "/api/v1/apps/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// Deactivate, then delete.
	resp = h.Do(ctx, t, http.MethodPost,
		"/api/v1/apps/"+created.ID+"/lifecycle/deactivate", nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = h.Do(ctx, t, http.MethodDelete, "/api/v1/apps/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

func TestContract_AppNonSAMLReturnsGap(t *testing.T) {
	h := Start(t)
	resp := h.Do(t.Context(), t, http.MethodPost, "/api/v1/apps", map[string]any{
		"label":      "oidc-app",
		"signOnMode": "OPENID_CONNECT",
	})
	ExpectStatus(t, resp, http.StatusNotImplemented)
	if got := resp.Header.Get("X-Mockta-Gap"); got != "MOCKTA_GAP_0006" {
		t.Errorf("X-Mockta-Gap = %q, want MOCKTA_GAP_0006", got)
	}
	_ = resp.Body.Close()
}
