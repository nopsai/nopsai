package gittrigger

import (
	"regexp"
	"strings"

	"nopsai/pkg/models"
)

type Event struct {
	Type              string
	Ref               string
	TargetRef         string
	RepositoryName    string
	ChangedFiles      []string
	ChangedFilesKnown bool
}

type Match struct {
	Pipelines []models.PipelineSource
	Scope     string
}

func Find(manifest models.Manifest, event Event) Match {
	event.Type = strings.TrimSpace(event.Type)
	event.Ref = strings.TrimSpace(event.Ref)
	event.TargetRef = strings.TrimSpace(event.TargetRef)
	event.RepositoryName = strings.TrimSpace(event.RepositoryName)

	for _, trigger := range manifest.Triggers {
		if !eventMatches(trigger.On, event.Type) {
			continue
		}
		if matchesAny(event.RepositoryName, trigger.SkipRepos) {
			continue
		}
		if !refMatches(trigger, event.Type, event.Ref, event.TargetRef) {
			continue
		}
		if !pathsMatch(trigger, event.ChangedFiles, event.ChangedFilesKnown) {
			continue
		}
		return Match{Pipelines: trigger.Pipelines, Scope: trigger.Scope}
	}
	return Match{}
}

func eventMatches(configured, actual string) bool {
	configured = strings.TrimSpace(configured)
	return configured == "all" || configured == actual
}

func refMatches(trigger models.Trigger, eventType, ref, targetRef string) bool {
	switch eventType {
	case "push":
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			branch := strings.TrimPrefix(ref, "refs/heads/")
			included := matchesAny(branch, trigger.Branches)
			if len(trigger.Branches) == 0 && (len(trigger.SkipBranches) > 0 || trigger.On == "all") {
				included = true
			}
			return included && !matchesAny(branch, trigger.SkipBranches)
		case strings.HasPrefix(ref, "refs/tags/"):
			return matchesAny(strings.TrimPrefix(ref, "refs/tags/"), trigger.Tags)
		default:
			return false
		}
	case "pull_request":
		branch := strings.TrimPrefix(firstNonEmpty(targetRef, ref), "refs/heads/")
		if len(trigger.Branches) > 0 && !matchesAny(branch, trigger.Branches) {
			return false
		}
		return !matchesAny(branch, trigger.SkipBranches)
	default:
		return true
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pathsMatch(trigger models.Trigger, changedFiles []string, known bool) bool {
	if len(trigger.IncludePaths) == 0 && len(trigger.ExcludePaths) == 0 {
		return true
	}
	// Missing provider data must not silently skip CI.
	if !known {
		return true
	}

	remaining := make([]string, 0, len(changedFiles))
	for _, file := range changedFiles {
		file = normalizePath(file)
		if file == "" || matchesAny(file, trigger.ExcludePaths) {
			continue
		}
		remaining = append(remaining, file)
	}
	if len(remaining) == 0 {
		return false
	}
	if len(trigger.IncludePaths) == 0 {
		return true
	}
	for _, file := range remaining {
		if matchesAny(file, trigger.IncludePaths) {
			return true
		}
	}
	return false
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if MatchPattern(pattern, value) {
			return true
		}
	}
	return false
}

func MatchPattern(pattern, value string) bool {
	pattern = normalizePath(pattern)
	value = normalizePath(value)
	if pattern == "" || value == "" {
		return false
	}

	var builder strings.Builder
	builder.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					builder.WriteString("(?:.*/)?")
					i += 2
				} else {
					builder.WriteString(".*")
					i++
				}
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	builder.WriteByte('$')

	re, err := regexp.Compile(builder.String())
	if err != nil {
		return pattern == value
	}
	return re.MatchString(value)
}

func normalizePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	return strings.TrimPrefix(value, "./")
}
