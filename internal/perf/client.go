package perf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// tokenRenewalMargin renews an access token before it actually expires so that
// load workers never observe an expiry-induced 401 mid-stage.
const tokenRenewalMargin = 30 * time.Second

// NewHTTPClient returns a client tuned for load generation. The connection pool
// is sized to the peak concurrency so that connection setup does not become the
// thing being measured, and redirects are not followed so that every observed
// latency belongs to exactly one request.
func NewHTTPClient(timeout time.Duration, peakConcurrency int) *http.Client {
	if peakConcurrency < 1 {
		peakConcurrency = 1
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          peakConcurrency * 2,
		MaxIdleConnsPerHost:   peakConcurrency * 2,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// TokenManager acquires and caches an access token for authenticated
// scenarios. A single token is shared by every worker, mirroring how a real
// client behaves and keeping login cost out of the read-path measurements.
type TokenManager struct {
	client     *http.Client
	apiURL     string
	identifier string
	password   string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewTokenManager returns a TokenManager bound to the given API.
func NewTokenManager(client *http.Client, apiURL, identifier, password string) *TokenManager {
	return &TokenManager{
		client:     client,
		apiURL:     apiURL,
		identifier: identifier,
		password:   password,
	}
}

type loginResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Login forces a fresh credential exchange and caches the resulting token.
func (t *TokenManager) Login(ctx context.Context) (string, error) {
	body := fmt.Sprintf(`{"identifier":%q,"password":%q}`, t.identifier, t.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(t.apiURL, "/v1/auth/login"), strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var decoded loginResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if strings.TrimSpace(decoded.AccessToken) == "" {
		return "", fmt.Errorf("login response contained no access token")
	}

	t.mu.Lock()
	t.token = decoded.AccessToken
	t.expiresAt = decoded.ExpiresAt
	t.mu.Unlock()
	return decoded.AccessToken, nil
}

// Token returns a cached token, refreshing it when it is missing or close to
// expiry. It is safe for concurrent use by every load worker.
func (t *TokenManager) Token() (string, error) {
	t.mu.Lock()
	token := t.token
	expiresAt := t.expiresAt
	t.mu.Unlock()

	if token != "" && (expiresAt.IsZero() || time.Until(expiresAt) > tokenRenewalMargin) {
		return token, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return t.Login(ctx)
}

// ExecuteRequest issues a request and returns the observation for it. The response
// body is always drained and closed so that the connection returns to the pool;
// a leaked body would throttle later stages and corrupt the ramp.
func ExecuteRequest(client *http.Client, scenario Scenario, req *http.Request) Result {
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Result{
			Scenario: scenario.Name,
			Service:  scenario.Service,
			Latency:  time.Since(start),
			Err:      classifyError(err),
		}
	}
	written, copyErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	latency := time.Since(start)

	result := Result{
		Scenario: scenario.Name,
		Service:  scenario.Service,
		Latency:  latency,
		Status:   resp.StatusCode,
		Bytes:    written,
	}
	if copyErr != nil {
		result.Err = classifyError(copyErr)
	} else if closeErr != nil {
		result.Err = classifyError(closeErr)
	}
	return result
}

// classifyError collapses transport failures into a small set of stable labels
// so the report groups them instead of listing thousands of unique messages
// that differ only by ephemeral port number.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "connection refused"):
		return "connection refused"
	case strings.Contains(message, "connection reset"):
		return "connection reset"
	case strings.Contains(message, "context deadline exceeded"):
		return "timeout"
	case strings.Contains(message, "context canceled"):
		return "canceled"
	case strings.Contains(message, "EOF"):
		return "unexpected EOF"
	case strings.Contains(message, "no such host"):
		return "dns failure"
	case strings.Contains(message, "cannot assign requested address"):
		return "ephemeral port exhaustion"
	case strings.Contains(message, "too many open files"):
		return "file descriptor exhaustion"
	default:
		return "transport error"
	}
}
