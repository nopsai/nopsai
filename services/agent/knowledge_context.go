package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

const knowledgeContextsRuntimeEnv = "NOPSAI_KNOWLEDGE_CONTEXTS"

const governanceContractInstruction = "NopsAI owns governance_level and decides whether your policy_review.decision can proceed. " +
	"History is not policy truth; it only describes what already happened. Current effective knowledge is authoritative. " +
	"Before planning or execution, review the current goal or direct script against effective policies and guardrails. " +
	"During planning, include policy_review next to the action you propose. " +
	"After choosing an action, inspect the exact final structured action before execution. " +
	"If the action is EXECUTE_COMMAND, inspect command_action.command, including shell text, scripts, arguments, and stdout/stderr-producing operations. " +
	"Policies and guardrails apply to goals, generated commands, file writes, MCP/tool actions, and their arguments. " +
	"Use policy_review.decision values allow, block, conflict, or uncertain. " +
	"Opposite policies conflict only when both are effective for the same decision."

type knowledgeContextScope string

const (
	knowledgeContextScopePipeline knowledgeContextScope = "pipeline"
	knowledgeContextScopeStep     knowledgeContextScope = "step"
	knowledgeContextScopeTask     knowledgeContextScope = "task"
)

type scopedKnowledgeContextRef struct {
	Ref        models.KnowledgeContextRef
	Scope      knowledgeContextScope
	ScopeOrder int
	Sequence   int
}

type scopedKnowledgeContextSnapshot struct {
	Snapshot   models.KnowledgeContextSnapshot
	Scope      knowledgeContextScope
	ScopeOrder int
	Sequence   int
	Key        string
}

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
	selected := selectScopedEffectiveKnowledgeContexts(pipeline, step, task, snapshots)
	if len(selected) == 0 {
		return ""
	}
	return formatScopedKnowledgeContextPrompt(selected, models.EffectiveGovernanceLevel(pipeline, step, task))
}

func selectEffectiveKnowledgeContexts(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task, snapshots []models.KnowledgeContextSnapshot) []models.KnowledgeContextSnapshot {
	scoped := selectScopedEffectiveKnowledgeContexts(pipeline, step, task, snapshots)
	selected := make([]models.KnowledgeContextSnapshot, 0, len(scoped))
	for _, item := range scoped {
		selected = append(selected, item.Snapshot)
	}
	return selected
}

func selectScopedEffectiveKnowledgeContexts(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task, snapshots []models.KnowledgeContextSnapshot) []scopedKnowledgeContextSnapshot {
	if pipeline == nil || step == nil || len(snapshots) == 0 {
		return nil
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

	refs := scopedKnowledgeContextRefs(pipeline, step, task)

	seen := map[string]int{}
	var selected []scopedKnowledgeContextSnapshot
	for _, ref := range refs {
		kind := normalizeKnowledgeRuntimeValue(ref.Ref.Kind)
		if kind == "" {
			continue
		}
		key := ""
		var snapshot models.KnowledgeContextSnapshot
		if refValue := normalizeKnowledgeRuntimeValue(ref.Ref.Ref); refValue != "" {
			key = "ref:" + kind + "|" + refValue
			snapshot = byRef[kind+"|"+refValue]
		} else if pathValue := normalizeKnowledgeRuntimePath(ref.Ref.Path); pathValue != "" {
			key = "path:" + kind + "|" + pathValue
			snapshot = byPath[kind+"|"+pathValue]
		}
		if key == "" || snapshot.Content == "" {
			continue
		}
		item := scopedKnowledgeContextSnapshot{
			Snapshot:   snapshot,
			Scope:      ref.Scope,
			ScopeOrder: ref.ScopeOrder,
			Sequence:   ref.Sequence,
			Key:        key,
		}
		if existingIdx, ok := seen[key]; ok {
			if item.ScopeOrder >= selected[existingIdx].ScopeOrder {
				selected[existingIdx] = item
			}
			continue
		}
		seen[key] = len(selected)
		selected = append(selected, item)
	}
	sort.SliceStable(selected, func(a, b int) bool {
		if selected[a].ScopeOrder != selected[b].ScopeOrder {
			return selected[a].ScopeOrder < selected[b].ScopeOrder
		}
		if selected[a].Sequence != selected[b].Sequence {
			return selected[a].Sequence < selected[b].Sequence
		}
		return selected[a].Key < selected[b].Key
	})
	return selected
}

func scopedKnowledgeContextRefs(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) []scopedKnowledgeContextRef {
	if pipeline == nil || step == nil {
		return nil
	}
	refs := make([]scopedKnowledgeContextRef, 0, len(pipeline.KnowledgeContext)+len(step.GetKnowledgeContext()))
	sequence := 0
	for _, ref := range pipeline.KnowledgeContext {
		refs = append(refs, scopedKnowledgeContextRef{Ref: ref, Scope: knowledgeContextScopePipeline, ScopeOrder: 0, Sequence: sequence})
		sequence++
	}
	for _, ref := range step.GetKnowledgeContext() {
		refs = append(refs, scopedKnowledgeContextRef{Ref: ref, Scope: knowledgeContextScopeStep, ScopeOrder: 1, Sequence: sequence})
		sequence++
	}
	if task != nil {
		for _, ref := range task.KnowledgeContext {
			refs = append(refs, scopedKnowledgeContextRef{Ref: ref, Scope: knowledgeContextScopeTask, ScopeOrder: 2, Sequence: sequence})
			sequence++
		}
	}
	return refs
}

func formatKnowledgeContextPrompt(snapshots []models.KnowledgeContextSnapshot) string {
	scoped := make([]scopedKnowledgeContextSnapshot, 0, len(snapshots))
	for idx, snapshot := range snapshots {
		scoped = append(scoped, scopedKnowledgeContextSnapshot{
			Snapshot:   snapshot,
			Scope:      knowledgeContextScopePipeline,
			ScopeOrder: 0,
			Sequence:   idx,
			Key:        knowledgeContextSnapshotKey(snapshot),
		})
	}
	return formatScopedKnowledgeContextPrompt(scoped, models.GovernanceLevelStrict)
}

func formatScopedKnowledgeContextPrompt(snapshots []scopedKnowledgeContextSnapshot, governanceLevel string) string {
	var builder strings.Builder
	hasGovernance := false
	rawSnapshots := make([]models.KnowledgeContextSnapshot, 0, len(snapshots))
	var blocking []scopedKnowledgeContextSnapshot
	var other []scopedKnowledgeContextSnapshot
	for _, item := range snapshots {
		rawSnapshots = append(rawSnapshots, item.Snapshot)
		if models.KnowledgeContextKindIsBlocking(item.Snapshot.Kind) {
			hasGovernance = true
			blocking = append(blocking, item)
		} else {
			other = append(other, item)
		}
	}
	governanceLevel = models.NormalizeGovernanceLevel(governanceLevel)
	if hasGovernance {
		builder.WriteString("NopsAI Governance Contract\n")
		builder.WriteString("knowledge_revision: ")
		builder.WriteString(knowledgeContextRevision(rawSnapshots, false))
		builder.WriteString("\npolicy_revision: ")
		builder.WriteString(knowledgeContextRevision(rawSnapshots, true))
		builder.WriteString("\neffective_policy_snapshot_hash: ")
		builder.WriteString(effectivePolicySnapshotHash(snapshots, governanceLevel))
		builder.WriteString("\ngovernance_level: ")
		builder.WriteString(governanceLevel)
		builder.WriteString("\ngovernance_contract_version: ")
		builder.WriteString(models.GovernanceContractVersion)
		builder.WriteString("\n\n")
		builder.WriteString(governanceContractInstruction)
		builder.WriteString("\n\n")
	}
	if len(blocking) > 0 {
		builder.WriteString("Effective Policies And Guardrails\n\n")
		for _, item := range blocking {
			writeKnowledgeContextPromptItem(&builder, item)
		}
	}
	if len(other) > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("Other Effective Knowledge\n\n")
		for _, item := range other {
			writeKnowledgeContextPromptItem(&builder, item)
		}
	}
	return strings.TrimSpace(builder.String())
}

func writeKnowledgeContextPromptItem(builder *strings.Builder, item scopedKnowledgeContextSnapshot) {
	if builder == nil {
		return
	}
	snapshot := item.Snapshot
	title := strings.TrimSpace(snapshot.Name)
	if title == "" {
		title = firstNonEmptyKnowledgeLabel(snapshot.Ref, snapshot.Path, snapshot.Kind)
	}
	labelParts := []string{strings.ToUpper(strings.TrimSpace(snapshot.Kind)), title}
	builder.WriteString("### ")
	builder.WriteString(strings.Join(nonEmptyKnowledgeParts(labelParts...), " - "))
	builder.WriteString("\n")
	meta := knowledgeContextMetadataForScope(snapshot, item.Scope)
	if len(meta) > 0 {
		builder.WriteString(strings.Join(meta, " | "))
		builder.WriteString("\n")
	}
	builder.WriteString(strings.TrimSpace(snapshot.Content))
	builder.WriteString("\n\n")
}

func effectivePolicySnapshotHash(snapshots []scopedKnowledgeContextSnapshot, governanceLevel string) string {
	type policyHashItem struct {
		Scope                 string `json:"scope"`
		Kind                  string `json:"kind"`
		Ref                   string `json:"ref,omitempty"`
		Path                  string `json:"path,omitempty"`
		Source                string `json:"source,omitempty"`
		ConfigSourcePath      string `json:"config_source_path,omitempty"`
		ConfigSourceCommitSHA string `json:"config_source_commit_sha,omitempty"`
		ContentSHA256         string `json:"content_sha256"`
		Required              bool   `json:"required"`
	}
	payload := struct {
		GovernanceLevel           string           `json:"governance_level"`
		GovernanceContractVersion string           `json:"governance_contract_version"`
		Items                     []policyHashItem `json:"items"`
	}{
		GovernanceLevel:           models.NormalizeGovernanceLevel(governanceLevel),
		GovernanceContractVersion: models.GovernanceContractVersion,
	}
	for _, item := range snapshots {
		snapshot := item.Snapshot
		if !models.KnowledgeContextKindIsBlocking(snapshot.Kind) {
			continue
		}
		payload.Items = append(payload.Items, policyHashItem{
			Scope:                 string(item.Scope),
			Kind:                  normalizeKnowledgeRuntimeValue(snapshot.Kind),
			Ref:                   normalizeKnowledgeRuntimeValue(snapshot.Ref),
			Path:                  normalizeKnowledgeRuntimePath(snapshot.Path),
			Source:                normalizeKnowledgeRuntimeValue(snapshot.Source),
			ConfigSourcePath:      normalizeKnowledgeRuntimePath(snapshot.ConfigSourcePath),
			ConfigSourceCommitSHA: normalizeKnowledgeRuntimeValue(snapshot.ConfigSourceCommitSHA),
			ContentSHA256:         fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.Content))),
			Required:              snapshot.Required,
		})
	}
	sort.Slice(payload.Items, func(a, b int) bool {
		left := payload.Items[a]
		right := payload.Items[b]
		return strings.Join([]string{left.Scope, left.Kind, left.Ref, left.Path, left.Source, left.ConfigSourcePath, left.ConfigSourceCommitSHA, left.ContentSHA256}, "\x00") <
			strings.Join([]string{right.Scope, right.Kind, right.Ref, right.Path, right.Source, right.ConfigSourcePath, right.ConfigSourceCommitSHA, right.ContentSHA256}, "\x00")
	})
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func knowledgeContextRevision(snapshots []models.KnowledgeContextSnapshot, blockingOnly bool) string {
	return models.KnowledgeContextRevision(snapshots, blockingOnly)
}

func effectiveBlockingKnowledgeContextKinds(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task, snapshots []models.KnowledgeContextSnapshot) []string {
	selected := selectEffectiveKnowledgeContexts(pipeline, step, task, snapshots)
	if len(selected) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var kinds []string
	for _, snapshot := range selected {
		kind := normalizeKnowledgeRuntimeValue(snapshot.Kind)
		if !isBlockingKnowledgeContextKind(kind) {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func isBlockingKnowledgeContextKind(kind string) bool {
	return models.KnowledgeContextKindIsBlocking(kind)
}

func knowledgeContextViolationFailureReason(action *proto.Action, pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task, snapshots []models.KnowledgeContextSnapshot) (string, []string, bool) {
	blockingKinds := effectiveBlockingKnowledgeContextKinds(pipeline, step, task, snapshots)
	if len(blockingKinds) == 0 || action == nil || action.GetType() != models.ActionTypeReturnAnswer {
		return "", blockingKinds, false
	}
	answerAction := action.GetAnswerAction()
	if answerAction == nil {
		return "", blockingKinds, false
	}
	answer := strings.TrimSpace(answerAction.Answer)
	if answer == "" || !answerLooksLikeKnowledgeContextRejection(answer, blockingKinds) {
		return "", blockingKinds, false
	}
	review := &models.PolicyReview{
		Decision: models.PolicyDecisionBlock,
		Reason:   answer,
		Refs:     blockingKinds,
	}
	if interpretation := models.InterpretPolicyReview(models.EffectiveGovernanceLevel(pipeline, step, task), review, false); interpretation.Allowed {
		return "", blockingKinds, false
	}
	return answer, blockingKinds, true
}

func answerLooksLikeKnowledgeContextRejection(answer string, blockingKinds []string) bool {
	normalized := strings.ToLower(answer)
	hasBlockingReference := false
	for _, kind := range blockingKinds {
		if strings.Contains(normalized, kind) {
			hasBlockingReference = true
			break
		}
	}
	if !hasBlockingReference {
		for _, phrase := range []string{
			"constraint",
			"conflict",
			"violat",
			"blocked",
			"forbidden",
			"prohibit",
			"not allowed",
			"not permitted",
			"safety",
			"security",
		} {
			if strings.Contains(normalized, phrase) {
				hasBlockingReference = true
				break
			}
		}
	}
	if !hasBlockingReference {
		return false
	}
	for _, phrase := range []string{
		"unable to",
		"cannot",
		"can't",
		"could not",
		"won't",
		"must not",
		"do not",
		"not execute",
		"not allowed",
		"not permitted",
		"refus",
		"conflict",
		"violat",
		"blocked",
		"forbidden",
		"prohibit",
		"would reveal",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func knowledgeContextMetadata(snapshot models.KnowledgeContextSnapshot) []string {
	return knowledgeContextMetadataForScope(snapshot, "")
}

func knowledgeContextMetadataForScope(snapshot models.KnowledgeContextSnapshot, scope knowledgeContextScope) []string {
	var meta []string
	if scope != "" {
		meta = append(meta, "scope: "+string(scope))
	}
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

func knowledgeContextSnapshotKey(snapshot models.KnowledgeContextSnapshot) string {
	kind := normalizeKnowledgeRuntimeValue(snapshot.Kind)
	if ref := normalizeKnowledgeRuntimeValue(snapshot.Ref); ref != "" {
		return "ref:" + kind + "|" + ref
	}
	if path := normalizeKnowledgeRuntimePath(snapshot.Path); path != "" {
		return "path:" + kind + "|" + path
	}
	return "content:" + kind + "|" + fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.Content)))
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
