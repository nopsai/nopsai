package configsync

import (
	"sort"
	"strings"
)

type DriftItem struct {
	Path           string  `json:"path"`
	Status         string  `json:"status"`
	GitContent     *string `json:"git_content,omitempty"`
	DesiredContent *string `json:"desired_content,omitempty"`
	Delete         bool    `json:"delete,omitempty"`
}

type FileEqualFunc func(filePath, gitContent, desiredContent string) bool

func DiffFiles(gitFiles, desiredFiles map[string]string, equal FileEqualFunc) []DriftItem {
	pathSet := make(map[string]struct{}, len(gitFiles)+len(desiredFiles))
	for filePath := range gitFiles {
		pathSet[filePath] = struct{}{}
	}
	for filePath := range desiredFiles {
		pathSet[filePath] = struct{}{}
	}

	paths := make([]string, 0, len(pathSet))
	for filePath := range pathSet {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)

	if equal == nil {
		equal = func(_ string, gitContent, desiredContent string) bool {
			return gitContent == desiredContent
		}
	}

	items := make([]DriftItem, 0, len(paths))
	for _, filePath := range paths {
		gitContent, inGit := gitFiles[filePath]
		desiredContent, inDesired := desiredFiles[filePath]
		switch {
		case !inGit && inDesired:
			content := desiredContent
			items = append(items, DriftItem{Path: filePath, Status: "added", DesiredContent: &content})
		case inGit && !inDesired:
			content := gitContent
			items = append(items, DriftItem{Path: filePath, Status: "deleted", GitContent: &content, Delete: true})
		case inGit && inDesired && !equal(filePath, gitContent, desiredContent):
			before, after := gitContent, desiredContent
			items = append(items, DriftItem{Path: filePath, Status: "modified", GitContent: &before, DesiredContent: &after})
		default:
			before, after := gitContent, desiredContent
			items = append(items, DriftItem{Path: filePath, Status: "unchanged", GitContent: &before, DesiredContent: &after})
		}
	}
	return items
}

func NormalizeFileContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n") + "\n"
	return content
}
