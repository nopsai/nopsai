package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestLicenseCommandPrintsOwnershipAndLicenceRequirement(t *testing.T) {
	command := newLicenseCommand()
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("execute license command: %v", err)
	}

	for _, expected := range []string{
		"Hossein Yousefi",
		"proprietary software",
		"written agreement",
		"THIRD_PARTY_NOTICES.md",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("license output does not contain %q: %s", expected, output.String())
		}
	}
}
