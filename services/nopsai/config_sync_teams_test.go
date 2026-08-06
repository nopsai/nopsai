package nopsai

import (
	"reflect"
	"sort"
	"testing"

	"nopsai/services/nopsai/internal/configsync"
)

func TestPipelineRunAuthTeamNamesForStructureUsesFullTeamPaths(t *testing.T) {
	got, err := pipelineRunAuthTeamNamesForStructure(map[string]*configsync.PipelineRunStructureNode{
		"engineering": {
			Children: map[string]*configsync.PipelineRunStructureNode{
				"platform": {
					Apps: []configsync.PipelineRunStructureApp{{Name: "service-api", RepoURL: "https://github.com/acme/service-api"}},
				},
				"service-owners": {},
			},
		},
		"platform": {
			Children: map[string]*configsync.PipelineRunStructureNode{
				"prod":           {},
				"infrastructure": {},
			},
		},
	})
	if err != nil {
		t.Fatalf("pipelineRunAuthTeamNamesForStructure() error = %v", err)
	}
	sort.Strings(got)

	want := []string{
		"engineering",
		"engineering/platform",
		"engineering/service-owners",
		"platform",
		"platform/infrastructure",
		"platform/prod",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auth team names = %#v, want %#v", got, want)
	}
}
