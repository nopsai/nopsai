package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/config"
	"nopsai/pkg/serviceauth"

	"github.com/rs/zerolog/log"
)

type nopsaiWebhookForwarder interface {
	ForwardWebhook(http.ResponseWriter, *http.Request, []byte)
}

type httpNopsaiWebhookForwarder struct {
	baseURL     string
	httpClient  *http.Client
	credentials *serviceauth.Credentials
}

func newNopsaiWebhookForwarder(cfg *config.Config, httpClient *http.Client, credentials *serviceauth.Credentials) nopsaiWebhookForwarder {
	baseURL := ""
	if cfg != nil {
		baseURL = strings.TrimRight(strings.TrimSpace(cfg.GitBotNopsaiAPIURL), "/")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return httpNopsaiWebhookForwarder{baseURL: baseURL, httpClient: httpClient, credentials: credentials}
}

func (f httpNopsaiWebhookForwarder) ForwardWebhook(w http.ResponseWriter, r *http.Request, body []byte) {
	forwardURL := fmt.Sprintf("%s/v1/git/events", f.baseURL)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, forwardURL, bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request to nopsai event endpoint")
		http.Error(w, "Failed to forward event", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for _, header := range []string{"X-GitHub-Event", "X-GitHub-Delivery", "X-GitHub-Enterprise-Host", "X-GitHub-Enterprise-Version"} {
		if value := r.Header.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
	req.Header.Set("X-Nopsai-Forwarded-By", "git-bot")
	if f.credentials == nil {
		http.Error(w, "git-bot service credentials are unavailable", http.StatusServiceUnavailable)
		return
	}
	token, err := f.credentials.MintToken(r.Context())
	if err != nil {
		http.Error(w, "failed to authenticate webhook forwarding", http.StatusServiceUnavailable)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to forward event to nopsai")
		http.Error(w, "Failed to forward event", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Error().Err(err).Msg("Failed to proxy response body")
	}
}
