// Command server is the CouchGraph GraphQL server entry point (env/config.yaml
// driven). For standalone projects driven by couchgraph.yaml, use cmd/couchgraph.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/99designs/gqlgen/graphql"
	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/engine"
	"github.com/devton/couchgraph/internal/graph/generated"
	"github.com/devton/couchgraph/internal/graph/resolver"
	"github.com/devton/couchgraph/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := server.NewLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	repo, err := server.Connect(context.Background(), cfg, log)
	if err != nil {
		log.Fatal("failed to connect to CouchDB", zap.Error(err))
	}

	var schema graphql.ExecutableSchema
	switch cfg.Server.Engine {
	case "", "gqlgen":
		schema = generated.NewExecutableSchema(generated.Config{
			Resolvers: &resolver.Resolver{Repo: repo},
		})
	case "dynamic":
		// Core API only; typed domains are served by `couchgraph serve`.
		dyn, err := engine.Build(engine.Options{Store: repo, Core: true})
		if err != nil {
			log.Fatal("failed to build dynamic schema", zap.Error(err))
		}
		schema = dyn
	default:
		log.Fatal("unknown ENGINE (expected gqlgen or dynamic)", zap.String("engine", cfg.Server.Engine))
	}
	log.Info("GraphQL engine selected", zap.String("engine", cfg.Server.Engine))

	if err := server.Run(cfg, log, server.Handler(cfg, log, schema)); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}
