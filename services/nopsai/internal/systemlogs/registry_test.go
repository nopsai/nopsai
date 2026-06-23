package systemlogs

import "testing"

func TestDefaultRegistryContainsOnlyPlatformServices(t *testing.T) {
	registry := DefaultRegistry()
	for _, id := range []string{"nopsai", "aaa", "dispatcher", "git-bot", "ui", "docker-runner", "k8s-runner"} {
		if _, ok := registry.Resolve(id); !ok {
			t.Fatalf("default registry missing %q", id)
		}
	}
	for _, id := range []string{"base", "agent", "pipeline", "db"} {
		if _, ok := registry.Resolve(id); ok {
			t.Fatalf("default registry unexpectedly contains %q", id)
		}
	}
}

func TestRegistryDropsInvalidDefinitionsAndReturnsSortedSources(t *testing.T) {
	registry := NewRegistry([]Source{{ID: "z", ContainerName: "z"}, {ID: "", ContainerName: "bad"}, {ID: "a", ContainerName: "a"}})
	sources := registry.Sources()
	if len(sources) != 2 || sources[0].ID != "a" || sources[1].ID != "z" {
		t.Fatalf("Sources() = %#v, want a,z", sources)
	}
}
