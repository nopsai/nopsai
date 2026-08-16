package perf

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestKnownSuiteAcceptsOnlyDeclaredSuites(t *testing.T) {
	for _, suite := range SuiteNames() {
		if !KnownSuite(suite) {
			t.Errorf("KnownSuite(%q) = false, want true", suite)
		}
	}
	if KnownSuite("not-a-suite") {
		t.Error("KnownSuite accepted an undeclared suite")
	}
}

func TestBuildMixSelectsRequestedSuitesOnly(t *testing.T) {
	mix := BuildMix([]string{SuiteWebhook})
	if mix.Len() != 1 {
		t.Fatalf("webhook mix has %d scenarios, want 1", mix.Len())
	}
	if got := mix.Names()[0]; got != "webhook.git_bot_push" {
		t.Fatalf("webhook mix contains %q", got)
	}

	full := BuildMix([]string{SuiteAPIRead, SuiteAuth, SuiteWebhook})
	if full.Len() <= mix.Len() {
		t.Fatalf("combined mix has %d scenarios, want more than the webhook-only mix", full.Len())
	}
}

// TestBuildMixIgnoresPipelineSuite documents that the pipeline suite is driven
// by its own runner and must contribute no request-mix scenarios.
func TestBuildMixIgnoresPipelineSuite(t *testing.T) {
	if mix := BuildMix([]string{SuitePipeline}); !mix.Empty() {
		t.Fatalf("pipeline suite contributed %d scenarios to the request mix, want 0", mix.Len())
	}
}

func TestMixPickHonoursWeights(t *testing.T) {
	mix := NewMix([]Scenario{
		{Name: "heavy", Weight: 3},
		{Name: "light", Weight: 1},
		{Name: "disabled", Weight: 0},
	})

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		counts[mix.Pick(uint64(i)).Name]++
	}
	if counts["disabled"] != 0 {
		t.Errorf("a zero-weight scenario was selected %d times", counts["disabled"])
	}
	if counts["heavy"] != 300 || counts["light"] != 100 {
		t.Fatalf("weighted selection gave heavy=%d light=%d, want 300/100", counts["heavy"], counts["light"])
	}
}

func TestMixEmptyWhenAllWeightsAreZero(t *testing.T) {
	if mix := NewMix([]Scenario{{Name: "a", Weight: 0}}); !mix.Empty() {
		t.Fatal("a mix of zero-weight scenarios should be empty")
	}
}

// TestSignPayloadMatchesGitBotVerification pins the signature scheme to the one
// services/git-bot verifies, so a change on either side breaks loudly here
// rather than silently producing 401s that look like saturation.
func TestSignPayloadMatchesGitBotVerification(t *testing.T) {
	const secret = "test-webhook-secret"
	payload := []byte(`{"ref":"refs/heads/main"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	if got := SignPayload(secret, payload); got != want {
		t.Fatalf("SignPayload = %q, want %q", got, want)
	}
}

func TestNewDeliveryIDProducesUniqueUUIDv4(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id, err := newDeliveryID()
		if err != nil {
			t.Fatalf("newDeliveryID returned %v", err)
		}
		if len(id) != 36 {
			t.Fatalf("delivery id %q is %d characters, want 36", id, len(id))
		}
		if id[14] != '4' {
			t.Fatalf("delivery id %q is not version 4", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("delivery id %q was generated twice", id)
		}
		seen[id] = struct{}{}
	}
}

func TestAuthenticatedScenariosCarryBearerToken(t *testing.T) {
	request := &RequestContext{
		APIURL:      "http://api.test",
		TokenSource: func() (string, error) { return "token-123", nil },
	}
	scenario := authenticatedGet("runs.list", SuiteAPIRead, ServiceAPI, 1, "/v1/runs")

	req, err := scenario.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization = %q, want a bearer token", got)
	}
	if req.URL.String() != "http://api.test/v1/runs" {
		t.Fatalf("URL = %q", req.URL.String())
	}
}

// TestAnonymousScenariosOmitCredentials protects the health-check control
// signal: it must stay unauthenticated so it measures the process rather than
// the auth path.
func TestAnonymousScenariosOmitCredentials(t *testing.T) {
	request := &RequestContext{
		APIURL:      "http://api.test",
		TokenSource: func() (string, error) { return "token-123", nil },
	}
	scenario := anonymousGet("health.livez", SuiteAPIRead, ServiceAPI, 1, "/healthz")

	req, err := scenario.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want no credentials", got)
	}
}

func TestWebhookScenarioSignsAndVariesDeliveryID(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main"}`)
	request := &RequestContext{
		WebhookURL: "http://git-bot.test/webhook",
		Payload:    payload,
		Signature:  SignPayload("secret", payload),
	}
	scenario := webhookScenarios()[0]

	first, err := scenario.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	second, err := scenario.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}

	if got := first.Header.Get("X-Hub-Signature-256"); !strings.HasPrefix(got, "sha256=") {
		t.Fatalf("signature header = %q, want a sha256= prefix", got)
	}
	if first.Header.Get("X-GitHub-Event") != "push" {
		t.Error("webhook scenario must declare the push event")
	}
	if first.Header.Get("X-GitHub-Delivery") == second.Header.Get("X-GitHub-Delivery") {
		t.Fatal("consecutive deliveries reused a delivery ID, which upstream deduplication would discard")
	}

	body, err := io.ReadAll(first.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != string(payload) {
		t.Fatalf("body = %q, want the configured payload", body)
	}
}

func TestAuthLoginScenarioSendsCredentialsAndNoToken(t *testing.T) {
	request := &RequestContext{
		APIURL:      "http://api.test",
		Identifier:  `admin@example.com`,
		Password:    `p"ass`,
		TokenSource: func() (string, error) { return "token-123", nil },
	}
	var login Scenario
	for _, scenario := range authScenarios() {
		if scenario.Name == "auth.login" {
			login = scenario
		}
	}
	req, err := login.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("the login scenario must not send a bearer token")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// The password contains a quote, so this also proves the body is built with
	// proper JSON escaping rather than raw concatenation.
	if !strings.Contains(string(body), `"password":"p\"ass"`) {
		t.Fatalf("login body = %s, want an escaped password", body)
	}
}

func TestJoinURLAvoidsDoubleSlash(t *testing.T) {
	if got := joinURL("http://api.test/", "/v1/runs"); got != "http://api.test/v1/runs" {
		t.Fatalf("joinURL = %q", got)
	}
	if got := joinURL("http://api.test", "/v1/runs"); got != "http://api.test/v1/runs" {
		t.Fatalf("joinURL = %q", got)
	}
}

func TestApplyInstallationIDRewritesAndPreservesPayload(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main","installation":{"id":987654},"repository":{"full_name":"nopsai/test-app"}}`)

	updated, err := ApplyInstallationID(payload, "149172622")
	if err != nil {
		t.Fatalf("ApplyInstallationID returned %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	installation := document["installation"].(map[string]any)
	if got := installation["id"].(float64); got != 149172622 {
		t.Errorf("installation.id = %v, want 149172622", got)
	}
	// The rest of the event must survive untouched, or the delivery would no
	// longer describe the same push.
	if document["ref"] != "refs/heads/main" {
		t.Errorf("ref was altered: %v", document["ref"])
	}
	repository := document["repository"].(map[string]any)
	if repository["full_name"] != "nopsai/test-app" {
		t.Errorf("repository was altered: %v", repository)
	}
}

// TestApplyInstallationIDSignsTheBytesThatAreSent guards the ordering rule: the
// signature has to cover the rewritten body, not the original file.
func TestApplyInstallationIDSignsTheBytesThatAreSent(t *testing.T) {
	original := []byte(`{"installation":{"id":1}}`)
	updated, err := ApplyInstallationID(original, "42")
	if err != nil {
		t.Fatalf("ApplyInstallationID returned %v", err)
	}
	if SignPayload("secret", updated) == SignPayload("secret", original) {
		t.Fatal("the rewritten payload signs identically to the original, so one of them is not what is sent")
	}
}

func TestApplyInstallationIDIsANoOpWhenUnset(t *testing.T) {
	payload := []byte(`{"installation":{"id":987654}}`)
	updated, err := ApplyInstallationID(payload, "   ")
	if err != nil {
		t.Fatalf("ApplyInstallationID returned %v", err)
	}
	if string(updated) != string(payload) {
		t.Fatalf("payload was rewritten despite no override: %s", updated)
	}
}

func TestApplyInstallationIDAddsInstallationWhenMissing(t *testing.T) {
	updated, err := ApplyInstallationID([]byte(`{"ref":"refs/heads/main"}`), "7")
	if err != nil {
		t.Fatalf("ApplyInstallationID returned %v", err)
	}
	if !strings.Contains(string(updated), `"installation"`) {
		t.Fatalf("installation was not added: %s", updated)
	}
}

func TestApplyInstallationIDRejectsBadInput(t *testing.T) {
	if _, err := ApplyInstallationID([]byte(`{"a":1}`), "not-a-number"); err == nil {
		t.Error("a non-numeric installation id was accepted")
	}
	if _, err := ApplyInstallationID([]byte(`{not json}`), "1"); err == nil {
		t.Error("a malformed payload was accepted")
	}
}
