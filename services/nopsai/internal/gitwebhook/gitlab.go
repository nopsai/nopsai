package gitwebhook

import (
	"fmt"
	"net/http"
	"strings"
)

func normalizeGitLab(headers http.Header, body []byte) (Event, error) {
	payload, err := decodeObject(body)
	if err != nil {
		return Event{}, err
	}
	kind := strings.ToLower(firstNonEmpty(stringAt(payload, "object_kind"), headers.Get("X-Gitlab-Event")))
	switch {
	case kind == "push", kind == "push hook", kind == "tag_push", kind == "tag push hook":
		return normalizeGitLabPush(headers, payload), nil
	case kind == "merge_request", kind == "merge request hook":
		return normalizeGitLabMergeRequest(headers, payload)
	default:
		return Event{}, fmt.Errorf("unsupported GitLab webhook event %q", kind)
	}
}

func normalizeGitLabPush(headers http.Header, payload map[string]any) Event {
	commits := arrayAt(payload, "commits")
	files := make([]string, 0)
	var headCommit map[string]any
	for _, item := range commits {
		commit, _ := item.(map[string]any)
		headCommit = commit
		files = append(files, stringArray(commit["added"])...)
		files = append(files, stringArray(commit["modified"])...)
		files = append(files, stringArray(commit["removed"])...)
	}
	ref := stringAt(payload, "ref")
	return Event{
		EventType:            "push",
		RepositoryFull:       stringAt(payload, "project", "path_with_namespace"),
		Ref:                  ref,
		CommitSHA:            firstNonEmpty(stringAt(payload, "checkout_sha"), stringAt(payload, "after")),
		BeforeSHA:            stringAt(payload, "before"),
		ChangedFiles:         files,
		ChangedFilesKnown:    len(commits) > 0,
		CloneURL:             firstNonEmpty(stringAt(payload, "project", "git_http_url"), stringAt(payload, "repository", "homepage")),
		SSHURL:               stringAt(payload, "project", "git_ssh_url"),
		CommitURL:            firstNonEmpty(stringAt(headCommit, "url"), stringAt(payload, "project", "web_url")),
		CommitMessage:        firstLine(stringAt(headCommit, "message")),
		CommitAuthorName:     stringAt(headCommit, "author", "name"),
		CommitAuthorEmail:    stringAt(headCommit, "author", "email"),
		CommitAuthorUsername: stringAt(payload, "user_username"),
		Actor:                firstNonEmpty(stringAt(payload, "user_name"), stringAt(payload, "user_username")),
		ActorEmail:           stringAt(payload, "user_email"),
		DeliveryID: firstNonEmpty(
			headers.Get("webhook-id"),
			headers.Get("Idempotency-Key"),
			headers.Get("X-Gitlab-Event-UUID"),
			headers.Get("X-Gitlab-Webhook-UUID"),
		),
	}
}

func normalizeGitLabMergeRequest(headers http.Header, payload map[string]any) (Event, error) {
	action := strings.ToLower(stringAt(payload, "object_attributes", "action"))
	switch action {
	case "close", "closed", "merge", "merged":
		return Event{}, fmt.Errorf("ignored GitLab merge request action %q", action)
	}
	return Event{
		EventType:         "pull_request",
		Action:            action,
		RepositoryFull:    stringAt(payload, "project", "path_with_namespace"),
		Ref:               fullRef("branch", stringAt(payload, "object_attributes", "source_branch")),
		TargetRef:         fullRef("branch", stringAt(payload, "object_attributes", "target_branch")),
		CommitSHA:         firstNonEmpty(stringAt(payload, "object_attributes", "last_commit", "id"), stringAt(payload, "object_attributes", "sha")),
		ChangedFilesKnown: false,
		CloneURL:          stringAt(payload, "project", "git_http_url"),
		SSHURL:            stringAt(payload, "project", "git_ssh_url"),
		CommitURL:         stringAt(payload, "object_attributes", "url"),
		Actor:             firstNonEmpty(stringAt(payload, "user", "name"), stringAt(payload, "user", "username")),
		ActorEmail:        stringAt(payload, "user", "email"),
		DeliveryID: firstNonEmpty(
			headers.Get("webhook-id"),
			headers.Get("Idempotency-Key"),
			headers.Get("X-Gitlab-Event-UUID"),
			headers.Get("X-Gitlab-Webhook-UUID"),
		),
	}, nil
}
