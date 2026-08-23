package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"nopsai/pkg/licensenotice"
)

// Acceptance recorded at first install is only meaningful if the text an
// administrator accepted is the same text shipped beside the binaries. go:embed
// cannot reach outside a package directory, so the notice is duplicated into
// pkg/licensenotice and this test is what keeps the two honest.
func TestLicenseNoticeMatchesShippedLicenseFile(t *testing.T) {
	shipped, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatalf("read LICENSE: %v", err)
	}
	if licensenotice.Text != string(shipped) {
		t.Fatal("pkg/licensenotice.Text has drifted from the LICENSE file; the accepted notice must be the shipped notice")
	}

	sum := sha256.Sum256(shipped)
	if want := hex.EncodeToString(sum[:]); licensenotice.SHA256() != want {
		t.Fatalf("licensenotice.SHA256() = %s, want %s", licensenotice.SHA256(), want)
	}

	if licensenotice.Version == "" {
		t.Fatal("licensenotice.Version must identify the notice document so a changed notice is re-accepted")
	}
}
