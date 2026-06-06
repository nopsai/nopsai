package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/config"

	"github.com/rs/zerolog/log"
)

type nopsaiWebhookForwarder interface {
	ForwardWebhook(http.ResponseWriter, *http.Request, []byte)
}

type httpNopsaiWebhookForwarder struct {
	baseURL    string
	httpClient *http.Client
}

func newNopsaiWebhookForwarder(cfg *config.Config, httpClient *http.Client) nopsaiWebhookForwarder {
	baseURL := ""
	if cfg != nil {
		baseURL = strings.TrimRight(strings.TrimSpace(cfg.GitBotNopsaiAPIURL), "/")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return httpNopsaiWebhookForwarder{baseURL: baseURL, httpClient: httpClient}
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
