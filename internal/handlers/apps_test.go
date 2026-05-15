package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/store"
)

func callAppsCreate(t *testing.T, s *store.Store, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/v1/apps", jsonBody(t, body))
	NewAppsCreate(s).ServeHTTP(rec, req)
	return rec
}

func TestAppsCreate_HappyPath(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callAppsCreate(t, s, map[string]any{
		"name":       "saml-app",
		"label":      "Acme SAML",
		"signOnMode": "SAML_2_0",
		"settings":   map[string]any{"signOn": map[string]string{"ssoUrl": "https://x"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got appResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != appStatusActive {
		t.Errorf("status = %q, want ACTIVE", got.Status)
	}
}

func TestAppsCreate_NonSAMLReturnsGap(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callAppsCreate(t, s, map[string]any{
		"label":      "oidc-app",
		"signOnMode": "OPENID_CONNECT",
	})
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
	if rec.Header().Get(middleware.GapHeader) != gapAppSignOnNonSAML {
		t.Errorf("gap header = %q", rec.Header().Get(middleware.GapHeader))
	}
}

func TestAppsCreate_MissingLabel(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callAppsCreate(t, s, map[string]any{
		"signOnMode": "SAML_2_0",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAppsCreate_DuplicateLabel(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	body := map[string]any{
		"label":      "Dup",
		"signOnMode": "SAML_2_0",
	}
	if rec := callAppsCreate(t, s, body); rec.Code != http.StatusOK {
		t.Fatalf("first create = %d", rec.Code)
	}
	if rec := callAppsCreate(t, s, body); rec.Code != http.StatusConflict {
		t.Errorf("second create = %d, want 409", rec.Code)
	}
}

func createAppForTest(t *testing.T, s *store.Store, label string) appResponse {
	t.Helper()
	rec := callAppsCreate(t, s, map[string]any{
		"label":      label,
		"signOnMode": "SAML_2_0",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create %q: status = %d", label, rec.Code)
	}
	var a appResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAppsGetUpdate(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	a := createAppForTest(t, s, "GetMe")

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/apps/{id}", NewAppsGet(s))
	mux.Handle("PUT /api/v1/apps/{id}", NewAppsUpdate(s))

	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/api/v1/apps/"+a.ID, http.NoBody))
	if r.Code != http.StatusOK {
		t.Fatalf("GET = %d", r.Code)
	}

	r2 := httptest.NewRecorder()
	mux.ServeHTTP(r2, httptest.NewRequestWithContext(t.Context(),
		http.MethodPut, "/api/v1/apps/"+a.ID,
		jsonBody(t, map[string]any{"label": "RenamedLabel"})))
	if r2.Code != http.StatusOK {
		t.Fatalf("PUT = %d, body=%s", r2.Code, r2.Body.String())
	}
	var updated appResponse
	if err := json.Unmarshal(r2.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Label != "RenamedLabel" {
		t.Errorf("label = %q", updated.Label)
	}
}

func TestAppsUpdate_SignOnModeImmutable(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	a := createAppForTest(t, s, "Immut")
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/apps/{id}", NewAppsUpdate(s))
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodPut, "/api/v1/apps/"+a.ID,
		jsonBody(t, map[string]any{"signOnMode": "OPENID_CONNECT"})))
	if r.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", r.Code)
	}
}

func TestAppsLifecycleAndDelete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	a := createAppForTest(t, s, "Lifecycle")

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/apps/{id}/lifecycle/activate", NewAppsActivate(s))
	mux.Handle("POST /api/v1/apps/{id}/lifecycle/deactivate", NewAppsDeactivate(s))
	mux.Handle("DELETE /api/v1/apps/{id}", NewAppsDelete(s))

	// DELETE while active must fail.
	r0 := httptest.NewRecorder()
	mux.ServeHTTP(r0, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/api/v1/apps/"+a.ID, http.NoBody))
	if r0.Code != http.StatusBadRequest {
		t.Errorf("DELETE active app = %d, want 400", r0.Code)
	}

	// Deactivate, then delete.
	r1 := httptest.NewRecorder()
	mux.ServeHTTP(r1, httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/api/v1/apps/"+a.ID+"/lifecycle/deactivate", http.NoBody))
	if r1.Code != http.StatusOK {
		t.Fatalf("deactivate = %d", r1.Code)
	}
	r2 := httptest.NewRecorder()
	mux.ServeHTTP(r2, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/api/v1/apps/"+a.ID, http.NoBody))
	if r2.Code != http.StatusNoContent {
		t.Errorf("DELETE inactive = %d, want 204", r2.Code)
	}

	// Re-activate after delete should 404.
	r3 := httptest.NewRecorder()
	mux.ServeHTTP(r3, httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/api/v1/apps/"+a.ID+"/lifecycle/activate", http.NoBody))
	if r3.Code != http.StatusNotFound {
		t.Errorf("activate-after-delete = %d, want 404", r3.Code)
	}
}

func TestAppsList_FilterAndLimit(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, l := range []string{"alpha-app", "alpha-bot", "beta-app"} {
		createAppForTest(t, s, l)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/apps?filter="+url.QueryEscape(`label sw "alpha"`), http.NoBody)
	NewAppsList(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []appResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("count = %d, want 2", len(got))
	}
	if rec.Header().Get("Link") == "" {
		t.Error("missing Link header")
	}
}

func TestAppsList_BadFilterReturnsGap(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/apps?filter="+url.QueryEscape(`unknown eq "x"`), http.NoBody)
	NewAppsList(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if rec.Header().Get(middleware.GapHeader) != gapAppListFilterBad {
		t.Errorf("gap header = %q, want %q",
			rec.Header().Get(middleware.GapHeader), gapAppListFilterBad)
	}
}
