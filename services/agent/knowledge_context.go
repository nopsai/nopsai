package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"nopsai/pkg/models"
)

const knowledgeContextsRuntimeEnv = "NOPSAI_KNOWLEDGE_CONTEXTS"

func loadRuntimeKnowledgeContexts() ([]models.KnowledgeContextSnapshot, error) {
	raw := strings.TrimSpace(os.Getenv(knowledgeContextsRuntimeEnv))
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var snapshots []models.KnowledgeContextSnapshot
	if err := json.Unmarshal(decoded, &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func buildEffectiveKnowledgeContextPrompt(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task, snapshots []models.KnowledgeContextSnapshot) string {
	if pipeline == nil || step == nil || len(snapshots) == 0 {
		return ""
	}
	byRef := map[string]models.KnowledgeContextSnapshot{}
	byPath := map[string]models.KnowledgeContextSnapshot{}
	for _, snapshot := range snapshots {
		kind := normalizeKnowledgeRuntimeValue(snapshot.Kind)
		if kind == "" {
			continue
		}
		if ref := normalizeKnowledgeRuntimeValue(snapshot.Ref); ref != "" {
			byRef[kind+"|"+ref] = snapshot
		}
		if path := normalizeKnowledgeRuntimePath(snapshot.Path); path != "" {
			byPath[kind+"|"+path] = snapshot
		}
	}

	var refs []models.KnowledgeContextRef
	refs = append(refs, pipeline.KnowledgeContext...)
	refs = append(refs, step.GetKnowledgeContext()...)
	if task != nil {
		refs = append(refs, task.KnowledgeContext...)
	}

	seen := map[string]struct{}{}
	var selected []models.KnowledgeContextSnapshot
	for _, ref := range refs {
		kind := normalizeKnowledgeRuntimeValue(ref.Kind)
		if kind == "" {
			continue
		}
		key := ""
		var snapshot models.KnowledgeContextSnapshot
		if refValue := normalizeKnowledgeRuntimeValue(ref.Ref); refValue != "" {
			key = "ref:" + kind + "|" + refValue
			snapshot = byRef[kind+"|"+refValue]
		} else if pathValue := normalizeKnowledgeRuntimePath(ref.Path); pathValue != "" {
			key = "path:" + kind + "|" + pathValue
			snapshot = byPath[kind+"|"+pathValue]
		}
		if key == "" || snapshot.Content == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, snapshot)
	}
	if len(selected) == 0 {
		return ""
	}
	return formatKnowledgeContextPrompt(selected)
}

func formatKnowledgeContextPrompt(snapshots []models.KnowledgeContextSnapshot) string {
	var builder strings.Builder
	hasStrict := false
	for _, snapshot := range snapshots {
		switch normalizeKnowledgeRuntimeValue(snapshot.Kind) {
		case "guardrail", "policy":
			hasStrict = true
		}
	}
	if hasStrict {
		builder.WriteString("If the requested action conflicts with guardrails or policies, do not execute it. Return an explanation instead.\n\n")
	}
	for _, snapshot := range snapshots {
		title := strings.TrimSpace(snapshot.Title)
		if title == "" {
			title = strings.TrimSpace(snapshot.Name)
		}
		if title == "" {
			title = firstNonEmptyKnowledgeLabel(snapshot.Ref, snapshot.Path, snapshot.Kind)
		}
		labelParts := []string{strings.ToUpper(strings.TrimSpace(snapshot.Kind)), title}
		builder.WriteString("### ")
		builder.WriteString(strings.Join(nonEmptyKnowledgeParts(labelParts...), " - "))
		builder.WriteString("\n")
		meta := knowledgeContextMetadata(snapshot)
		if len(meta) > 0 {
			builder.WriteString(strings.Join(meta, " | "))
			builder.WriteString("\n")
		}
		builder.WriteString(strings.TrimSpace(snapshot.Content))
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String())
}

func knowledgeContextMetadata(snapshot models.KnowledgeContextSnapshot) []string {
	var meta []string
	if snapshot.Ref != "" {
		meta = append(meta, "ref: "+snapshot.Ref)
	}
	if snapshot.Path != "" {
		meta = append(meta, "path: "+snapshot.Path)
	}
	if snapshot.Source != "" {
		meta = append(meta, "source: "+snapshot.Source)
	}
	if snapshot.Required {
		meta = append(meta, "required: true")
	}
	sort.Strings(meta)
	return meta
}

func normalizeKnowledgeRuntimeValue(value string) string {
	return strings.Trim(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")), "/")
}

func normalizeKnowledgeRuntimePath(value string) string {
	value = normalizeKnowledgeRuntimeValue(value)
	if value == "." {
		return ""
	}
	return value
}

func firstNonEmptyKnowledgeLabel(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "knowledge"
}

func nonEmptyKnowledgeParts(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("%s", "knowledge"))
	}
	return out
}
