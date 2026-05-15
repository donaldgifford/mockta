package contract

import (
	"net/http"
	"testing"
)

// TestContract_Group walks the okta_group lifecycle. Mirrors the
// provider's plan/apply/destroy: create, read, update, list, delete.
func TestContract_Group(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	resp := h.Do(ctx, t, http.MethodPost, "/api/v1/groups", map[string]any{
		"profile": map[string]any{
			"name":        "engineers",
			"description": "engineering team",
		},
	})
	ExpectStatus(t, resp, http.StatusOK)
	var created struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	DecodeJSON(t, resp, &created)
	if created.ID == "" {
		t.Fatal("missing id")
	}
	if created.Type != "OKTA_GROUP" {
		t.Errorf("type = %q, want OKTA_GROUP", created.Type)
	}

	// Read.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/groups/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Update.
	resp = h.Do(ctx, t, http.MethodPut, "/api/v1/groups/"+created.ID, map[string]any{
		"profile": map[string]any{
			"name":        "engineers",
			"description": "engineering team — renamed",
		},
	})
	ExpectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// List with q=prefix.
	resp = h.Do(ctx, t, http.MethodGet, "/api/v1/groups?q=eng", nil)
	ExpectStatus(t, resp, http.StatusOK)
	var listed []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	DecodeJSON(t, resp, &listed)
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Errorf("list = %+v, want [%s]", listed, created.ID)
	}

	// Delete.
	resp = h.Do(ctx, t, http.MethodDelete, "/api/v1/groups/"+created.ID, nil)
	ExpectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

func TestContract_Group_AppGroupReturnsGap(t *testing.T) {
	h := Start(t)
	resp := h.Do(t.Context(), t, http.MethodPost, "/api/v1/groups", map[string]any{
		"profile": map[string]any{"name": "appgrp"},
		"type":    "APP_GROUP",
	})
	ExpectStatus(t, resp, http.StatusNotImplemented)

	var env struct {
		Code string `json:"errorCode"`
	}
	DecodeJSON(t, resp, &env)
	if env.Code != "MOCKTA_GAP_0004" {
		t.Errorf("errorCode = %q, want MOCKTA_GAP_0004", env.Code)
	}
	if got := resp.Header.Get("X-Mockta-Gap"); got != "MOCKTA_GAP_0004" {
		t.Errorf("X-Mockta-Gap = %q, want MOCKTA_GAP_0004", got)
	}
}
