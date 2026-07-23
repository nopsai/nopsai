package nopsai

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/runnerinstall"
)

type runnerBootstrapToken = runnerinstall.BootstrapToken

func (a *App) createRunnerBootstrapToken(content string, ttl time.Duration, contentType string) (string, time.Time, error) {
	return a.createRunnerBootstrapTokenWithBuilder(content, ttl, contentType, nil)
}

func (a *App) createRunnerBootstrapTokenWithBuilder(
	content string,
	ttl time.Duration,
	contentType string,
	builder runnerinstall.BootstrapContentBuilder,
) (string, time.Time, error) {
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
		Content:        content,
		ContentType:    contentType,
		ExpiresAt:      expiresAt,
		ContentBuilder: builder,
	}
	return token, expiresAt, nil
}

func (a *App) consumeRunnerBootstrapToken(ctx context.Context, token string) (runnerBootstrapToken, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return runnerBootstrapToken{}, false, nil
	}
	a.runnerBootstrapMu.Lock()
	if a.runnerBootstrapTokens == nil {
		a.runnerBootstrapMu.Unlock()
		return runnerBootstrapToken{}, false, nil
	}
	entry, ok := a.runnerBootstrapTokens[token]
	if !ok {
		a.runnerBootstrapMu.Unlock()
		return runnerBootstrapToken{}, false, nil
	}
	delete(a.runnerBootstrapTokens, token)
	a.runnerBootstrapMu.Unlock()
	if time.Now().After(entry.ExpiresAt) {
		return runnerBootstrapToken{}, false, nil
	}
	if strings.TrimSpace(entry.ContentType) == "" {
		entry.ContentType = "text/plain; charset=utf-8"
	}
	if entry.ContentBuilder != nil {
		content, contentType, err := entry.ContentBuilder(ctx)
		if err != nil {
			return runnerBootstrapToken{}, false, err
		}
		if strings.TrimSpace(content) == "" {
			return runnerBootstrapToken{}, false, fmt.Errorf("runner bootstrap content is empty")
		}
		entry.Content = content
		if strings.TrimSpace(contentType) != "" {
			entry.ContentType = contentType
		}
		entry.ContentBuilder = nil
	}
	return entry, true, nil
}
