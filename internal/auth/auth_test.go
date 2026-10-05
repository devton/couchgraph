package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/auth"
	"github.com/devton/couchgraph/internal/config"
)

func TestUser_CanWrite(t *testing.T) {
	tests := []struct {
		name     string
		user     *auth.User
		canWrite bool
	}{
		{
			name:     "Nil user",
			user:     nil,
			canWrite: false,
		},
		{
			name:     "User with admin role",
			user:     &auth.User{Roles: []string{"admin"}},
			canWrite: true,
		},
		{
			name:     "User with write scope",
			user:     &auth.User{Scopes: []string{"read", "write"}},
			canWrite: true,
		},
		{
			name:     "User with wildcard role",
			user:     &auth.User{Roles: []string{"*"}},
			canWrite: true,
		},
		{
			name:     "User with reader role only",
			user:     &auth.User{Roles: []string{"reader"}, Scopes: []string{"read"}},
			canWrite: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.CanWrite(); got != tt.canWrite {
				t.Errorf("CanWrite() = %v, want %v", got, tt.canWrite)
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	secret := "my-ultra-secret-test-key-12345"

	// Valid admin token
	token, err := auth.GenerateTestToken(secret, "user-123", []string{"admin"}, []string{"write"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	user, err := auth.ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if user.Subject != "user-123" {
		t.Errorf("expected subject user-123, got %s", user.Subject)
	}
	if !user.CanWrite() {
		t.Errorf("expected user to have write permissions")
	}

	// Invalid secret
	_, err = auth.ValidateToken(token, "wrong-secret")
	if err == nil {
		t.Errorf("expected error with wrong secret, got nil")
	}

	// Expired token
	expiredToken, _ := auth.GenerateTestToken(secret, "user-exp", []string{"admin"}, nil, -1*time.Hour)
	_, err = auth.ValidateToken(expiredToken, secret)
	if err == nil {
		t.Errorf("expected error for expired token, got nil")
	}
}

func TestMiddleware(t *testing.T) {
	secret := "test-secret"
	cfg := config.AuthConfig{
		Enabled:   true,
		JWTSecret: secret,
	}

	mw := auth.Middleware(cfg, zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.ForContext(r.Context())
		if user != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(user.Subject))
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))

	// 1. Unauthenticated request (allowed when require_auth is false)
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for unauthenticated when require_auth=false, got %d", rec.Code)
	}

	// 2. Valid token request
	token, _ := auth.GenerateTestToken(secret, "user-456", []string{"editor"}, nil, 1*time.Hour)
	req2 := httptest.NewRequest(http.MethodGet, "/query", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid token, got %d", rec2.Code)
	}
	if rec2.Body.String() != "user-456" {
		t.Errorf("expected body user-456, got %s", rec2.Body.String())
	}
}
