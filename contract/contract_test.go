// Package contract holds the cross-cutting tests that assert repository-wide
// invariants: that the shipped licence notice matches the one the UI and CLI
// quote, that every registered route is documented, that the Compose file and
// Dockerfiles agree with the release contract, and so on.
//
// They are not unit tests of any one package. They read files from all over the
// repository, which is why they live in one place rather than being scattered
// into whichever package happens to be nearby.
package contract

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain moves the working directory to the repository root before any test
// runs. These tests assert things about repository layout using paths relative
// to the root, so anchoring once here keeps every path readable instead of
// prefixing hundreds of them with "../".
func TestMain(m *testing.M) {
	root, err := repositoryRoot()
	if err != nil {
		panic("contract tests cannot locate the repository root: " + err.Error())
	}
	if err := os.Chdir(root); err != nil {
		panic("contract tests cannot enter the repository root: " + err.Error())
	}
	os.Exit(m.Run())
}

// repositoryRoot walks up from the package directory to the directory holding
// go.mod, so the tests keep working if this package is moved again.
func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", os.ErrNotExist
		}
		directory = parent
	}
}
