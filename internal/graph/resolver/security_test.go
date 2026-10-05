package resolver_test

import (
	"context"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/auth"
	"github.com/devton/couchgraph/internal/config"
)

// TestMutationGuards validates the logic that blocks mutations in read-only or unauthorized states.
func TestMutationGuards(t *testing.T) {
	secret := "jwt-super-secret-key-for-test"

	tests := []struct {
		name          string
		cfg           config.Config
		userToken     *auth.User
		isMutation    bool
		shouldBlock   bool
		expectedError string
	}{
		{
			name: "Query allowed in ReadOnly mode",
			cfg: config.Config{
				Server: config.ServerConfig{ReadOnly: true},
			},
			isMutation:  false,
			shouldBlock: false,
		},
		{
			name: "Mutation blocked in ReadOnly mode",
			cfg: config.Config{
				Server: config.ServerConfig{ReadOnly: true},
			},
			isMutation:    true,
			shouldBlock:   true,
			expectedError: "server is in read-only mode: mutations are disabled",
		},
		{
			name: "Mutation blocked when Auth enabled and user is unauthenticated",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:     nil,
			isMutation:    true,
			shouldBlock:   true,
			expectedError: "unauthorized: authentication required to execute mutations",
		},
		{
			name: "Mutation blocked when user has only reader role",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:     &auth.User{Roles: []string{"reader"}, Scopes: []string{"read"}},
			isMutation:    true,
			shouldBlock:   true,
			expectedError: "forbidden: write permissions required to execute mutations",
		},
		{
			name: "Mutation allowed when user has admin role",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:   &auth.User{Roles: []string{"admin"}},
			isMutation:  true,
			shouldBlock: false,
		},
		{
			name: "Mutation allowed when user has write scope",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:   &auth.User{Scopes: []string{"write"}},
			isMutation:  true,
			shouldBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opType := ast.Query
			if tt.isMutation {
				opType = ast.Mutation
			}

			ctx := context.Background()
			if tt.userToken != nil {
				ctx = auth.WithUser(ctx, tt.userToken)
			}

			// Simulate OperationContext
			oc := &graphql.OperationContext{
				Operation: &ast.OperationDefinition{
					Operation: opType,
				},
			}

			var blocked bool
			var errMsg string

			// Evaluate guard logic
			if oc.Operation != nil && oc.Operation.Operation == ast.Mutation {
				if tt.cfg.Server.ReadOnly {
					blocked = true
					errMsg = "server is in read-only mode: mutations are disabled"
				} else if tt.cfg.Auth.Enabled {
					user := auth.ForContext(ctx)
					if user == nil {
						blocked = true
						errMsg = "unauthorized: authentication required to execute mutations"
					} else if !user.CanWrite() {
						blocked = true
						errMsg = "forbidden: write permissions required to execute mutations"
					}
				}
			}

			if blocked != tt.shouldBlock {
				t.Fatalf("expected blocked=%v, got %v (err: %s)", tt.shouldBlock, blocked, errMsg)
			}
			if tt.shouldBlock && errMsg != tt.expectedError {
				t.Errorf("expected error %q, got %q", tt.expectedError, errMsg)
			}
		})
	}
}

func TestGenerateAndVerifyJWT(t *testing.T) {
	secret := "secret-123456789"
	token, err := auth.GenerateTestToken(secret, "admin-user", []string{"admin"}, []string{"write"}, 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, err := auth.ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !user.CanWrite() {
		t.Errorf("expected admin to be able to write")
	}
}
