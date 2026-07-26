package models

import "testing"

func TestParseRuntimeOutputRefAcceptsTaskQualifiedReference(t *testing.T) {
	ref, found, err := ParseRuntimeOutputRef("$steps.prepare.generate-tag.outputs.image_tag")
	if err != nil {
		t.Fatalf("ParseRuntimeOutputRef() error = %v", err)
	}
	if !found {
		t.Fatal("ParseRuntimeOutputRef() found = false, want true")
	}
	if ref.StepName != "prepare" || ref.TaskName != "generate-tag" || ref.OutputName != "image_tag" {
		t.Fatalf("ref = %#v, want prepare.generate-tag.outputs.image_tag", ref)
	}
	if ref.Key() != "prepare/generate-tag/image_tag" {
		t.Fatalf("ref key = %q", ref.Key())
	}
}

func TestParseRuntimeOutputRefAllowsDotsInStepName(t *testing.T) {
	ref, found, err := ParseRuntimeOutputRef("$steps.platform.prepare.generate.outputs.IMAGE_TAG")
	if err != nil {
		t.Fatalf("ParseRuntimeOutputRef() error = %v", err)
	}
	if !found {
		t.Fatal("ParseRuntimeOutputRef() found = false, want true")
	}
	if ref.StepName != "platform.prepare" || ref.TaskName != "generate" || ref.OutputName != "IMAGE_TAG" {
		t.Fatalf("ref = %#v", ref)
	}
}

func TestParseRuntimeOutputRefRejectsInvalidOutputName(t *testing.T) {
	if _, found, err := ParseRuntimeOutputRef("$steps.prepare.generate.outputs.image-tag"); !found || err == nil {
		t.Fatalf("ParseRuntimeOutputRef() found=%t err=%v, want invalid output error", found, err)
	}
}

func TestIsValidTaskOutputNameIsCaseSensitiveAndIdentifierShaped(t *testing.T) {
	for _, name := range []string{"image_tag", "IMAGE_TAG", "releaseVersion", "_token"} {
		if !IsValidTaskOutputName(name) {
			t.Fatalf("IsValidTaskOutputName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "image-tag", "1tag", "tag.name"} {
		if IsValidTaskOutputName(name) {
			t.Fatalf("IsValidTaskOutputName(%q) = true, want false", name)
		}
	}
}
