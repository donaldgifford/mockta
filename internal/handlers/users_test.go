package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/store"
)

// newTestStore builds a fresh store; if construction fails the test
// fails (the schema is hard-coded so this is unreachable in practice).
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New()
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

// callUsersCreate issues POST /api/v1/users against a fresh handler.
func callUsersCreate(t *testing.T, s *store.Store, strict bool, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/api/v1/users", jsonBody(t, body))
	NewUsersCreate(s, strict).ServeHTTP(rec, req)
	return rec
}

func TestUsersCreate_HappyPath(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login":     "alice@example.com",
			"email":     "alice@example.com",
			"firstName": "Alice",
			"lastName":  "Liddell",
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID == "" {
		t.Error("id is empty")
	}
	if got.Status != userStatusProvisioned {
		t.Errorf("status = %q, want %q", got.Status, userStatusProvisioned)
	}
}

func TestUsersCreate_ActivateFalseGivesStaged(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/v1/users?activate=false",
		jsonBody(t, map[string]any{"profile": map[string]string{
			"login": "bob@example.com",
			"email": "bob@example.com",
		}}))
	NewUsersCreate(s, true).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != userStatusStaged {
		t.Errorf("status = %q, want %q", got.Status, userStatusStaged)
	}
}

func TestUsersCreate_StrictRequiresLogin(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{"email": "x@example.com"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "login") {
		t.Errorf("body missing 'login': %s", rec.Body.String())
	}
}

func TestUsersCreate_StrictRequiresEmail(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{"login": "a@b.com"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUsersCreate_PermissiveAcceptsBadFormat(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// Permissive mode skips the RFC-5322-ish login format check, so a
	// login without an @ is accepted as long as it's non-empty.
	rec := callUsersCreate(t, s, false, map[string]any{
		"profile": map[string]string{
			"login": "no-at-sign",
			"email": "anything",
		},
	})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (permissive), body=%s",
			rec.Code, rec.Body.String())
	}
}

func TestUsersCreate_DuplicateLoginConflict(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	body := map[string]any{
		"profile": map[string]string{
			"login": "dup@example.com",
			"email": "dup@example.com",
		},
	}
	first := callUsersCreate(t, s, true, body)
	if first.Code != http.StatusOK {
		t.Fatalf("first create status = %d", first.Code)
	}
	second := callUsersCreate(t, s, true, body)
	if second.Code != http.StatusConflict {
		t.Errorf("second create status = %d, want 409", second.Code)
	}
}

func TestUsersCreate_StrictRejectsBadLoginFormat(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "no-at-sign",
			"email": "x@example.com",
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUsersCreate_BadJSON(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/api/v1/users", strings.NewReader("not json"))
	NewUsersCreate(s, true).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUsersGet_ByIDAndLogin(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "carol@example.com",
			"email": "carol@example.com",
		},
	})
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{created.ID, "carol@example.com"} {
		mux := http.NewServeMux()
		mux.Handle("GET /api/v1/users/{idOrLogin}", NewUsersGet(s))
		recGet := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/api/v1/users/"+key, http.NoBody)
		mux.ServeHTTP(recGet, req)
		if recGet.Code != http.StatusOK {
			t.Errorf("key=%q: status = %d, want 200", key, recGet.Code)
		}
	}
}

func TestUsersGet_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/users/{idOrLogin}", NewUsersGet(s))
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/users/missing", http.NoBody)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUsersUpdate_PreservesStatus(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "dave@example.com",
			"email": "dave@example.com",
		},
	})
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/users/{id}", NewUsersUpdate(s, true))
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/v1/users/"+created.ID,
		jsonBody(t, map[string]any{
			"profile": map[string]string{
				"login":     "dave@example.com",
				"email":     "dave@example.com",
				"firstName": "David",
			},
		}))
	mux.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var got userResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != created.Status {
		t.Errorf("status changed via PUT: was %q, now %q",
			created.Status, got.Status)
	}
}

func TestUsersUpdate_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/users/{id}", NewUsersUpdate(s, true))
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/v1/users/nope",
		jsonBody(t, map[string]any{"profile": map[string]string{
			"login": "x@example.com", "email": "x@example.com",
		}}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUsersLifecycle_ActivateDeactivate(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "eve@example.com",
			"email": "eve@example.com",
		},
	})
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		h    http.Handler
		want string
	}{
		{
			"/api/v1/users/" + created.ID + "/lifecycle/activate",
			NewUsersActivate(s), userStatusActive,
		},
		{
			"/api/v1/users/" + created.ID + "/lifecycle/deactivate",
			NewUsersDeactivate(s), userStatusDeprovisioned,
		},
	} {
		mux := http.NewServeMux()
		mux.Handle("POST /api/v1/users/{id}/lifecycle/activate", NewUsersActivate(s))
		mux.Handle("POST /api/v1/users/{id}/lifecycle/deactivate", NewUsersDeactivate(s))
		r := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tc.path, http.NoBody)
		mux.ServeHTTP(r, req)
		if r.Code != http.StatusOK {
			t.Fatalf("lifecycle %s: status = %d", tc.path, r.Code)
		}
		got, err := s.GetUser(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != tc.want {
			t.Errorf("after %s: status = %q, want %q", tc.path, got.Status, tc.want)
		}
	}
}

func TestUsersDelete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "frank@example.com",
			"email": "frank@example.com",
		},
	})
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("DELETE /api/v1/users/{id}", NewUsersDelete(s))

	// Real Okta requires two DELETEs: the first deactivates, the
	// second removes. mockta mirrors that so the okta terraform
	// provider's destroy path works.
	doDelete := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
			"/api/v1/users/"+created.ID, http.NoBody)
		mux.ServeHTTP(r, req)
		return r
	}

	r1 := doDelete()
	if r1.Code != http.StatusNoContent {
		t.Errorf("first delete status = %d, want 204", r1.Code)
	}
	if _, err := s.GetUser(created.ID); err != nil {
		t.Errorf("user gone after first delete; want it to be DEPROVISIONED still in store: %v", err)
	}

	r2 := doDelete()
	if r2.Code != http.StatusNoContent {
		t.Errorf("second delete status = %d, want 204", r2.Code)
	}
	if _, err := s.GetUser(created.ID); err == nil {
		t.Error("user still present after second delete")
	}
}

func TestUsersList_FilterEqAndSw(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, login := range []string{"al@example.com", "alex@example.com", "bob@example.com"} {
		callUsersCreate(t, s, true, map[string]any{
			"profile": map[string]string{"login": login, "email": login},
		})
	}

	cases := []struct {
		filter string
		want   int
	}{
		{`profile.login eq "al@example.com"`, 1},
		{`profile.login sw "al"`, 2},
		{`status eq "PROVISIONED"`, 3},
		{`profile.email sw "bob"`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				"/api/v1/users?filter="+url.QueryEscape(tc.filter), http.NoBody)
			NewUsersList(s).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
			}
			var got []userResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("count = %d, want %d", len(got), tc.want)
			}
			if rec.Header().Get("Link") == "" {
				t.Error("missing Link header")
			}
		})
	}
}

func TestUsersList_BadFilter(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	cases := []string{
		`profile.login co "x"`,      // unsupported operator
		`unknown eq "x"`,            // unsupported attribute
		`profile.login eq unquoted`, // missing quotes
		`malformed`,                 // wrong arity
	}
	for _, f := range cases {
		t.Run(f, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				"/api/v1/users?filter="+url.QueryEscape(f), http.NoBody)
			NewUsersList(s).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if rec.Header().Get(middleware.GapHeader) == "" {
				t.Error("missing gap header on bad filter")
			}
		})
	}
}

func TestUsersList_LimitTrims(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		login := string(rune('a'+i)) + "@example.com"
		callUsersCreate(t, s, true, map[string]any{
			"profile": map[string]string{"login": login, "email": login},
		})
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/users?limit=2", http.NoBody)
	NewUsersList(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("count = %d, want 2", len(got))
	}
}

func TestParseLimit(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"":    0,
		"0":   0,
		"5":   5,
		"-3":  0,
		"abc": 0,
		"10x": 0,
		"42":  42,
	}
	for in, want := range cases {
		if got := parseLimit(in); got != want {
			t.Errorf("parseLimit(%q) = %d, want %d", in, got, want)
		}
	}
}
