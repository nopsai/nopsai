package models

import (
	"fmt"
	"strings"
)

// ScopedRuntimeRef describes a variable or secret reference in pipeline YAML.
// Bare names resolve in the current run scope. Prefixing with "scope:" resolves
// from that explicit scope while still injecting the bare name at runtime.
type ScopedRuntimeRef struct {
	Raw           string
	Scope         string
	Name          string
	ExplicitScope bool
}

func ParseScopedRuntimeRef(raw, currentScope string) (ScopedRuntimeRef, error) {
	ref := ScopedRuntimeRef{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ref, fmt.Errorf("runtime reference is empty")
	}

	if scopePart, namePart, ok := strings.Cut(trimmed, ":"); ok {
		scopePart = strings.Trim(strings.TrimSpace(scopePart), "/")
		namePart = strings.TrimSpace(namePart)
		if scopePart == "" {
			return ref, fmt.Errorf("scope is empty in runtime reference %q", trimmed)
		}
		if namePart == "" {
			return ref, fmt.Errorf("name is empty in runtime reference %q", trimmed)
		}
		if strings.Contains(namePart, ":") {
			return ref, fmt.Errorf("name contains ':' in runtime reference %q", trimmed)
		}
		return ScopedRuntimeRef{
			Raw:           trimmed,
			Scope:         normalizeRuntimeRefScope(scopePart),
			Name:          namePart,
			ExplicitScope: true,
		}, nil
	}

	return ScopedRuntimeRef{
		Raw:   trimmed,
		Scope: normalizeRuntimeRefScope(currentScope),
		Name:  trimmed,
	}, nil
}

func (r ScopedRuntimeRef) Key() string {
	name := strings.TrimSpace(r.Name)
	if !r.ExplicitScope {
		return name
	}
	scope := strings.Trim(strings.TrimSpace(r.Scope), "/")
	if scope == "" {
		scope = "default"
	}
	return scope + ":" + name
}

func (r ScopedRuntimeRef) LookupKey() string {
	return strings.Trim(strings.TrimSpace(r.Scope), "/") + "\x00" + strings.TrimSpace(r.Name)
}

func (r ScopedRuntimeRef) DisplayScope() string {
	scope := strings.Trim(strings.TrimSpace(r.Scope), "/")
	if scope == "" {
		return "default"
	}
	return scope
}

func normalizeRuntimeRefScope(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if strings.EqualFold(scope, "default") {
		return ""
	}
	return scope
}
