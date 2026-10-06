package auth

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/config"
)

// Mutation guard rejection messages.
const (
	ErrReadOnly     = "server is in read-only mode: mutations are disabled"
	ErrUnauthorized = "unauthorized: authentication required to execute mutations"
	ErrForbidden    = "forbidden: write permissions required to execute mutations"
)

// MutationRejection returns the reason a mutation must be rejected for the
// given context, or "" when it is allowed.
func MutationRejection(ctx context.Context, readOnly bool, authCfg config.AuthConfig) string {
	if readOnly {
		return ErrReadOnly
	}
	if authCfg.Enabled {
		user := ForContext(ctx)
		if user == nil {
			return ErrUnauthorized
		}
		if !user.CanWrite() {
			return ErrForbidden
		}
	}
	return ""
}

// MutationGuard is a gqlgen operation middleware enforcing read-only mode and
// JWT write permissions on mutations. It is engine-agnostic: it works with
// both the gqlgen-generated schema and the dynamic schema engine.
//
// Rejections are wrapped in graphql.OneShot: a handler returning a response on
// every call never terminates on streaming transports (WebSocket).
func MutationGuard(readOnly bool, authCfg config.AuthConfig) graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil && oc.Operation.Operation == ast.Mutation {
			if msg := MutationRejection(ctx, readOnly, authCfg); msg != "" {
				return graphql.OneShot(graphql.ErrorResponse(ctx, "%s", msg))
			}
		}
		return next(ctx)
	}
}
