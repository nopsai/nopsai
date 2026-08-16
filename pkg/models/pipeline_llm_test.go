package models

import "testing"

func TestPipelineRequiresLLMProfilesSkipsScriptOnlyPipeline(t *testing.T) {
	pipeline := &Pipeline{
		Name:           "script-only",
		ContainerImage: "alpine:3.20",
		LLMProfile:     "standard",
		Steps: []PipelineStep{{
			Step: &ScriptStep{
				BaseStep: BaseStep{Name: "runner-smoke"},
				Script:   "echo ok",
			},
		}},
	}

	if PipelineRequiresLLMProfiles(pipeline) {
		t.Fatal("script-only pipeline should not require LLM profiles")
	}
}

func TestPipelineRequiresLLMProfilesForBlockingKnowledgeScriptValidation(t *testing.T) {
	tests := []struct {
		name     string
		pipeline Pipeline
	}{
		{
			name: "pipeline-level guardrail on script step",
			pipeline: Pipeline{
				Name:             "guarded-script",
				ContainerImage:   "alpine:3.20",
				KnowledgeContext: []KnowledgeContextRef{{Kind: "guardrail", Ref: "data-team/runtime-output-safety"}},
				Steps: []PipelineStep{{
					Step: &ScriptStep{
						BaseStep: BaseStep{Name: "hello"},
						Script:   "env",
					},
				}},
			},
		},
		{
			name: "step-level policy on script task",
			pipeline: Pipeline{
				Name:           "guarded-task",
				ContainerImage: "alpine:3.20",
				Steps: []PipelineStep{{
					Step: &TaskStep{
						BaseStep: BaseStep{
							Name:             "deploy",
							KnowledgeContext: []KnowledgeContextRef{{Kind: "policy", Ref: "data-team/deploy-policy"}},
						},
						Tasks: []Task{{Name: "run", Script: "./deploy.sh"}},
					},
				}},
			},
		},
		{
			name: "task-level guardrail on script task",
			pipeline: Pipeline{
				Name:           "task-guarded",
				ContainerImage: "alpine:3.20",
				Steps: []PipelineStep{{
					Step: &TaskStep{
						BaseStep: BaseStep{Name: "inspect"},
						Tasks: []Task{{
							Name:             "env",
							Script:           "env",
							KnowledgeContext: []KnowledgeContextRef{{Kind: "guardrail", Ref: "data-team/runtime-output-safety"}},
						}},
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !PipelineRequiresLLMProfiles(&tt.pipeline) {
				t.Fatal("blocking knowledge direct script should require LLM profiles")
			}
		})
	}
}

func TestPipelineRequiresLLMProfilesIgnoresNonBlockingKnowledgeScript(t *testing.T) {
	pipeline := &Pipeline{
		Name:             "documented-script",
		ContainerImage:   "alpine:3.20",
		KnowledgeContext: []KnowledgeContextRef{{Kind: "guideline", Ref: "data-team/shell-style"}},
		Steps: []PipelineStep{{
			Step: &ScriptStep{
				BaseStep: BaseStep{Name: "hello"},
				Script:   "echo ok",
			},
		}},
	}

	if PipelineRequiresLLMProfiles(pipeline) {
		t.Fatal("non-blocking knowledge on script-only pipeline should not require LLM profiles")
	}
}

func TestPipelineRequiresLLMProfilesDetectsAISurfaces(t *testing.T) {
	tests := []struct {
		name     string
		pipeline Pipeline
	}{
		{
			name: "goal step",
			pipeline: Pipeline{
				Name:           "goal",
				ContainerImage: "alpine:3.20",
				Steps: []PipelineStep{{
					Step: &GoalStep{
						BaseStep: BaseStep{Name: "review"},
						Goal:     "Review the change.",
					},
				}},
			},
		},
		{
			name: "condition",
			pipeline: Pipeline{
				Name:           "condition",
				ContainerImage: "alpine:3.20",
				Steps: []PipelineStep{{
					Step: &ScriptStep{
						BaseStep: BaseStep{Name: "build", Condition: "Run only for risky changes."},
						Script:   "echo ok",
					},
				}},
			},
		},
		{
			name: "final output",
			pipeline: Pipeline{
				Name:           "output",
				ContainerImage: "alpine:3.20",
				Output: PipelineOutput{
					Items: []PipelineOutputItem{{
						Name:   "summary",
						Type:   "markdown",
						Prompt: "Summarize the run.",
					}},
				},
				Steps: []PipelineStep{{
					Step: &ScriptStep{
						BaseStep: BaseStep{Name: "build"},
						Script:   "echo ok",
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !PipelineRequiresLLMProfiles(&tt.pipeline) {
				t.Fatal("pipeline should require LLM profiles")
			}
		})
	}
}

func TestPipelineRequiresLLMProfilesHonorsDisabledFlag(t *testing.T) {
	disabled := false
	pipeline := &Pipeline{
		Name:           "disabled-goal",
		ContainerImage: "alpine:3.20",
		LLMEnabled:     &disabled,
		Steps: []PipelineStep{{
			Step: &GoalStep{
				BaseStep: BaseStep{Name: "review"},
				Goal:     "Review the change.",
			},
		}},
	}

	if PipelineRequiresLLMProfiles(pipeline) {
		t.Fatal("llm_enabled=false pipeline should not require LLM profiles")
	}
}

func TestPipelineLLMContentPreloadDefaultsFalse(t *testing.T) {
	if PipelineLLMContentPreload(nil) {
		t.Fatal("nil pipeline content sharing = true, want false")
	}
	if PipelineLLMContentPreload(&Pipeline{}) {
		t.Fatal("omitted llm_content_preload = true, want false")
	}
}

func TestPipelineLLMContentPreloadUsesExplicitValue(t *testing.T) {
	enabled := true
	disabled := false

	if !PipelineLLMContentPreload(&Pipeline{LlmContentPreload: &enabled}) {
		t.Fatal("explicit true llm_content_preload = false, want true")
	}
	if PipelineLLMContentPreload(&Pipeline{LlmContentPreload: &disabled}) {
		t.Fatal("explicit false llm_content_preload = true, want false")
	}
}
