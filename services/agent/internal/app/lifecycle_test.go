package app

import (
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestStartPipelineTimeoutTriggersOnce(t *testing.T) {
	logger := zerolog.New(io.Discard)
	triggered := make(chan string, 1)
	controller := StartPipelineTimeout("10ms", &logger, func(reason string) {
		triggered <- reason
	})
	defer controller.Stop()

	select {
	case reason := <-triggered:
		if reason != "timeout" {
			t.Fatalf("reason = %q, want timeout", reason)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timeout controller did not trigger")
	}

	if !controller.Triggered() {
		t.Fatal("Triggered() = false, want true")
	}
	if !controller.Stopping() {
		t.Fatal("Stopping() = false, want true")
	}

	select {
	case reason := <-triggered:
		t.Fatalf("timeout triggered more than once with reason %q", reason)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestStartPipelineTimeoutIgnoresInvalidDuration(t *testing.T) {
	logger := zerolog.New(io.Discard)
	controller := StartPipelineTimeout("not-a-duration", &logger, func(reason string) {
		t.Fatalf("unexpected timeout callback: %s", reason)
	})

	if controller.Context() != nil {
		t.Fatal("Context() is configured for invalid duration")
	}
	if controller.Triggered() {
		t.Fatal("Triggered() = true, want false")
	}
}
