package main

import (
	"fmt"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

const defaultRuntimeScope = "default"

func runtimeScopeForStorage(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if scope == "" || strings.EqualFold(scope, defaultRuntimeScope) {
		return defaultRuntimeScope
	}
	return scope
}

func runtimeScopeForResource(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if strings.EqualFold(scope, defaultRuntimeScope) {
		return ""
	}
	return scope
}

func runtimeScopeForDisplay(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if scope == "" || strings.EqualFold(scope, defaultRuntimeScope) {
		return defaultRuntimeScope
	}
	return scope
}

func runtimeScopeIsDefault(scope string) bool {
	return runtimeScopeForStorage(scope) == defaultRuntimeScope
}

func runtimeScopeEqualsSQL(column string, argPosition int, scope string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		column = "scope"
	}
	placeholder := fmt.Sprintf("$%d", argPosition)
	if runtimeScopeIsDefault(scope) {
		return fmt.Sprintf("(%s = %s OR %s IS NULL OR %s = '')", column, placeholder, column, column)
	}
	return fmt.Sprintf("%s = %s", column, placeholder)
}

func runtimeNamedResourceIDForResource(resourceID string) string {
	resourceID = strings.TrimSpace(resourceID)
	repoName, scope, name := model.ParseNamedResourceID(resourceID)
	if strings.TrimSpace(name) == "" {
		return resourceID
	}
	return model.BuildNamedResourceID(repoName, runtimeScopeForResource(scope), name)
}
