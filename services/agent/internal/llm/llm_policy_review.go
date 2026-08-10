package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

const (
	PolicyReviewPhaseBefore = "before"
	PolicyReviewPhaseAfter  = "after"
)

type PolicyReviewRequest struct {
	Phase            string
	Goal             string
	History          string
	Variables        map[string]string
	KnowledgeContext string
	ProposedAction   *proto.Action
}

func (c *LLMClient) ReviewPolicy(ctx context.Context, req PolicyReviewRequest) (*models.PolicyReview, error) {
	return c.ReviewPolicyWithAgentProfile(ctx, req, defaultAgentPromptProfile())
}

func (c *LLMClient) ReviewPolicyWithAgentProfile(ctx context.Context, req PolicyReviewRequest, agentProfile AgentPromptProfile) (*models.PolicyReview, error) {
	prompt := c.buildPolicyReviewPrompt(req, agentProfile)
	response, err := c.CompleteRequest(ctx, CompletionRequest{
		Feature: "policy_review_" + normalizePolicyReviewPhase(req.Phase),
		Prompt:  prompt,
		Session: NewGoalSession(""),
	})
	if err != nil {
		return nil, err
	}
	return decodePolicyReviewResponse(response.Text)
}

func (c *LLMClient) buildPolicyReviewPrompt(req PolicyReviewRequest, agentProfile AgentPromptProfile) string {
	phase := normalizePolicyReviewPhase(req.Phase)
	history := strings.TrimSpace(req.History)
	if history == "" {
		history = "No history yet."
	}
	proposedAction := "No proposed action. Review the current goal or direct script before planning or execution."
	if req.ProposedAction != nil {
		proposedAction = formatProtoActionForPolicyReview(req.ProposedAction)
	}

	promptTemplate := `%s

Your task is to perform the %s AI policy check for NopsAI.
You must only respond with a single JSON object with this shape:
{"policy_review":{"decision":"allow","confidence":"high","reason":"short policy reason","refs":[]}}

policy_review.decision must be one of "allow", "block", "conflict", or "uncertain".
NopsAI owns governance_level and will decide whether your policy_review.decision can proceed.
Use effective policies and guardrails as authoritative. Other effective knowledge can inform interpretation, but execution history only describes what already happened.
If policies or guardrails oppose each other for this same decision, return "conflict". If the applicable policy result is unclear, return "uncertain".

---
%s
---
%s
---
**Execution History (Previous Steps):**
%s
---
**Current Goal Or Script:**
"%s"
---
**Proposed Structured Action:**
%s
---
Return only the policy_review JSON object.`

	return fmt.Sprintf(
		promptTemplate,
		formatAgentPromptProfile(agentProfile),
		phase,
		buildVariablesSection(req.Variables),
		buildKnowledgeContextSection(req.KnowledgeContext),
		history,
		strings.TrimSpace(req.Goal),
		proposedAction,
	)
}

func decodePolicyReviewResponse(raw string) (*models.PolicyReview, error) {
	reviewJSON := cleanModelTextResponse(raw)
	review, err := decodePolicyReviewJSON(reviewJSON)
	if err == nil {
		return review, nil
	}
	strictErr := err
	for _, candidate := range extractActionJSONCandidates(raw) {
		review, err := decodePolicyReviewJSON(cleanModelTextResponse(candidate))
		if err == nil {
			return review, nil
		}
	}
	return nil, fmt.Errorf(
		"failed to unmarshal policy review response: %w. response_sha256=%s response_bytes=%d",
		strictErr,
		promptSHA256(reviewJSON),
		len([]byte(reviewJSON)),
	)
}

func decodePolicyReviewJSON(reviewJSON string) (*models.PolicyReview, error) {
	var wrapper struct {
		PolicyReview models.PolicyReview `json:"policy_review"`
	}
	if err := json.Unmarshal([]byte(reviewJSON), &wrapper); err != nil {
		return nil, err
	}
	review := wrapper.PolicyReview
	review.Decision = models.NormalizePolicyDecision(review.Decision)
	review.Confidence = strings.ToLower(strings.TrimSpace(review.Confidence))
	review.Reason = strings.TrimSpace(review.Reason)
	review.Refs = normalizedPolicyReviewRefs(review.Refs)
	if !models.SupportedPolicyDecision(review.Decision) {
		return nil, fmt.Errorf("policy_review.decision must be one of %q, %q, %q, or %q",
			models.PolicyDecisionAllow,
			models.PolicyDecisionBlock,
			models.PolicyDecisionConflict,
			models.PolicyDecisionUncertain,
		)
	}
	return &review, nil
}

func normalizedPolicyReviewRefs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	refs := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Trim(strings.TrimSpace(value), "/")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		refs = append(refs, value)
	}
	sort.Strings(refs)
	return refs
}

func normalizePolicyReviewPhase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PolicyReviewPhaseAfter:
		return PolicyReviewPhaseAfter
	default:
		return PolicyReviewPhaseBefore
	}
}

func formatProtoActionForPolicyReview(action *proto.Action) string {
	if action == nil {
		return "null"
	}
	payload := map[string]any{"type": strings.TrimSpace(action.Type)}
	switch action.Type {
	case models.ActionTypeExecuteCommand:
		if command := action.GetCommandAction(); command != nil {
			payload["command_action"] = map[string]any{"command": command.Command}
		}
	case models.ActionTypeReplaceFile:
		if file := action.GetFileAction(); file != nil {
			payload["file_action"] = map[string]any{"path": file.Path, "content": file.Content}
		}
	case models.ActionTypeReturnAnswer:
		if answer := action.GetAnswerAction(); answer != nil {
			payload["answer_action"] = map[string]any{"answer": answer.Answer}
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

func governanceLevelFromPrompt(knowledgeContext string) string {
	if value := firstMetadataLineValue(knowledgeContext, "governance_level"); value != "" {
		return models.NormalizeGovernanceLevel(value)
	}
	if value := firstMetadataLineValue(knowledgeContext, "policy_merge_mode"); value != "" {
		return models.NormalizeGovernanceLevel(value)
	}
	return ""
}

func policyReviewRequired(knowledgeContext string) bool {
	return governanceLevelFromPrompt(knowledgeContext) != ""
}

func interpretPromptPolicyReview(knowledgeContext string, review *models.PolicyReview) models.GovernanceInterpretation {
	level := governanceLevelFromPrompt(knowledgeContext)
	if level == "" {
		level = models.GovernanceLevelStrict
	}
	return models.InterpretPolicyReview(level, review, false)
}

func governancePolicyError(phase, knowledgeContext string, review *models.PolicyReview) error {
	interpretation := interpretPromptPolicyReview(knowledgeContext, review)
	reason := strings.TrimSpace(interpretation.Reason)
	if reason == "" {
		reason = "policy review did not allow the action"
	}
	level := governanceLevelFromPrompt(knowledgeContext)
	if level == "" {
		level = models.GovernanceLevelStrict
	}
	return newNonRetryableGoalResolutionError(
		"governance_level %s blocked %s policy decision %q: %s",
		models.NormalizeGovernanceLevel(level),
		strings.TrimSpace(phase),
		interpretation.Decision,
		reason,
	)
}
