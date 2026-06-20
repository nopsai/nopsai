package nopsai

import (
	"strings"

	"gopkg.in/yaml.v3"
)

func hostedMCPCompleteGeneratedDockerPipelineYAML(raw string) string {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return raw
	}
	root := hostedMCPYAMLDocumentMapping(&doc)
	if root == nil {
		return raw
	}
	steps := hostedMCPYAMLMappingValue(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return raw
	}

	stepIndexes := map[string]int{}
	stepNames := map[string]string{}
	for idx, step := range steps.Content {
		if step == nil || step.Kind != yaml.MappingNode {
			continue
		}
		name := hostedMCPYAMLScalarString(hostedMCPYAMLMappingValue(step, "name"))
		kind := hostedMCPDockerPipelineStepKind(name)
		if kind == "" {
			continue
		}
		if _, exists := stepIndexes[kind]; !exists {
			stepIndexes[kind] = idx
			stepNames[kind] = name
		}
	}
	if _, ok := stepIndexes["clone"]; !ok {
		return raw
	}
	if _, ok := stepIndexes["build"]; !ok {
		return raw
	}
	if _, ok := stepIndexes["push"]; !ok {
		return raw
	}

	changed := false
	changed = hostedMCPYAMLSetScalarIfMissing(root, "container_image", "docker:27-cli") || changed
	changed = hostedMCPYAMLSetScalarIfMissing(root, "working_directory", "/workspace") || changed
	changed = hostedMCPYAMLAppendStringSequenceMissing(root, "variables", []string{"REPOSITORY_URL", "IMAGE_NAME"}) || changed

	cloneStep := steps.Content[stepIndexes["clone"]]
	buildStep := steps.Content[stepIndexes["build"]]
	pushStep := steps.Content[stepIndexes["push"]]
	changed = hostedMCPYAMLSetScriptIfMissing(cloneStep, `#!/bin/sh
set -eu
: "${REPOSITORY_URL:?REPOSITORY_URL is required}"
if ! command -v git >/dev/null 2>&1; then
  apk add --no-cache git
fi
git clone "$REPOSITORY_URL" .
`) || changed
	changed = hostedMCPYAMLSetScriptIfMissing(buildStep, `#!/bin/sh
set -eu
: "${IMAGE_NAME:?IMAGE_NAME is required}"
docker build -t "${IMAGE_NAME}:${IMAGE_TAG:-latest}" .
`) || changed
	changed = hostedMCPYAMLSetScriptIfMissing(pushStep, `#!/bin/sh
set -eu
: "${IMAGE_NAME:?IMAGE_NAME is required}"
docker push "${IMAGE_NAME}:${IMAGE_TAG:-latest}"
`) || changed
	changed = hostedMCPYAMLSetDependsOnIfMissing(buildStep, []string{stepNames["clone"]}) || changed
	changed = hostedMCPYAMLSetDependsOnIfMissing(pushStep, []string{stepNames["build"]}) || changed
	if !changed {
		return raw
	}
	encoded, err := yaml.Marshal(root)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func hostedMCPDockerPipelineStepKind(name string) string {
	normalized := strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(strings.ToLower(strings.TrimSpace(name)))
	switch {
	case strings.Contains(normalized, "clone") && (strings.Contains(normalized, "repo") || strings.Contains(normalized, "git")):
		return "clone"
	case strings.Contains(normalized, "checkout") && strings.Contains(normalized, "repo"):
		return "clone"
	case strings.Contains(normalized, "build") && strings.Contains(normalized, "image") && strings.Contains(normalized, "docker"):
		return "build"
	case strings.Contains(normalized, "push") && (strings.Contains(normalized, "image") || strings.Contains(normalized, "registry") || strings.Contains(normalized, "docker")):
		return "push"
	case strings.Contains(normalized, "publish") && (strings.Contains(normalized, "image") || strings.Contains(normalized, "registry") || strings.Contains(normalized, "docker")):
		return "push"
	default:
		return ""
	}
}

func hostedMCPYAMLDocumentMapping(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) != 1 {
			return nil
		}
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil
	}
	return doc
}

func hostedMCPYAMLMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		if mapping.Content[idx] != nil && mapping.Content[idx].Value == key {
			return mapping.Content[idx+1]
		}
	}
	return nil
}

func hostedMCPYAMLScalarString(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func hostedMCPYAMLSetScalarIfMissing(mapping *yaml.Node, key, value string) bool {
	existing := hostedMCPYAMLMappingValue(mapping, key)
	if existing != nil && hostedMCPYAMLScalarString(existing) != "" {
		return false
	}
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if existing != nil {
		*existing = *valueNode
		return true
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, valueNode)
	return true
}

func hostedMCPYAMLAppendStringSequenceMissing(mapping *yaml.Node, key string, values []string) bool {
	existing := hostedMCPYAMLMappingValue(mapping, key)
	if existing != nil && existing.Kind != yaml.SequenceNode {
		return false
	}
	if existing == nil {
		existing = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, existing)
	}
	seen := map[string]bool{}
	for _, item := range existing.Content {
		if item != nil && item.Kind == yaml.ScalarNode {
			seen[strings.TrimSpace(item.Value)] = true
		}
	}
	changed := false
	for _, value := range values {
		if seen[value] {
			continue
		}
		existing.Content = append(existing.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
		changed = true
	}
	return changed
}

func hostedMCPYAMLSetScriptIfMissing(step *yaml.Node, script string) bool {
	if step == nil || step.Kind != yaml.MappingNode || hostedMCPYAMLStepHasExecution(step) {
		return false
	}
	scriptNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: script, Style: yaml.LiteralStyle}
	if existing := hostedMCPYAMLMappingValue(step, "script"); existing != nil {
		*existing = *scriptNode
		return true
	}
	step.Content = append(step.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "script"}, scriptNode)
	return true
}

func hostedMCPYAMLStepHasExecution(step *yaml.Node) bool {
	for _, key := range []string{"include", "goal", "script"} {
		if hostedMCPYAMLScalarString(hostedMCPYAMLMappingValue(step, key)) != "" {
			return true
		}
	}
	if tasks := hostedMCPYAMLMappingValue(step, "tasks"); tasks != nil && tasks.Kind == yaml.SequenceNode && len(tasks.Content) > 0 {
		return true
	}
	if approval := hostedMCPYAMLMappingValue(step, "approval"); approval != nil {
		return true
	}
	return false
}

func hostedMCPYAMLSetDependsOnIfMissing(step *yaml.Node, dependencies []string) bool {
	if step == nil || step.Kind != yaml.MappingNode || hostedMCPYAMLMappingValue(step, "depends_on") != nil || len(dependencies) == 0 {
		return false
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, dependency := range dependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: dependency})
	}
	if len(sequence.Content) == 0 {
		return false
	}
	step.Content = append(step.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "depends_on"}, sequence)
	return true
}
