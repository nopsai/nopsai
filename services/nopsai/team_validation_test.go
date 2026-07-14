package nopsai

import (
	"encoding/json"
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

func TestTeamWriteRequestPreservesOmittedParentAndAcceptsExplicitRoot(t *testing.T) {
	parent := 7
	var omitted teamWriteRequest
	if err := json.Unmarshal([]byte(`{"name":"platform"}`), &omitted); err != nil {
		t.Fatalf("unmarshal omitted parent: %v", err)
	}
	if got := parentIDFromTeamWriteRequest(omitted, &parent); got == nil || *got != parent {
		t.Fatalf("omitted parent = %v, want fallback %d", got, parent)
	}

	var explicitRoot teamWriteRequest
	if err := json.Unmarshal([]byte(`{"name":"platform","parent_team_id":null}`), &explicitRoot); err != nil {
		t.Fatalf("unmarshal explicit root parent: %v", err)
	}
	if got := parentIDFromTeamWriteRequest(explicitRoot, &parent); got != nil {
		t.Fatalf("explicit null parent = %v, want nil", *got)
	}

	var moved teamWriteRequest
	if err := json.Unmarshal([]byte(`{"name":"platform","parent_id":42}`), &moved); err != nil {
		t.Fatalf("unmarshal numeric parent: %v", err)
	}
	if got := parentIDFromTeamWriteRequest(moved, &parent); got == nil || *got != 42 {
		t.Fatalf("numeric parent = %v, want 42", got)
	}
}

func TestValidateTeamParentIDRejectsInvalidMoves(t *testing.T) {
	parent := 1
	child := 2
	records := map[int]teamPathRecord{
		parent: {ID: parent, Name: "platform", Kind: "team"},
		child:  {ID: child, Name: "payments", Kind: "team", ParentID: &parent},
		3:      {ID: 3, Name: "checkout-api", Kind: "app", ParentID: &child, RepoURL: "https://github.com/acme/checkout-api", RepositoryFullName: "acme/checkout-api"},
	}

	for name, testCase := range map[string]struct {
		teamID   int
		parentID *int
		wantErr  bool
	}{
		"moves under sibling team": {teamID: child, parentID: &parent, wantErr: false},
		"rejects self parent":      {teamID: child, parentID: &child, wantErr: true},
		"rejects descendant":       {teamID: parent, parentID: &child, wantErr: true},
		"rejects application":      {teamID: parent, parentID: intPointer(3), wantErr: true},
		"rejects missing parent":   {teamID: parent, parentID: intPointer(99), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateTeamParentID(records, testCase.teamID, testCase.parentID)
			if testCase.wantErr && err == nil {
				t.Fatal("validateTeamParentID() error = nil, want error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("validateTeamParentID() error = %v", err)
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
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
