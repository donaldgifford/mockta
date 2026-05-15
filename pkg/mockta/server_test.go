package mockta

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/donaldgifford/mockta/internal/config"
)

// Server tests run the actual two-listener machinery against
// httptest-style probes. The Server binds to its real ports
// (:8080/:9090) in production; for tests we accept that and add
// listen retry / cleanup to keep parallel runs from clashing.
//
// For simplicity, tests run the Server serially (no t.Parallel on
// these because they share the listener ports).

func startTestServer(t *testing.T, cfg config.Config) func() {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := New(cfg, logger)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Wait briefly for listeners to bind.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://localhost"+AdminAddr+"/health", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cleanup := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("server did not shut down within 5s")
		}
	}
	return cleanup
}

func TestServer_EndToEnd(t *testing.T) {
	cfg := config.Config{
		AdminToken: "test-token",
		OrgName:    "acme",
		StrictMode: true,
		LogLevel:   slog.LevelInfo,
	}
	cleanup := startTestServer(t, cfg)
	defer cleanup()

	tests := []struct {
		name   string
		method string
		url    string
		token  string
		want   int
	}{
		{"health no auth", "GET", "http://localhost:9090/health", "", 200},
		{"org no auth", "GET", "http://localhost:8080/api/v1/org", "", 401},
		{"org wrong token", "GET", "http://localhost:8080/api/v1/org", "wrong", 401},
		{"org correct token", "GET", "http://localhost:8080/api/v1/org", "test-token", 200},
		{"unimplemented", "POST", "http://localhost:8080/api/v1/policies", "test-token", 501},
		{"reset no auth", "POST", "http://localhost:9090/admin/reset", "", 401},
		{"reset with auth", "POST", "http://localhost:9090/admin/reset", "test-token", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), tt.method, tt.url, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestServer_OrgPayloadShape(t *testing.T) {
	cfg := config.Config{
		AdminToken: "tok",
		OrgName:    "acme",
		StrictMode: true,
	}
	cleanup := startTestServer(t, cfg)
	defer cleanup()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://localhost:8080/api/v1/org", http.NoBody)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["name"] != "acme" {
		t.Errorf("org.name = %v, want acme", got["name"])
	}
}

// Smoke test that the package-level addresses match what
// healthcheck.go expects (the URL is hard-coded there).
func TestAddresses_HealthcheckCompatible(t *testing.T) {
	t.Parallel()
	if AdminAddr != ":9090" {
		t.Errorf("AdminAddr = %q, want :9090 (matches internal/cli/healthcheck.go)", AdminAddr)
	}
	if APIAddr != ":8080" {
		t.Errorf("APIAddr = %q, want :8080", APIAddr)
	}
}

func TestStop_IdempotentBeforeStart(t *testing.T) {
	t.Parallel()
	s := New(config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.Stop(); err != nil {
		t.Errorf("Stop on unstarted server = %v, want no error", err)
	}
}

func TestNotImplementedBodyContainsGap(t *testing.T) {
	// Per Phase 3 success criteria, the 501 body must carry a
	// MOCKTA_GAP_* code so consumers can file gap-tracker issues.
	cfg := config.Config{AdminToken: "t", OrgName: "x"}
	cleanup := startTestServer(t, cfg)
	defer cleanup()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://localhost:8080/api/v1/i-do-not-exist", http.NoBody)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if !contains(string(body), "MOCKTA_GAP_") {
		t.Errorf("body missing MOCKTA_GAP_ prefix: %s", body)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestServer_ResourceRoundTrip walks the curl sequence from IMPL-0001
// §Phase 4 success criteria. It exists to catch routing or wiring
// regressions even when handler-level tests pass.
func TestServer_ResourceRoundTrip(t *testing.T) {
	cfg := config.Config{AdminToken: "tok", OrgName: "acme", StrictMode: true}
	cleanup := startTestServer(t, cfg)
	defer cleanup()

	do := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rdr io.Reader = http.NoBody
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			rdr = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(t.Context(), method,
			"http://localhost:8080"+path, rdr)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, respBody
	}

	// 1. Create alice.
	code, body := do(http.MethodPost, "/api/v1/users", map[string]any{
		"profile": map[string]string{
			"login":     "alice@example.com",
			"email":     "alice@example.com",
			"firstName": "Alice",
			"lastName":  "Liddell",
		},
	})
	if code != http.StatusOK {
		t.Fatalf("create user = %d, body=%s", code, body)
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatal(err)
	}

	// 2. Create engineers group.
	code, body = do(http.MethodPost, "/api/v1/groups", map[string]any{
		"profile": map[string]string{"name": "engineers"},
	})
	if code != http.StatusOK {
		t.Fatalf("create group = %d, body=%s", code, body)
	}
	var group struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &group); err != nil {
		t.Fatal(err)
	}

	// 3. Add alice to engineers.
	code, body = do(http.MethodPut,
		"/api/v1/groups/"+group.ID+"/users/"+user.ID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("add membership = %d, body=%s", code, body)
	}

	// 4. List members.
	code, body = do(http.MethodGet,
		"/api/v1/groups/"+group.ID+"/users", nil)
	if code != http.StatusOK {
		t.Fatalf("list members = %d, body=%s", code, body)
	}
	if !contains(string(body), user.ID) {
		t.Errorf("members body missing %q: %s", user.ID, body)
	}

	// 5. Create SAML app.
	code, body = do(http.MethodPost, "/api/v1/apps", map[string]any{
		"label":      "Acme SAML",
		"signOnMode": "SAML_2_0",
	})
	if code != http.StatusOK {
		t.Fatalf("create app = %d, body=%s", code, body)
	}
	var app struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &app); err != nil {
		t.Fatal(err)
	}

	// 6. DELETE each resource. App must be deactivated first.
	if code, body := do(http.MethodPost,
		"/api/v1/apps/"+app.ID+"/lifecycle/deactivate", nil); code != http.StatusOK {
		t.Fatalf("deactivate app = %d, body=%s", code, body)
	}
	for _, p := range []string{
		"/api/v1/groups/" + group.ID + "/users/" + user.ID,
		"/api/v1/apps/" + app.ID,
		"/api/v1/groups/" + group.ID,
		"/api/v1/users/" + user.ID,
	} {
		if code, body := do(http.MethodDelete, p, nil); code != http.StatusNoContent {
			t.Errorf("DELETE %s = %d, body=%s", p, code, body)
		}
	}
}
