package gitwebhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type genericPayload struct {
	EventType  string `json:"event_type"`
	Action     string `json:"action"`
	Repository struct {
		FullName string `json:"full_name"`
		Owner    string `json:"owner"`
		Name     string `json:"name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
	Ref          string   `json:"ref"`
	TargetRef    string   `json:"target_ref"`
	CommitSHA    string   `json:"commit_sha"`
	BeforeSHA    string   `json:"before_sha"`
	ChangedFiles []string `json:"changed_files"`
	Commit       struct {
		URL      string `json:"url"`
		Message  string `json:"message"`
		Author   string `json:"author"`
		Email    string `json:"email"`
		Username string `json:"username"`
	} `json:"commit"`
	Actor struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"actor"`
	DeliveryID string `json:"delivery_id"`
}

func normalizeGeneric(headers http.Header, body []byte) (Event, error) {
	var payload genericPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, fmt.Errorf("invalid generic webhook payload: %w", err)
	}
	eventType := strings.ToLower(strings.TrimSpace(payload.EventType))
	switch eventType {
	case "push", "pull_request":
	case "merge_request":
		eventType = "pull_request"
	default:
		return Event{}, fmt.Errorf("unsupported generic webhook event %q", payload.EventType)
	}
	return Event{
		EventType:            eventType,
		Action:               strings.ToLower(strings.TrimSpace(payload.Action)),
		RepositoryFull:       payload.Repository.FullName,
		Owner:                payload.Repository.Owner,
		Repo:                 payload.Repository.Name,
		Ref:                  payload.Ref,
		TargetRef:            payload.TargetRef,
		CommitSHA:            payload.CommitSHA,
		BeforeSHA:            payload.BeforeSHA,
		ChangedFiles:         payload.ChangedFiles,
		ChangedFilesKnown:    payload.ChangedFiles != nil,
		CloneURL:             payload.Repository.CloneURL,
		SSHURL:               payload.Repository.SSHURL,
		CommitURL:            payload.Commit.URL,
		CommitMessage:        firstLine(payload.Commit.Message),
		CommitAuthorName:     payload.Commit.Author,
		CommitAuthorEmail:    payload.Commit.Email,
		CommitAuthorUsername: payload.Commit.Username,
		Actor:                payload.Actor.Name,
		ActorEmail:           payload.Actor.Email,
		DeliveryID:           firstNonEmpty(payload.DeliveryID, headers.Get("X-Nopsai-Delivery")),
	}, nil
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
