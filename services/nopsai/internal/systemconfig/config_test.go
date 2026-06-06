package systemconfig

import "testing"

func TestNormalizeRunnerScopesTrimsDeduplicatesAndDropsEmpty(t *testing.T) {
	got := NormalizeRunnerScopes(" /prod/, prod, team/a, TEAM/a, , /staging ")
	want := "prod,team/a,staging"
	if got != want {
		t.Fatalf("NormalizeRunnerScopes() = %q, want %q", got, want)
	}
}

func TestCloneDispatcherRoutingNormalizesScopesAndRunners(t *testing.T) {
	got := CloneDispatcherRouting(map[string][]string{
		"":      {" runner-1 ", "", "runner-2"},
		" prod": {" runner-prod "},
	})

	if runners := got["*"]; len(runners) != 2 || runners[0] != "runner-1" || runners[1] != "runner-2" {
		t.Fatalf("root runners = %#v, want trimmed non-empty runners", runners)
	}
	if runners := got["prod"]; len(runners) != 1 || runners[0] != "runner-prod" {
		t.Fatalf("prod runners = %#v, want trimmed runner", runners)
	}
}
