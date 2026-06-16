package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"nopsai/pkg/proxyhttp"

	aaaauthz "nopsai/services/aaa/pkg/authz"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/aaaclient"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/authz"

	"github.com/rs/zerolog/log"
)

type appSecurityRuntime struct {
	authService  *auth.Service
	authz        *authz.Enforcer
	aaaClient    AAAClient
	localAAA     AAAClient
	auditLogger  *audit.Logger
	internalHTTP *http.Client
	gitProvider  GitProvider
}

func newAppSecurityRuntime(ctx context.Context, options AppOptions) (appSecurityRuntime, error) {
	authCfg := auth.Config{
		LocalEnabled:       options.Config.EffectiveAuthProviderLocalEnabled(),
		SigningKey:         options.Config.JWTSigningKey,
		JWTIssuer:          options.Config.JWTIssuer,
		JWTAudience:        options.Config.JWTAudience,
		AccessTTL:          time.Duration(options.Config.JWTExpiryMinutes) * time.Minute,
		RefreshTTL:         time.Duration(options.Config.RefreshTokenTTLMinutes) * time.Minute,
		LoginRateLimit:     options.Config.RateLimitLoginPerMinute,
		LoginLockoutThresh: options.Config.LoginLockoutThreshold,
		LoginLockoutWindow: time.Duration(options.Config.LoginLockoutWindowMin) * time.Minute,
	}

	authService, err := auth.NewService(ctx, options.Database, authCfg)
	if err != nil {
		return appSecurityRuntime{}, fmt.Errorf("initialize authentication service: %w", err)
	}
	authzEnforcer, err := authz.NewEnforcer(ctx, options.Database)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load RBAC policies")
	}

	httpClient := proxyhttp.NewInternalAwareClient(10 * time.Second)
	gitProvider := options.GitProvider
	aaaClient := options.AAAClient
	if aaaClient == nil {
		aaaClient = aaaclient.New(options.Config.AAAAPIURL, options.Config.AAASharedToken)
	}
	localAAAClient := options.LocalAAAClient
	if localAAAClient == nil {
		localAAAClient = aaaauthz.NewEvaluator(aaastore.NewPGStore(options.Database))
	}

	return appSecurityRuntime{
		authService:  authService,
		authz:        authzEnforcer,
		aaaClient:    aaaClient,
		localAAA:     localAAAClient,
		auditLogger:  audit.NewLogger(options.Database),
		internalHTTP: httpClient,
		gitProvider:  gitProvider,
	}, nil
}
