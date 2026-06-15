package gitwebhook

import (
	"fmt"
	"net/http"
	"strings"
)

func normalizeGitea(headers http.Header, body []byte) (Event, error) {
	payload, err := decodeObject(body)
	if err != nil {
		return Event{}, err
	}
	eventName := strings.ToLower(firstNonEmpty(headers.Get("X-Gitea-Event"), headers.Get("X-Gogs-Event"), headers.Get("X-GitHub-Event")))
	switch eventName {
	case "push":
		return normalizeGiteaPush(headers, payload), nil
	case "pull_request":
		action := strings.ToLower(stringAt(payload, "action"))
		if action == "closed" {
			return Event{}, fmt.Errorf("ignored Gitea pull request action %q", action)
		}
		return normalizeGiteaPullRequest(headers, payload, action), nil
	default:
		return Event{}, fmt.Errorf("unsupported Gitea webhook event %q", eventName)
	}
}

func normalizeGiteaPush(headers http.Header, payload map[string]any) Event {
	commits := arrayAt(payload, "commits")
	files := make([]string, 0)
	for _, item := range commits {
		commit, _ := item.(map[string]any)
		files = append(files, stringArray(commit["added"])...)
		files = append(files, stringArray(commit["modified"])...)
		files = append(files, stringArray(commit["removed"])...)
	}
	return Event{
		EventType:            "push",
		RepositoryFull:       stringAt(payload, "repository", "full_name"),
		Ref:                  stringAt(payload, "ref"),
		CommitSHA:            stringAt(payload, "after"),
		BeforeSHA:            stringAt(payload, "before"),
		ChangedFiles:         files,
		ChangedFilesKnown:    len(commits) > 0,
		CloneURL:             stringAt(payload, "repository", "clone_url"),
		SSHURL:               stringAt(payload, "repository", "ssh_url"),
		CommitURL:            stringAt(payload, "head_commit", "url"),
		CommitMessage:        firstLine(stringAt(payload, "head_commit", "message")),
		CommitAuthorName:     stringAt(payload, "head_commit", "author", "name"),
		CommitAuthorEmail:    stringAt(payload, "head_commit", "author", "email"),
		CommitAuthorUsername: stringAt(payload, "head_commit", "author", "username"),
		Actor:                firstNonEmpty(stringAt(payload, "pusher", "full_name"), stringAt(payload, "pusher", "login")),
		ActorEmail:           stringAt(payload, "pusher", "email"),
		DeliveryID:           firstNonEmpty(headers.Get("X-Gitea-Delivery"), headers.Get("X-Gogs-Delivery"), headers.Get("X-GitHub-Delivery")),
	}
}

func normalizeGiteaPullRequest(headers http.Header, payload map[string]any, action string) Event {
	return Event{
		EventType:         "pull_request",
		Action:            action,
		RepositoryFull:    stringAt(payload, "repository", "full_name"),
		Ref:               fullRef("branch", stringAt(payload, "pull_request", "head", "ref")),
		TargetRef:         fullRef("branch", stringAt(payload, "pull_request", "base", "ref")),
		CommitSHA:         stringAt(payload, "pull_request", "head", "sha"),
		ChangedFilesKnown: false,
		CloneURL:          stringAt(payload, "repository", "clone_url"),
		SSHURL:            stringAt(payload, "repository", "ssh_url"),
		CommitURL:         stringAt(payload, "pull_request", "html_url"),
		Actor:             firstNonEmpty(stringAt(payload, "sender", "full_name"), stringAt(payload, "sender", "login")),
		ActorEmail:        stringAt(payload, "sender", "email"),
		DeliveryID:        firstNonEmpty(headers.Get("X-Gitea-Delivery"), headers.Get("X-Gogs-Delivery"), headers.Get("X-GitHub-Delivery")),
	}
}
