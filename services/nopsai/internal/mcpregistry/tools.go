package mcpregistry

import (
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"
)

func CompareToolSchemas(ref models.MCPProfileServerRef, cachedTools, liveTools []models.MCPTool) []string {
	if ProfileRefSelectsAllTools(ref) {
		if len(liveTools) == 0 {
			return []string{fmt.Sprintf("Server %s did not return any live tools", ref.ServerName)}
		}
		return nil
	}
	cached := map[string]models.MCPTool{}
	for _, tool := range cachedTools {
		cached[tool.Name] = tool
	}
	live := map[string]models.MCPTool{}
	for _, tool := range liveTools {
		live[tool.Name] = tool
	}
	var warnings []string
	for _, toolName := range ref.Tools {
		liveTool, ok := live[toolName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Tool %s no longer exists on server %s", toolName, ref.ServerName))
			continue
		}
		if cachedTool, ok := cached[toolName]; ok && cachedTool.SchemaHash != "" && liveTool.SchemaHash != cachedTool.SchemaHash {
			warnings = append(warnings, fmt.Sprintf("Tool %s on server %s has a changed schema", toolName, ref.ServerName))
		}
	}
	return warnings
}

func SelectTools(serverName string, discovered []models.MCPTool, names []string) []models.MCPTool {
	byName := map[string]models.MCPTool{}
	for _, tool := range discovered {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			continue
		}
		if strings.TrimSpace(tool.ServerName) == "" {
			tool.ServerName = serverName
		}
		if strings.TrimSpace(tool.InputSchema) == "" {
			tool.InputSchema = "{}"
		}
		byName[tool.Name] = tool
	}
	if toolNamesSelectAll(names) {
		orderedNames := make([]string, 0, len(byName))
		for name := range byName {
			orderedNames = append(orderedNames, name)
		}
		sort.Strings(orderedNames)
		selected := make([]models.MCPTool, 0, len(orderedNames))
		for _, name := range orderedNames {
			selected = append(selected, byName[name])
		}
		return selected
	}
	selected := map[string]models.MCPTool{}
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" || isAllToolsSelector(name) {
			continue
		}
		if tool, ok := byName[name]; ok {
			selected[name] = tool
			continue
		}
		selected[name] = models.MCPTool{
			ServerName:  serverName,
			Name:        name,
			InputSchema: "{}",
		}
	}
	orderedNames := make([]string, 0, len(selected))
	for name := range selected {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)
	filtered := make([]models.MCPTool, 0, len(orderedNames))
	for _, name := range orderedNames {
		filtered = append(filtered, selected[name])
	}
	return filtered
}

func MergeTools(existing, next []models.MCPTool) []models.MCPTool {
	byName := map[string]models.MCPTool{}
	for _, tool := range existing {
		byName[tool.Name] = tool
	}
	for _, tool := range next {
		byName[tool.Name] = tool
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	merged := make([]models.MCPTool, 0, len(names))
	for _, name := range names {
		merged = append(merged, byName[name])
	}
	return merged
}

func toolNamesSelectAll(names []string) bool {
	for _, name := range names {
		if isAllToolsSelector(name) {
			return true
		}
	}
	return false
}
