package config

import (
	"os"
	"testing"
)

func TestEnvLoading(t *testing.T) {
	os.Setenv("COUCHGRAPH_COUCHDB_PASSWORD", "minha_senha_super_secreta")
	os.Setenv("COUCHGRAPH_COUCHDB_USER", "custom_admin")
	os.Setenv("COUCHGRAPH_COUCHDB_URL", "http://couchdb.internal:5984")
	os.Setenv("COUCHGRAPH_COUCHDB_DATABASE", "my_custom_db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Logf("cfg.CouchDB.Password: %q", cfg.CouchDB.Password)
	t.Logf("cfg.CouchDB.User: %q", cfg.CouchDB.User)
	t.Logf("cfg.CouchDB.URL: %q", cfg.CouchDB.URL)
	t.Logf("cfg.CouchDB.Database: %q", cfg.CouchDB.Database)

	if cfg.CouchDB.Password != "minha_senha_super_secreta" {
		t.Errorf("expected 'minha_senha_super_secreta', got %q", cfg.CouchDB.Password)
	}

	// Test un-prefixed environment variables fallback
	os.Unsetenv("COUCHGRAPH_COUCHDB_PASSWORD")
	os.Unsetenv("COUCHGRAPH_COUCHDB_USER")
	os.Setenv("COUCHDB_PASSWORD", "senha_direta_dokploy")
	os.Setenv("COUCHDB_USER", "user_direto")
	os.Setenv("PORT", "9090")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load with un-prefixed envs failed: %v", err)
	}
	if cfg2.CouchDB.Password != "senha_direta_dokploy" {
		t.Errorf("expected 'senha_direta_dokploy', got %q", cfg2.CouchDB.Password)
	}
	if cfg2.CouchDB.User != "user_direto" {
		t.Errorf("expected 'user_direto', got %q", cfg2.CouchDB.User)
	}
	if cfg2.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg2.Server.Port)
	}
}
