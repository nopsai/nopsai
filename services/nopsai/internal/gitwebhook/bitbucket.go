package gitwebhook

import (
	"fmt"
	"net/http"
	"strings"
)

func normalizeBitbucket(headers http.Header, body []byte) (Event, error) {
	payload, err := decodeObject(body)
	if err != nil {
		return Event{}, err
	}
	eventKey := strings.ToLower(strings.TrimSpace(headers.Get("X-Event-Key")))
	switch {
	case eventKey == "repo:push":
		return normalizeBitbucketPush(headers, payload)
	case strings.HasPrefix(eventKey, "pullrequest:"):
		action := strings.TrimPrefix(eventKey, "pullrequest:")
		if action == "fulfilled" || action == "rejected" {
			return Event{}, fmt.Errorf("ignored Bitbucket pull request action %q", action)
		}
		return normalizeBitbucketPullRequest(headers, payload, action), nil
	default:
		return Event{}, fmt.Errorf("unsupported Bitbucket webhook event %q", eventKey)
	}
}

func normalizeBitbucketPush(headers http.Header, payload map[string]any) (Event, error) {
	changes := arrayAt(payload, "push", "changes")
	if len(changes) == 0 {
		return Event{}, fmt.Errorf("Bitbucket push payload has no changes")
	}
	change, _ := changes[0].(map[string]any)
	newRef := objectAt(change, "new")
	if newRef == nil {
		return Event{}, fmt.Errorf("ignored Bitbucket branch deletion")
	}
	oldRef := objectAt(change, "old")
	refType := stringAt(newRef, "type")
	return Event{
		EventType:         "push",
		RepositoryFull:    stringAt(payload, "repository", "full_name"),
		Ref:               fullRef(refType, stringAt(newRef, "name")),
		CommitSHA:         stringAt(newRef, "target", "hash"),
		BeforeSHA:         stringAt(oldRef, "target", "hash"),
		ChangedFilesKnown: false,
		CloneURL:          bitbucketCloneURL(payload, "https"),
		SSHURL:            bitbucketCloneURL(payload, "ssh"),
		CommitURL:         stringAt(newRef, "target", "links", "html", "href"),
		CommitMessage:     firstLine(stringAt(newRef, "target", "message")),
		CommitAuthorName:  stringAt(newRef, "target", "author", "raw"),
		Actor:             firstNonEmpty(stringAt(payload, "actor", "display_name"), stringAt(payload, "actor", "nickname")),
		DeliveryID:        headers.Get("X-Request-UUID"),
	}, nil
}

func normalizeBitbucketPullRequest(headers http.Header, payload map[string]any, action string) Event {
	return Event{
		EventType:         "pull_request",
		Action:            action,
		RepositoryFull:    stringAt(payload, "repository", "full_name"),
		Ref:               fullRef("branch", stringAt(payload, "pullrequest", "source", "branch", "name")),
		TargetRef:         fullRef("branch", stringAt(payload, "pullrequest", "destination", "branch", "name")),
		CommitSHA:         stringAt(payload, "pullrequest", "source", "commit", "hash"),
		ChangedFilesKnown: false,
		CloneURL:          bitbucketCloneURL(payload, "https"),
		SSHURL:            bitbucketCloneURL(payload, "ssh"),
		CommitURL:         stringAt(payload, "pullrequest", "links", "html", "href"),
		Actor:             firstNonEmpty(stringAt(payload, "actor", "display_name"), stringAt(payload, "actor", "nickname")),
		DeliveryID:        headers.Get("X-Request-UUID"),
	}
}

func bitbucketCloneURL(payload map[string]any, name string) string {
	for _, item := range arrayAt(payload, "repository", "links", "clone") {
		link, _ := item.(map[string]any)
		if strings.EqualFold(stringAt(link, "name"), name) {
			return stringAt(link, "href")
		}
	}
	return ""
}
