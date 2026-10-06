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

// TestMutationGuards exercises auth.MutationGuard (the middleware wired in
// cmd/server) for read-only and unauthorized states.
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
			expectedError: auth.ErrReadOnly,
		},
		{
			name: "Mutation blocked when Auth enabled and user is unauthenticated",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:     nil,
			isMutation:    true,
			shouldBlock:   true,
			expectedError: auth.ErrUnauthorized,
		},
		{
			name: "Mutation blocked when user has only reader role",
			cfg: config.Config{
				Auth: config.AuthConfig{Enabled: true, JWTSecret: secret},
			},
			userToken:     &auth.User{Roles: []string{"reader"}, Scopes: []string{"read"}},
			isMutation:    true,
			shouldBlock:   true,
			expectedError: auth.ErrForbidden,
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
			ctx = graphql.WithOperationContext(ctx, &graphql.OperationContext{
				Operation: &ast.OperationDefinition{Operation: opType},
			})
			ctx = graphql.WithResponseContext(ctx, graphql.DefaultErrorPresenter, graphql.DefaultRecover)

			nextCalled := false
			next := func(ctx context.Context) graphql.ResponseHandler {
				nextCalled = true
				return graphql.OneShot(&graphql.Response{Data: []byte(`{}`)})
			}

			guard := auth.MutationGuard(tt.cfg.Server.ReadOnly, tt.cfg.Auth)
			handler := guard(ctx, next)

			if blocked := !nextCalled; blocked != tt.shouldBlock {
				t.Fatalf("expected blocked=%v, got %v", tt.shouldBlock, blocked)
			}

			resp := handler(ctx)
			if resp == nil {
				t.Fatal("expected a response on first call")
			}
			if tt.shouldBlock {
				if len(resp.Errors) != 1 || resp.Errors[0].Message != tt.expectedError {
					t.Errorf("expected error %q, got %v", tt.expectedError, resp.Errors)
				}
			}

			// Streaming transports (WebSocket) keep calling the handler until
			// it returns nil; rejections must terminate after one response.
			if again := handler(ctx); again != nil {
				t.Errorf("expected nil on second call (one-shot), got %+v", again)
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
