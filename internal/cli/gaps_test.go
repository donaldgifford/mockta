package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGapsList_PrintsRegistry(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "test", Commit: "x"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"gaps", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"ID", "ENDPOINT", "RESOURCE", "STATUS",
		"MOCKTA_GAP_0001",
		"MOCKTA_GAP_0010",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestGapsList_RuntimeIsReserved(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "test"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"gaps", "list", "--runtime"})

	if err := root.Execute(); err == nil {
		t.Error("execute with --runtime returned nil, want error")
	}
}

func TestGapsExport_ToStdout(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "test"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"gaps", "export"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "# mockta gap list") {
		t.Errorf("export output missing markdown header:\n%s", buf.String())
	}
}

func TestGapsExport_ToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "gaps.md")

	root := NewRootCmd(BuildInfo{Version: "test"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"gaps", "export", "--out", path})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# mockta gap list") {
		t.Errorf("file output missing markdown header:\n%s", body)
	}
}
