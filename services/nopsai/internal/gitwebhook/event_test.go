package gitwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestNormalizeGeneric(t *testing.T) {
	event, err := Normalize(ProviderGeneric, http.Header{"X-Nopsai-Delivery": []string{"delivery-1"}}, []byte(`{
		"event_type":"push",
		"repository":{"full_name":"acme/api","clone_url":"https://git.example/acme/api.git"},
		"ref":"refs/heads/main",
		"commit_sha":"abc123",
		"changed_files":["services/api/main.go","services/api/main.go"],
		"actor":{"name":"Alice"}
	}`))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if event.Owner != "acme" || event.Repo != "api" || event.DeliveryID != "delivery-1" {
		t.Fatalf("Normalize() identity = %#v", event)
	}
	if !event.ChangedFilesKnown || len(event.ChangedFiles) != 1 {
		t.Fatalf("Normalize() changed files = %#v, known=%v", event.ChangedFiles, event.ChangedFilesKnown)
	}
}

func TestNormalizeGitLabPush(t *testing.T) {
	headers := http.Header{
		"X-Gitlab-Event":      []string{"Push Hook"},
		"X-Gitlab-Event-Uuid": []string{"delivery-2"},
	}
	event, err := Normalize(ProviderGitLab, headers, []byte(`{
		"object_kind":"push",
		"ref":"refs/heads/main",
		"before":"before",
		"after":"after",
		"checkout_sha":"after",
		"user_name":"Alice",
		"project":{"path_with_namespace":"acme/api","git_http_url":"https://gitlab/acme/api.git","git_ssh_url":"git@gitlab:acme/api.git"},
		"commits":[{"id":"after","message":"change\nbody","url":"https://gitlab/acme/api/-/commit/after","author":{"name":"Alice","email":"a@example.com"},"added":["a.go"],"modified":["b.go"],"removed":["old.go"]}]
	}`))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if event.EventType != "push" || event.CommitSHA != "after" || event.CommitMessage != "change" {
		t.Fatalf("Normalize() = %#v", event)
	}
	if len(event.ChangedFiles) != 3 || !event.ChangedFilesKnown {
		t.Fatalf("Normalize() changed files = %#v", event.ChangedFiles)
	}
}

func TestNormalizeBitbucketPullRequest(t *testing.T) {
	headers := http.Header{
		"X-Event-Key":    []string{"pullrequest:updated"},
		"X-Request-Uuid": []string{"delivery-3"},
	}
	event, err := Normalize(ProviderBitbucket, headers, []byte(`{
		"actor":{"display_name":"Alice"},
		"repository":{"full_name":"acme/api","links":{"clone":[{"name":"https","href":"https://bitbucket/acme/api.git"}]}},
		"pullrequest":{
			"source":{"branch":{"name":"feature/api"},"commit":{"hash":"abc123"}},
			"destination":{"branch":{"name":"main"}},
			"links":{"html":{"href":"https://bitbucket/acme/api/pull-requests/1"}}
		}
	}`))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if event.EventType != "pull_request" || event.Ref != "refs/heads/feature/api" || event.TargetRef != "refs/heads/main" {
		t.Fatalf("Normalize() = %#v", event)
	}
	if event.ChangedFilesKnown {
		t.Fatal("Bitbucket PR changed files should be unavailable")
	}
}

func TestNormalizeBitbucketDoesNotUseAttemptNumberAsDeliveryID(t *testing.T) {
	headers := http.Header{
		"X-Event-Key":      []string{"pullrequest:updated"},
		"X-Attempt-Number": []string{"1"},
	}
	event, err := Normalize(ProviderBitbucket, headers, []byte(`{
		"repository":{"full_name":"acme/api"},
		"pullrequest":{
			"source":{"branch":{"name":"feature/api"},"commit":{"hash":"abc123"}},
			"destination":{"branch":{"name":"main"}}
		}
	}`))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if event.DeliveryID == "1" || len(event.DeliveryID) != sha256.Size*2 {
		t.Fatalf("DeliveryID = %q, want payload hash", event.DeliveryID)
	}
}

func TestNormalizeGiteaPush(t *testing.T) {
	headers := http.Header{
		"X-Gitea-Event":    []string{"push"},
		"X-Gitea-Delivery": []string{"delivery-4"},
	}
	event, err := Normalize(ProviderGitea, headers, []byte(`{
		"ref":"refs/heads/main",
		"before":"before",
		"after":"after",
		"repository":{"full_name":"acme/api","clone_url":"https://gitea/acme/api.git","ssh_url":"git@gitea:acme/api.git"},
		"pusher":{"login":"alice"},
		"head_commit":{"url":"https://gitea/acme/api/commit/after","message":"change","author":{"name":"Alice"}},
		"commits":[{"added":["main.go"],"modified":[],"removed":[]}]
	}`))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if event.RepositoryFull != "acme/api" || event.Actor != "alice" || len(event.ChangedFiles) != 1 {
		t.Fatalf("Normalize() = %#v", event)
	}
}

func TestVerifyHMACAndStaticToken(t *testing.T) {
	body := []byte(`{"ok":true}`)
	secret := "secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := Verify(ProviderGeneric, AuthModeHMAC, secret, http.Header{
		"X-Nopsai-Signature-256": []string{signature},
	}, body, time.Now()); err != nil {
		t.Fatalf("Verify(HMAC) error = %v", err)
	}
	if err := Verify(ProviderGitLab, AuthModeStaticToken, secret, http.Header{
		"X-Gitlab-Token": []string{secret},
	}, body, time.Now()); err != nil {
		t.Fatalf("Verify(static token) error = %v", err)
	}
	if err := Verify(ProviderGeneric, AuthModeHMAC, secret, http.Header{
		"X-Nopsai-Signature-256": []string{"sha256=bad"},
	}, body, time.Now()); err == nil {
		t.Fatal("Verify() accepted invalid signature")
	}
}

func TestVerifyGitLabStandardWebhook(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"ok":true}`)
	rawKey := []byte("standard-signing-key")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(rawKey)
	id := "delivery-5"
	timestamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, rawKey)
	_, _ = mac.Write([]byte(id + "." + timestamp + "." + string(body)))
	signature := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	err := Verify(ProviderGitLab, AuthModeHMAC, secret, http.Header{
		"Webhook-Id":        []string{id},
		"Webhook-Timestamp": []string{timestamp},
		"Webhook-Signature": []string{signature},
	}, body, now)
	if err != nil {
		t.Fatalf("Verify(Standard Webhook) error = %v", err)
	}
}
