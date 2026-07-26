package nopsai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func normalizeRunVariableOverrides(raw map[string]string) (map[string]string, []string) {
	overrides := make(map[string]string)
	var invalid []string
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if !envKeyPattern.MatchString(trimmedKey) {
			invalid = append(invalid, trimmedKey)
			continue
		}
		overrides[trimmedKey] = value
	}
	sort.Strings(invalid)
	return overrides, invalid
}

func normalizeSensitiveRunVariableOverrides(raw []string) (map[string]bool, []string) {
	sensitive := make(map[string]bool)
	var invalid []string
	for _, key := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if !envKeyPattern.MatchString(trimmedKey) {
			invalid = append(invalid, trimmedKey)
			continue
		}
		sensitive[trimmedKey] = true
	}
	sort.Strings(invalid)
	return sensitive, invalid
}

func publicRunVariableOverrides(overrides map[string]string, sensitiveNames map[string]bool) map[string]string {
	public := make(map[string]string)
	for key, value := range overrides {
		if sensitiveNames[key] {
			continue
		}
		public[key] = value
	}
	return public
}

func sensitiveRunVariableOverrides(overrides map[string]string, sensitiveNames map[string]bool) map[string]string {
	sensitive := make(map[string]string)
	for key, value := range overrides {
		if sensitiveNames[key] {
			sensitive[key] = value
		}
	}
	return sensitive
}

func (a *App) encryptRunVariableOverrides(overrides map[string]string) (map[string]string, error) {
	encrypted := make(map[string]string)
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		encryptedValue, err := a.encrypt(value)
		if err != nil {
			return nil, fmt.Errorf("encrypt sensitive variable override %s: %w", key, err)
		}
		encrypted[key] = encryptedValue
	}
	return encrypted, nil
}

func (a *App) decryptRunVariableOverridesJSON(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	encrypted := map[string]string{}
	if err := json.Unmarshal(raw, &encrypted); err != nil {
		return nil, err
	}
	decrypted := make(map[string]string, len(encrypted))
	for key, value := range encrypted {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		plain, err := a.decrypt(value)
		if err != nil {
			return nil, fmt.Errorf("decrypt sensitive variable override %s: %w", key, err)
		}
		decrypted[key] = plain
	}
	return decrypted, nil
}
