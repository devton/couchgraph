// Package couch provides a CouchDB client and repository built on top of Kivik.
package couch

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	kivik "github.com/go-kivik/kivik/v4"
	"github.com/go-kivik/kivik/v4/couchdb"

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
	targetURL := cfg.URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	var opts []kivik.Option
	if cfg.User != "" && cfg.Password != "" {
		// Use explicit HTTP Basic Authentication on every request (exact same behavior as curl)
		opts = append(opts, couchdb.BasicAuth(cfg.User, cfg.Password))
	}

	kv, err := kivik.New("couch", targetURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("couch: failed to create kivik client: %w", err)
	}

	// Verify connectivity with a lightweight ping.
	if _, err := kv.Ping(ctx); err != nil {
		return nil, fmt.Errorf("couch: ping failed (%s): %w", cfg.URL, err)
	}

	// Ensure system databases (_users, _replicator, _global_changes) and the target database exist.
	dbsToEnsure := []string{"_users", "_replicator", "_global_changes", cfg.Database}
	for _, dbName := range dbsToEnsure {
		exists, err := kv.DBExists(ctx, dbName)
		if err != nil {
			return nil, fmt.Errorf("couch: cannot check database existence (%s): %w", dbName, err)
		}
		if !exists {
			if err := kv.CreateDB(ctx, dbName); err != nil {
				return nil, fmt.Errorf("couch: cannot create database %q: %w", dbName, err)
			}
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

// HTTPClient returns a plain *http.Client for raw CouchDB HTTP calls
// (e.g. _bulk_get) that Kivik does not yet expose via its high-level API.
func (c *Client) HTTPClient() *http.Client {
	return &http.Client{}
}
