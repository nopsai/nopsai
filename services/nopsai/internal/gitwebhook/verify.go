package gitwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	AuthModeHMAC        = "hmac"
	AuthModeStaticToken = "static_token"
	AuthModeNone        = "none"
)

func Verify(provider, authMode, secret string, headers http.Header, body []byte, now time.Time) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	authMode = strings.ToLower(strings.TrimSpace(authMode))
	secret = strings.TrimSpace(secret)
	switch authMode {
	case AuthModeNone:
		return nil
	case AuthModeStaticToken:
		if secret == "" {
			return fmt.Errorf("webhook credential is empty")
		}
		return verifyStaticToken(provider, secret, headers)
	case AuthModeHMAC:
		if secret == "" {
			return fmt.Errorf("webhook credential is empty")
		}
		if provider == ProviderGitLab && strings.TrimSpace(headers.Get("webhook-signature")) != "" {
			return verifyStandardWebhook(secret, headers, body, now)
		}
		return verifyHMAC(provider, secret, headers, body)
	default:
		return fmt.Errorf("unsupported webhook auth mode %q", authMode)
	}
}

func verifyStaticToken(provider, secret string, headers http.Header) error {
	provided := ""
	switch provider {
	case ProviderGitLab:
		provided = headers.Get("X-Gitlab-Token")
	default:
		provided = firstNonEmpty(headers.Get("X-Nopsai-Token"), strings.TrimPrefix(headers.Get("Authorization"), "Bearer "))
	}
	if !secureEqual(secret, strings.TrimSpace(provided)) {
		return fmt.Errorf("webhook token verification failed")
	}
	return nil
}

func verifyHMAC(provider, secret string, headers http.Header, body []byte) error {
	signature := ""
	switch provider {
	case ProviderBitbucket:
		signature = headers.Get("X-Hub-Signature")
	case ProviderGitea:
		signature = headers.Get("X-Gitea-Signature")
		if signature == "" {
			signature = headers.Get("X-Hub-Signature-256")
		}
	default:
		signature = firstNonEmpty(headers.Get("X-Nopsai-Signature-256"), headers.Get("X-Hub-Signature-256"))
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return fmt.Errorf("webhook signature is missing")
	}
	algorithm, digest := "sha256", signature
	if prefix, value, ok := strings.Cut(signature, "="); ok {
		algorithm, digest = strings.ToLower(strings.TrimSpace(prefix)), strings.TrimSpace(value)
	}
	if algorithm != "sha256" {
		return fmt.Errorf("unsupported webhook signature algorithm %q", algorithm)
	}
	provided, err := hex.DecodeString(digest)
	if err != nil {
		return fmt.Errorf("webhook signature is not valid hexadecimal")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return fmt.Errorf("webhook signature verification failed")
	}
	return nil
}

func verifyStandardWebhook(secret string, headers http.Header, body []byte, now time.Time) error {
	messageID := strings.TrimSpace(headers.Get("webhook-id"))
	timestamp := strings.TrimSpace(headers.Get("webhook-timestamp"))
	signatures := strings.Fields(headers.Get("webhook-signature"))
	if messageID == "" || timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("standard webhook signature headers are incomplete")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("standard webhook timestamp is invalid")
	}
	sentAt := time.Unix(seconds, 0)
	if now.IsZero() {
		now = time.Now()
	}
	if delta := now.Sub(sentAt); delta > 5*time.Minute || delta < -5*time.Minute {
		return fmt.Errorf("standard webhook timestamp is outside the allowed window")
	}

	keyText := strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil {
		return fmt.Errorf("standard webhook signing token is invalid")
	}
	message := messageID + "." + timestamp + "." + string(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	expected := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, signature := range signatures {
		if secureEqual(expected, signature) {
			return nil
		}
	}
	return fmt.Errorf("webhook signature verification failed")
}

func secureEqual(expected, actual string) bool {
	return hmac.Equal([]byte(expected), []byte(actual))
}
