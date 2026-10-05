// Package config loads and exposes application configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration for CouchGraph.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	CouchDB CouchDBConfig `mapstructure:"couchdb"`
	Auth    AuthConfig    `mapstructure:"auth"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Log     LogConfig     `mapstructure:"log"`
}

// ServerConfig controls the HTTP server and global read-only mode.
type ServerConfig struct {
	// Port to listen on. Default: 8080.
	Port int `mapstructure:"port"`
	// PlaygroundEnabled serves the GraphQL Playground at /. Default: true.
	PlaygroundEnabled bool `mapstructure:"playground_enabled"`
	// ReadOnly disables all GraphQL mutations globally across the server. Default: false.
	ReadOnly bool `mapstructure:"read_only"`
}

// AuthConfig controls JWT authentication and mutation role enforcement.
type AuthConfig struct {
	// Enabled toggles JWT verification on incoming requests. Default: false.
	Enabled bool `mapstructure:"enabled"`
	// JWTSecret is the HMAC-SHA256 secret key used to verify tokens.
	JWTSecret string `mapstructure:"jwt_secret"`
	// RequireAuth requires a valid JWT even for read operations (queries). Default: false.
	RequireAuth bool `mapstructure:"require_auth"`
}

// MetricsConfig controls Prometheus metrics exposition.
type MetricsConfig struct {
	// Enabled exposes the Prometheus /metrics endpoint. Default: true.
	Enabled bool `mapstructure:"enabled"`
	// Path to expose metrics on. Default: "/metrics".
	Path string `mapstructure:"path"`
}

// CouchDBConfig holds CouchDB connection parameters.
type CouchDBConfig struct {
	// URL of the CouchDB instance, e.g. "http://localhost:5984".
	URL string `mapstructure:"url"`
	// User for CouchDB authentication.
	User string `mapstructure:"user"`
	// Password for CouchDB authentication.
	Password string `mapstructure:"password"`
	// Database name to use by default.
	Database string `mapstructure:"database"`
}

// LogConfig controls log output.
type LogConfig struct {
	// Level: debug | info | warn | error. Default: info.
	Level string `mapstructure:"level"`
	// Format: json | console. Default: json.
	Format string `mapstructure:"format"`
}

// Load reads configuration from environment variables (prefixed with COUCHGRAPH_)
// and, if present, from a config.yaml file in the working directory.
func Load() (*Config, error) {
	// ── Auto-load .env file if present ────────────────────────────────────
	loadDotEnv()

	v := viper.New()

	// ── Defaults ──────────────────────────────────────────────────────────
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.playground_enabled", true)
	v.SetDefault("server.read_only", false)
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.require_auth", false)
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("couchdb.url", "http://localhost:5984")
	v.SetDefault("couchdb.user", "admin")
	v.SetDefault("couchdb.password", "password")
	v.SetDefault("couchdb.database", "couchgraph")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// ── Config file (optional) ────────────────────────────────────────────
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	_ = v.ReadInConfig() // ignore "not found" error; env vars take precedence

	// ── Environment variables ─────────────────────────────────────────────
	v.SetEnvPrefix("COUCHGRAPH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal failed: %w", err)
	}

	// ── Fallback for direct un-prefixed env vars (e.g. COUCHDB_PASSWORD, PORT) ─
	if val := os.Getenv("COUCHDB_URL"); val != "" {
		cfg.CouchDB.URL = val
	}
	if val := os.Getenv("COUCHDB_USER"); val != "" {
		cfg.CouchDB.User = val
	}
	if val := os.Getenv("COUCHDB_PASSWORD"); val != "" {
		cfg.CouchDB.Password = val
	}
	if val := os.Getenv("COUCHDB_DATABASE"); val != "" {
		cfg.CouchDB.Database = val
	}
	if val := os.Getenv("PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if val := os.Getenv("PLAYGROUND_ENABLED"); val != "" {
		cfg.Server.PlaygroundEnabled = val == "true" || val == "1"
	}
	if val := os.Getenv("READ_ONLY"); val != "" {
		cfg.Server.ReadOnly = val == "true" || val == "1"
	}
	if val := os.Getenv("MUTATIONS_ENABLED"); val != "" {
		cfg.Server.ReadOnly = !(val == "true" || val == "1")
	}
	if val := os.Getenv("AUTH_ENABLED"); val != "" {
		cfg.Auth.Enabled = val == "true" || val == "1"
	}
	if val := os.Getenv("JWT_SECRET"); val != "" {
		cfg.Auth.JWTSecret = val
	}
	if val := os.Getenv("REQUIRE_AUTH"); val != "" {
		cfg.Auth.RequireAuth = val == "true" || val == "1"
	}
	if val := os.Getenv("METRICS_ENABLED"); val != "" {
		cfg.Metrics.Enabled = val == "true" || val == "1"
	}
	if val := os.Getenv("METRICS_PATH"); val != "" {
		cfg.Metrics.Path = val
	}
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		cfg.Log.Level = val
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		cfg.Log.Format = val
	}

	// ── Sanitize strings (strip whitespace and accidental quotes from env vars) ─
	cfg.CouchDB.User = strings.Trim(strings.TrimSpace(cfg.CouchDB.User), "\"'`")
	cfg.CouchDB.Password = strings.Trim(strings.TrimSpace(cfg.CouchDB.Password), "\"'`")
	cfg.CouchDB.URL = strings.Trim(strings.TrimSpace(cfg.CouchDB.URL), "\"'`")
	cfg.CouchDB.Database = strings.Trim(strings.TrimSpace(cfg.CouchDB.Database), "\"'`")
	cfg.Auth.JWTSecret = strings.Trim(strings.TrimSpace(cfg.Auth.JWTSecret), "\"'`")

	return &cfg, nil
}

// loadDotEnv reads key-value pairs from a local .env file if it exists and sets
// them into the process environment without overriding already-set variables.
func loadDotEnv() {
	content, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, "\"'`")
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}
