package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donaldgifford/mockta/internal/store"
)

type fakeSink struct {
	entries []*store.AuditEntry
}

func (f *fakeSink) AppendAudit(e *store.AuditEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

func TestAudit_RecordsRequest(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	h := Audit(sink)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/org", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if len(sink.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(sink.entries))
	}
	got := sink.entries[0]
	if got.Method != http.MethodGet || got.Path != "/api/v1/org" || got.Status != http.StatusTeapot {
		t.Errorf("entry = %+v, want method/path/status to match request", got)
	}
	if got.GapID != "" {
		t.Errorf("GapID = %q, want empty (no gap)", got.GapID)
	}
}

func TestAudit_ForwardsGapHeader(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	h := Audit(sink)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(GapHeader, "MOCKTA_GAP_0042")
		w.WriteHeader(http.StatusNotImplemented)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/policies", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if len(sink.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(sink.entries))
	}
	if got := sink.entries[0].GapID; got != "MOCKTA_GAP_0042" {
		t.Errorf("GapID = %q, want MOCKTA_GAP_0042", got)
	}
	// Header should be stripped before the response leaves.
	if rec.Header().Get(GapHeader) != "" {
		t.Errorf("gap header leaked to client: %q", rec.Header().Get(GapHeader))
	}
}

func TestChain_OrderingOutermostFirst(t *testing.T) {
	t.Parallel()

	var log []string
	outer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log = append(log, "outer-pre")
			next.ServeHTTP(w, r)
			log = append(log, "outer-post")
		})
	}
	inner := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log = append(log, "inner-pre")
			next.ServeHTTP(w, r)
			log = append(log, "inner-post")
		})
	}
	terminal := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		log = append(log, "handler")
	})

	Chain(terminal, outer, inner).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))

	want := []string{"outer-pre", "inner-pre", "handler", "inner-post", "outer-post"}
	if len(log) != len(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("log[%d] = %q, want %q", i, log[i], want[i])
		}
	}
}

func TestEmitNextLink(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/api/v1/users?limit=200", http.NoBody)
	EmitNextLink(rec, req, "cursor123")

	got := rec.Header().Get("Link")
	if got == "" {
		t.Fatal("Link header not set")
	}
	// Just check the structural pieces.
	for _, want := range []string{`rel="next"`, "after=cursor123", "/api/v1/users", "limit=200"} {
		if !contains(got, want) {
			t.Errorf("Link = %q, missing substring %q", got, want)
		}
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
