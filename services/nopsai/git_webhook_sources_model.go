package nopsai

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/gittrigger"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/gitwebhook"
)

const (
	gitWebhookDeliveryPending   = "pending"
	gitWebhookDeliveryProcessed = "processed"
	gitWebhookDeliveryNoMatch   = "no_match"
	gitWebhookDeliveryPartial   = "partial"
	gitWebhookDeliveryFailed    = "failed"
)

var gitWebhookSourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)

type gitWebhookSourceRecord struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Description           string         `json:"description"`
	Provider              string         `json:"provider"`
	Enabled               bool           `json:"enabled"`
	AuthMode              string         `json:"auth_mode"`
	CredentialRef         string         `json:"credential_ref,omitempty"`
	RepositoryAllowlist   []string       `json:"repository_allowlist"`
	RateLimit             map[string]any `json:"rate_limit"`
	CreatedBy             string         `json:"created_by"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	LastUsedAt            *time.Time     `json:"last_used_at,omitempty"`
	Source                string         `json:"source"`
	ConfigRepoID          *int64         `json:"config_repo_id,omitempty"`
	ConfigSourcePath      string         `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string         `json:"config_source_commit_sha,omitempty"`
	ManagedByGitOps       bool           `json:"managed_by_config_repo"`
}

type gitWebhookSourceInput struct {
	ID                  string         `json:"id" yaml:"id,omitempty"`
	Name                string         `json:"name" yaml:"name,omitempty"`
	Description         string         `json:"description" yaml:"description,omitempty"`
	Provider            string         `json:"provider" yaml:"provider"`
	Enabled             *bool          `json:"enabled" yaml:"enabled,omitempty"`
	AuthMode            string         `json:"auth_mode" yaml:"auth_mode"`
	CredentialRef       string         `json:"credential_ref" yaml:"credential_ref,omitempty"`
	RepositoryAllowlist []string       `json:"repository_allowlist" yaml:"repository_allowlist"`
	RateLimit           map[string]any `json:"rate_limit" yaml:"rate_limit,omitempty"`
}

type gitWebhookDeliveryRecord struct {
	ID                 string     `json:"id"`
	SourceID           string     `json:"source_id"`
	DeliveryID         string     `json:"delivery_id"`
	Provider           string     `json:"provider"`
	EventType          string     `json:"event_type"`
	RepositoryFullName string     `json:"repository_full_name"`
	Status             string     `json:"status"`
	RunIDs             []string   `json:"run_ids"`
	Error              string     `json:"error,omitempty"`
	SourceIP           string     `json:"source_ip,omitempty"`
	ReceivedAt         time.Time  `json:"received_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type gitWebhookDeliveryResponse struct {
	DeliveryID         string   `json:"delivery_id"`
	Status             string   `json:"status"`
	Provider           string   `json:"provider"`
	EventType          string   `json:"event_type"`
	RepositoryFullName string   `json:"repository_full_name"`
	MatchedPipelines   []string `json:"matched_pipelines"`
	RunIDs             []string `json:"run_ids"`
	Errors             []string `json:"errors,omitempty"`
}

func normalizeGitWebhookSourceInput(input gitWebhookSourceInput, pathID string) (gitWebhookSourceRecord, error) {
	id := strings.TrimSpace(firstNonEmptyString(pathID, input.ID))
	name := strings.TrimSpace(input.Name)
	if id == "" {
		id = slugifyExternalTriggerID(name)
	}
	if !gitWebhookSourceIDPattern.MatchString(id) {
		return gitWebhookSourceRecord{}, fmt.Errorf("id must be 1-160 characters using letters, numbers, dots, underscores, or hyphens")
	}
	if name == "" {
		name = id
	}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	switch provider {
	case gitwebhook.ProviderGeneric, gitwebhook.ProviderGitLab, gitwebhook.ProviderBitbucket, gitwebhook.ProviderGitea:
	default:
		return gitWebhookSourceRecord{}, fmt.Errorf("provider must be generic, gitlab, bitbucket, or gitea")
	}
	authMode := strings.ToLower(strings.TrimSpace(input.AuthMode))
	if authMode == "" {
		authMode = gitwebhook.AuthModeHMAC
	}
	switch authMode {
	case gitwebhook.AuthModeHMAC, gitwebhook.AuthModeStaticToken, gitwebhook.AuthModeNone:
	default:
		return gitWebhookSourceRecord{}, fmt.Errorf("auth_mode must be hmac, static_token, or none")
	}
	credentialRef := strings.TrimSpace(input.CredentialRef)
	if authMode != gitwebhook.AuthModeNone {
		if credentialRef == "" {
			return gitWebhookSourceRecord{}, fmt.Errorf("credential_ref is required for %s authentication", authMode)
		}
		if _, err := credentials.ParseReference(credentialRef); err != nil {
			return gitWebhookSourceRecord{}, fmt.Errorf("invalid credential_ref: %w", err)
		}
	} else {
		credentialRef = ""
	}
	allowlist, err := normalizeGitWebhookRepositoryAllowlist(input.RepositoryAllowlist)
	if err != nil {
		return gitWebhookSourceRecord{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	return gitWebhookSourceRecord{
		ID:                  id,
		Name:                name,
		Description:         strings.TrimSpace(input.Description),
		Provider:            provider,
		Enabled:             enabled,
		AuthMode:            authMode,
		CredentialRef:       credentialRef,
		RepositoryAllowlist: allowlist,
		RateLimit:           normalizeObjectMap(input.RateLimit),
	}, nil
}

func normalizeGitWebhookRepositoryAllowlist(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("repository_allowlist must contain at least one repository pattern")
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "/"))
		if value == "" || !strings.Contains(value, "/") {
			return nil, fmt.Errorf("repository_allowlist entries must use owner/repository patterns")
		}
		if strings.Contains(value, "..") {
			return nil, fmt.Errorf("repository_allowlist entry %q contains an invalid path segment", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func gitWebhookRepositoryAllowed(repository string, allowlist []string) bool {
	repository = strings.ToLower(strings.Trim(strings.TrimSpace(repository), "/"))
	for _, pattern := range allowlist {
		if gittrigger.MatchPattern(pattern, repository) {
			return true
		}
	}
	return false
}

const gitWebhookSecretCredentialKind = "webhook_secret"
