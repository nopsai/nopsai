package nopsai

import (
	"testing"

	"nopsai/config"

	"github.com/rs/zerolog"
)

func TestApplyRuntimeProcessSettingsDefaultsBlankLevelToInfo(t *testing.T) {
	previousLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })
	zerolog.SetGlobalLevel(zerolog.Disabled)

	blank := ""
	applyRuntimeProcessSettings(config.Config{}, systemConfigPayload{LogLevel: &blank})

	if got := zerolog.GlobalLevel(); got != zerolog.InfoLevel {
		t.Fatalf("GlobalLevel() = %s, want %s", got, zerolog.InfoLevel)
	}
}

func TestApplyRuntimeProcessSettingsLeavesOmittedLevelUnchanged(t *testing.T) {
	previousLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	applyRuntimeProcessSettings(config.Config{}, systemConfigPayload{})

	if got := zerolog.GlobalLevel(); got != zerolog.WarnLevel {
		t.Fatalf("GlobalLevel() = %s, want %s", got, zerolog.WarnLevel)
	}
}
