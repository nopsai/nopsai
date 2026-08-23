package contract

import (
	"os"
	"strings"
	"testing"
)

// The About dialog in the web UI quotes the proprietary notice verbatim. So does
// `nopsai license`, and so does the LICENSE file shipped in every artifact. A
// legal notice that says three different things depending on where a user reads
// it is worse than one that is only in one place, so this test fails if the UI
// copy drifts from the CLI's.
func TestAboutDialogQuotesTheShippedLicenseNotice(t *testing.T) {
	cliSource, err := os.ReadFile("internal/cli/command/license.go")
	if err != nil {
		t.Fatalf("read CLI license command: %v", err)
	}
	uiSource, err := os.ReadFile("services/ui/src/app/AboutDialog.tsx")
	if err != nil {
		t.Fatalf("read About dialog: %v", err)
	}

	cliNotice := noticeLiteral(t, string(cliSource))
	uiNotice := noticeLiteral(t, string(uiSource))

	if cliNotice != uiNotice {
		t.Fatalf("the About dialog notice differs from `nopsai license`:\nCLI:\n%s\n\nUI:\n%s", cliNotice, uiNotice)
	}
	if !strings.Contains(cliNotice, "NopsAI Proprietary Software Notice") {
		t.Fatalf("expected the proprietary notice, got: %s", cliNotice)
	}

	// The same first line has to survive in the file shipped beside the binaries.
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatalf("read LICENSE: %v", err)
	}
	firstLine := strings.SplitN(cliNotice, "\n", 2)[0]
	if !strings.HasPrefix(string(license), firstLine) {
		t.Fatalf("LICENSE does not open with %q", firstLine)
	}
}

// Reads the backtick-quoted string assigned to proprietaryLicenseNotice, which
// both files declare under that name.
func noticeLiteral(t *testing.T, source string) string {
	t.Helper()
	const marker = "proprietaryLicenseNotice = "
	declaration := strings.LastIndex(source, marker)
	if declaration < 0 {
		t.Fatalf("no proprietaryLicenseNotice declaration found")
	}
	source = source[declaration+len(marker):]
	start := strings.Index(source, "`")
	if start < 0 {
		t.Fatal("no backtick-quoted notice found")
	}
	rest := source[start+1:]
	end := strings.Index(rest, "`")
	if end < 0 {
		t.Fatal("unterminated backtick-quoted notice")
	}
	return strings.TrimSpace(rest[:end])
}
