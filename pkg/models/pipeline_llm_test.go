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

func TestPipelineLLMContentSharingDefaultsFalse(t *testing.T) {
	if PipelineLLMContentSharing(nil) {
		t.Fatal("nil pipeline content sharing = true, want false")
	}
	if PipelineLLMContentSharing(&Pipeline{}) {
		t.Fatal("omitted llm_content_sharing = true, want false")
	}
}

func TestPipelineLLMContentSharingUsesExplicitValue(t *testing.T) {
	enabled := true
	disabled := false

	if !PipelineLLMContentSharing(&Pipeline{LlmContentSharing: &enabled}) {
		t.Fatal("explicit true llm_content_sharing = false, want true")
	}
	if PipelineLLMContentSharing(&Pipeline{LlmContentSharing: &disabled}) {
		t.Fatal("explicit false llm_content_sharing = true, want false")
	}
}
