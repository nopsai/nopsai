package resolver

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/proto"
	workspacectx "nopsai/services/agent/internal/workspace"
)

type SharedFileIdentity struct {
	Path              string
	SHA256            string
	Size              int
	WorkspaceRevision uint64
}

type FilePrecondition struct {
	Path              string
	ExpectedSHA256    string
	Size              int
	WorkspaceRevision uint64
}

func buildSharedDirectoryContext(directoryListing map[string]string, workspaceRevision uint64) (map[string]string, map[string]SharedFileIdentity) {
	if len(directoryListing) == 0 {
		return map[string]string{}, map[string]SharedFileIdentity{}
	}
	annotated := make(map[string]string, len(directoryListing))
	identities := make(map[string]SharedFileIdentity, len(directoryListing))
	for path, content := range directoryListing {
		normalized := normalizeWorkspacePath(path)
		if normalized == "" {
			continue
		}
		identity := SharedFileIdentity{
			Path:              normalized,
			SHA256:            textSHA256(content),
			Size:              len([]byte(content)),
			WorkspaceRevision: workspaceRevision,
		}
		identities[normalized] = identity
		annotated[normalized] = formatSharedFileForPrompt(identity, content)
	}
	return annotated, identities
}

func buildSharedDirectoryContextFromWorkspaceIndex(index *workspacectx.Index) (map[string]string, map[string]SharedFileIdentity) {
	if index == nil {
		return map[string]string{}, map[string]SharedFileIdentity{}
	}
	entries := index.ListFiles(0)
	annotated := make(map[string]string, len(entries))
	identities := make(map[string]SharedFileIdentity, len(entries))
	for _, entry := range entries {
		identity := SharedFileIdentity{
			Path:              normalizeWorkspacePath(entry.Path),
			SHA256:            entry.SHA256,
			Size:              int(entry.Size),
			WorkspaceRevision: entry.WorkspaceRevision,
		}
		if identity.Path == "" || identity.SHA256 == "" {
			continue
		}
		content := fmt.Sprintf("[non-text file: sha256=%s size=%d workspace_revision=%d]", entry.SHA256, entry.Size, entry.WorkspaceRevision)
		if entry.Text {
			read, err := index.ReadFile(entry.Path, 0)
			if err != nil {
				content = fmt.Sprintf("[unreadable file: %s]", err.Error())
			} else {
				identity.SHA256 = read.SHA256
				identity.Size = int(read.Size)
				identity.WorkspaceRevision = read.WorkspaceRevision
				content = read.Content
				if read.Truncated {
					content += "\n...[truncated]"
				}
			}
		}
		identities[identity.Path] = identity
		annotated[identity.Path] = formatSharedFileForPrompt(identity, content)
	}
	return annotated, identities
}

func formatSharedFileForPrompt(identity SharedFileIdentity, content string) string {
	return fmt.Sprintf(
		"NopsAI File Identity:\npath: %s\nsha256: %s\nsize: %d\nworkspace_revision: %d\n--- Content ---\n%s",
		identity.Path,
		identity.SHA256,
		identity.Size,
		identity.WorkspaceRevision,
		content,
	)
}

func replaceFilePrecondition(action *proto.Action, identities map[string]SharedFileIdentity, workspaceRevision uint64) FilePrecondition {
	fileAction := action.GetFileAction()
	if fileAction == nil {
		return FilePrecondition{}
	}
	actionPath := normalizeWorkspacePath(fileAction.GetPath())
	if actionPath == "" {
		return FilePrecondition{}
	}
	precondition := FilePrecondition{
		Path:              actionPath,
		WorkspaceRevision: workspaceRevision,
	}
	if identity, ok := identities[actionPath]; ok {
		precondition.ExpectedSHA256 = identity.SHA256
		precondition.Size = identity.Size
		precondition.WorkspaceRevision = identity.WorkspaceRevision
	}
	return precondition
}

func ValidateReplaceFilePrecondition(precondition FilePrecondition, workspaceRoot string, currentWorkspaceRevision uint64) error {
	if strings.TrimSpace(precondition.Path) == "" {
		return nil
	}
	if precondition.ExpectedSHA256 == "" {
		if precondition.WorkspaceRevision != 0 && currentWorkspaceRevision != precondition.WorkspaceRevision {
			return fmt.Errorf("workspace changed from revision %d to %d before replacing %s; re-read the file before writing", precondition.WorkspaceRevision, currentWorkspaceRevision, precondition.Path)
		}
		return nil
	}

	currentSHA, err := fileSHA256(filepath.Join(strings.TrimSpace(workspaceRoot), filepath.FromSlash(precondition.Path)))
	if err != nil {
		return fmt.Errorf("cannot validate %s before replacing it: %w", precondition.Path, err)
	}
	if currentSHA != precondition.ExpectedSHA256 {
		return fmt.Errorf("file %s changed before replace: expected sha256 %s, current sha256 %s; re-read the file before writing", precondition.Path, precondition.ExpectedSHA256, currentSHA)
	}
	return nil
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

func textSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

func fileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return textSHA256(string(content)), nil
}

func sortedSharedFileIdentities(identities map[string]SharedFileIdentity) []SharedFileIdentity {
	out := make([]SharedFileIdentity, 0, len(identities))
	for _, identity := range identities {
		out = append(out, identity)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}
