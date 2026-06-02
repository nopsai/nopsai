package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type LocalJWTService struct {
	signingKey []byte
	issuer     string
	audience   string
	accessTTL  time.Duration
}

func NewLocalJWTService(signingKey []byte, issuer, audience string, accessTTL time.Duration) *LocalJWTService {
	return &LocalJWTService{
		signingKey: signingKey,
		issuer:     issuer,
		audience:   audience,
		accessTTL:  accessTTL,
	}
}

func (s *LocalJWTService) MintAccessToken(ctx context.Context, baseClaims Claims) (string, time.Time, error) {
	if len(s.signingKey) == 0 {
		return "", time.Time{}, fmt.Errorf("local JWT signing key is not configured")
	}
	now := time.Now()
	exp := now.Add(s.accessTTL)
	baseClaims.Issuer = s.issuer
	if baseClaims.Audience == nil && s.audience != "" {
		baseClaims.Audience = []string{s.audience}
	}
	baseClaims.ExpiresAt = jwt.NewNumericDate(exp)
	baseClaims.IssuedAt = jwt.NewNumericDate(now)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims)
	signed, err := token.SignedString(s.signingKey)
	return signed, exp, err
}

func (s *LocalJWTService) ParseAndValidate(raw string) (*Claims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	}
	if s.issuer != "" {
		opts = append(opts, jwt.WithIssuer(s.issuer))
	}
	if s.audience != "" {
		opts = append(opts, jwt.WithAudience(s.audience))
	}
	parser := jwt.NewParser(opts...)
	claims := &Claims{}
	_, err := parser.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		return s.signingKey, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func GenerateRefreshToken() (string, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
