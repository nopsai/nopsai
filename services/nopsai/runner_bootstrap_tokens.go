package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/runnerinstall"
)

type runnerBootstrapToken = runnerinstall.BootstrapToken

func (a *App) createRunnerBootstrapToken(content string, ttl time.Duration, contentType string) (string, time.Time, error) {
	if strings.TrimSpace(content) == "" {
		return "", time.Time{}, fmt.Errorf("runner bootstrap content is empty")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, fmt.Errorf("generate runner bootstrap token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	expiresAt := time.Now().Add(ttl)

	a.runnerBootstrapMu.Lock()
	defer a.runnerBootstrapMu.Unlock()
	if a.runnerBootstrapTokens == nil {
		a.runnerBootstrapTokens = make(map[string]runnerBootstrapToken)
	}
	now := time.Now()
	for existing, entry := range a.runnerBootstrapTokens {
		if now.After(entry.ExpiresAt) {
			delete(a.runnerBootstrapTokens, existing)
		}
	}
	a.runnerBootstrapTokens[token] = runnerBootstrapToken{
		Content:     content,
		ContentType: contentType,
		ExpiresAt:   expiresAt,
	}
	return token, expiresAt, nil
}

func (a *App) consumeRunnerBootstrapToken(token string) (runnerBootstrapToken, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return runnerBootstrapToken{}, false
	}
	a.runnerBootstrapMu.Lock()
	defer a.runnerBootstrapMu.Unlock()
	if a.runnerBootstrapTokens == nil {
		return runnerBootstrapToken{}, false
	}
	entry, ok := a.runnerBootstrapTokens[token]
	if !ok {
		return runnerBootstrapToken{}, false
	}
	delete(a.runnerBootstrapTokens, token)
	if time.Now().After(entry.ExpiresAt) {
		return runnerBootstrapToken{}, false
	}
	if strings.TrimSpace(entry.ContentType) == "" {
		entry.ContentType = "text/plain; charset=utf-8"
	}
	return entry, true
}
