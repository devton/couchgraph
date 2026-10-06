// Package server wires an ExecutableSchema (gqlgen-generated or the dynamic
// engine) into the CouchGraph HTTP server: transports, introspection, mutation
// guard, JWT middleware, Prometheus metrics, playground and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/auth"
	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/couch"
)

// NewLogger builds the zap logger configured by cfg.Log.
func NewLogger(cfg *config.Config) (*zap.Logger, error) {
	if cfg.Log.Format == "console" {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

// Connect opens the CouchDB connection and returns a repository.
func Connect(ctx context.Context, cfg *config.Config, log *zap.Logger) (*couch.Repository, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	log.Info("connecting to CouchDB",
		zap.String("url", cfg.CouchDB.URL),
		zap.String("user", cfg.CouchDB.User),
		zap.String("database", cfg.CouchDB.Database),
	)
	client, err := couch.New(ctx, &cfg.CouchDB)
	if err != nil {
		return nil, fmt.Errorf("connect to CouchDB at %s: %w", cfg.CouchDB.URL, err)
	}
	log.Info("connected to CouchDB", zap.String("url", cfg.CouchDB.URL), zap.String("database", cfg.CouchDB.Database))
	return couch.NewRepository(client), nil
}

// Handler builds the HTTP handler serving schema at /query.
func Handler(cfg *config.Config, log *zap.Logger, schema graphql.ExecutableSchema) http.Handler {
	srv := handler.NewDefaultServer(schema)
	srv.AddTransport(transport.Websocket{KeepAlivePingInterval: 10 * time.Second})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.MultipartForm{})
	srv.Use(extension.Introspection{})

	// Mutation guard & auth enforcement (engine-agnostic).
	srv.AroundOperations(auth.MutationGuard(cfg.Server.ReadOnly, cfg.Auth))

	mux := http.NewServeMux()
	mux.Handle("/query", auth.Middleware(cfg.Auth, log)(srv))

	if cfg.Metrics.Enabled {
		path := cfg.Metrics.Path
		if path == "" {
			path = "/metrics"
		}
		mux.Handle(path, promhttp.Handler())
		log.Info("Prometheus metrics enabled", zap.String("path", path))
	}

	if cfg.Server.PlaygroundEnabled {
		mux.Handle("/", playground.Handler("CouchGraph", "/query", playground.WithGraphiqlEnablePluginExplorer(true)))
		log.Info("GraphQL Playground enabled", zap.String("url", fmt.Sprintf("http://localhost:%d", cfg.Server.Port)))
	}
	if cfg.Server.ReadOnly {
		log.Warn("server is running in READ-ONLY mode (all mutations disabled)")
	}
	if cfg.Auth.Enabled {
		log.Info("JWT authentication enabled", zap.Bool("require_auth_for_queries", cfg.Auth.RequireAuth))
	}
	return mux
}

// Run serves h on cfg.Server.Port until SIGINT/SIGTERM, then shuts down
// gracefully.
func Run(cfg *config.Config, log *zap.Logger, h http.Handler) error {
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting CouchGraph server", zap.String("addr", addr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-quit:
	}

	log.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	log.Info("server stopped")
	return nil
}
