package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ctxKeyClaims contextKey = "nopsai-auth-claims"
	ctxKeyTenant contextKey = "nopsai-tenant"
)

// Claims is the normalized token payload used across authenticated requests.
type Claims struct {
	Sub           string   `json:"sub"`
	Email         string   `json:"email,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	TenantIDs     []string `json:"tenant_ids,omitempty"`
	Roles         []string `json:"roles,omitempty"`
	DefaultTenant string   `json:"default_tenant,omitempty"`
	jwt.RegisteredClaims
}

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	val := ctx.Value(ctxKeyClaims)
	if val == nil {
		return nil, false
	}
	claims, ok := val.(*Claims)
	return claims, ok
}

func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKeyTenant, tenantID)
}

func TenantFromContext(ctx context.Context) string {
	val := ctx.Value(ctxKeyTenant)
	if val == nil {
		return ""
	}
	if tenant, ok := val.(string); ok {
		return tenant
	}
	return ""
}
