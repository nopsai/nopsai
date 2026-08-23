package nopsai

import (
	"fmt"
	"math"
	"strings"
)

func composeOptimizationReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.find_optimization_opportunities")
	if call.Status == "" {
		return ""
	}
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load optimization evidence.", call)
	}

	efficiency := assistantMonitoringInsightResponse(call.Output, "efficiency")
	pipelinePerformance := assistantMonitoringInsightResponse(call.Output, "pipeline_performance")
	aiUsage := assistantMonitoringInsightResponse(call.Output, "ai_usage")

	lines := []string{"Optimization opportunities:"}
	target := assistantOptimizationTargetLabel(call)
	if target != "" {
		lines = append(lines, "- Target: "+target)
	}

	performanceItem, hasPerformanceItem := assistantOptimizationPipelinePerformanceItem(call, pipelinePerformance)
	if hasPerformanceItem {
		item := performanceItem
		lines = append(lines, assistantOptimizationPerformanceLine(item))
	}
	if runtime := assistantOutputFloat(efficiency, "total_runtime_seconds"); runtime > 0 {
		lines = append(lines, fmt.Sprintf("- Runtime in window: %.0f runner-seconds", runtime))
	}
	if spendLine := assistantOptimizationSpendLine(efficiency, aiUsage); spendLine != "" {
		lines = append(lines, spendLine)
	}

	recommendations := assistantOptimizationRecommendations(call.Output, efficiency, target)
	if len(recommendations) > 0 {
		lines = append(lines, "", "Recommended next steps:")
		for idx, recommendation := range recommendations {
			if idx >= 3 {
				break
			}
			lines = append(lines, "- "+recommendation)
		}
	} else {
		lines = append(lines, "", "Recommended next step:")
		if hasPerformanceItem {
			lines = append(lines, "- "+assistantOptimizationFallbackRecommendation(performanceItem, target))
		} else {
			lines = append(lines, "- No ranked optimization finding was returned for this window; widen the monitoring window or choose a specific pipeline, step, or schedule.")
		}
	}

	if source := assistantOptimizationSourceLine(call); source != "" {
		lines = append(lines, "", source)
	}
	lines = append(lines, "No changes were applied.")
	return strings.Join(assistantCompactLines(lines), "\n")
}

func assistantMonitoringInsightResponse(output map[string]any, key string) map[string]any {
	part, _ := output[key].(map[string]any)
	if len(part) == 0 {
		return nil
	}
	if response, ok := part["response"].(map[string]any); ok {
		return response
	}
	return part
}

func assistantMonitoringInsightRawResponse(output map[string]any, key string) any {
	part, _ := output[key].(map[string]any)
	if len(part) == 0 {
		return nil
	}
	if response, ok := part["response"]; ok {
		return response
	}
	return part
}

func assistantOptimizationTargetLabel(call assistantToolActivity) string {
	for _, key := range []string{"pipeline", "pipeline_id"} {
		if value := assistantOutputString(call.Input, key); value != "" {
			return value
		}
	}
	path := firstNonEmptyString(assistantOutputString(call.Input, "pipelinePath"), assistantOutputString(call.Input, "pipeline_path"))
	name := firstNonEmptyString(assistantOutputString(call.Input, "pipelineName"), assistantOutputString(call.Input, "pipeline_name"))
	if path != "" && name != "" {
		return strings.Trim(path, "/") + "/" + name
	}
	if name != "" {
		return name
	}
	return ""
}

func assistantOptimizationPipelinePerformanceItem(call assistantToolActivity, performance map[string]any) (map[string]any, bool) {
	items := assistantMapSlice(performance["items"])
	if len(items) == 0 {
		return nil, false
	}
	target := strings.ToLower(strings.Trim(assistantOptimizationTargetLabel(call), "/"))
	if target != "" {
		for _, item := range items {
			label := strings.ToLower(strings.Trim(assistantOptimizationPipelineLabel(item), "/"))
			if label == target || strings.HasSuffix(label, "/"+target) || strings.HasSuffix(target, "/"+label) {
				return item, true
			}
		}
	}
	return items[0], true
}

func assistantOptimizationPipelineLabel(item map[string]any) string {
	if label := firstNonEmptyString(assistantOutputString(item, "label"), assistantOutputString(item, "key")); label != "" {
		return label
	}
	path := assistantOutputString(item, "pipeline_path")
	name := assistantOutputString(item, "pipeline_name")
	if path != "" && name != "" {
		return strings.Trim(path, "/") + "/" + name
	}
	return firstNonEmptyString(name, path)
}

func assistantOptimizationPerformanceLine(item map[string]any) string {
	label := assistantOptimizationPipelineLabel(item)
	if label == "" {
		label = "selected pipeline"
	}
	parts := []string{}
	if total := assistantOutputFloat(item, "total_runs"); total > 0 {
		parts = append(parts, fmt.Sprintf("%.0f runs", total))
	}
	if failed := assistantOutputFloat(item, "failed_runs"); failed > 0 {
		parts = append(parts, fmt.Sprintf("%.0f failed", failed))
	}
	if failureRate := assistantOutputFloat(item, "failure_rate"); failureRate > 0 {
		parts = append(parts, assistantFormatOptimizationPercent(failureRate)+" failure rate")
	}
	if average := assistantOutputFloat(item, "average_duration_seconds"); average > 0 {
		parts = append(parts, fmt.Sprintf("%.0fs average", average))
	}
	if p99 := assistantOutputFloat(item, "p99_duration_seconds"); p99 > 0 {
		parts = append(parts, fmt.Sprintf("%.0fs p99", p99))
	}
	if queue := assistantOutputFloat(item, "average_queue_seconds"); queue > 0 {
		parts = append(parts, fmt.Sprintf("%.0fs average queue", queue))
	}
	if len(parts) == 0 {
		return "- Pipeline performance: " + label
	}
	return "- Pipeline performance: " + label + " has " + strings.Join(parts, ", ")
}

func assistantOptimizationSpendLine(efficiency, aiUsage map[string]any) string {
	spend := firstPositiveFloat(
		assistantOutputFloat(aiUsage, "spend_usd"),
		assistantOutputFloat(efficiency, "total_ai_spend_usd"),
		assistantOutputFloat(aiUsage, "total_cost_usd"),
	)
	if spend > 0 {
		return fmt.Sprintf("- AI spend in window: $%.2f", spend)
	}
	if len(aiUsage) > 0 || len(efficiency) > 0 || assistantOutputFloat(aiUsage, "priced_calls") > 0 ||
		assistantOutputFloat(aiUsage, "unpriced_calls") > 0 || assistantOutputFloat(aiUsage, "total_tokens") > 0 {
		return "- AI spend in window: $0.00 recorded"
	}
	return ""
}

func assistantOptimizationRecommendations(output, efficiency map[string]any, target string) []string {
	recommendations := []string{}
	for _, value := range assistantStringSlice(efficiency["recommendations"]) {
		if !assistantOptimizationRecommendationApplies(value, nil, target) {
			continue
		}
		recommendations = appendOptimizationRecommendation(recommendations, value)
	}
	for _, row := range assistantMapSlice(assistantMonitoringInsightRawResponse(output, "recommendations")) {
		message := firstNonEmptyString(
			assistantOutputString(row, "message"),
			assistantOutputString(row, "summary"),
			assistantOutputString(row, "title"),
		)
		if !assistantOptimizationRecommendationApplies(message, row, target) {
			continue
		}
		recommendations = appendOptimizationRecommendation(recommendations, message)
	}
	for _, item := range assistantMapSlice(efficiency["costly_low_success_pipelines"]) {
		label := assistantOptimizationPipelineLabel(item)
		if label == "" {
			continue
		}
		if target != "" && !assistantOptimizationMapMatchesTarget(item, target) {
			continue
		}
		recommendations = appendOptimizationRecommendation(recommendations, fmt.Sprintf(
			"Fix reliability for %s first; it has %s success across %.0f runs.",
			label,
			assistantFormatOptimizationPercent(assistantOutputFloat(item, "success_rate")),
			assistantOutputFloat(item, "total_runs"),
		))
	}
	return recommendations
}

func assistantOptimizationRecommendationApplies(message string, row map[string]any, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return true
	}
	if len(row) > 0 && assistantOptimizationMapMatchesTarget(row, target) {
		return true
	}
	if assistantOptimizationTextMatchesTarget(message, target) {
		return true
	}
	return false
}

func assistantOptimizationMapMatchesTarget(item map[string]any, target string) bool {
	if len(item) == 0 || strings.TrimSpace(target) == "" {
		return false
	}
	candidates := []string{
		assistantOptimizationPipelineLabel(item),
		assistantOutputString(item, "pipeline"),
		assistantOutputString(item, "pipeline_id"),
		assistantOutputString(item, "pipelineId"),
	}
	path := firstNonEmptyString(assistantOutputString(item, "pipelinePath"), assistantOutputString(item, "pipeline_path"), assistantOutputString(item, "path"))
	name := firstNonEmptyString(assistantOutputString(item, "pipelineName"), assistantOutputString(item, "pipeline_name"), assistantOutputString(item, "name"))
	if path != "" && name != "" {
		candidates = append(candidates, strings.Trim(path, "/")+"/"+strings.Trim(name, "/"))
	}
	candidates = append(candidates, path, name)
	for _, candidate := range candidates {
		if assistantOptimizationLabelMatchesTarget(candidate, target) {
			return true
		}
	}
	return false
}

func assistantOptimizationTextMatchesTarget(message, target string) bool {
	message = assistantNormalizeOptimizationTarget(message)
	if message == "" {
		return false
	}
	for _, term := range assistantOptimizationTargetTerms(target) {
		if strings.Contains(message, term) {
			return true
		}
	}
	return false
}

func assistantOptimizationLabelMatchesTarget(label, target string) bool {
	label = assistantNormalizeOptimizationTarget(label)
	target = assistantNormalizeOptimizationTarget(target)
	if label == "" || target == "" {
		return false
	}
	if label == target || strings.HasSuffix(label, "/"+target) || strings.HasSuffix(target, "/"+label) {
		return true
	}
	return assistantOptimizationLastTargetSegment(label) == assistantOptimizationLastTargetSegment(target)
}

func assistantOptimizationTargetTerms(target string) []string {
	target = assistantNormalizeOptimizationTarget(target)
	if target == "" {
		return nil
	}
	terms := []string{target}
	if last := assistantOptimizationLastTargetSegment(target); last != "" && last != target {
		terms = append(terms, last)
	}
	return terms
}

func assistantNormalizeOptimizationTarget(value string) string {
	value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "/"))
	replacer := strings.NewReplacer("`", "", "'", "", "\"", "", "\n", " ", "\t", " ")
	value = replacer.Replace(value)
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	return value
}

func assistantOptimizationLastTargetSegment(value string) string {
	value = strings.Trim(assistantNormalizeOptimizationTarget(value), "/")
	if value == "" {
		return ""
	}
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[index+1:]
	}
	return value
}

func assistantOptimizationFallbackRecommendation(item map[string]any, target string) string {
	label := strings.TrimSpace(target)
	if label == "" {
		label = firstNonEmptyString(assistantOptimizationPipelineLabel(item), "the selected pipeline")
	}
	if queue := assistantOutputFloat(item, "average_queue_seconds"); queue > 300 {
		return "Reduce queue time for " + label + ": check runner capacity, dispatcher health, and whether this pipeline is waiting on a constrained runner pool before changing step logic."
	}
	if failureRate := assistantOutputFloat(item, "failure_rate"); failureRate > 0 {
		return "Start with reliability for " + label + ": inspect the first failed step and recurring log signature before tuning runtime or AI usage."
	}
	average := assistantOutputFloat(item, "average_duration_seconds")
	p99 := assistantOutputFloat(item, "p99_duration_seconds")
	if average > 0 && p99 > average*1.5 {
		return fmt.Sprintf("Inspect step and task timing for %s: p99 is %.0fs versus %.0fs average, so tail latency is the likely speed lever.", label, p99, average)
	}
	return "Inspect step and task timing for " + label + " to find the longest repeated step before changing the pipeline definition."
}

func appendOptimizationRecommendation(recommendations []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return recommendations
	}
	for _, existing := range recommendations {
		if strings.EqualFold(existing, value) {
			return recommendations
		}
	}
	return append(recommendations, value)
}

func assistantOptimizationSourceLine(call assistantToolActivity) string {
	paths := assistantStringSlice(call.Output["source_paths"])
	if len(paths) == 0 {
		return "Data source: NopsAI monitoring evidence via `nopsai.find_optimization_opportunities`."
	}
	cleanPaths := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = assistantOptimizationSourcePath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		cleanPaths = append(cleanPaths, path)
		if len(cleanPaths) >= 4 {
			break
		}
	}
	if len(cleanPaths) == 0 {
		return "Data source: NopsAI monitoring evidence via `nopsai.find_optimization_opportunities`."
	}
	return "Data source: NopsAI monitoring evidence via `nopsai.find_optimization_opportunities` (" + strings.Join(cleanPaths, ", ") + ")."
}

func assistantOptimizationSourcePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if idx := strings.Index(path, "://"); idx >= 0 {
		rest := path[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path = rest[slash:]
		}
	}
	return strings.TrimSpace(path)
}

func assistantFormatOptimizationPercent(value float64) string {
	if value > 0 && value <= 1 {
		value *= 100
	}
	rounded := math.Round(value)
	if math.Abs(value-rounded) < 0.05 {
		return fmt.Sprintf("%.0f%%", rounded)
	}
	return fmt.Sprintf("%.1f%%", value)
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
