package runmetadata

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestVariableOverridesMetadataRoundTripNormalizesNames(t *testing.T) {
	encoded, err := EncodeVariableOverrides(VariableOverrides{
		Variables: map[string]string{
			" CHANNEL ": "stable",
			"TOKEN":     "secret",
			"":          "ignored",
		},
		SensitiveVariables: []string{" TOKEN ", "TOKEN", ""},
	})
	if err != nil {
		t.Fatalf("EncodeVariableOverrides() error = %v", err)
	}

	got, err := DecodeVariableOverrides(encoded)
	if err != nil {
		t.Fatalf("DecodeVariableOverrides() error = %v", err)
	}
	if got.Variables["CHANNEL"] != "stable" || got.Variables["TOKEN"] != "secret" {
		t.Fatalf("variables = %#v, want normalized CHANNEL and TOKEN", got.Variables)
	}
	if len(got.SensitiveVariables) != 1 || got.SensitiveVariables[0] != "TOKEN" {
		t.Fatalf("sensitive variables = %#v, want TOKEN", got.SensitiveVariables)
	}
}

func TestVariableOverridesFromIncomingContext(t *testing.T) {
	encoded, err := EncodeVariableOverrides(VariableOverrides{
		Variables:          map[string]string{"TOKEN": "secret"},
		SensitiveVariables: []string{"TOKEN"},
	})
	if err != nil {
		t.Fatalf("EncodeVariableOverrides() error = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(VariableOverridesMetadataKey, encoded))

	got, ok, err := VariableOverridesFromIncomingContext(ctx)
	if err != nil {
		t.Fatalf("VariableOverridesFromIncomingContext() error = %v", err)
	}
	if !ok {
		t.Fatal("VariableOverridesFromIncomingContext() ok = false, want true")
	}
	if got.Variables["TOKEN"] != "secret" || len(got.SensitiveVariables) != 1 || got.SensitiveVariables[0] != "TOKEN" {
		t.Fatalf("overrides = %#v", got)
	}
}
