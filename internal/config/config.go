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
	Server   ServerConfig   `mapstructure:"server"`
	CouchDB  CouchDBConfig  `mapstructure:"couchdb"`
	Log      LogConfig      `mapstructure:"log"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	// Port to listen on. Default: 8080.
	Port int `mapstructure:"port"`
	// PlaygroundEnabled serves the GraphQL Playground at /. Default: true.
	PlaygroundEnabled bool `mapstructure:"playground_enabled"`
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
//
// Environment variable mapping (examples):
//
//	COUCHGRAPH_SERVER_PORT=8080
//	COUCHGRAPH_COUCHDB_URL=http://localhost:5984
//	COUCHGRAPH_COUCHDB_USER=admin
//	COUCHGRAPH_COUCHDB_PASSWORD=secret
//	COUCHGRAPH_COUCHDB_DATABASE=mydb
//	COUCHGRAPH_LOG_LEVEL=debug
func Load() (*Config, error) {
	v := viper.New()

	// ── Defaults ──────────────────────────────────────────────────────────
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.playground_enabled", true)
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

	return &cfg, nil
}
