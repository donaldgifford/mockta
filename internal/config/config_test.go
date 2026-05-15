package config

import (
	"log/slog"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv(EnvAdminToken, "")
	t.Setenv(EnvOrgName, "")
	t.Setenv(EnvStrictMode, "")
	t.Setenv(EnvLogLevel, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.OrgName != DefaultOrgName {
		t.Errorf("OrgName = %q, want %q", cfg.OrgName, DefaultOrgName)
	}
	if cfg.StrictMode != DefaultStrictMode {
		t.Errorf("StrictMode = %v, want %v", cfg.StrictMode, DefaultStrictMode)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv(EnvAdminToken, "secret")
	t.Setenv(EnvOrgName, "acme")
	t.Setenv(EnvStrictMode, "false")
	t.Setenv(EnvLogLevel, "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.AdminToken != "secret" {
		t.Errorf("AdminToken = %q, want %q", cfg.AdminToken, "secret")
	}
	if cfg.OrgName != "acme" {
		t.Errorf("OrgName = %q, want %q", cfg.OrgName, "acme")
	}
	if cfg.StrictMode {
		t.Errorf("StrictMode = true, want false")
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelDebug)
	}
}

func TestLoad_BadStrictMode(t *testing.T) {
	t.Setenv(EnvStrictMode, "not-a-bool")
	if _, err := Load(); err == nil {
		t.Error("Load() with bad strict_mode = nil, want error")
	}
}

func TestLoad_BadLogLevel(t *testing.T) {
	t.Setenv(EnvLogLevel, "shouting")
	if _, err := Load(); err == nil {
		t.Error("Load() with bad log_level = nil, want error")
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got, err := parseLogLevel(in)
			if err != nil {
				t.Errorf("parseLogLevel(%q) error = %v", in, err)
			}
			if got != want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", in, got, want)
			}
		})
	}
}
