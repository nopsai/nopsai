package nopsai

import (
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func TestNormalizeTeamForWriteRejectsReservedRootTeamName(t *testing.T) {
	tests := []string{"root", " Root ", "/root/", "__general__"}
	for _, name := range tests {
		team := &Team{Kind: "team", Name: name}
		if err := normalizeTeamForWrite(team); err == nil {
			t.Fatalf("normalizeTeamForWrite(%q) error = nil, want reserved root error", name)
		}
	}
}

func TestNormalizeStructureNameRejectsReservedRootTeamName(t *testing.T) {
	tests := []string{"root", " Root ", "/root/", "__general__"}
	for _, name := range tests {
		if _, err := configsync.NormalizeStructureName(name); err == nil {
			t.Fatalf("normalizeStructureName(%q) error = nil, want reserved root error", name)
		}
	}
}

func TestVisibleTeamTeamIDsIncludesAncestorContainers(t *testing.T) {
	teamID := 1
	devID := 2
	teams := []Team{
		{ID: teamID, Name: "team-1"},
		{ID: devID, Name: "dev", ParentID: &teamID},
		{ID: 3, Name: "other-team"},
	}
	pathRecords := map[int]teamPathRecord{
		teamID: {ID: teamID, Path: "team-1"},
		devID:  {ID: devID, Path: "team-1/dev", ParentID: &teamID},
		3:      {ID: 3, Path: "other-team"},
	}
	allowedSet := map[string]struct{}{
		resourceKey(model.ResourceRef{Type: grantResourceTeam, ID: "team-1/dev"}): {},
	}

	visible, direct := visibleTeamTeamIDs(teams, pathRecords, allowedSet)

	if _, ok := visible[teamID]; !ok {
		t.Fatal("expected parent team-1 to be visible as an ancestor container")
	}
	if _, ok := visible[devID]; !ok {
		t.Fatal("expected directly allowed team-1/dev to be visible")
	}
	if _, ok := visible[3]; ok {
		t.Fatal("did not expect unrelated sibling team to be visible")
	}
	if _, ok := direct[teamID]; ok {
		t.Fatal("did not expect parent team-1 to be directly allowed")
	}
	if _, ok := direct[devID]; !ok {
		t.Fatal("expected team-1/dev to be directly allowed")
	}
}
