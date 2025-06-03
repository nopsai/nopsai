// pipeline/parser.go
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
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Steps       []Step `yaml:"steps"`
}

func LoadPipeline(filePath string) (*Pipeline, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline file %s: %w", filePath, err)
	}
	var p Pipeline
	err = yaml.Unmarshal(data, &p)
	if err != nil {
		var simplePrompts []string
		errSimple := yaml.Unmarshal(data, &simplePrompts)
		if errSimple == nil && len(simplePrompts) > 0 {
			return nil, fmt.Errorf("failed to parse structured pipeline YAML from %s (error: %w). Simple list format no longer supported. Please update YAML.", filePath, err)
		}
		return nil, fmt.Errorf("failed to parse pipeline YAML from %s: %w", filePath, err)
	}
	if p.Name == "" {
		p.Name = filePath
	}
	if len(p.Steps) == 0 {
		return nil, fmt.Errorf("no steps found in pipeline YAML: %s", filePath)
	}
	stepNames := make(map[string]bool)
	for i, step := range p.Steps {
		if step.Name == "" {
			return nil, fmt.Errorf("step at index %d in pipeline '%s' is missing a 'name'. Step names are required.", i, p.Name)
		}
		if stepNames[step.Name] {
			return nil, fmt.Errorf("duplicate step name '%s' found in pipeline '%s'. Step names must be unique.", step.Name, p.Name)
		}
		stepNames[step.Name] = true
	}
	return &p, nil
}
