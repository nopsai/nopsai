package perf

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Suite names selectable through --suites.
const (
	// SuiteAPIRead exercises the read/query surface: run listings, monitoring
	// aggregates and health endpoints. It isolates API serialization and
	// Postgres query cost.
	SuiteAPIRead = "api-read"
	// SuiteAuth exercises the authentication and authorization path that sits
	// in front of every other request, including the aaa service.
	SuiteAuth = "auth"
	// SuiteWebhook exercises trigger ingestion: HMAC verification and dispatch
	// enqueue, without waiting for pipelines to finish.
	SuiteWebhook = "webhook"
	// SuiteRuntime reproduces the traffic a pipeline generates against the
	// platform while it runs: log batches, status polling and log reads. This is
	// the load the services actually absorb during execution, measured without
	// involving the runner or the agent.
	SuiteRuntime = "runtime"
	// SuiteUI loads the UI container, which serves static assets.
	SuiteUI = "ui"
	// SuitePipeline runs whole pipelines end to end. It has its own stage shape
	// because a run takes minutes rather than milliseconds.
	SuitePipeline = "pipeline"
)

// SuiteNames returns every selectable suite in a stable order.
func SuiteNames() []string {
	return []string{SuiteAPIRead, SuiteAuth, SuiteRuntime, SuiteUI, SuiteWebhook, SuitePipeline}
}

// KnownSuite reports whether the name matches a selectable suite.
func KnownSuite(name string) bool {
	for _, suite := range SuiteNames() {
		if suite == name {
			return true
		}
	}
	return false
}

// RequestContext carries everything a scenario needs to build a request. It is
// read concurrently by every worker in a stage, so the token accessor is
// indirected through TokenSource rather than being a mutable field.
type RequestContext struct {
	APIURL     string
	WebhookURL string
	UIURL      string
	// Runtime holds the runs the runtime suite writes against.
	Runtime     *RuntimeTargets
	Payload     []byte
	Signature   string
	Identifier  string
	Password    string
	TokenSource func() (string, error)
}

// Service names the component a scenario primarily loads. Reporting is grouped
// by this so a run answers "which service carries load best" rather than only
// "how fast was the platform overall".
const (
	ServiceAPI        = "nopsai"
	ServiceAuth       = "aaa"
	ServiceDispatcher = "dispatcher"
	ServiceUI         = "ui"
)

// Services returns the measurable services in report order.
func Services() []string {
	return []string{ServiceAPI, ServiceAuth, ServiceDispatcher, ServiceUI}
}

// Scenario is one request shape in a suite's mix.
type Scenario struct {
	Name  string
	Suite string
	// Service is the component this scenario puts under load. Every scenario
	// also touches Postgres, which is why the database is reported through
	// container resource usage rather than as a request target of its own.
	Service string
	Weight  int
	Build   func(ctx context.Context, rc *RequestContext) (*http.Request, error)
}

// Mix is a weighted set of scenarios drawn from during a stage.
type Mix struct {
	scenarios []Scenario
	// expanded indexes into scenarios once per weight unit, turning weighted
	// selection into a single modulo lookup on the hot path.
	expanded []int
}

// NewMix builds a Mix from scenarios, dropping any with a non-positive weight.
func NewMix(scenarios []Scenario) *Mix {
	mix := &Mix{}
	for _, scenario := range scenarios {
		if scenario.Weight <= 0 {
			continue
		}
		index := len(mix.scenarios)
		mix.scenarios = append(mix.scenarios, scenario)
		for i := 0; i < scenario.Weight; i++ {
			mix.expanded = append(mix.expanded, index)
		}
	}
	return mix
}

// Len returns the number of distinct scenarios in the mix.
func (m *Mix) Len() int { return len(m.scenarios) }

// Empty reports whether the mix would issue no requests.
func (m *Mix) Empty() bool { return len(m.expanded) == 0 }

// Pick returns the scenario for the given sequence number. Selection is
// deterministic round-robin over the weighted expansion so that every stage
// issues the same request proportions and stages stay comparable.
func (m *Mix) Pick(sequence uint64) Scenario {
	index := m.expanded[sequence%uint64(len(m.expanded))]
	return m.scenarios[index]
}

// Names returns the distinct scenario names in the mix, sorted.
func (m *Mix) Names() []string {
	out := make([]string, 0, len(m.scenarios))
	for _, scenario := range m.scenarios {
		out = append(out, scenario.Name)
	}
	sort.Strings(out)
	return out
}

// BuildMix assembles the request mix for the selected suites. The pipeline
// suite contributes nothing here because it is driven by the end-to-end runner.
func BuildMix(suites []string) *Mix {
	var scenarios []Scenario
	for _, suite := range suites {
		switch suite {
		case SuiteAPIRead:
			scenarios = append(scenarios, apiReadScenarios()...)
		case SuiteAuth:
			scenarios = append(scenarios, authScenarios()...)
		case SuiteRuntime:
			scenarios = append(scenarios, runtimeScenarios()...)
		case SuiteUI:
			scenarios = append(scenarios, uiScenarios()...)
		case SuiteWebhook:
			scenarios = append(scenarios, webhookScenarios()...)
		}
	}
	return NewMix(scenarios)
}

// apiReadScenarios weights the mix toward the endpoints a UI session actually
// polls. Run listings and monitoring aggregates are the expensive queries, so
// they carry the most weight; health endpoints are included at low weight as a
// control signal that stays flat unless the process itself is starved.
func apiReadScenarios() []Scenario {
	return []Scenario{
		authenticatedGet("runs.list", SuiteAPIRead, ServiceAPI, 5, "/v1/runs"),
		authenticatedGet("monitoring.summary", SuiteAPIRead, ServiceAPI, 3, "/v1/monitoring/summary"),
		authenticatedGet("monitoring.run_analytics", SuiteAPIRead, ServiceAPI, 3, "/v1/monitoring/runs/analytics"),
		authenticatedGet("monitoring.pipeline_performance", SuiteAPIRead, ServiceAPI, 2, "/v1/monitoring/pipelines/performance"),
		// The dispatcher exposes no HTTP surface of its own, so this endpoint,
		// which proxies to it over gRPC, is the only way to put it under load.
		authenticatedGet("dispatcher.status", SuiteAPIRead, ServiceDispatcher, 2, "/v1/monitoring/dispatcher"),
		authenticatedGet("dispatcher.runner_history", SuiteAPIRead, ServiceDispatcher, 1, "/v1/monitoring/runners/history"),
		authenticatedGet("pipelines.list", SuiteAPIRead, ServiceAPI, 3, "/v1/pipelines"),
		authenticatedGet("teams.list", SuiteAPIRead, ServiceAPI, 2, "/v1/teams"),
		authenticatedGet("metrics.scrape", SuiteAPIRead, ServiceAPI, 1, "/metrics"),
		anonymousGet("health.livez", SuiteAPIRead, ServiceAPI, 1, "/healthz"),
	}
}

// authScenarios covers the credential path. Login carries a deliberately small
// weight relative to token-bearing calls because password hashing is expensive
// by design, and a realistic mix has far more authenticated requests than
// logins.
func authScenarios() []Scenario {
	return []Scenario{
		{
			Name:    "auth.login",
			Suite:   SuiteAuth,
			Service: ServiceAuth,
			Weight:  1,
			Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
				body := fmt.Sprintf(`{"identifier":%q,"password":%q}`, rc.Identifier, rc.Password)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(rc.APIURL, "/v1/auth/login"), strings.NewReader(body))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		authenticatedGet("auth.me", SuiteAuth, ServiceAuth, 4, "/v1/auth/me"),
		// /v1/access/grants is used rather than /v1/access/effective-permissions
		// because the latter requires action, resource_type and resource_id
		// query parameters that must resolve to a resource that actually exists
		// in the target environment. A load scenario cannot assume any specific
		// resource, so it would fail with HTTP 400 everywhere. Listing grants
		// exercises the same AAA decision path without that coupling.
		authenticatedGet("access.grants_list", SuiteAuth, ServiceAuth, 3, "/v1/access/grants"),
		{
			Name:    "authz.resource_use_check",
			Suite:   SuiteAuth,
			Service: ServiceAuth,
			Weight:  3,
			Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
				// A denied decision is still a successful request: the harness
				// measures how fast the authorization path answers, not what it
				// answers.
				const body = `{"action":"use","resource_type":"pipeline","resource_id":"perf/probe"}`
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(rc.APIURL, "/v1/authz/resource-use/check"), strings.NewReader(body))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, authorize(req, rc)
			},
		},
	}
}

// webhookScenarios drives trigger ingestion. Every delivery carries a fresh
// delivery ID so that upstream deduplication does not silently discard load.
func webhookScenarios() []Scenario {
	return []Scenario{
		{
			Name:    "webhook.git_bot_push",
			Suite:   SuiteWebhook,
			Service: ServiceAPI,
			Weight:  1,
			Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, rc.WebhookURL, bytes.NewReader(rc.Payload))
				if err != nil {
					return nil, err
				}
				deliveryID, err := newDeliveryID()
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-GitHub-Event", "push")
				req.Header.Set("X-GitHub-Delivery", deliveryID)
				req.Header.Set("X-Hub-Signature-256", "sha256="+rc.Signature)
				return req, nil
			},
		},
	}
}

// authenticatedGet builds a GET scenario that carries the bearer token.
func authenticatedGet(name, suite, service string, weight int, path string) Scenario {
	return Scenario{
		Name:    name,
		Suite:   suite,
		Service: service,
		Weight:  weight,
		Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(rc.APIURL, path), nil)
			if err != nil {
				return nil, err
			}
			return req, authorize(req, rc)
		},
	}
}

// anonymousGet builds a GET scenario that deliberately carries no credentials.
func anonymousGet(name, suite, service string, weight int, path string) Scenario {
	return Scenario{
		Name:    name,
		Suite:   suite,
		Service: service,
		Weight:  weight,
		Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodGet, joinURL(rc.APIURL, path), nil)
		},
	}
}

func authorize(req *http.Request, rc *RequestContext) error {
	if rc.TokenSource == nil {
		return nil
	}
	token, err := rc.TokenSource()
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return nil
}

// joinURL concatenates a base URL and an absolute path without producing a
// double slash.
func joinURL(base, path string) string {
	return strings.TrimSuffix(base, "/") + path
}

// ApplyInstallationID rewrites installation.id in a git event payload. git-bot
// resolves the installation before forwarding a delivery, so a payload carrying
// an unregistered installation is rejected regardless of its signature.
func ApplyInstallationID(payload []byte, installationID string) ([]byte, error) {
	if strings.TrimSpace(installationID) == "" {
		return payload, nil
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("parse webhook payload: %w", err)
	}
	numeric, err := strconv.ParseInt(strings.TrimSpace(installationID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("webhook installation id must be numeric: %w", err)
	}
	installation, _ := document["installation"].(map[string]any)
	if installation == nil {
		installation = map[string]any{}
	}
	installation["id"] = numeric
	document["installation"] = installation

	// The signature must cover the exact bytes that are sent, so the rewritten
	// document is what gets both marshalled and signed.
	updated, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("re-encode webhook payload: %w", err)
	}
	return updated, nil
}

// SignPayload returns the hex-encoded HMAC-SHA256 of the body, matching the
// X-Hub-Signature-256 scheme verified by services/git-bot.
func SignPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// newDeliveryID returns a random RFC 4122 version 4 UUID for use as a webhook
// delivery identifier.
func newDeliveryID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate delivery id: %w", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
