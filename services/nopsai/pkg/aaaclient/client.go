package aaaclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/proxyhttp"
	"nopsai/services/aaa/pkg/model"
)

const (
	defaultBaseURL       = "http://aaa:8082"
	defaultInternalToken = "dev-default-for-local-only"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return NewWithHTTPClient(baseURL, token, proxyhttp.NewInternalAwareClient(2*time.Second))
}

func NewWithHTTPClient(baseURL, token string, httpClient *http.Client) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	token = strings.TrimSpace(token)
	if token == "" {
		token = defaultInternalToken
	}
	if httpClient == nil {
		httpClient = proxyhttp.NewInternalAwareClient(2 * time.Second)
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}
}

func (c *Client) Introspect(ctx context.Context, subject model.Subject) (*model.IntrospectResponse, error) {
	var resp model.IntrospectResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/authn/introspect", model.IntrospectRequest{Subject: subject}, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Check(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef, requestContext map[string]any) (model.Decision, error) {
	var decision model.Decision
	err := c.doJSON(ctx, http.MethodPost, "/v1/authz/check", model.CheckRequest{
		Subject:  subject,
		Action:   action,
		Resource: resource,
		Context:  requestContext,
	}, requestContext, &decision)
	return decision, err
}

func (c *Client) BatchCheck(ctx context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error) {
	var resp model.BatchCheckResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/authz/batch-check", model.BatchCheckRequest{
		Subject: subject,
		Checks:  checks,
		Context: requestContext,
	}, requestContext, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Decisions, nil
}

func (c *Client) Filter(ctx context.Context, subject model.Subject, action string, resources []model.ResourceRef, requestContext map[string]any) ([]model.ResourceRef, error) {
	var resp model.FilterResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/authz/filter", model.FilterRequest{
		Subject:   subject,
		Action:    action,
		Resources: resources,
		Context:   requestContext,
	}, requestContext, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Resources, nil
}

func (c *Client) RecordAudit(ctx context.Context, req model.AuditRecordRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/audit/record", req, req.Context, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, requestContext map[string]any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal aaa request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build aaa request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	if requestID := requestIDFromContext(requestContext); requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call aaa: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read aaa response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("aaa %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode aaa response: %w", err)
	}
	return nil
}

func requestIDFromContext(values map[string]any) string {
	if values == nil {
		return ""
	}
	if requestID, ok := values["request_id"].(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
}
