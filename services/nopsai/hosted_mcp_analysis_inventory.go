package nopsai

import (
	"fmt"
	"sort"
	"strings"
)

// Inventory findings are about what a team owns rather than what it ran: two
// credentials with the same name, a pipeline nobody can attribute, half the
// resources in Git and half in the database. Run metrics never show these, and
// they are the ones that make ownership expensive later.
//
// Ported from the browser catalogue rules in
// services/ui/src/features/analysis/model.ts.

type analysisInventoryItem struct {
	Kind        string
	ID          string
	Label       string
	Description string
	TeamPath    string
	Source      string
	Active      bool
}

var analysisReusableKinds = map[string]bool{
	"pipeline": true, "step": true, "schedule": true, "trigger": true,
}

var analysisSensitiveKinds = map[string]bool{
	"credential": true, "trigger": true, "external_trigger": true,
	"git_webhook_source": true, "knowledge_context": true, "scope": true,
}

var analysisInventoryStopTokens = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "pipeline": true,
	"enabled": true, "global": true, "team": true,
}

func analysisInventoryFindings(items []analysisInventoryItem, teamPath string) []analysisFinding {
	if len(items) == 0 {
		return []analysisFinding{{
			Category:   "organization",
			Severity:   "high",
			Title:      "No resources are attributed to this team",
			Summary:    "Nothing is owned here that the current user can see, so ownership is either unset or held somewhere else.",
			Evidence:   []analysisEvidenceItem{{Label: "Visible resources", Value: "0", Kind: "metric"}},
			Confidence: 0.86,
			Recommendations: []analysisRecommendation{{
				Title:  "Attach ownership explicitly",
				Detail: "Give pipelines, schedules, and triggers a team path in GitOps instead of relying on naming conventions.",
			}},
		}}
	}

	findings := []analysisFinding{}
	findings = append(findings, analysisDuplicateInventoryFindings(items)...)
	if finding := analysisSimilarInventoryFinding(items); finding != nil {
		findings = append(findings, *finding)
	}
	if finding := analysisInheritedInventoryFinding(items, teamPath); finding != nil {
		findings = append(findings, *finding)
	}
	if finding := analysisMixedSourceFinding(items); finding != nil {
		findings = append(findings, *finding)
	}
	if finding := analysisInactiveInventoryFinding(items); finding != nil {
		findings = append(findings, *finding)
	}
	if finding := analysisDeepHierarchyFinding(items); finding != nil {
		findings = append(findings, *finding)
	}
	return findings
}

func analysisDuplicateInventoryFindings(items []analysisInventoryItem) []analysisFinding {
	groups := map[string][]analysisInventoryItem{}
	for _, item := range items {
		key := item.Kind + "|" + analysisNormalizeResourceName(item.Label)
		if strings.TrimSpace(analysisNormalizeResourceName(item.Label)) == "" {
			continue
		}
		groups[key] = append(groups[key], item)
	}
	keys := make([]string, 0, len(groups))
	for key, group := range groups {
		if len(group) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	findings := make([]analysisFinding, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		kind := group[0].Kind
		category, severity := "efficiency", "medium"
		if analysisSensitiveKinds[kind] {
			category, severity = "security", "high"
		}
		findings = append(findings, analysisFinding{
			Category: category,
			Severity: severity,
			Title:    fmt.Sprintf("%d %s resources share one name", len(group), kind),
			Summary:  "Resources that normalise to the same name are indistinguishable to whoever has to pick one, and rotating or retiring the wrong one is a real outage.",
			Evidence: analysisInventoryEvidence(group),
			Recommendations: []analysisRecommendation{{
				Title:  "Keep one canonical resource",
				Detail: "Consolidate the variants, or make the difference explicit in the name and description.",
			}},
			Confidence: 0.82,
		})
	}
	return findings
}

func analysisSimilarInventoryFinding(items []analysisInventoryItem) *analysisFinding {
	reusable := make([]analysisInventoryItem, 0, len(items))
	for _, item := range items {
		if analysisReusableKinds[item.Kind] {
			reusable = append(reusable, item)
		}
	}
	used := map[string]bool{}
	group := []analysisInventoryItem{}
	for index, left := range reusable {
		if used[left.ID] {
			continue
		}
		matches := []analysisInventoryItem{left}
		for _, right := range reusable[index+1:] {
			if used[right.ID] || analysisResourceSimilarity(left, right) < 0.8 {
				continue
			}
			matches = append(matches, right)
		}
		if len(matches) < 2 {
			continue
		}
		for _, item := range matches {
			used[item.ID] = true
		}
		group = matches
		break
	}
	if len(group) == 0 {
		return nil
	}
	return &analysisFinding{
		Category:   "efficiency",
		Severity:   "opportunity",
		Title:      fmt.Sprintf("%d resources look like copies of each other", len(group)),
		Summary:    "Near-identical definitions drift apart one fix at a time, so the copy nobody remembered stays broken.",
		Evidence:   analysisInventoryEvidence(group),
		Confidence: 0.72,
		Recommendations: []analysisRecommendation{{
			Title:  "Extract the shared part",
			Detail: "Move the common behaviour into a reusable step or template and keep only the differences as parameters.",
		}},
	}
}

func analysisInheritedInventoryFinding(items []analysisInventoryItem, teamPath string) *analysisFinding {
	if strings.TrimSpace(strings.Trim(teamPath, "/")) == "" {
		return nil
	}
	inherited := []analysisInventoryItem{}
	sensitive := false
	for _, item := range items {
		if strings.TrimSpace(strings.Trim(item.TeamPath, "/")) != "" {
			continue
		}
		inherited = append(inherited, item)
		if analysisSensitiveKinds[item.Kind] {
			sensitive = true
		}
	}
	if len(inherited) == 0 {
		return nil
	}
	severity := "medium"
	if sensitive {
		severity = "high"
	}
	return &analysisFinding{
		Category: "security",
		Severity: severity,
		Title:    fmt.Sprintf("%d resource%s %s used here but owned globally", len(inherited), plural(len(inherited)), isAre(len(inherited))),
		Summary:  "A global resource used by one team is a boundary nobody declared: every other team can change it, and this team will be surprised when they do.",
		Evidence: append(
			[]analysisEvidenceItem{{Label: "Team", Value: teamPath, Kind: "fact"}},
			analysisInventoryEvidence(inherited)...,
		),
		Confidence: 0.78,
		Recommendations: []analysisRecommendation{{
			Title:  "Move team-specific resources into the team",
			Detail: "Keep global ownership for genuinely shared, stable resources; anything this team alone depends on belongs to it.",
		}},
	}
}

func analysisMixedSourceFinding(items []analysisInventoryItem) *analysisFinding {
	sources := map[string]int{}
	database := []analysisInventoryItem{}
	for _, item := range items {
		source := strings.ToLower(strings.TrimSpace(item.Source))
		if source == "" {
			continue
		}
		sources[source]++
		if source == "database" {
			database = append(database, item)
		}
	}
	if sources["git"] == 0 || sources["database"] == 0 {
		return nil
	}
	return &analysisFinding{
		Category: "organization",
		Severity: "medium",
		Title:    "Resources are split between GitOps and the database",
		Summary:  "Two sources of truth for the same kind of resource means drift is invisible until something is applied over something else.",
		Evidence: append(
			[]analysisEvidenceItem{{Label: "Sources", Value: fmt.Sprintf("%d from git, %d from database", sources["git"], sources["database"]), Kind: "metric"}},
			analysisInventoryEvidence(database)...,
		),
		Confidence: 0.8,
		Recommendations: []analysisRecommendation{{
			Title:  "Move long-lived resources into GitOps",
			Detail: "Or write down which database-managed resources are intentionally runtime-only, so the split is a decision rather than an accident.",
		}},
	}
}

func analysisInactiveInventoryFinding(items []analysisInventoryItem) *analysisFinding {
	inactive := []analysisInventoryItem{}
	for _, item := range items {
		if item.Active && !analysisTextLooksInactive(item.Description+" "+item.Source) {
			continue
		}
		inactive = append(inactive, item)
	}
	if len(inactive) == 0 {
		return nil
	}
	return &analysisFinding{
		Category:   "efficiency",
		Severity:   "opportunity",
		Title:      fmt.Sprintf("%d resource%s %s disabled, stale, or deprecated", len(inactive), plural(len(inactive)), isAre(len(inactive))),
		Summary:    "Disabled resources still have to be read, reviewed, and migrated by everyone who touches the team's configuration.",
		Evidence:   analysisInventoryEvidence(inactive),
		Confidence: 0.7,
		Recommendations: []analysisRecommendation{{
			Title:  "Archive what nothing consumes",
			Detail: "Confirm no schedule, trigger, or pipeline still references them, then remove them rather than leaving them off.",
		}},
	}
}

func analysisDeepHierarchyFinding(items []analysisInventoryItem) *analysisFinding {
	deep := []analysisInventoryItem{}
	for _, item := range items {
		if analysisPathDepth(item.TeamPath) > 3 {
			deep = append(deep, item)
		}
	}
	if len(deep) == 0 {
		return nil
	}
	return &analysisFinding{
		Category:   "organization",
		Severity:   "low",
		Title:      fmt.Sprintf("%d resource%s sit four or more levels deep", len(deep), plural(len(deep))),
		Summary:    "Every extra level is another place to look when something is missing and another boundary to grant across.",
		Evidence:   analysisInventoryEvidence(deep),
		Confidence: 0.64,
		Recommendations: []analysisRecommendation{{
			Title:  "Flatten nesting that maps to nothing",
			Detail: "Keep a level only where it is a real ownership or security boundary.",
		}},
	}
}

func analysisInventoryEvidence(items []analysisInventoryItem) []analysisEvidenceItem {
	evidence := make([]analysisEvidenceItem, 0, analysisMaxEvidenceRows)
	for index, item := range items {
		if index >= analysisMaxEvidenceRows {
			break
		}
		value := item.Kind
		if path := strings.Trim(item.TeamPath, "/"); path != "" {
			value += " in " + path
		} else {
			value += " (global)"
		}
		if source := strings.TrimSpace(item.Source); source != "" {
			value += ", " + source
		}
		evidence = append(evidence, analysisEvidenceItem{Label: item.Label, Value: value, Kind: "fact"})
	}
	return evidence
}

func analysisResourceSimilarity(left, right analysisInventoryItem) float64 {
	if left.Kind != right.Kind {
		return 0
	}
	leftTokens := analysisResourceTokens(left)
	rightTokens := analysisResourceTokens(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	union := map[string]bool{}
	for token := range leftTokens {
		union[token] = true
	}
	shared := 0
	for token := range rightTokens {
		if leftTokens[token] {
			shared++
		}
		union[token] = true
	}
	return float64(shared) / float64(len(union))
}

func analysisResourceTokens(item analysisInventoryItem) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(item.Label+" "+item.Description), func(char rune) bool {
		return !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9')
	}) {
		if len(token) <= 2 || analysisInventoryStopTokens[token] {
			continue
		}
		tokens[token] = true
	}
	return tokens
}

func analysisNormalizeResourceName(label string) string {
	normalized := strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			return char
		case char >= 'A' && char <= 'Z':
			return char + 32
		default:
			return -1
		}
	}, label)
	return normalized
}

func analysisTextLooksInactive(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"disabled", "deprecated", "stale", "inactive", "archived"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func analysisPathDepth(path string) int {
	depth := 0
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if strings.TrimSpace(segment) != "" {
			depth++
		}
	}
	return depth
}

// analysisInventoryFromEvidence reads the normalised inventory the team analysis
// gathered. The shape is uniform across resource kinds so the rules never have to
// know which endpoint a row came from.
func analysisInventoryFromEvidence(set analysisEvidenceSet) []analysisInventoryItem {
	rows := analysisRows(set.section("inventory"), "items")
	items := make([]analysisInventoryItem, 0, len(rows))
	for _, row := range rows {
		active := true
		if value, ok := row["active"].(bool); ok {
			active = value
		}
		items = append(items, analysisInventoryItem{
			Kind:        analysisString(row, "kind"),
			ID:          analysisString(row, "id"),
			Label:       analysisString(row, "label"),
			Description: analysisString(row, "description"),
			TeamPath:    analysisString(row, "team_path"),
			Source:      analysisString(row, "source"),
			Active:      active,
		})
	}
	return items
}
