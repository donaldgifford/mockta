package oktaerr

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWrite_Shape(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	Write(rec, 404, CodeResourceNotFound, "Resource not found",
		Cause{Summary: "user with login \"x\" not found"})

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}

	var got Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got.Code != CodeResourceNotFound {
		t.Errorf("errorCode = %q, want %q", got.Code, CodeResourceNotFound)
	}
	if got.Summary != "Resource not found" {
		t.Errorf("errorSummary = %q, want %q", got.Summary, "Resource not found")
	}
	if got.Link != CodeResourceNotFound {
		t.Errorf("errorLink = %q, want %q", got.Link, CodeResourceNotFound)
	}
	if !strings.HasPrefix(got.ID, "mockta-") {
		t.Errorf("errorId = %q, want mockta- prefix", got.ID)
	}
	if len(got.Causes) != 1 {
		t.Fatalf("causes = %d, want 1", len(got.Causes))
	}
}

func TestWrite_NoCauses(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	Write(rec, 500, CodeAPIValidationFailed, "boom")

	var got Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Real Okta serializes empty causes as [], not null.
	if got.Causes == nil {
		t.Errorf("Causes = nil, want empty slice")
	}
}

func TestErrorID_Uniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	const n = 100
	for range n {
		id := newErrorID()
		if !strings.HasPrefix(id, "mockta-") {
			t.Errorf("id %q does not have mockta- prefix", id)
		}
		if _, dup := seen[id]; dup {
			t.Errorf("duplicate id %q (after %d ids)", id, len(seen))
		}
		seen[id] = struct{}{}
	}
}
