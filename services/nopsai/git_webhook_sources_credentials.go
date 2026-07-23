package nopsai

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/gitwebhook"
)

var (
	errGitWebhookCredentialForbidden           = errors.New("git webhook credential access forbidden")
	errGitWebhookCredentialAuthorizationFailed = errors.New("git webhook credential authorization unavailable")
	gitWebhookCredentialSegmentUnsafePattern   = regexp.MustCompile(`[^a-z0-9._-]+`)
)

type gitWebhookGeneratedCredential struct {
	Reference string `json:"reference"`
	Value     string `json:"value"`
	AuthMode  string `json:"auth_mode"`
}

type gitWebhookSourceCreateResponse struct {
	gitWebhookSourceRecord
	GeneratedCredential *gitWebhookGeneratedCredential `json:"generated_credential,omitempty"`
}

func (a *App) ensureGitWebhookCredentialAllowed(
	r *http.Request,
	subject aaamodel.Subject,
	source gitWebhookSourceRecord,
) error {
	if source.AuthMode == gitwebhook.AuthModeNone {
		return nil
	}
	ref, err := gitWebhookSourceCredentialReference(source)
	if err != nil {
		return err
	}
	if credentialReferenceIsTeamScoped(ref) {
		if err := validateCredentialTeamScope(ref, source.TeamPath); err != nil {
			return err
		}
	}
	admin, err := a.credentialSubjectIsNopsAIAdmin(r, subject)
	if err != nil {
		return errGitWebhookCredentialAuthorizationFailed
	}
	if admin {
		return nil
	}
	if !credentialReferenceIsTeamScoped(ref) {
		return errGitWebhookCredentialForbidden
	}
	existing, err := a.credentialStore.GetCredentialByReference(r.Context(), ref)
	if err == nil {
		allowed, err := a.credentialTeamScopeActionAllowed(r, subject, existing, "credential.use")
		if err != nil {
			return errGitWebhookCredentialAuthorizationFailed
		}
		if !allowed {
			return errGitWebhookCredentialForbidden
		}
		return nil
	}
	if !errors.Is(err, credentials.ErrNotFound) {
		return err
	}
	allowed, err := a.canCreateCredential(r, subject, ref, source.TeamPath)
	if err != nil {
		return errGitWebhookCredentialAuthorizationFailed
	}
	if !allowed {
		return errGitWebhookCredentialForbidden
	}
	return nil
}

func (a *App) prepareGitWebhookSourceCredential(
	ctx context.Context,
	source gitWebhookSourceRecord,
	actor string,
) (gitWebhookSourceRecord, *gitWebhookGeneratedCredential, *uuid.UUID, error) {
	if source.AuthMode == gitwebhook.AuthModeNone {
		source.CredentialRef = ""
		return source, nil, nil, nil
	}
	ref, err := gitWebhookSourceCredentialReference(source)
	if err != nil {
		return source, nil, nil, err
	}
	source.CredentialRef = ref.String()
	existing, err := a.credentialStore.GetCredentialByReference(ctx, ref)
	if err == nil {
		if existing.Kind != gitWebhookSecretCredentialKind {
			return source, nil, nil, fmt.Errorf(
				"credential %s already exists with kind %q; expected kind %q",
				ref.String(),
				existing.Kind,
				gitWebhookSecretCredentialKind,
			)
		}
		return source, nil, nil, nil
	}
	if !errors.Is(err, credentials.ErrNotFound) {
		return source, nil, nil, err
	}
	value, err := generatedGitWebhookSecretValue(source)
	if err != nil {
		return source, nil, nil, err
	}
	created, err := a.credentials.Create(ctx, createCredentialInput{
		Reference:   ref,
		Kind:        gitWebhookSecretCredentialKind,
		Description: "Webhook secret for Git source " + source.ID,
		Value:       []byte(value),
		Actor:       actor,
	})
	if err != nil {
		return source, nil, nil, err
	}
	return source, &gitWebhookGeneratedCredential{
		Reference: ref.String(),
		Value:     value,
		AuthMode:  source.AuthMode,
	}, &created.ID, nil
}

func gitWebhookSourceCredentialReference(source gitWebhookSourceRecord) (credentials.Reference, error) {
	if raw := strings.TrimSpace(source.CredentialRef); raw != "" {
		return credentials.ParseReference(raw)
	}
	segment := gitWebhookCredentialReferenceSegment(source.ID)
	if source.TeamPath != "" {
		return credentials.NewReference("team", strings.Trim(source.TeamPath, "/")+"/webhooks/"+segment)
	}
	return credentials.NewReference("system", "webhooks/"+segment)
}

func gitWebhookCredentialReferenceSegment(raw string) string {
	segment := strings.ToLower(strings.TrimSpace(raw))
	segment = gitWebhookCredentialSegmentUnsafePattern.ReplaceAllString(segment, "-")
	segment = strings.Trim(segment, "._-")
	if segment == "" {
		segment = "webhook-source"
	}
	if segment[0] < 'a' || segment[0] > 'z' {
		if segment[0] < '0' || segment[0] > '9' {
			segment = "webhook-" + segment
		}
	}
	return segment
}

func generatedGitWebhookSecretValue(source gitWebhookSourceRecord) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	if source.AuthMode == gitwebhook.AuthModeHMAC && source.Provider == gitwebhook.ProviderGitLab {
		return "whsec_" + base64.StdEncoding.EncodeToString(raw), nil
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func writeGitWebhookCredentialPreparationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errGitWebhookCredentialForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, errGitWebhookCredentialAuthorizationFailed):
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
