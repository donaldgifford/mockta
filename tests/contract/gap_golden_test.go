//go:build mockta_v0_undersized

// Package contract: gap-list determinism golden test.
//
// This file is gated on the `mockta_v0_undersized` build tag, which
// swaps the production groups-route wiring for a no-op stub
// (pkg/mockta/groups_routes_undersized.go). With group routes
// disabled, every /api/v1/groups* request falls through to the
// notimplemented catch-all and emits the registry's
// UncataloguedID — the gap registry's path-matched entries don't
// cover /api/v1/groups since the production build serves those.
//
// Phase 7 success criterion: this test, when run with
// `go test -tags mockta_v0_undersized ./tests/contract/`, must
// produce the same sequence of gap IDs across runs and must match
// the committed golden file. Drift means either (a) the registry
// changed (deliberate — regenerate the golden), or (b) the
// undersized handler graph changed (catch regression in review).
//
// Run / regenerate:
//
//	go test -tags mockta_v0_undersized -run TestGapGolden \
//	    -update ./tests/contract/
package contract

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden is set by the -update flag. When true, the test
// rewrites the golden file with the observed sequence instead of
// comparing — used to refresh the golden after intentional registry
// changes.
var updateGolden = flag.Bool("update", false, "rewrite the gap-golden file with observed output")

// goldenPath is the canonical filename. Kept relative to the
// tests/contract directory so the test binary works from any cwd.
const goldenPath = "testdata/gap-golden.txt"

// TestGapGolden exercises the endpoints expected to 501 under the
// undersized build (group CRUD + memberships + a known not-routed
// path) and captures the gap IDs from the X-Mockta-Gap response
// header. The recorded sequence is compared against the committed
// golden file; mismatches fail the test with a diff hint.
func TestGapGolden(t *testing.T) {
	h := Start(t)
	ctx := t.Context()

	type step struct {
		method, path string
	}
	steps := []step{
		// Group CRUD — all routed in production, all stubbed here.
		{http.MethodPost, "/api/v1/groups"},
		{http.MethodGet, "/api/v1/groups"},
		{http.MethodGet, "/api/v1/groups/anything"},
		{http.MethodPut, "/api/v1/groups/anything"},
		{http.MethodDelete, "/api/v1/groups/anything"},
		// Memberships also live under the groups handler.
		{http.MethodPut, "/api/v1/groups/g/users/u"},
		{http.MethodDelete, "/api/v1/groups/g/users/u"},
		{http.MethodGet, "/api/v1/groups/g/users"},
		// One always-501 path that exists in both builds — sanity-
		// check that registry IDs flow consistently.
		{http.MethodGet, "/api/v1/policies/abc"},
		{http.MethodGet, "/api/v1/idps/xyz"},
	}

	observed := make([]string, 0, len(steps))
	for _, s := range steps {
		resp := h.Do(ctx, t, s.method, s.path, nil)
		ExpectStatus(t, resp, http.StatusNotImplemented)
		gap := resp.Header.Get("X-Mockta-Gap")
		if gap == "" {
			t.Fatalf("%s %s: missing X-Mockta-Gap header", s.method, s.path)
		}
		observed = append(observed, s.method+" "+s.path+" -> "+gap)
		_ = resp.Body.Close()
	}

	got := strings.Join(observed, "\n") + "\n"

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden rewritten: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to seed)", goldenPath, err)
	}
	if string(want) != got {
		t.Errorf("gap-golden drift — observed:\n%s\nexpected:\n%s\n"+
			"If the change is intentional, regenerate with:\n"+
			"  go test -tags mockta_v0_undersized -run TestGapGolden -update ./tests/contract/",
			got, want)
	}
}
