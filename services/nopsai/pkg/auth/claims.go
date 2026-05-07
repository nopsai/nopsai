package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ctxKeyClaims contextKey = "nopsai-auth-claims"
)

// Claims is the normalized token payload used across authenticated requests.
type Claims struct {
	Sub      string   `json:"sub"`
	Email    string   `json:"email,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Roles    []string `json:"roles,omitempty"`
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
