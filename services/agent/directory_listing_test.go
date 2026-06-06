package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestGetDirectoryListingIncludeLimitsSharedFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "root docs")
	writeTestFile(t, root, "src/main.go", "package main")
	writeTestFile(t, root, "src/vendor/generated.go", "package generated")
	writeTestFile(t, root, "docs/guide.md", "guide")

	logger := zerolog.Nop()
	listing := getDirectoryListing(&logger, root, []string{"src/"}, []string{"src/vendor/"})

	if _, ok := listing["src/main.go"]; !ok {
		t.Fatalf("expected included source file, got %#v", listing)
	}
	for _, unexpected := range []string{"README.md", "docs/guide.md", "src/vendor/generated.go"} {
		if _, ok := listing[unexpected]; ok {
			t.Fatalf("did not expect %q in directory listing: %#v", unexpected, listing)
		}
	}
}

func TestGetDirectoryListingIncludeFolderWithoutTrailingSlash(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "package main")
	writeTestFile(t, root, "README.md", "root docs")

	logger := zerolog.Nop()
	listing := getDirectoryListing(&logger, root, []string{"src"}, nil)

	if _, ok := listing["src/main.go"]; !ok {
		t.Fatalf("expected files under included folder, got %#v", listing)
	}
	if _, ok := listing["README.md"]; ok {
		t.Fatalf("did not expect README.md in directory listing: %#v", listing)
	}
}

func TestGetDirectoryListingEmptyIncludePreservesIgnoreOnlyBehavior(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "root docs")
	writeTestFile(t, root, "build/output.log", "compiled")

	logger := zerolog.Nop()
	listing := getDirectoryListing(&logger, root, nil, []string{"build/"})

	if _, ok := listing["README.md"]; !ok {
		t.Fatalf("expected non-ignored file, got %#v", listing)
	}
	if _, ok := listing["build/output.log"]; ok {
		t.Fatalf("did not expect ignored file in directory listing: %#v", listing)
	}
}

func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
