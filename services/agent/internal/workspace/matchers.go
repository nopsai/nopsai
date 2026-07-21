package workspace

import (
	"path/filepath"
	"strings"
)

type matcher struct {
	pattern  string
	isDir    bool
	isGlobal bool
}

func (m matcher) matches(relPath string, isDir bool) bool {
	if m.isDir {
		if !isDir && !strings.HasPrefix(relPath, m.pattern) {
			return false
		}
		return relPath == m.pattern || strings.HasPrefix(relPath, m.pattern+"/")
	}
	if m.isGlobal {
		matched, _ := filepath.Match(m.pattern, filepath.Base(relPath))
		return matched
	}
	matched, _ := filepath.Match(m.pattern, relPath)
	return matched
}

func buildMatchers(patterns []string) []matcher {
	matchers := []matcher{}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		pattern = filepath.ToSlash(pattern)
		pattern = strings.TrimPrefix(pattern, "./")
		isDir := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		if pattern == "" {
			continue
		}
		matchers = append(matchers, matcher{
			pattern:  pattern,
			isDir:    isDir,
			isGlobal: !strings.Contains(pattern, "/"),
		})
	}
	return matchers
}

func isIgnored(path string, matchers []matcher, root string, isDir bool) bool {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	for _, matcher := range matchers {
		if matcher.matches(relPath, isDir) {
			return true
		}
	}
	return false
}

func isIncluded(path string, matchers []matcher, root string, isDir bool) bool {
	if len(matchers) == 0 {
		return true
	}
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	for _, matcher := range matchers {
		if matcher.matches(relPath, isDir) {
			return true
		}
	}
	if isDir {
		return false
	}
	dir := filepath.ToSlash(filepath.Dir(relPath))
	for dir != "." && dir != "/" && dir != "" {
		for _, matcher := range matchers {
			if matcher.matches(dir, true) {
				return true
			}
		}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

func normalizeWorkspacePath(input string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(input))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}
