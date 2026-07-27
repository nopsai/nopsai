package agent

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

type Matcher struct {
	Pattern  string
	IsDir    bool
	IsGlobal bool
}

func (m Matcher) Matches(relPath string, isDir bool) bool {
	if m.IsDir {
		if !isDir && !strings.HasPrefix(relPath, m.Pattern) {
			return false
		}
		// Match if the path is the directory itself or a path within it
		return relPath == m.Pattern || strings.HasPrefix(relPath, m.Pattern+"/")
	}

	if m.IsGlobal {
		// If the pattern has no '/', it should match the basename of the path.
		base := filepath.Base(relPath)
		matched, _ := filepath.Match(m.Pattern, base)
		return matched
	}

	// It's a full path pattern
	matched, _ := filepath.Match(m.Pattern, relPath)
	return matched
}

func buildPathMatchers(patterns []string) []Matcher {
	var matchers []Matcher
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		p = filepath.ToSlash(p)
		p = strings.TrimPrefix(p, "./")
		isDir := strings.HasSuffix(p, "/")
		pattern := strings.TrimSuffix(p, "/")
		if pattern == "" {
			continue
		}
		matchers = append(matchers, Matcher{
			Pattern:  pattern,
			IsDir:    isDir,
			IsGlobal: !strings.Contains(pattern, "/"),
		})
	}
	return matchers
}

func isIgnored(path string, matchers []Matcher, root string, isDir bool) bool {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	// On Windows, convert backslashes to forward slashes for consistent matching
	relPath = filepath.ToSlash(relPath)

	for _, matcher := range matchers {
		if matcher.Matches(relPath, isDir) {
			return true
		}
	}
	return false
}

func isIncluded(path string, matchers []Matcher, root string, isDir bool) bool {
	if len(matchers) == 0 {
		return true
	}
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)

	for _, matcher := range matchers {
		if matcher.Matches(relPath, isDir) {
			return true
		}
	}
	if isDir {
		return false
	}

	dir := filepath.ToSlash(filepath.Dir(relPath))
	for dir != "." && dir != "/" && dir != "" {
		for _, matcher := range matchers {
			if matcher.Matches(dir, true) {
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

func getDirectoryListing(logger *zerolog.Logger, root string, includePatterns, ignorePatterns []string) map[string]string {
	directoryListing := make(map[string]string)
	includeMatchers := buildPathMatchers(includePatterns)
	ignoreMatchers := buildPathMatchers(ignorePatterns)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error().Err(err).Str("path", path).Msg("Error accessing path")
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Check if the path should be ignored by the patterns from the pipeline directive
		if isIgnored(path, ignoreMatchers, root, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir // Skip the entire directory
			}
			return nil // Skip this file
		}

		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			if !isIncluded(path, includeMatchers, root, false) {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Error().Err(readErr).Str("file", path).Msg("Failed to read file")
				return nil
			}
			relPath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relPath = filepath.ToSlash(relPath)
			contentType := http.DetectContentType(content)
			if strings.HasPrefix(contentType, "text/") {
				directoryListing[relPath] = string(content)
			} else {
				directoryListing[relPath] = fmt.Sprintf("[non-text file: %s]", contentType)
			}
		}
		return nil
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to walk directory")
	}
	return directoryListing
}
