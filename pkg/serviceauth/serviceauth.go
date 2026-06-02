package serviceauth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const (
	ProviderInternalService = "internal-service"

	RoleNopsai     = "nopsai"
	RoleRunner     = "runner"
	RoleAgent      = "agent"
	RoleDispatcher = "dispatcher"

	EnvSigningKey = "SERVICE_JWT_SIGNING_KEY"
	EnvIssuer     = "SERVICE_JWT_ISSUER"
	EnvAudience   = "SERVICE_JWT_AUDIENCE"
	EnvServiceID  = "SERVICE_ID"

	DefaultIssuer   = "nopsai.internal"
	DefaultAudience = "nopsai-dispatcher"
)

type contextKey string

const ctxKeyClaims contextKey = "nopsai-service-auth-claims"

// Claims identifies an internal service caller. Subject carries the service
// identity, while Role is the dispatchable service class used for RPC access.
type Claims struct {
	Provider string `json:"provider,omitempty"`
	Role     string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func (c *Claims) ServiceID() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Subject)
}

func (c *Claims) ServiceRole() string {
	if c == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(c.Role))
}

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	value := ctx.Value(ctxKeyClaims)
	if value == nil {
		return nil, false
	}
	claims, ok := value.(*Claims)
	return claims, ok
}

type Config struct {
	SigningKey string
	Issuer     string
	Audience   string
	TTL        time.Duration
	Role       string
	ServiceID  string
}

func (c Config) normalized() Config {
	c.SigningKey = strings.TrimSpace(c.SigningKey)
	c.Issuer = strings.TrimSpace(c.Issuer)
	if c.Issuer == "" {
		c.Issuer = DefaultIssuer
	}
	c.Audience = strings.TrimSpace(c.Audience)
	if c.Audience == "" {
		c.Audience = DefaultAudience
	}
	c.Role = strings.ToLower(strings.TrimSpace(c.Role))
	c.ServiceID = strings.TrimSpace(c.ServiceID)
	if c.ServiceID == "" {
		c.ServiceID = c.Role
	}
	if c.TTL <= 0 {
		c.TTL = 5 * time.Minute
	}
	return c
}

type Credentials struct {
	cfg Config
}

func NewCredentials(cfg Config) (*Credentials, error) {
	cfg = cfg.normalized()
	if cfg.SigningKey == "" {
		return nil, fmt.Errorf("service JWT signing key is not configured")
	}
	if cfg.Role == "" {
		return nil, fmt.Errorf("service role is required")
	}
	if cfg.ServiceID == "" {
		return nil, fmt.Errorf("service id is required")
	}
	return &Credentials{cfg: cfg}, nil
}

func (c *Credentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	token, err := c.MintToken(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

func (c *Credentials) RequireTransportSecurity() bool {
	return false
}

func (c *Credentials) MintToken(ctx context.Context) (string, error) {
	_ = ctx
	if c == nil {
		return "", fmt.Errorf("service credentials are not configured")
	}

	now := time.Now()
	claims := Claims{
		Provider: ProviderInternalService,
		Role:     c.cfg.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.cfg.ServiceID,
			Issuer:    c.cfg.Issuer,
			Audience:  []string{c.cfg.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(c.cfg.TTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(c.cfg.SigningKey))
}

var _ credentials.PerRPCCredentials = (*Credentials)(nil)

type Authenticator struct {
	signingKey string
	issuer     string
	audience   string
}

func NewAuthenticator(cfg Config) (*Authenticator, error) {
	cfg = cfg.normalized()
	if cfg.SigningKey == "" {
		return nil, fmt.Errorf("service JWT signing key is not configured")
	}
	return &Authenticator{
		signingKey: cfg.SigningKey,
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
	}, nil
}

func (a *Authenticator) AuthenticateContext(ctx context.Context) (*Claims, error) {
	if a == nil {
		return nil, fmt.Errorf("service authenticator is not configured")
	}
	raw, err := bearerTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	}
	if a.issuer != "" {
		opts = append(opts, jwt.WithIssuer(a.issuer))
	}
	if a.audience != "" {
		opts = append(opts, jwt.WithAudience(a.audience))
	}

	claims := &Claims{}
	token, err := jwt.NewParser(opts...).ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.signingKey), nil
	})
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, fmt.Errorf("service token is invalid")
	}
	if strings.TrimSpace(claims.Provider) != ProviderInternalService {
		return nil, fmt.Errorf("service token provider is invalid")
	}
	if claims.ServiceRole() == "" {
		return nil, fmt.Errorf("service token role is required")
	}
	if claims.ServiceID() == "" {
		return nil, fmt.Errorf("service token subject is required")
	}
	return claims, nil
}

func bearerTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("authorization metadata is missing")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", fmt.Errorf("authorization metadata is missing")
	}
	for _, value := range values {
		header := strings.TrimSpace(value)
		if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
			continue
		}
		token := strings.TrimSpace(header[len("Bearer "):])
		if token == "" {
			return "", fmt.Errorf("bearer token is empty")
		}
		return token, nil
	}
	return "", fmt.Errorf("bearer token is missing")
}
