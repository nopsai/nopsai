package nopsai

import (
	"sort"
	"strings"
)

// Tool routing used to be a hand-written keyword table in the planner: every new
// tool stayed invisible until somebody remembered to add a branch for it. The
// routing signal is derived here instead, from what a tool already declares —
// its name, description, AAA action, and resource type — so a tool is routable
// the day it is registered.
//
// What stays hand-written is human vocabulary, not tools: the words people use
// for a domain ("env var" for variables, "cron" for schedules) do not follow
// from any field a tool carries. That map grows with the language, not with the
// catalogue.

type hostedMCPToolRouting struct {
	Name       string
	Domain     string
	Capability string
	NameTerms  map[string]struct{}
	TextTerms  map[string]struct{}
}

const (
	hostedMCPCapabilityRead     = "read"
	hostedMCPCapabilityProposal = "proposal"
	hostedMCPCapabilityMutation = "mutation"
)

// hostedMCPDomainVocabulary maps how people ask to the domain that answers.
// Terms are matched as whole words against the request.
var hostedMCPDomainVocabulary = map[string][]string{
	"monitoring": {
		"monitoring", "analytics", "statistics", "metrics", "usage", "token", "tokens", "spend", "cost", "costs",
		"performance", "slowest", "fastest", "longest", "duration", "latency", "bottleneck", "bottlenecks",
		"p95", "p99", "reliability", "efficiency", "flaky", "utilization", "throughput", "trend", "trends",
	},
	"pipeline_run": {
		"run", "runs", "execution", "executed", "failed", "failing", "failure", "failures", "error", "errors",
		"log", "logs", "rerun", "retry", "crashed", "stuck", "approval", "approvals", "lab",
	},
	"pipeline": {
		"pipeline", "pipelines", "yaml", "definition", "step", "steps", "build", "deploy", "deployment",
		"release", "rollout", "publish", "workflow",
	},
	"pipeline_schedule":  {"schedule", "schedules", "cron", "nightly", "recurring", "scheduled"},
	"trigger":            {"trigger", "triggers", "webhook", "webhooks", "push", "repository"},
	"external_trigger":   {"external", "invoke", "invocation", "callback"},
	"git_webhook_source": {"webhook", "source", "provider", "github", "gitlab", "bitbucket"},
	"variable":           {"variable", "variables", "var", "vars", "env", "envs", "environment"},
	"secret":             {"secret", "secrets", "encrypted", "sops"},
	"credential":         {"credential", "credentials", "api key", "apikey", "rotate", "rotation", "expiry"},
	"knowledge_context": {"docs", "doc", "documentation", "knowledge", "guardrail", "guardrails", "guideline",
		"policy", "policies", "sample", "samples", "example", "examples", "template", "templates", "wiki"},
	"knowledge_connection": {"notion", "confluence", "connection", "connections"},
	"dashboard":            {"dashboard", "dashboards", "chart", "charts", "panel", "report", "reports"},
	"team":                 {"team", "teams", "squad", "organisation", "organization", "application", "applications", "ownership"},
	"scope":                {"scope", "scopes", "environment path"},
	"iam": {"permission", "permissions", "access", "grant", "grants", "role", "roles", "aaa", "rbac",
		"user", "users", "admin", "service account", "audit", "identity", "sso", "oidc"},
	"dispatcher": {"runner", "runners", "dispatcher", "kubernetes", "k8s", "docker", "pool", "pools",
		"capacity", "queue", "queued", "worker", "workers", "bootstrap"},
	"step":       {"reusable", "reusable step", "shared step", "library"},
	"system_log": {"system log", "system logs", "tail", "platform log"},
	"audit":      {"audit", "audit log", "history"},
	"system": {"system", "status", "health", "version", "setup", "install", "installation", "config repo",
		"configuration", "gitops", "sync", "drift", "backup", "cleanup", "retention", "notification",
		"notifications", "mail", "smtp", "alert", "alerts", "recommendation", "recommendations",
		"llm", "model", "models", "profile", "profiles", "mcp", "capability", "capabilities", "feature"},
}

var hostedMCPRoutingStopWords = map[string]struct{}{}

func init() {
	for _, word := range []string{
		"the", "and", "for", "with", "without", "from", "into", "that", "this", "current", "user", "users",
		"nopsai", "requires", "require", "required", "optional", "returns", "return", "when", "only", "not",
		"applying", "changes", "change", "using", "use", "used", "uses", "than", "then", "its", "his", "her",
		"are", "was", "were", "has", "have", "had", "can", "may", "must", "should", "all", "any", "each",
		"per", "via", "true", "false", "confirm", "confirmation", "safe", "check", "checks", "value", "values",
		"name", "names", "path", "paths", "true:", "false:", "one", "two", "new", "old", "get", "set",
	} {
		hostedMCPRoutingStopWords[word] = struct{}{}
	}
}

func hostedMCPToolRoutingFor(tool hostedMCPTool) hostedMCPToolRouting {
	domain := hostedMCPToolDomain(tool)
	return hostedMCPToolRouting{
		Name:       tool.Name,
		Domain:     domain,
		Capability: hostedMCPToolCapability(tool),
		NameTerms:  hostedMCPRoutingTermSet(strings.NewReplacer(".", " ", "_", " ").Replace(strings.TrimPrefix(tool.Name, "nopsai."))),
		TextTerms:  hostedMCPRoutingTermSet(tool.Description),
	}
}

// hostedMCPToolDomain prefers the AAA resource type, which is already curated per
// tool, and overrides it where the name says the tool answers a different kind of
// question than the resource it reads. Monitoring tools read runs; people asking
// about them are not asking about a run.
func hostedMCPToolDomain(tool hostedMCPTool) string {
	name := strings.ToLower(tool.Name)
	switch {
	case strings.Contains(name, "monitoring") || strings.Contains(name, "analytics") ||
		strings.Contains(name, "statistics") || strings.Contains(name, "cost") ||
		strings.Contains(name, "efficiency") || strings.Contains(name, "optimization"):
		return "monitoring"
	case strings.Contains(name, "runner") || strings.Contains(name, "dispatcher"):
		return "dispatcher"
	case strings.Contains(name, "notification"):
		return "system"
	case strings.Contains(name, "team"):
		return "team"
	}
	if resource := strings.TrimSpace(strings.ToLower(tool.Resource.Type)); resource != "" {
		return resource
	}
	return "system"
}

func hostedMCPToolCapability(tool hostedMCPTool) string {
	if assistantPlannedToolIsProposal(tool.Name) {
		return hostedMCPCapabilityProposal
	}
	if assistantToolRequiresActionExecution(tool) {
		return hostedMCPCapabilityMutation
	}
	return hostedMCPCapabilityRead
}

func hostedMCPRoutingTermSet(text string) map[string]struct{} {
	terms := map[string]struct{}{}
	for _, token := range hostedMCPRoutingTokens(text) {
		terms[token] = struct{}{}
	}
	return terms
}

func hostedMCPRoutingTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(char rune) bool {
		return !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9')
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, stop := hostedMCPRoutingStopWords[field]; stop {
			continue
		}
		tokens = append(tokens, hostedMCPRoutingSingular(field))
	}
	return tokens
}

// hostedMCPRoutingSingular is a deliberately small stemmer: it folds the plural a
// person writes into the singular a tool name uses, and does nothing clever.
//
// The endings it refuses to touch are the ones that are not plurals: "status",
// "analysis", and "access" are whole words, and folding them produces a token
// that matches nothing a tool actually says.
func hostedMCPRoutingSingular(token string) string {
	switch {
	case strings.HasSuffix(token, "ies") && len(token) > 4:
		return token[:len(token)-3] + "y"
	case strings.HasSuffix(token, "sses"):
		return token[:len(token)-2]
	case strings.HasSuffix(token, "ss"), strings.HasSuffix(token, "us"),
		strings.HasSuffix(token, "is"), strings.HasSuffix(token, "as"):
		return token
	case strings.HasSuffix(token, "s") && len(token) > 3:
		return token[:len(token)-1]
	default:
		return token
	}
}

// hostedMCPRequestDomains scores the domains a request is asking about. A request
// usually names one or two; everything else stays out of the schema budget.
func hostedMCPRequestDomains(request string) map[string]int {
	lower := " " + strings.ToLower(strings.TrimSpace(request)) + " "
	tokens := map[string]struct{}{}
	for _, token := range hostedMCPRoutingTokens(request) {
		tokens[token] = struct{}{}
	}
	domains := map[string]int{}
	for domain, vocabulary := range hostedMCPDomainVocabulary {
		for _, phrase := range vocabulary {
			if strings.Contains(phrase, " ") {
				if strings.Contains(lower, " "+phrase+" ") {
					domains[domain] += 2
				}
				continue
			}
			if _, ok := tokens[hostedMCPRoutingSingular(phrase)]; ok {
				domains[domain]++
			}
		}
	}
	return domains
}

// hostedMCPToolIsGitOpsMode marks the tools that produce a file plan rather than
// a runtime change. "Encrypt this secret for GitOps" is one even though its name
// does not start with propose_.
func hostedMCPToolIsGitOpsMode(routing hostedMCPToolRouting) bool {
	return routing.Capability == hostedMCPCapabilityProposal || strings.Contains(routing.Name, "gitops")
}

// assistantPlannerModePolicy decides which classes of tool may appear at all.
// This is policy, not relevance: a request to set a value must not be offered a
// GitOps proposal, a GitOps request must not be offered a runtime write, and a
// read must be offered neither. Ranking cannot express that — only exclusion can.
type assistantPlannerModePolicy struct {
	AllowProposal bool
	AllowMutation bool
}

func (policy assistantPlannerModePolicy) allows(routing hostedMCPToolRouting) bool {
	if hostedMCPToolIsGitOpsMode(routing) {
		return policy.AllowProposal
	}
	if routing.Capability == hostedMCPCapabilityMutation {
		return policy.AllowMutation
	}
	return true
}

var assistantPlannerDiscoveryPolicy = assistantPlannerModePolicy{AllowProposal: true, AllowMutation: true}

// hostedMCPRoutingScores ranks tools for a request from metadata alone, within
// the classes the mode policy permits.
func hostedMCPRoutingScores(request string, tools []hostedMCPTool, policy assistantPlannerModePolicy) map[string]int {
	domains := hostedMCPRequestDomains(request)
	requestTerms := map[string]struct{}{}
	for _, token := range hostedMCPRoutingTokens(request) {
		requestTerms[token] = struct{}{}
	}
	if len(domains) == 0 && len(requestTerms) == 0 {
		return map[string]int{}
	}

	scores := map[string]int{}
	for _, tool := range tools {
		routing := hostedMCPToolRoutingFor(tool)
		if !policy.allows(routing) {
			continue
		}
		score := 0
		if hits := domains[routing.Domain]; hits > 0 {
			score += 30 + 6*minInt(hits, 3)
			if routing.Capability == hostedMCPCapabilityRead {
				score += 10
			}
		}
		nameHits := 0
		for term := range routing.NameTerms {
			if _, ok := requestTerms[term]; ok {
				nameHits++
			}
		}
		score += 18 * minInt(nameHits, 4)
		textHits := 0
		for term := range routing.TextTerms {
			if _, ok := requestTerms[term]; ok {
				textHits++
			}
		}
		score += 4 * minInt(textHits, 4)

		if score <= 0 {
			continue
		}
		scores[tool.Name] = score
	}
	return scores
}

// hostedMCPTopRoutingScores keeps the strongest matches and drops the long tail.
// A vague request matches a whole domain weakly; shipping every one of those
// schemas spends the budget without adding an answer.
func hostedMCPTopRoutingScores(scores map[string]int, limit int) map[string]int {
	if len(scores) == 0 {
		return scores
	}
	best := 0
	for _, score := range scores {
		if score > best {
			best = score
		}
	}
	floor := best / 2
	names := make([]string, 0, len(scores))
	for name, score := range scores {
		if score >= floor {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if scores[names[i]] != scores[names[j]] {
			return scores[names[i]] > scores[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > limit {
		names = names[:limit]
	}
	top := make(map[string]int, len(names))
	for _, name := range names {
		top[name] = scores[name]
	}
	return top
}

// hostedMCPFindTools lets a planner search the catalogue instead of failing when
// the request uses words no rule anticipated.
func (a *App) hostedMCPFindTools(tools []hostedMCPTool, args map[string]any) map[string]any {
	query := strings.TrimSpace(stringArg(args, "query"))
	limit := intArg(args, "limit", 10, 40)
	scores := hostedMCPRoutingScores(query, tools, assistantPlannerDiscoveryPolicy)
	if domain := strings.ToLower(strings.TrimSpace(stringArg(args, "domain"))); domain != "" {
		for _, tool := range tools {
			if hostedMCPToolDomain(tool) == domain {
				scores[tool.Name] += 25
			}
		}
	}

	ranked := make([]string, 0, len(scores))
	for name := range scores {
		ranked = append(ranked, name)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if scores[ranked[i]] != scores[ranked[j]] {
			return scores[ranked[i]] > scores[ranked[j]]
		}
		return ranked[i] < ranked[j]
	})

	byName := map[string]hostedMCPTool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	items := make([]map[string]any, 0, limit)
	for _, name := range ranked {
		if len(items) >= limit {
			break
		}
		tool := byName[name]
		routing := hostedMCPToolRoutingFor(tool)
		items = append(items, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"domain":       routing.Domain,
			"capability":   routing.Capability,
			"input_schema": assistantPlannerCompactInputSchema(tool.InputSchema),
		})
	}
	return map[string]any{
		"query":   query,
		"tools":   items,
		"count":   len(items),
		"total":   len(tools),
		"applied": false,
		"ok":      true,
		"note":    "Only tools the current user may call are listed. Use the returned input_schema for exact argument names.",
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
