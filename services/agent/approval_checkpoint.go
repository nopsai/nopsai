package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	approvalCheckpointMaxBytesEnv           = "NOPSAI_APPROVAL_CHECKPOINT_MAX_BYTES"
	defaultApprovalCheckpointMaxBytes int64 = 50 * 1024 * 1024
)

type agentApprovalCheckpointResponse struct {
	CheckpointID           string            `json:"checkpoint_id"`
	RunID                  string            `json:"run_id"`
	StepName               string            `json:"step_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64,omitempty"`
	WorkspaceArchiveFormat string            `json:"workspace_archive_format,omitempty"`
}

func approvalCheckpointMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv(approvalCheckpointMaxBytesEnv))
	if raw == "" {
		return defaultApprovalCheckpointMaxBytes
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultApprovalCheckpointMaxBytes
	}
	return value
}

type limitedBuffer struct {
	buf bytes.Buffer
	max int64
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if w == nil {
		return 0, fmt.Errorf("checkpoint writer is not configured")
	}
	if w.max > 0 && int64(w.buf.Len()+len(p)) > w.max {
		return 0, fmt.Errorf("workspace checkpoint exceeds %d bytes", w.max)
	}
	return w.buf.Write(p)
}

func (w *limitedBuffer) Bytes() []byte {
	if w == nil {
		return nil
	}
	return w.buf.Bytes()
}

func archiveWorkspace(root string, maxBytes int64) ([]byte, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}

	limited := &limitedBuffer{max: maxBytes}
	gzipWriter := gzip.NewWriter(limited)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.Walk(root, func(entryPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if entryPath == root {
			return nil
		}
		mode := info.Mode()
		if !mode.IsRegular() && !info.IsDir() && mode&os.ModeSymlink == 0 {
			return nil
		}

		rel, err := filepath.Rel(root, entryPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		linkName := ""
		if mode&os.ModeSymlink != 0 {
			linkName, err = os.Readlink(entryPath)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, linkName)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !mode.IsRegular() {
			return nil
		}
		file, err := os.Open(entryPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeErr := tarWriter.Close()
	gzipErr := gzipWriter.Close()
	if walkErr != nil {
		return nil, fmt.Errorf("archive workspace: %w", walkErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("finalize workspace archive: %w", closeErr)
	}
	if gzipErr != nil {
		return nil, fmt.Errorf("compress workspace archive: %w", gzipErr)
	}
	return limited.Bytes(), nil
}

func restoreWorkspaceArchive(root string, archive []byte) error {
	if len(archive) == 0 {
		return nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return fmt.Errorf("workspace root is required")
	}
	if err := clearWorkspaceContents(root); err != nil {
		return err
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open workspace archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read workspace archive: %w", err)
		}

		relPath, err := safeArchiveRelativePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := ensureSafeSymlinkTarget(relPath, header.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func clearWorkspaceContents(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read workspace root: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("clear workspace entry %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func safeArchiveRelativePath(name string) (string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(name)))
	if rel == "." || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workspace archive contains unsafe path %q", name)
	}
	return rel, nil
}

func ensureSafeSymlinkTarget(relPath, linkName string) error {
	linkName = strings.TrimSpace(linkName)
	if linkName == "" || filepath.IsAbs(linkName) {
		return fmt.Errorf("workspace archive contains unsafe symlink target")
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(relPath), filepath.FromSlash(linkName)))
	if target == ".." || strings.HasPrefix(target, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("workspace archive contains symlink escaping workspace")
	}
	return nil
}
