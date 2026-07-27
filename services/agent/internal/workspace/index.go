package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

const (
	defaultMaxReadBytes      = 24000
	defaultMaxSearchResults  = 20
	defaultMaxListFiles      = 500
	defaultMaxSearchLineSize = 240
)

type FileEntry struct {
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	WorkspaceRevision uint64 `json:"workspace_revision"`
	Text              bool   `json:"text"`
}

type SearchResult struct {
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	WorkspaceRevision uint64 `json:"workspace_revision"`
	Line              int    `json:"line"`
	Preview           string `json:"preview"`
}

type ListFilesPage struct {
	Files             []FileEntry `json:"files"`
	NextCursor        string      `json:"next_cursor,omitempty"`
	WorkspaceRevision uint64      `json:"workspace_revision"`
}

type SearchCodePage struct {
	Query             string         `json:"query"`
	Results           []SearchResult `json:"results"`
	NextCursor        string         `json:"next_cursor,omitempty"`
	WorkspaceRevision uint64         `json:"workspace_revision"`
}

type ReadFileResult struct {
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	WorkspaceRevision uint64 `json:"workspace_revision"`
	Offset            int64  `json:"offset"`
	NextOffset        int64  `json:"next_offset,omitempty"`
	EOF               bool   `json:"eof"`
	Content           string `json:"content"`
	Truncated         bool   `json:"truncated"`
}

type Index struct {
	mu       sync.RWMutex
	root     string
	include  []matcher
	ignore   []matcher
	revision uint64
	entries  map[string]FileEntry
}

func NewIndex(root string, includePatterns, ignorePatterns []string) *Index {
	return &Index{
		root:    strings.TrimSpace(root),
		include: buildMatchers(includePatterns),
		ignore:  buildMatchers(ignorePatterns),
		entries: map[string]FileEntry{},
	}
}

func (i *Index) Refresh(logger *zerolog.Logger, revision uint64) error {
	if i == nil {
		return nil
	}
	root := strings.TrimSpace(i.root)
	if root == "" {
		root = "."
	}
	entries := map[string]FileEntry{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if logger != nil {
				logger.Warn().Err(err).Str("path", path).Msg("Skipping workspace path during index refresh")
			}
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			if logger != nil {
				logger.Warn().Err(infoErr).Str("path", path).Msg("Skipping workspace path without file info")
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if isIgnored(path, i.ignore, root, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isIncluded(path, i.include, root, false) {
			return nil
		}
		entry, scanErr := scanFileEntry(path, relPath(root, path), info, revision)
		if scanErr != nil {
			if logger != nil {
				logger.Warn().Err(scanErr).Str("path", path).Msg("Skipping unreadable workspace file")
			}
			return nil
		}
		entries[entry.Path] = entry
		return nil
	})
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.revision = revision
	i.entries = entries
	i.mu.Unlock()
	return nil
}

func (i *Index) Revision() uint64 {
	if i == nil {
		return 0
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.revision
}

func (i *Index) ListFiles(limit int) []FileEntry {
	page := i.ListFilesPage("", limit)
	return page.Files
}

func (i *Index) ListFilesPage(cursor string, limit int) ListFilesPage {
	if i == nil {
		return ListFilesPage{}
	}
	if limit <= 0 || limit > defaultMaxListFiles {
		limit = defaultMaxListFiles
	}
	i.mu.RLock()
	entries := make([]FileEntry, 0, len(i.entries))
	for _, entry := range i.entries {
		entries = append(entries, entry)
	}
	i.mu.RUnlock()
	sort.Slice(entries, func(a, b int) bool {
		return entries[a].Path < entries[b].Path
	})
	start := 0
	cursor = normalizeWorkspacePath(cursor)
	if cursor != "" {
		for idx, entry := range entries {
			if entry.Path > cursor {
				start = idx
				break
			}
			start = idx + 1
		}
	}
	if start > len(entries) {
		start = len(entries)
	}
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	files := append([]FileEntry(nil), entries[start:end]...)
	nextCursor := ""
	if end < len(entries) && len(files) > 0 {
		nextCursor = files[len(files)-1].Path
	}
	return ListFilesPage{Files: files, NextCursor: nextCursor, WorkspaceRevision: i.Revision()}
}

func (i *Index) SharedFileContents(limit int) map[string]string {
	files := i.ListFiles(limit)
	out := make(map[string]string, len(files))
	for _, file := range files {
		if !file.Text {
			out[file.Path] = fmt.Sprintf("[non-text file: sha256=%s size=%d workspace_revision=%d]", file.SHA256, file.Size, file.WorkspaceRevision)
			continue
		}
		content, err := i.readFileContent(file.Path, defaultMaxReadBytes)
		if err != nil {
			out[file.Path] = fmt.Sprintf("[unreadable file: %s]", err.Error())
			continue
		}
		out[file.Path] = content
	}
	return out
}

func (i *Index) SearchCode(query string, maxResults int) ([]SearchResult, error) {
	page, err := i.SearchCodePage(query, "", maxResults)
	if err != nil {
		return nil, err
	}
	return page.Results, nil
}

func (i *Index) SearchCodePage(query string, cursor string, maxResults int) (SearchCodePage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchCodePage{}, fmt.Errorf("query is required")
	}
	if maxResults <= 0 || maxResults > defaultMaxSearchResults {
		maxResults = defaultMaxSearchResults
	}
	offset := parseNonNegativeCursor(cursor)
	queryLower := strings.ToLower(query)
	results := []SearchResult{}
	matched := 0
	nextCursor := ""
	for _, entry := range i.ListFiles(defaultMaxListFiles) {
		if len(results) >= maxResults && nextCursor != "" {
			break
		}
		if !entry.Text {
			continue
		}
		content, err := i.readFileContent(entry.Path, defaultMaxReadBytes)
		if err != nil {
			continue
		}
		lines := strings.Split(content, "\n")
		for idx, line := range lines {
			if len(results) >= maxResults && nextCursor != "" {
				break
			}
			if !strings.Contains(strings.ToLower(line), queryLower) {
				continue
			}
			matched++
			if matched <= offset {
				continue
			}
			if len(results) >= maxResults {
				nextCursor = fmt.Sprintf("%d", matched-1)
				break
			}
			results = append(results, SearchResult{
				Path:              entry.Path,
				SHA256:            entry.SHA256,
				Size:              entry.Size,
				WorkspaceRevision: entry.WorkspaceRevision,
				Line:              idx + 1,
				Preview:           truncateSingleLine(line, defaultMaxSearchLineSize),
			})
		}
	}
	return SearchCodePage{
		Query:             query,
		Results:           results,
		NextCursor:        nextCursor,
		WorkspaceRevision: i.Revision(),
	}, nil
}

func (i *Index) ReadFile(path string, maxBytes int) (ReadFileResult, error) {
	return i.ReadFileRange(path, 0, maxBytes)
}

func (i *Index) ReadFileRange(path string, offset int64, maxBytes int) (ReadFileResult, error) {
	normalized := normalizeWorkspacePath(path)
	if normalized == "" {
		return ReadFileResult{}, fmt.Errorf("path is required")
	}
	if offset < 0 {
		return ReadFileResult{}, fmt.Errorf("offset must be non-negative")
	}
	if maxBytes <= 0 || maxBytes > defaultMaxReadBytes {
		maxBytes = defaultMaxReadBytes
	}
	i.mu.RLock()
	entry, ok := i.entries[normalized]
	i.mu.RUnlock()
	if !ok {
		return ReadFileResult{}, fmt.Errorf("file %q is not available in the workspace index", normalized)
	}
	if !entry.Text {
		return ReadFileResult{}, fmt.Errorf("file %q is not text content", normalized)
	}
	root := strings.TrimSpace(i.root)
	if root == "" {
		root = "."
	}
	fullPath := filepath.Join(root, filepath.FromSlash(normalized))
	info, err := os.Lstat(fullPath)
	if err != nil {
		return ReadFileResult{}, err
	}
	currentEntry, err := scanFileEntry(fullPath, normalized, info, entry.WorkspaceRevision)
	if err != nil {
		return ReadFileResult{}, err
	}
	if !currentEntry.Text {
		return ReadFileResult{}, fmt.Errorf("file %q is no longer text content", normalized)
	}
	if offset > currentEntry.Size {
		return ReadFileResult{}, fmt.Errorf("offset %d exceeds file size %d", offset, currentEntry.Size)
	}
	content, truncated, err := i.readFileContentWithRange(normalized, offset, maxBytes)
	if err != nil {
		return ReadFileResult{}, err
	}
	nextOffset := offset + int64(len([]byte(content)))
	eof := !truncated && nextOffset >= currentEntry.Size
	if eof {
		nextOffset = 0
	}
	return ReadFileResult{
		Path:              currentEntry.Path,
		SHA256:            currentEntry.SHA256,
		Size:              currentEntry.Size,
		WorkspaceRevision: currentEntry.WorkspaceRevision,
		Offset:            offset,
		NextOffset:        nextOffset,
		EOF:               eof,
		Content:           content,
		Truncated:         truncated,
	}, nil
}

func (i *Index) ToolResultJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"error":"failed to marshal workspace tool result"}`)
	}
	return payload
}

func (i *Index) readFileContent(path string, maxBytes int) (string, error) {
	content, _, err := i.readFileContentWithLimit(path, maxBytes)
	return content, err
}

func (i *Index) readFileContentWithLimit(path string, maxBytes int) (string, bool, error) {
	return i.readFileContentWithRange(path, 0, maxBytes)
}

func (i *Index) readFileContentWithRange(path string, offset int64, maxBytes int) (string, bool, error) {
	if i == nil {
		return "", false, fmt.Errorf("workspace index is not available")
	}
	normalized := normalizeWorkspacePath(path)
	if normalized == "" {
		return "", false, fmt.Errorf("path is required")
	}
	root := strings.TrimSpace(i.root)
	if root == "" {
		root = "."
	}
	fullPath := filepath.Join(root, filepath.FromSlash(normalized))
	info, err := os.Lstat(fullPath)
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("symlink %q is not available in the workspace index", normalized)
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return "", false, err
		}
	}

	var content []byte
	if maxBytes > 0 {
		content, err = io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	} else {
		content, err = io.ReadAll(file)
	}
	if err != nil {
		return "", false, err
	}
	truncated := false
	if maxBytes > 0 && len(content) > maxBytes {
		content = content[:maxBytes]
		truncated = true
	}
	return string(content), truncated, nil
}

func parseNonNegativeCursor(cursor string) int {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0
	}
	var value int
	if _, err := fmt.Sscanf(cursor, "%d", &value); err != nil || value < 0 {
		return 0
	}
	return value
}

func isTextContent(content []byte) bool {
	if len(content) == 0 {
		return true
	}
	contentType := http.DetectContentType(content)
	return strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml")
}

func scanFileEntry(path, relPath string, info fs.FileInfo, revision uint64) (FileEntry, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return FileEntry{}, fmt.Errorf("symlink %q is not available in the workspace index", relPath)
	}
	file, err := os.Open(path)
	if err != nil {
		return FileEntry{}, err
	}
	defer file.Close()

	hasher := sha256.New()
	firstBytes := make([]byte, 512)
	n, readErr := file.Read(firstBytes)
	if readErr != nil && readErr != io.EOF {
		return FileEntry{}, readErr
	}
	if n > 0 {
		if _, err := hasher.Write(firstBytes[:n]); err != nil {
			return FileEntry{}, err
		}
	}
	if readErr != io.EOF {
		if _, err := io.Copy(hasher, file); err != nil {
			return FileEntry{}, err
		}
	}
	return FileEntry{
		Path:              relPath,
		SHA256:            fmt.Sprintf("%x", hasher.Sum(nil)),
		Size:              info.Size(),
		WorkspaceRevision: revision,
		Text:              isTextContent(firstBytes[:n]),
	}, nil
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func bytesSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func truncateSingleLine(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
