package service

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"

	"nopsai/pkg/serviceauth"
)

func (a *GitBotApp) authenticateInternalRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/webhook" {
			next.ServeHTTP(w, r)
			return
		}
		if a == nil || a.serviceAuth == nil {
			http.Error(w, "service authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		ctx := metadata.NewIncomingContext(r.Context(), metadata.Pairs("authorization", authorization))
		claims, err := a.serviceAuth.AuthenticateContext(ctx)
		if err != nil {
			http.Error(w, "invalid service token", http.StatusUnauthorized)
			return
		}
		if claims.ServiceRole() != serviceauth.RoleNopsai {
			http.Error(w, "nopsai service role required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
