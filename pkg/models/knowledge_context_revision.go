package models

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func KnowledgeContextRevision(snapshots []KnowledgeContextSnapshot, blockingOnly bool) string {
	if len(snapshots) == 0 {
		return ""
	}
	parts := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		kind := NormalizeKnowledgeContextRevisionValue(snapshot.Kind)
		if blockingOnly && !KnowledgeContextKindIsBlocking(kind) {
			continue
		}
		contentHash := sha256.Sum256([]byte(strings.TrimSpace(snapshot.Content)))
		parts = append(parts, strings.Join([]string{
			kind,
			NormalizeKnowledgeContextRevisionValue(snapshot.Ref),
			NormalizeKnowledgeContextRevisionPath(snapshot.Path),
			strings.TrimSpace(snapshot.ConfigSourceCommitSHA),
			fmt.Sprintf("%x", contentHash),
		}, "|"))
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("%x", sum)
}

func KnowledgeContextKindIsBlocking(kind string) bool {
	switch NormalizeKnowledgeContextRevisionValue(kind) {
	case "guardrail", "policy":
		return true
	default:
		return false
	}
}

func NormalizeKnowledgeContextRevisionValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeKnowledgeContextRevisionPath(value string) string {
	value = NormalizeKnowledgeContextRevisionValue(value)
	if value == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(value))
}
