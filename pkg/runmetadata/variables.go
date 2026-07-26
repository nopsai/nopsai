package runmetadata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc/metadata"
)

const VariableOverridesMetadataKey = "nopsai-runtime-variable-overrides"

type VariableOverrides struct {
	Variables          map[string]string `json:"variables,omitempty"`
	SensitiveVariables []string          `json:"sensitive_variables,omitempty"`
}

func AppendOutgoingVariableOverrides(ctx context.Context, overrides VariableOverrides) (context.Context, error) {
	ctx = defaultContext(ctx)
	normalized := NormalizeVariableOverrides(overrides)
	if len(normalized.Variables) == 0 && len(normalized.SensitiveVariables) == 0 {
		return ctx, nil
	}
	payload, err := EncodeVariableOverrides(normalized)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, VariableOverridesMetadataKey, payload), nil
}

func VariableOverridesFromIncomingContext(ctx context.Context) (VariableOverrides, bool, error) {
	md, ok := metadata.FromIncomingContext(defaultContext(ctx))
	if !ok {
		return VariableOverrides{}, false, nil
	}
	values := md.Get(VariableOverridesMetadataKey)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		overrides, err := DecodeVariableOverrides(value)
		return overrides, true, err
	}
	return VariableOverrides{}, false, nil
}

func EncodeVariableOverrides(overrides VariableOverrides) (string, error) {
	normalized := NormalizeVariableOverrides(overrides)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal variable overrides metadata: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func DecodeVariableOverrides(encoded string) (VariableOverrides, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return VariableOverrides{}, fmt.Errorf("decode variable overrides metadata: %w", err)
	}
	var overrides VariableOverrides
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return VariableOverrides{}, fmt.Errorf("parse variable overrides metadata: %w", err)
	}
	return NormalizeVariableOverrides(overrides), nil
}

func NormalizeVariableOverrides(overrides VariableOverrides) VariableOverrides {
	normalized := VariableOverrides{
		Variables: make(map[string]string, len(overrides.Variables)),
	}
	for key, value := range overrides.Variables {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized.Variables[key] = value
	}

	seenSensitive := make(map[string]bool, len(overrides.SensitiveVariables))
	for _, key := range overrides.SensitiveVariables {
		key = strings.TrimSpace(key)
		if key == "" || seenSensitive[key] {
			continue
		}
		seenSensitive[key] = true
		normalized.SensitiveVariables = append(normalized.SensitiveVariables, key)
	}
	sort.Strings(normalized.SensitiveVariables)
	if len(normalized.Variables) == 0 {
		normalized.Variables = nil
	}
	return normalized
}

func defaultContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
