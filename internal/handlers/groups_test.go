package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/donaldgifford/mockta/internal/gaps"
	"github.com/donaldgifford/mockta/internal/middleware"
	"github.com/donaldgifford/mockta/internal/store"
)

func callGroupsCreate(t *testing.T, s *store.Store, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/api/v1/groups", jsonBody(t, body))
	NewGroupsCreate(s).ServeHTTP(rec, req)
	return rec
}

func TestGroupsCreate_HappyPath(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{
			"name":        "engineers",
			"description": "the eng team",
		},
		"type": "OKTA_GROUP",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got groupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "OKTA_GROUP" {
		t.Errorf("type = %q, want OKTA_GROUP", got.Type)
	}
	if got.ID == "" {
		t.Error("id empty")
	}
}

func TestGroupsCreate_DefaultsToOktaGroup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"name": "default-type"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupsCreate_AppGroupReturnsGap(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"name": "app-group"},
		"type":    "APP_GROUP",
	})
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
	if got := rec.Header().Get(middleware.GapHeader); got != gaps.IDGroupTypeAppGroup {
		t.Errorf("gap header = %q, want %q", got, gaps.IDGroupTypeAppGroup)
	}
}

func TestGroupsCreate_BuiltInReturnsGap(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"name": "built-in"},
		"type":    "BUILT_IN",
	})
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestGroupsCreate_UnknownTypeRejected(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"name": "weird"},
		"type":    "WEIRD",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGroupsCreate_MissingName(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"description": "no name"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGroupsCreate_DuplicateName(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	body := map[string]any{"profile": map[string]any{"name": "dup"}}
	if rec := callGroupsCreate(t, s, body); rec.Code != http.StatusOK {
		t.Fatalf("first create = %d", rec.Code)
	}
	if rec := callGroupsCreate(t, s, body); rec.Code != http.StatusConflict {
		t.Errorf("second create = %d, want 409", rec.Code)
	}
}

func createGroupForTest(t *testing.T, s *store.Store, name string) groupResponse {
	t.Helper()
	rec := callGroupsCreate(t, s, map[string]any{
		"profile": map[string]any{"name": name},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create %s: status = %d", name, rec.Code)
	}
	var g groupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGroupsGetUpdateDelete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	g := createGroupForTest(t, s, "ops")

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/groups/{id}", NewGroupsGet(s))
	mux.Handle("PUT /api/v1/groups/{id}", NewGroupsUpdate(s))
	mux.Handle("DELETE /api/v1/groups/{id}", NewGroupsDelete(s))

	// GET
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/api/v1/groups/"+g.ID, http.NoBody))
	if r.Code != http.StatusOK {
		t.Fatalf("GET = %d", r.Code)
	}

	// PUT
	r2 := httptest.NewRecorder()
	mux.ServeHTTP(r2, httptest.NewRequestWithContext(t.Context(),
		http.MethodPut, "/api/v1/groups/"+g.ID,
		jsonBody(t, map[string]any{
			"profile": map[string]any{"name": "ops", "description": "renamed"},
		})))
	if r2.Code != http.StatusOK {
		t.Fatalf("PUT = %d, body=%s", r2.Code, r2.Body.String())
	}

	// DELETE
	r3 := httptest.NewRecorder()
	mux.ServeHTTP(r3, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/api/v1/groups/"+g.ID, http.NoBody))
	if r3.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", r3.Code)
	}
}

func TestGroupsUpdate_TypeImmutable(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	g := createGroupForTest(t, s, "immut")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/groups/{id}", NewGroupsUpdate(s))
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodPut, "/api/v1/groups/"+g.ID,
		jsonBody(t, map[string]any{
			"profile": map[string]any{"name": "immut"},
			"type":    "APP_GROUP",
		})))
	if r.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", r.Code)
	}
}

func TestGroupsList_QPrefixSearch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, n := range []string{"alpha", "beta", "alphabet"} {
		createGroupForTest(t, s, n)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/groups?q="+url.QueryEscape("alp"), http.NoBody)
	NewGroupsList(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []groupResponse
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

func TestGroupsList_FilterReturnsGap(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/groups?filter="+url.QueryEscape(`name eq "x"`), http.NoBody)
	NewGroupsList(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Header().Get(middleware.GapHeader), "GAP") {
		t.Error("missing gap header")
	}
}

// Membership round-trip: add, list, remove.
func TestMemberships_Lifecycle(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	g := createGroupForTest(t, s, "team")
	// Create a user the standard way so login/email are set.
	rec := callUsersCreate(t, s, true, map[string]any{
		"profile": map[string]string{
			"login": "u@example.com",
			"email": "u@example.com",
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create user = %d", rec.Code)
	}
	var u userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/groups/{gid}/users/{uid}", NewGroupMembershipAdd(s))
	mux.Handle("DELETE /api/v1/groups/{gid}/users/{uid}", NewGroupMembershipRemove(s))
	mux.Handle("GET /api/v1/groups/{gid}/users", NewGroupMembershipList(s))

	// Add (idempotent: do it twice)
	for i := 0; i < 2; i++ {
		r := httptest.NewRecorder()
		mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
			http.MethodPut, "/api/v1/groups/"+g.ID+"/users/"+u.ID, http.NoBody))
		if r.Code != http.StatusNoContent {
			t.Fatalf("PUT iter=%d status = %d", i, r.Code)
		}
	}

	// List
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/api/v1/groups/"+g.ID+"/users", http.NoBody))
	if r.Code != http.StatusOK {
		t.Fatalf("LIST status = %d", r.Code)
	}
	var listed []userResponse
	if err := json.Unmarshal(r.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != u.ID {
		t.Errorf("members = %+v, want exactly user %q", listed, u.ID)
	}

	// Remove
	r2 := httptest.NewRecorder()
	mux.ServeHTTP(r2, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/api/v1/groups/"+g.ID+"/users/"+u.ID, http.NoBody))
	if r2.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", r2.Code)
	}
}

func TestMemberships_AddUnknownGroup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/groups/{gid}/users/{uid}", NewGroupMembershipAdd(s))
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodPut, "/api/v1/groups/nope/users/nope", http.NoBody))
	if r.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", r.Code)
	}
}

func TestMemberships_ListUnknownGroup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/groups/{gid}/users", NewGroupMembershipList(s))
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/api/v1/groups/nope/users", http.NoBody))
	if r.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", r.Code)
	}
}
