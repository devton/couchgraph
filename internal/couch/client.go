// Package couch provides a CouchDB client and repository built on top of Kivik.
package couch

import (
	"context"
	"fmt"
	"net/http"

	_ "github.com/go-kivik/kivik/v4/couchdb" // register CouchDB driver
	kivik "github.com/go-kivik/kivik/v4"

	"github.com/ton/couchgraph/internal/config"
)

// Client wraps a Kivik client and exposes the active database.
type Client struct {
	kv  *kivik.Client
	db  *kivik.DB
	cfg *config.CouchDBConfig
}

// New creates and validates a connection to CouchDB using the provided config.
func New(ctx context.Context, cfg *config.CouchDBConfig) (*Client, error) {
	dsn := buildDSN(cfg)

	kv, err := kivik.New("couch", dsn)
	if err != nil {
		return nil, fmt.Errorf("couch: failed to create kivik client: %w", err)
	}

	// Verify connectivity with a lightweight ping.
	if _, err := kv.Ping(ctx); err != nil {
		return nil, fmt.Errorf("couch: ping failed (%s): %w", cfg.URL, err)
	}

	// Ensure the target database exists; create it if it does not.
	exists, err := kv.DBExists(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("couch: cannot check database existence: %w", err)
	}
	if !exists {
		if err := kv.CreateDB(ctx, cfg.Database); err != nil {
			return nil, fmt.Errorf("couch: cannot create database %q: %w", cfg.Database, err)
		}
	}

	db := kv.DB(cfg.Database)

	return &Client{
		kv:  kv,
		db:  db,
		cfg: cfg,
	}, nil
}

// DB returns the active Kivik database handle.
func (c *Client) DB() *kivik.DB { return c.db }

// Kivik returns the raw Kivik client for advanced operations.
func (c *Client) Kivik() *kivik.Client { return c.kv }

// buildDSN constructs a CouchDB DSN with embedded credentials.
// Format: http://user:password@host:port
func buildDSN(cfg *config.CouchDBConfig) string {
	// Insert credentials into the URL.
	// Kivik accepts the standard http://user:pass@host DSN format.
	url := cfg.URL

	// Only inject credentials if both are present.
	if cfg.User != "" && cfg.Password != "" {
		// Strip any existing scheme prefix, then re-attach with credentials.
		const httpScheme  = "http://"
		const httpsScheme = "https://"

		switch {
		case len(url) > len(httpsScheme) && url[:len(httpsScheme)] == httpsScheme:
			url = httpsScheme + cfg.User + ":" + cfg.Password + "@" + url[len(httpsScheme):]
		case len(url) > len(httpScheme) && url[:len(httpScheme)] == httpScheme:
			url = httpScheme + cfg.User + ":" + cfg.Password + "@" + url[len(httpScheme):]
		}
	}

	return url
}

// HTTPClient returns a plain *http.Client for raw CouchDB HTTP calls
// (e.g. _bulk_get) that Kivik does not yet expose via its high-level API.
func (c *Client) HTTPClient() *http.Client {
	return &http.Client{}
}
