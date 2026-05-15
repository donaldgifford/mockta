package gaps

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdown_Deterministic(t *testing.T) {
	t.Parallel()
	entries := Static().All()

	var a, b bytes.Buffer
	if err := Markdown(&a, entries); err != nil {
		t.Fatal(err)
	}
	if err := Markdown(&b, entries); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("Markdown output not deterministic across runs")
	}
}

func TestMarkdown_SortsByID(t *testing.T) {
	t.Parallel()
	// Pass entries in reverse order — Markdown must still sort by ID.
	entries := Static().All()
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	i1 := strings.Index(out, IDUserFilter)
	i2 := strings.Index(out, "MOCKTA_GAP_0020")
	if i1 < 0 || i2 < 0 || i1 >= i2 {
		t.Errorf("expected 0001 to come before 0020 in output\n%s", out)
	}
}

func TestMarkdown_HeaderPresent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Markdown(&buf, Static().All()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"# mockta gap list",
		"`X-Mockta-Gap`",
		"| ID | Endpoint | Resource | Status | Notes |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestMarkdown_EscapesPipes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := Markdown(&buf, []Gap{{
		ID:       "MOCKTA_GAP_9999",
		Endpoint: "/x",
		Resource: "r",
		Status:   StatusOpen,
		Notes:    "pipe | inside | notes",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), `\|`) != 2 {
		t.Errorf("pipes in notes not escaped: %s", buf.String())
	}
}
