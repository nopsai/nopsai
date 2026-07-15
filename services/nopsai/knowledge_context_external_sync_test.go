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
