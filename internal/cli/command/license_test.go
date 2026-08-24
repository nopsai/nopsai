package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestLicenseCommandPrintsTheNonCommercialGrantAndTheCommercialBoundary(t *testing.T) {
	command := newLicenseCommand(&rootOptions{})
	command.SetArgs([]string{})
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("execute license command: %v", err)
	}

	for _, expected := range []string{
		"Hossein Yousefi",
		"PolyForm Noncommercial License 1.0.0",
		"free",
		"Commercial use is not granted by this licence",
		"written agreement",
		"contact@nopsai.com",
		"THIRD_PARTY_NOTICES.md",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("license output does not contain %q: %s", expected, output.String())
		}
	}
}

// A limit of zero means unlimited. Printing it as a ceiling of nothing would
// tell an operator their licence forbids everything.
func TestLicenseStatusRendersUnlimitedRatherThanZero(t *testing.T) {
	if got := limitLine(3, 0); got != "3 of unlimited" {
		t.Errorf("limitLine(3, 0) = %q, want %q", got, "3 of unlimited")
	}
	if got := limitLine(3, 10); got != "3 of 10" {
		t.Errorf("limitLine(3, 10) = %q, want %q", got, "3 of 10")
	}
	if got := ceilingLine(0); got != "unlimited" {
		t.Errorf("ceilingLine(0) = %q, want %q", got, "unlimited")
	}
	if got := ceilingLine(4); got != "up to 4 concurrent" {
		t.Errorf("ceilingLine(4) = %q, want %q", got, "up to 4 concurrent")
	}
}

func TestLicenseStatusExplainsANonCommercialInstallation(t *testing.T) {
	command := newLicenseCommand(&rootOptions{})
	var output bytes.Buffer
	command.SetOut(&output)

	status := licenseStatusResponse{
		Tier:   "noncommercial",
		Reason: "No commercial licence key is configured. Running under the non-commercial licence, which is free and uncapped for any non-commercial purpose.",
	}
	status.Usage.Users = 1

	if err := renderLicenseStatus(command, status, "text"); err != nil {
		t.Fatalf("renderLicenseStatus: %v", err)
	}
	// An uncapped limit must read as unlimited, never as a ceiling of nothing.
	for _, expected := range []string{"Non-commercial use", "noncommercial", "free and uncapped", "1 of unlimited"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("licence status output is missing %q:\n%s", expected, output.String())
		}
	}
}

// The interactive console must cover the same ground as the CLI, so the
// entitlement screen degrades to a readable notice rather than failing when the
// API cannot be reached.
func TestInteractiveLicenseScreenSurvivesAnUnreachableAPI(t *testing.T) {
	command := newLicenseCommand(&rootOptions{})
	lines := interactiveLicenseEntitlementLines(command, homeState{})
	if len(lines) == 0 {
		t.Fatal("the entitlement summary must always render something")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Entitlement") {
		t.Errorf("expected an entitlement line, got %#v", lines)
	}
}
