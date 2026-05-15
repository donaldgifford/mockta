package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donaldgifford/mockta/internal/gaps"
)

func TestOrg_PlausibleResponse(t *testing.T) {
	t.Parallel()
	h := NewOrg("acme")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/org", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var got orgResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != "acme" || got.Subdomain != "acme" {
		t.Errorf("org payload name/subdomain = %q/%q, want acme/acme", got.Name, got.Subdomain)
	}
	if got.Status != "ACTIVE" {
		t.Errorf("org status = %q, want ACTIVE", got.Status)
	}
	if !strings.HasPrefix(got.WebSite, "https://acme.") {
		t.Errorf("website = %q, want https://acme.* prefix", got.WebSite)
	}
}

func TestHealth_OK(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	NewHealth().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q, want it to contain status:ok", rec.Body.String())
	}
}

type fakeResetter struct {
	called bool
	err    error
}

func (f *fakeResetter) Reset() error {
	f.called = true
	return f.err
}

func TestAdminReset_Success(t *testing.T) {
	t.Parallel()
	r := &fakeResetter{}
	rec := httptest.NewRecorder()
	NewAdminReset(r).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/reset", http.NoBody))

	if !r.called {
		t.Error("Reset() was not called")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestAdminReset_Error(t *testing.T) {
	t.Parallel()
	r := &fakeResetter{err: errors.New("disk on fire")}
	rec := httptest.NewRecorder()
	NewAdminReset(r).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/reset", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestNotImplemented_EmitsGapID(t *testing.T) {
	t.Parallel()
	h := NewNotImplemented(gaps.StubRegistry{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/policies", http.NoBody))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
	if gap := rec.Header().Get("X-Mockta-Gap"); gap != gaps.UncataloguedID {
		t.Errorf("gap header = %q, want %q", gap, gaps.UncataloguedID)
	}
	if !strings.Contains(rec.Body.String(), gaps.UncataloguedID) {
		t.Errorf("body missing gap ID: %q", rec.Body.String())
	}
}
