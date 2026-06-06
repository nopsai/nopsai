package nopsai

import (
	"testing"

	"nopsai/services/nopsai/internal/configsync"
)

func TestNormalizeGroupForWriteRejectsReservedRootGroupName(t *testing.T) {
	tests := []string{"root", " Root ", "/root/", "__general__"}
	for _, name := range tests {
		group := &Group{Kind: "group", Name: name}
		if err := normalizeGroupForWrite(group); err == nil {
			t.Fatalf("normalizeGroupForWrite(%q) error = nil, want reserved root error", name)
		}
	}
}

func TestNormalizeStructureNameRejectsReservedRootGroupName(t *testing.T) {
	tests := []string{"root", " Root ", "/root/", "__general__"}
	for _, name := range tests {
		if _, err := configsync.NormalizeStructureName(name); err == nil {
			t.Fatalf("normalizeStructureName(%q) error = nil, want reserved root error", name)
		}
	}
}
