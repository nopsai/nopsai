package app

import "testing"

func TestStepSessionRegistryTracksAndClearsSessions(t *testing.T) {
	registry := NewStepSessionRegistry()
	registry.Set("deploy", "container-2")
	registry.Set("build", "container-1")
	registry.Set(" ", "ignored")
	registry.Set("test", " ")

	if id, ok := registry.Get("build"); !ok || id != "container-1" {
		t.Fatalf("Get(build) = (%q, %v), want (container-1, true)", id, ok)
	}

	sessions := registry.Clear()
	if len(sessions) != 2 {
		t.Fatalf("Clear() returned %d sessions, want 2", len(sessions))
	}
	if sessions[0] != (StepSession{Name: "build", ID: "container-1"}) {
		t.Fatalf("first session = %#v", sessions[0])
	}
	if sessions[1] != (StepSession{Name: "deploy", ID: "container-2"}) {
		t.Fatalf("second session = %#v", sessions[1])
	}

	if id, ok := registry.Get("build"); ok || id != "" {
		t.Fatalf("Get(build) after Clear() = (%q, %v), want empty false", id, ok)
	}
}

func TestStepSessionRegistryGetOrCreateCreatesOnce(t *testing.T) {
	registry := NewStepSessionRegistry()
	created := 0
	create := func() (string, error) {
		created++
		return "container-1", nil
	}

	id, wasCreated, err := registry.GetOrCreate("build", create)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if id != "container-1" || !wasCreated {
		t.Fatalf("GetOrCreate() = (%q, %v), want (container-1, true)", id, wasCreated)
	}

	id, wasCreated, err = registry.GetOrCreate("build", create)
	if err != nil {
		t.Fatalf("GetOrCreate() second call error = %v", err)
	}
	if id != "container-1" || wasCreated {
		t.Fatalf("GetOrCreate() second call = (%q, %v), want (container-1, false)", id, wasCreated)
	}
	if created != 1 {
		t.Fatalf("create called %d times, want 1", created)
	}
}
