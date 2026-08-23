package nopsai

import "strings"

const (
	assistantPageContextValueLimit = 240
	assistantPageContextMapLimit   = 16
)

var assistantPageContextAllowedQueryKeys = map[string]bool{
	"area":      true,
	"dashboard": true,
	"id":        true,
	"owner":     true,
	"pipeline":  true,
	"profile":   true,
	"provider":  true,
	"q":         true,
	"resource":  true,
	"run":       true,
	"schedule":  true,
	"scope":     true,
	"source":    true,
	"status":    true,
	"tab":       true,
	"team":      true,
	"view":      true,
}

var assistantPageContextAllowedParamKeys = map[string]bool{
	"dashboard_id":  true,
	"pipeline_id":   true,
	"repository":    true,
	"resource_id":   true,
	"resource_type": true,
	"run_id":        true,
	"schedule_id":   true,
	"scope":         true,
	"tab":           true,
	"team_path":     true,
}

type assistantPageContext struct {
	Title        string            `json:"title,omitempty"`
	Path         string            `json:"path,omitempty"`
	Route        string            `json:"route,omitempty"`
	Area         string            `json:"area,omitempty"`
	Tab          string            `json:"tab,omitempty"`
	TeamPath     string            `json:"team_path,omitempty"`
	ResourceType string            `json:"resource_type,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty"`
	ResourceName string            `json:"resource_name,omitempty"`
	Scope        string            `json:"scope,omitempty"`
	PipelineID   string            `json:"pipeline_id,omitempty"`
	RunID        string            `json:"run_id,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	Query        map[string]string `json:"query,omitempty"`
	Params       map[string]string `json:"params,omitempty"`
}

func normalizeAssistantPageContext(context assistantPageContext) assistantPageContext {
	context.Title = assistantPageContextText(context.Title)
	context.Path = assistantPageContextPath(context.Path)
	context.Route = assistantPageContextPath(context.Route)
	context.Area = assistantPageContextToken(context.Area)
	context.Tab = assistantPageContextToken(context.Tab)
	context.TeamPath = assistantPageContextPathValue(context.TeamPath)
	context.ResourceType = assistantPageContextToken(context.ResourceType)
	context.ResourceID = assistantPageContextPathValue(context.ResourceID)
	context.ResourceName = assistantPageContextText(context.ResourceName)
	context.Scope = assistantPageContextPathValue(context.Scope)
	context.PipelineID = assistantPageContextPathValue(context.PipelineID)
	context.RunID = assistantPageContextText(context.RunID)
	context.Repository = assistantPageContextPathValue(context.Repository)
	context.Query = normalizeAssistantPageContextQueryMap(context.Query)
	context.Params = normalizeAssistantPageContextParamMap(context.Params)
	if context.ResourceName == "" {
		context.ResourceName = assistantPageContextResourceName(context.ResourceID)
	}
	if context.Scope == "" {
		context.Scope = context.TeamPath
	}
	return context
}

func assistantPageContextEmpty(context assistantPageContext) bool {
	context = normalizeAssistantPageContext(context)
	return context.Title == "" &&
		context.Path == "" &&
		context.Route == "" &&
		context.Area == "" &&
		context.Tab == "" &&
		context.TeamPath == "" &&
		context.ResourceType == "" &&
		context.ResourceID == "" &&
		context.ResourceName == "" &&
		context.Scope == "" &&
		context.PipelineID == "" &&
		context.RunID == "" &&
		context.Repository == "" &&
		len(context.Query) == 0 &&
		len(context.Params) == 0
}

func assistantPageContextPromptMap(context assistantPageContext) map[string]any {
	context = normalizeAssistantPageContext(context)
	if assistantPageContextEmpty(context) {
		return map[string]any{}
	}
	out := map[string]any{}
	put := func(key, value string) {
		if value != "" {
			out[key] = value
		}
	}
	put("title", context.Title)
	put("path", context.Path)
	put("route", context.Route)
	put("area", context.Area)
	put("tab", context.Tab)
	put("team_path", context.TeamPath)
	put("resource_type", context.ResourceType)
	put("resource_id", context.ResourceID)
	put("resource_name", context.ResourceName)
	put("scope", context.Scope)
	put("pipeline_id", context.PipelineID)
	put("run_id", context.RunID)
	put("repository", context.Repository)
	if len(context.Query) > 0 {
		out["query"] = context.Query
	}
	if len(context.Params) > 0 {
		out["params"] = context.Params
	}
	return out
}

func assistantPageContextSummary(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	parts := []string{
		context.Title,
		context.Area,
		context.Route,
		context.Tab,
		context.ResourceType,
		context.ResourceID,
		context.PipelineID,
		context.RunID,
		context.Scope,
		context.Repository,
	}
	for key, value := range context.Query {
		parts = append(parts, key, value)
	}
	for key, value := range context.Params {
		parts = append(parts, key, value)
	}
	return strings.Join(normalizeAssistantStringList(parts), "\n")
}

func assistantPageContextRunID(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	if context.RunID != "" {
		return context.RunID
	}
	if context.ResourceType == "pipeline_run" {
		return context.ResourceID
	}
	return firstNonEmpty(context.Query["run"], context.Params["run_id"])
}

func assistantPageContextPipelineID(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	if context.PipelineID != "" {
		return context.PipelineID
	}
	if context.ResourceType == "pipeline" {
		if context.ResourceID == "" && context.ResourceName != "" {
			if context.Scope != "" {
				return strings.Trim(context.Scope, "/") + "/" + strings.Trim(context.ResourceName, "/")
			}
			return context.ResourceName
		}
		if context.ResourceID != "" && !strings.Contains(context.ResourceID, "/") && context.Scope != "" {
			return strings.Trim(context.Scope, "/") + "/" + strings.Trim(context.ResourceID, "/")
		}
		return context.ResourceID
	}
	return firstNonEmpty(context.Query["pipeline"], context.Params["pipeline_id"])
}

func assistantPageContextScheduleID(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	if context.ResourceType == "schedule" {
		return context.ResourceID
	}
	return firstNonEmpty(context.Query["schedule"], context.Params["schedule_id"])
}

func assistantPageContextScope(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	return firstNonEmpty(context.Scope, context.TeamPath, context.Query["scope"], context.Query["team"], context.Params["scope"])
}

func assistantPageContextRepository(context assistantPageContext) string {
	context = normalizeAssistantPageContext(context)
	return firstNonEmpty(context.Repository, context.Query["owner"], context.Params["repository"])
}

func normalizeAssistantPageContextMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	for key, value := range values {
		if len(out) >= assistantPageContextMapLimit {
			break
		}
		key = assistantPageContextToken(key)
		value = assistantPageContextText(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func normalizeAssistantPageContextQueryMap(values map[string]string) map[string]string {
	return normalizeAssistantPageContextAllowedMap(values, assistantPageContextAllowedQueryKeys)
}

func normalizeAssistantPageContextParamMap(values map[string]string) map[string]string {
	return normalizeAssistantPageContextAllowedMap(values, assistantPageContextAllowedParamKeys)
}

func normalizeAssistantPageContextAllowedMap(values map[string]string, allowed map[string]bool) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	for key, value := range values {
		key = assistantPageContextToken(key)
		if !allowed[key] {
			continue
		}
		value = assistantPageContextText(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
		if len(out) >= assistantPageContextMapLimit {
			break
		}
	}
	return out
}

func assistantPageContextText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > assistantPageContextValueLimit {
		value = strings.TrimSpace(value[:assistantPageContextValueLimit])
	}
	return value
}

func assistantPageContextPath(value string) string {
	value = assistantPageContextText(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "//", "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func assistantPageContextPathValue(value string) string {
	value = assistantPageContextText(value)
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return strings.Trim(value, "/")
}

func assistantPageContextToken(value string) string {
	value = strings.ToLower(assistantPageContextText(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if char == '_' || char == ' ' || char == '/' {
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(builder.String(), "_")
}

func assistantPageContextResourceName(identifier string) string {
	parts := strings.Split(strings.Trim(identifier, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}
