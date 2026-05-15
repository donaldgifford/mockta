// Package contract holds the contract test suite for mockta.
//
// "Contract" here means: assert that mockta's wire shape matches what
// an Okta-API client (specifically the okta/okta Terraform provider)
// expects on each round-trip. The suite is intentionally pinned to
// plain HTTP rather than the Okta Go SDK — the SDK drags hundreds of
// transitive deps and would couple our tests to a specific SDK
// version. The smoke fixture in tests/smoke/ exercises the real
// provider end-to-end and catches anything contract misses.
//
// Tests start mockta in-process via httptest.Server (no listener
// binding, parallel-safe) and drive it with stdlib net/http. The
// harness is small on purpose: a single Client value with the bearer
// token, an httptest.Server already started, and a Close hook.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donaldgifford/mockta/internal/config"
	"github.com/donaldgifford/mockta/pkg/mockta"
)

// adminToken is the fixed bearer used across the suite. The value is
// arbitrary — strict-mode auth just needs a non-empty string.
const adminToken = "contract-test-token"

// Harness is the per-test mockta instance. Construct via Start; the
// returned value carries a base URL plus an HTTP client preconfigured
// with the bearer token.
type Harness struct {
	BaseURL string
	Client  *http.Client
	server  *httptest.Server
}

// Start spins up an in-process mockta API server bound to a random
// localhost port via httptest. Cleanup is registered on t so callers
// don't need to defer Close.
func Start(t *testing.T) *Harness {
	t.Helper()

	cfg := config.Config{
		AdminToken: adminToken,
		OrgName:    "contract-org",
		StrictMode: true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(mockta.New(cfg, logger).APIHandler())

	h := &Harness{
		BaseURL: srv.URL,
		Client:  srv.Client(),
		server:  srv,
	}
	t.Cleanup(h.Close)
	return h
}

// Close shuts down the underlying httptest.Server. Safe to call
// multiple times; httptest.Server.Close is idempotent.
func (h *Harness) Close() {
	if h.server != nil {
		h.server.Close()
	}
}

// Do is the convenience wrapper for an authenticated JSON request.
// body may be nil; non-nil values are JSON-marshaled.
func (h *Harness) Do(ctx context.Context, t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// DecodeJSON reads resp.Body into v and closes it. Fails the test on
// any error to keep call sites flat.
func DecodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// ExpectStatus fails the test if resp.StatusCode != want. The body is
// drained either way so the connection can be reused.
func ExpectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode == want {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, want, body)
}
