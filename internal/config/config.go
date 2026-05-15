// Package config loads mockta's runtime configuration from environment
// variables.
//
// Per DESIGN-0001 and IMPL-0001 Q2, v0 keeps config strictly env-driven
// — no viper, no config files. Future durability (file snapshots, etc.)
// will arrive as new env vars rather than a new mechanism.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// Environment variable names. Exported as constants so handlers and tests
// can reference them by symbol rather than re-typing string literals.
const (
	//nolint:gosec // env var name, not a credential value
	EnvAdminToken = "MOCKTA_ADMIN_TOKEN"
	EnvOrgName    = "MOCKTA_ORG_NAME"
	EnvStrictMode = "MOCKTA_STRICT_MODE"
	EnvLogLevel   = "MOCKTA_LOG_LEVEL"
)

// Defaults for env vars that fall back when unset.
const (
	DefaultOrgName    = "mockta-dev"
	DefaultStrictMode = true
	DefaultLogLevel   = slog.LevelInfo
)

// Config is the resolved runtime configuration. It is passed into
// pkg/mockta.New and used by handlers via the Server.
type Config struct {
	// AdminToken is the bearer token clients must present. An empty value
	// means "generate one at startup" (Phase 3 wires the generation up);
	// loading config does not enforce non-empty.
	AdminToken string

	// OrgName is surfaced via /api/v1/org and embedded in generated IDs.
	OrgName string

	// StrictMode toggles Okta-documented validation. False is "accept
	// anything well-formed," useful for negative tests.
	StrictMode bool

	// LogLevel controls the slog.Handler verbosity.
	LogLevel slog.Level
}

// Load reads environment variables into a Config. Defaults are applied for
// missing values. Parse errors on optional fields are returned to the
// caller rather than logged-and-ignored, so a misconfigured container
// fails fast at startup.
func Load() (Config, error) {
	cfg := Config{
		AdminToken: os.Getenv(EnvAdminToken),
		OrgName:    lookupOrDefault(EnvOrgName, DefaultOrgName),
		StrictMode: DefaultStrictMode,
		LogLevel:   DefaultLogLevel,
	}

	if v, ok := os.LookupEnv(EnvStrictMode); ok {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s=%q: %w", EnvStrictMode, v, err)
		}
		cfg.StrictMode = parsed
	}

	if v, ok := os.LookupEnv(EnvLogLevel); ok {
		level, err := parseLogLevel(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", EnvLogLevel, err)
		}
		cfg.LogLevel = level
	}

	return cfg, nil
}

func lookupOrDefault(env, def string) string {
	if v, ok := os.LookupEnv(env); ok {
		return v
	}
	return def
}

func parseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown level %q (want debug|info|warn|error)", s)
	}
}
