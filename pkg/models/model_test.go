package models

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPipelineRejectsLegacyEnvironmentYAML(t *testing.T) {
	data := []byte(`
name: sample
description: desc
container_image: ubuntu:latest
environment:
  - FOO
steps:
  - name: first
    goal: do something
`)
	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err == nil {
		t.Fatalf("expected error for legacy environment key, got none")
	}
}

func TestPipelineRejectsLegacyEnvironmentJSON(t *testing.T) {
	payload := []byte(`{
		"name": "sample",
		"description": "desc",
		"container_image": "ubuntu:latest",
		"environment": ["FOO"],
		"steps": [
			{"name": "first", "goal": "do something"}
		]
	}`)
	var p Pipeline
	if err := json.Unmarshal(payload, &p); err == nil {
		t.Fatalf("expected error for legacy environment key, got none")
	}
}
