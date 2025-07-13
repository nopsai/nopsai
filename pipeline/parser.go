package pipeline

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Step struct {
	Name          string   `yaml:"name"`
	Prompt        string   `yaml:"prompt"`
	Dependencies  []string `yaml:"dependencies,omitempty"`
	IgnoreFailure bool     `yaml:"ignore_failure,omitempty"`
}

type Pipeline struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description,omitempty"`
	ContainerImage string            `yaml:"container_image"`
	WorkspaceMount string            `yaml:"workspace_mount,omitempty"`
	Environment    map[string]string `yaml:"environment,omitempty"`
	Steps          []Step            `yaml:"steps"`
}

func LoadPipeline(filePath string) (*Pipeline, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline file %s: %w", filePath, err)
	}
	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline YAML from %s: %w", filePath, err)
	}

	if p.Name == "" {
		p.Name = filePath
	}
	if p.ContainerImage == "" {
		return nil, fmt.Errorf("pipeline YAML '%s' must specify a 'container_image'", filePath)
	}
	if len(p.Steps) == 0 {
		return nil, fmt.Errorf("no steps found in pipeline YAML: %s", filePath)
	}

	stepNames := make(map[string]bool)
	for i, step := range p.Steps {
		if step.Name == "" {
			return nil, fmt.Errorf("step at index %d in pipeline '%s' is missing a 'name'", i, p.Name)
		}
		if stepNames[step.Name] {
			return nil, fmt.Errorf("duplicate step name '%s' found in pipeline '%s'", step.Name, p.Name)
		}
		stepNames[step.Name] = true
	}
	return &p, nil
}
