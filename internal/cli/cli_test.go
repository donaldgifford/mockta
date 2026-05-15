package cli

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewLogger_RespectsLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := newLogger(&buf, slog.LevelInfo)

	// Debug should be suppressed at info level.
	logger.Debug("hidden")
	if buf.Len() != 0 {
		t.Errorf("debug emitted at info level: %s", buf.String())
	}

	logger.Info("visible", "key", "value")
	if !strings.Contains(buf.String(), `"visible"`) {
		t.Errorf("info missing from output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"key":"value"`) {
		t.Errorf("info attrs missing: %s", buf.String())
	}
}

func TestNewLogger_JSONFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := newLogger(&buf, slog.LevelDebug)
	logger.Info("hello")
	// JSON handler always wraps output in braces.
	if !strings.HasPrefix(buf.String(), "{") {
		t.Errorf("not JSON: %s", buf.String())
	}
}

func TestProbeHealth_OK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := probeHealth(t.Context(), srv.URL); err != nil {
		t.Errorf("probeHealth = %v, want nil", err)
	}
}

func TestProbeHealth_Non200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := probeHealth(t.Context(), srv.URL)
	if err == nil {
		t.Error("probeHealth = nil, want non-nil for 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %v, want it to mention 503", err)
	}
}

func TestProbeHealth_TransportError(t *testing.T) {
	t.Parallel()
	// Closed server → connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	err := probeHealth(t.Context(), url)
	if err == nil {
		t.Error("probeHealth against closed server = nil, want error")
	}
}

func TestProbeHealth_BadURL(t *testing.T) {
	t.Parallel()
	if err := probeHealth(t.Context(), "://not-a-url"); err == nil {
		t.Error("probeHealth bad url = nil, want error")
	}
}

func TestVersionCmd_PrintsBuildInfo(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "v1.2.3", Commit: "abc123"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"v1.2.3", "abc123"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestHealthcheckCmd_HitsConfiguredURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := healthcheckURL
	healthcheckURL = srv.URL
	defer func() { healthcheckURL = orig }()

	root := NewRootCmd(BuildInfo{Version: "test"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"healthcheck"})
	if err := root.Execute(); err != nil {
		t.Errorf("execute healthcheck = %v, want nil", err)
	}
}

func TestHealthcheckCmd_FailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	orig := healthcheckURL
	healthcheckURL = srv.URL
	defer func() { healthcheckURL = orig }()

	root := NewRootCmd(BuildInfo{Version: "test"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"healthcheck"})
	if err := root.Execute(); err == nil {
		t.Error("execute healthcheck against 503 = nil, want error")
	}
}

func TestServeCmd_BadConfigReturnsError(t *testing.T) {
	// Set an invalid log level to force config.Load to fail; this
	// covers the early-error path of newServeCmd's RunE without
	// having to bind the real listeners.
	t.Setenv("MOCKTA_LOG_LEVEL", "not-a-level")

	root := NewRootCmd(BuildInfo{Version: "test"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"serve"})
	if err := root.Execute(); err == nil {
		t.Error("execute serve with bad config = nil, want error")
	}
}

func TestHealthcheckCmd_RegisteredOnRoot(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "test"})
	// Discard help output during lookup; cobra prints usage on no-op.
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	found, _, err := root.Find([]string{"healthcheck"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Name() != "healthcheck" {
		t.Errorf("found = %q, want healthcheck", found.Name())
	}
}

func TestServeCmd_RegisteredOnRoot(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(BuildInfo{Version: "test"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	found, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Name() != "serve" {
		t.Errorf("found = %q, want serve", found.Name())
	}
}
