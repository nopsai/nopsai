package auth

import (
	"crypto/rand"
	"encoding/base64"
)

const (
	ProviderPersonalAccessToken = "personal-token"
	PersonalAccessTokenPrefix   = "nopat_"
	personalAccessTokenBytes    = 32
	personalAccessTokenSuffix   = 8
)

func GeneratePersonalAccessToken() (string, error) {
	buf := make([]byte, personalAccessTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return PersonalAccessTokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func PersonalAccessTokenSuffix(token string) string {
	if len(token) <= personalAccessTokenSuffix {
		return token
	}
	return token[len(token)-personalAccessTokenSuffix:]
}
