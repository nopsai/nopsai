package nopsai

import (
	"errors"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestApplyExternalKnowledgeFailureModeUsesCachedSnapshot(t *testing.T) {
	snapshot := models.KnowledgeContextSnapshot{
		Content: "cached provider content",
	}

	resolved, ok := applyExternalKnowledgeFailureMode(snapshot, models.KnowledgeContextRef{Required: true}, knowledgeFailureModeUseCached, errors.New("provider unavailable"))
	if !ok {
		t.Fatal("applyExternalKnowledgeFailureMode() ok = false, want true")
	}
	if resolved.Content != snapshot.Content {
		t.Fatalf("resolved content = %q, want cached snapshot", resolved.Content)
	}
}

func TestApplyExternalKnowledgeFailureModeRequiresCache(t *testing.T) {
	_, ok := applyExternalKnowledgeFailureMode(models.KnowledgeContextSnapshot{}, models.KnowledgeContextRef{Required: false}, knowledgeFailureModeUseCached, errors.New("provider unavailable"))
	if ok {
		t.Fatal("applyExternalKnowledgeFailureMode() ok = true, want false without cached content")
	}
}

func TestProviderHTTPErrorClassifiesStatusWithoutSecretLeak(t *testing.T) {
	err := newKnowledgeProviderError(knowledgeProviderErrorAuthentication, 401, "Knowledge provider credential could not be resolved.")
	if got := knowledgeProviderErrorStatus(err); got != knowledgeConnectionStatusAuthenticationRequired {
		t.Fatalf("knowledgeProviderErrorStatus() = %q", got)
	}
	if strings.Contains(err.Error(), "token") {
		t.Fatalf("provider error leaked credential-like value: %q", err.Error())
	}
}

func TestExternalKnowledgeSyncStatusClassifiesOperationalFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "disabled connection",
			err:  newKnowledgeProviderError(knowledgeProviderErrorDisabled, 0, "Knowledge connection is disabled."),
			want: "connection_disabled",
		},
		{
			name: "page too large",
			err:  newKnowledgeProviderError(knowledgeProviderErrorPageTooLarge, 413, "Provider page is too large to use as Knowledge Context."),
			want: "page_too_large",
		},
		{
			name: "invalid request",
			err:  newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, 400, "external_page_id or external_page_url is required"),
			want: "invalid_request",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := externalKnowledgeSyncStatus(tt.err); got != tt.want {
				t.Fatalf("externalKnowledgeSyncStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
