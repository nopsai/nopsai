package nopsai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"nopsai/pkg/gittrigger"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/gitwebhook"
)

type gitWebhookDispatchResult struct {
	Status           string
	HTTPStatus       int
	MatchedPipelines []string
	RunIDs           []string
	Errors           []string
}

func (a *App) dispatchGitWebhookEvent(r *http.Request, source gitWebhookSourceRecord, event gitwebhook.Event) gitWebhookDispatchResult {
	manifest, _, found, err := a.loadTriggerManifestOverride(r.Context(), event.Owner, event.Repo)
	if err != nil {
		return gitWebhookDispatchResult{
			Status:     gitWebhookDeliveryFailed,
			HTTPStatus: http.StatusInternalServerError,
			Errors:     []string{"failed to load trigger manifest"},
		}
	}
	if !found {
		return gitWebhookDispatchResult{Status: gitWebhookDeliveryNoMatch}
	}
	match := gittrigger.Find(manifest, gittrigger.Event{
		Type:              event.EventType,
		Ref:               event.Ref,
		TargetRef:         event.TargetRef,
		RepositoryName:    event.Repo,
		ChangedFiles:      event.ChangedFiles,
		ChangedFilesKnown: event.ChangedFilesKnown,
	})
	if len(match.Pipelines) == 0 {
		return gitWebhookDispatchResult{Status: gitWebhookDeliveryNoMatch}
	}

	result := gitWebhookDispatchResult{
		Status:           gitWebhookDeliveryProcessed,
		HTTPStatus:       http.StatusAccepted,
		MatchedPipelines: make([]string, 0, len(match.Pipelines)),
		RunIDs:           make([]string, 0, len(match.Pipelines)),
	}
	for _, pipeline := range match.Pipelines {
		identifier := strings.TrimSpace(pipeline.Path)
		result.MatchedPipelines = append(result.MatchedPipelines, identifier)
		runID, status, err := a.startGitWebhookRun(r, source, event, identifier, match.Scope)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", identifier, err.Error()))
			if result.HTTPStatus < 400 {
				result.HTTPStatus = status
			}
			continue
		}
		result.RunIDs = append(result.RunIDs, runID)
	}
	switch {
	case len(result.RunIDs) == 0:
		result.Status = gitWebhookDeliveryFailed
	case len(result.Errors) > 0:
		result.Status = gitWebhookDeliveryPartial
	}
	return result
}

func (a *App) startGitWebhookRun(
	original *http.Request,
	source gitWebhookSourceRecord,
	event gitwebhook.Event,
	pipelineID,
	scope string,
) (string, int, error) {
	if pipelineID == "" {
		return "", http.StatusBadRequest, fmt.Errorf("pipeline entry is empty")
	}
	if strings.HasPrefix(pipelineID, "http://") || strings.HasPrefix(pipelineID, "https://") {
		return "", http.StatusBadRequest, fmt.Errorf("remote pipeline URLs are not supported")
	}
	pathPart, namePart, _, err := configsync.SplitPipelineIdentifier(pipelineID)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	normalizedID := configsync.BuildPipelineIdentifier(pathPart, namePart)
	if exists, err := a.pipelineExistsInDB(pathPart, namePart); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to validate pipeline")
	} else if !exists {
		return "", http.StatusNotFound, fmt.Errorf("pipeline is not available in the database; sync it through GitOps first")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run/"+normalizedID, bytes.NewReader(nil)).WithContext(original.Context())
	req.SetPathValue("pipelineName", normalizedID)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Git-Repo-Owner", event.Owner)
	req.Header.Set("X-Git-Repo-Name", event.Repo)
	req.Header.Set("X-Git-Commit-SHA", event.CommitSHA)
	req.Header.Set("X-Git-Ref", event.Ref)
	req.Header.Set("X-Git-Target-Ref", event.TargetRef)
	req.Header.Set("X-Git-Clone-URL", event.CloneURL)
	req.Header.Set("X-Git-SSH-URL", event.SSHURL)
	req.Header.Set("X-Git-Commit-URL", event.CommitURL)
	req.Header.Set("X-Git-Commit-Message", event.CommitMessage)
	req.Header.Set("X-Git-Commit-Author-Name", event.CommitAuthorName)
	req.Header.Set("X-Git-Commit-Author-Email", event.CommitAuthorEmail)
	req.Header.Set("X-Git-Commit-Author-Username", event.CommitAuthorUsername)
	req.Header.Set("X-Git-Pusher-Name", event.Actor)
	req.Header.Set("X-Git-Pusher-Email", event.ActorEmail)
	req.Header.Set("X-Nopsai-Scope", strings.Trim(strings.TrimSpace(scope), "/"))
	req.Header.Set("X-Nopsai-Pipeline-Path", pathPart)
	req.Header.Set("X-Nopsai-Pipeline-Source", "git_webhook")
	req.Header.Set("X-Nopsai-Trigger-Source", "git_webhook_"+source.Provider)
	req.Header.Set("X-Nopsai-Git-Event-Type", event.EventType)
	req.Header.Set("X-Nopsai-Trigger-Event-ID", event.DeliveryID)
	req.Header.Set("X-Nopsai-Caller-Type", model.SubjectTypeRepository)
	req.Header.Set("X-Nopsai-Caller-ID", event.RepositoryFull)
	req = a.withDispatcherInternalSubject(req)

	recorder := httptest.NewRecorder()
	a.handleRunPipeline(recorder, req)
	response := recorder.Result()
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = "failed to start pipeline"
		}
		return "", response.StatusCode, fmt.Errorf("%s", message)
	}
	var payload struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.RunID) == "" {
		return "", http.StatusInternalServerError, fmt.Errorf("pipeline run response was invalid")
	}
	return payload.RunID, response.StatusCode, nil
}
