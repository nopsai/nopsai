package auth

import (
	"crypto/rand"
	"encoding/base64"
)

const (
	ProviderPersonalAccessToken = "personal-token"
	ProviderServiceAccount      = "service-account"
	ProviderServiceAccountToken = "service-account-token"
	PersonalAccessTokenPrefix   = "nopat_"
	ServiceAccountTokenPrefix   = "nopsat_"
	personalAccessTokenBytes    = 32
	personalAccessTokenSuffix   = 8
)

func GeneratePersonalAccessToken() (string, error) {
	return generateOpaqueToken(PersonalAccessTokenPrefix)
}

func GenerateServiceAccountToken() (string, error) {
	return generateOpaqueToken(ServiceAccountTokenPrefix)
}

func generateOpaqueToken(prefix string) (string, error) {
	buf := make([]byte, personalAccessTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func PersonalAccessTokenSuffix(token string) string {
	if len(token) <= personalAccessTokenSuffix {
		return token
	}
	return token[len(token)-personalAccessTokenSuffix:]
}
