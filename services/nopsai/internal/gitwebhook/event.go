package gitwebhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	ProviderGeneric   = "generic"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
	ProviderGitea     = "gitea"
)

type Event struct {
	Provider             string
	EventType            string
	Action               string
	RepositoryFull       string
	Owner                string
	Repo                 string
	Ref                  string
	TargetRef            string
	CommitSHA            string
	BeforeSHA            string
	ChangedFiles         []string
	ChangedFilesKnown    bool
	CloneURL             string
	SSHURL               string
	CommitURL            string
	CommitMessage        string
	CommitAuthorName     string
	CommitAuthorEmail    string
	CommitAuthorUsername string
	Actor                string
	ActorEmail           string
	DeliveryID           string
}

func Normalize(provider string, headers http.Header, body []byte) (Event, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	var (
		event Event
		err   error
	)
	switch provider {
	case ProviderGeneric:
		event, err = normalizeGeneric(headers, body)
	case ProviderGitLab:
		event, err = normalizeGitLab(headers, body)
	case ProviderBitbucket:
		event, err = normalizeBitbucket(headers, body)
	case ProviderGitea:
		event, err = normalizeGitea(headers, body)
	default:
		return Event{}, fmt.Errorf("unsupported git webhook provider %q", provider)
	}
	if err != nil {
		return Event{}, err
	}
	event.Provider = provider
	event.RepositoryFull = strings.Trim(strings.TrimSpace(event.RepositoryFull), "/")
	if event.Owner == "" || event.Repo == "" {
		event.Owner, event.Repo = splitRepository(event.RepositoryFull)
	}
	if event.RepositoryFull == "" {
		event.RepositoryFull = joinRepository(event.Owner, event.Repo)
	}
	if event.DeliveryID == "" {
		sum := sha256.Sum256(body)
		event.DeliveryID = hex.EncodeToString(sum[:])
	}
	event.ChangedFiles = uniqueStrings(event.ChangedFiles)
	if event.EventType == "" {
		return Event{}, fmt.Errorf("webhook event type is missing or unsupported")
	}
	if event.Owner == "" || event.Repo == "" {
		return Event{}, fmt.Errorf("webhook repository identity is missing")
	}
	if event.CommitSHA == "" {
		return Event{}, fmt.Errorf("webhook commit SHA is missing")
	}
	return event, nil
}

func decodeObject(body []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON webhook payload: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("webhook payload must be a JSON object")
	}
	return payload, nil
}

func splitRepository(full string) (string, string) {
	full = strings.Trim(strings.TrimSpace(full), "/")
	owner, repo, ok := strings.Cut(full, "/")
	if !ok {
		return "", full
	}
	return strings.TrimSpace(owner), strings.Trim(strings.TrimSpace(repo), "/")
}

func joinRepository(owner, repo string) string {
	owner = strings.Trim(strings.TrimSpace(owner), "/")
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if owner == "" {
		return repo
	}
	if repo == "" {
		return owner
	}
	return owner + "/" + repo
}

func fullRef(kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "refs/") {
		return value
	}
	if kind == "tag" {
		return "refs/tags/" + value
	}
	return "refs/heads/" + value
}

func stringAt(root map[string]any, path ...string) string {
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = object[part]
		if !ok {
			return ""
		}
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}

func objectAt(root map[string]any, path ...string) map[string]any {
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	object, _ := current.(map[string]any)
	return object
}

func arrayAt(root map[string]any, path ...string) []any {
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	values, _ := current.([]any)
	return values
}

func stringArray(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
