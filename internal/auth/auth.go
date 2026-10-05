// Package auth provides JWT parsing, role validation, and context injection.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/config"
)

type contextKey string

const userCtxKey contextKey = "couchgraph_auth_user"

// User represents the authenticated identity extracted from a JWT token.
type User struct {
	Subject   string         `json:"sub"`
	Email     string         `json:"email,omitempty"`
	Roles     []string       `json:"roles,omitempty"`
	Scopes    []string       `json:"scopes,omitempty"`
	RawClaims map[string]any `json:"raw_claims,omitempty"`
}

// CanWrite returns true if the user possesses admin, editor, or write permissions.
func (u *User) CanWrite() bool {
	if u == nil {
		return false
	}
	for _, r := range u.Roles {
		rLower := strings.ToLower(r)
		if rLower == "admin" || rLower == "editor" || rLower == "writer" || rLower == "write" || rLower == "*" {
			return true
		}
	}
	for _, s := range u.Scopes {
		sLower := strings.ToLower(s)
		if sLower == "write" || sLower == "admin" || sLower == "*" || strings.HasPrefix(sLower, "write:") {
			return true
		}
	}
	return false
}

// WithUser returns a new Context bearing the given User.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// ForContext retrieves the authenticated User from Context, or nil if unauthenticated.
func ForContext(ctx context.Context) *User {
	if u, ok := ctx.Value(userCtxKey).(*User); ok {
		return u
	}
	return nil
}

// Middleware creates an HTTP middleware that extracts and verifies Bearer JWT tokens.
func Middleware(cfg config.AuthConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				if cfg.RequireAuth {
					http.Error(w, `{"errors":[{"message":"unauthorized: authorization header required"}]}`, http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"errors":[{"message":"unauthorized: invalid authorization header format (expected Bearer <token>)"}]}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			user, err := ValidateToken(tokenStr, cfg.JWTSecret)
			if err != nil {
				logger.Debug("jwt validation failed", zap.Error(err))
				http.Error(w, fmt.Sprintf(`{"errors":[{"message":"unauthorized: %s"}]}`, err.Error()), http.StatusUnauthorized)
				return
			}

			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ValidateToken parses and validates a JWT HMAC token string against the given secret.
func ValidateToken(tokenStr, secret string) (*User, error) {
	if secret == "" {
		return nil, errors.New("jwt secret is not configured on the server")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	user := &User{
		RawClaims: make(map[string]any),
	}

	for k, v := range claims {
		user.RawClaims[k] = v
	}

	if sub, ok := claims["sub"].(string); ok {
		user.Subject = sub
	}
	if email, ok := claims["email"].(string); ok {
		user.Email = email
	}

	// Extract roles (supports "roles": ["admin"] or "role": "admin")
	if rawRoles, ok := claims["roles"].([]any); ok {
		for _, r := range rawRoles {
			if str, ok := r.(string); ok {
				user.Roles = append(user.Roles, str)
			}
		}
	} else if singleRole, ok := claims["role"].(string); ok {
		user.Roles = append(user.Roles, singleRole)
	}

	// Extract scopes (supports "scopes": ["read", "write"] or "scope": "read write")
	if rawScopes, ok := claims["scopes"].([]any); ok {
		for _, s := range rawScopes {
			if str, ok := s.(string); ok {
				user.Scopes = append(user.Scopes, str)
			}
		}
	} else if scopeStr, ok := claims["scope"].(string); ok {
		user.Scopes = append(user.Scopes, strings.Fields(scopeStr)...)
	}

	return user, nil
}

// GenerateTestToken generates a signed HMAC-SHA256 token for testing and seeding purposes.
func GenerateTestToken(secret, subject string, roles, scopes []string, duration time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":    subject,
		"roles":  roles,
		"scopes": scopes,
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(duration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
