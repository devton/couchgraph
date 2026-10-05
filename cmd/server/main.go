// Command server is the CouchGraph GraphQL server entry point.
package main

import (
	"context"
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
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"

	"github.com/ton/couchgraph/internal/auth"
	"github.com/ton/couchgraph/internal/config"
	"github.com/ton/couchgraph/internal/couch"
	"github.com/ton/couchgraph/internal/graph/generated"
	"github.com/ton/couchgraph/internal/graph/resolver"
)

func main() {
	// ── Config ────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ────────────────────────────────────────────────────────────
	var log *zap.Logger
	if cfg.Log.Format == "console" {
		log, err = zap.NewDevelopment()
	} else {
		log, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	// ── CouchDB ───────────────────────────────────────────────────────────
	connCtx, connCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer connCancel()

	log.Info("connecting to CouchDB",
		zap.String("url", cfg.CouchDB.URL),
		zap.String("user", cfg.CouchDB.User),
		zap.String("database", cfg.CouchDB.Database),
		zap.Int("password_len", len(cfg.CouchDB.Password)),
	)

	couchClient, err := couch.New(connCtx, &cfg.CouchDB)
	if err != nil {
		log.Fatal("failed to connect to CouchDB",
			zap.String("url", cfg.CouchDB.URL),
			zap.String("user", cfg.CouchDB.User),
			zap.Int("password_len", len(cfg.CouchDB.Password)),
			zap.Error(err),
		)
	}
	log.Info("connected to CouchDB",
		zap.String("url", cfg.CouchDB.URL),
		zap.String("database", cfg.CouchDB.Database),
	)

	repo := couch.NewRepository(couchClient)

	// ── GraphQL Server ────────────────────────────────────────────────────
	schema := generated.NewExecutableSchema(generated.Config{
		Resolvers: &resolver.Resolver{Repo: repo},
	})

	srv := handler.NewDefaultServer(schema)

	// Subscriptions & WebSocket transport
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})

	// Additional transports
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.MultipartForm{})

	// Enable introspection
	srv.Use(extension.Introspection{})

	// ── Mutation Guard & Auth Enforcement ─────────────────────────────────
	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil && oc.Operation.Operation == ast.Mutation {
			// 1. Global Read-Only Mode Check
			if cfg.Server.ReadOnly {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "server is in read-only mode: mutations are disabled")
				}
			}

			// 2. JWT Auth & Mutation Role Check (when Auth is enabled)
			if cfg.Auth.Enabled {
				user := auth.ForContext(ctx)
				if user == nil {
					return func(ctx context.Context) *graphql.Response {
						return graphql.ErrorResponse(ctx, "unauthorized: authentication required to execute mutations")
					}
				}
				if !user.CanWrite() {
					return func(ctx context.Context) *graphql.Response {
						return graphql.ErrorResponse(ctx, "forbidden: write permissions required to execute mutations")
					}
				}
			}
		}
		return next(ctx)
	})

	mux := http.NewServeMux()

	// Wrap /query with JWT Auth Middleware
	queryHandler := auth.Middleware(cfg.Auth, log)(srv)
	mux.Handle("/query", queryHandler)

	// Prometheus Metrics Endpoint
	if cfg.Metrics.Enabled {
		metricsPath := cfg.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.Handle(metricsPath, promhttp.Handler())
		log.Info("Prometheus metrics enabled", zap.String("path", metricsPath))
	}

	if cfg.Server.PlaygroundEnabled {
		mux.Handle("/", playground.Handler("CouchGraph", "/query", playground.WithGraphiqlEnablePluginExplorer(true)))
		log.Info("GraphQL Playground enabled",
			zap.String("url", fmt.Sprintf("http://localhost:%d", cfg.Server.Port)),
		)
	}

	if cfg.Server.ReadOnly {
		log.Warn("🔒 Server is running in READ-ONLY mode (all mutations disabled)")
	}

	if cfg.Auth.Enabled {
		log.Info("🛡️ JWT Authentication enabled",
			zap.Bool("require_auth_for_queries", cfg.Auth.RequireAuth),
		)
	}

	// ── HTTP Server ───────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("starting CouchGraph server", zap.String("addr", addr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down server...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}
	log.Info("server stopped")
}
