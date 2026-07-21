package models

import "testing"

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
