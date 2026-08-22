package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	aaamodel "nopsai/services/aaa/pkg/model"
)

// Team is the ownership unit every other tool takes as an argument, so it needs
// first-class tools of its own; analysis tools return conclusions rather than
// the raw monitoring payloads a caller would otherwise have to interpret.
func hostedMCPAnalysisTools() []hostedMCPTool {
	return []hostedMCPTool{
		toolDef(
			"nopsai.list_teams",
			"List teams and applications visible to the current user, including the team paths that other team-scoped tools accept.",
			"team.list", "team", "*",
			objectSchema(map[string]any{"query": stringSchema(), "limit": numberSchema()}),
		),
		toolDef(
			"nopsai.get_team",
			"Read one team or application by id or path, including its parent, source, and last run time.",
			"team.read", "team", "*",
			objectSchema(map[string]any{"team": stringSchema(), "team_id": stringSchema(), "team_path": stringSchema()}),
		),
		toolDef(
			"nopsai.analyze_team",
			"Analyze a team's delivery health over a time window and return ranked findings with evidence, category scores, and the next investigation step. Use this for \"how is this team doing\", \"what should we fix first\", or a team review instead of reading monitoring endpoints one by one.",
			"team.read", "team", "*",
			objectSchema(map[string]any{"team": stringSchema(), "team_id": stringSchema(), "team_path": stringSchema(), "days": numberSchema(), "include_inventory": booleanSchema()}),
		),
		toolDef(
			"nopsai.analyze_run",
			"Analyze one pipeline run and return the likely failure domain (application code, pipeline definition, credentials, runner infrastructure, timeout, approval, AI provider, or trigger input), the first failure point, what changed since the last successful run, and the next investigation step. Use this for \"why did this run fail\".",
			"pipeline_run.read", "pipeline_run", "*",
			objectSchema(map[string]any{"run_id": stringSchema()}),
		),
		toolDef(
			"nopsai.analyze_pipeline",
			"Analyze one pipeline's reliability, run duration, step timings, and spend over a time window and return ranked findings with evidence and the next investigation step. Use this for \"why is this pipeline slow/failing\" or a pipeline review.",
			"pipeline.read", "pipeline", "*",
			objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema(), "days": numberSchema()}),
		),
	}
}

func (a *App) executeHostedMCPAnalysisTool(ctx context.Context, subject aaamodel.Subject, name string, args map[string]any) (map[string]any, bool, error) {
	switch name {
	case "nopsai.list_teams":
		result, err := a.hostedMCPListTeams(ctx, subject, args)
		return result, true, err
	case "nopsai.get_team":
		result, err := a.hostedMCPGetTeam(ctx, subject, args)
		return result, true, err
	case "nopsai.analyze_team":
		result, err := a.hostedMCPAnalyzeTeam(ctx, subject, args)
		return result, true, err
	case "nopsai.analyze_pipeline":
		result, err := a.hostedMCPAnalyzePipeline(ctx, subject, args)
		return result, true, err
	case "nopsai.analyze_run":
		result, err := a.hostedMCPAnalyzeRun(ctx, subject, args)
		return result, true, err
	}
	return nil, false, nil
}

func (a *App) hostedMCPListTeams(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	teams, err := a.analysisTeamDirectory(ctx, subject)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	limit := intArg(args, "limit", 100, 500)
	items := make([]map[string]any, 0, len(teams))
	for _, team := range teams {
		if query != "" && !analysisTeamMatches(team, query) {
			continue
		}
		items = append(items, team)
		if len(items) >= limit {
			break
		}
	}
	return map[string]any{
		"teams":   items,
		"count":   len(items),
		"total":   len(teams),
		"applied": false,
		"ok":      true,
	}, nil
}

func (a *App) hostedMCPGetTeam(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	team, resolveErr := a.analysisResolveTeam(ctx, subject, args)
	if resolveErr != nil {
		return resolveErr.result, nil
	}
	return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/teams/"+url.PathEscape(team.ID), nil, false, false, false, ""), nil
}

func (a *App) hostedMCPAnalyzeTeam(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	team, resolveErr := a.analysisResolveTeam(ctx, subject, args)
	if resolveErr != nil {
		return resolveErr.result, nil
	}
	window := analysisWindowFromArgs(args)
	query := url.Values{}
	query.Set("from", window.From.UTC().Format(time.RFC3339))
	query.Set("to", window.To.UTC().Format(time.RFC3339))
	if team.ID != "" {
		query.Set("teamId", team.ID)
	}

	set := a.analysisEvidence(ctx, subject, query, []hostedMCPMonitoringInsightPath{
		{Key: "summary", Path: "/v1/monitoring/summary"},
		{Key: "reliability", Path: "/v1/monitoring/reliability"},
		{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
		{Key: "security", Path: "/v1/monitoring/security"},
		{Key: "pipeline_performance", Path: "/v1/monitoring/pipelines/performance"},
	})
	// What a team owns is a different question from what it ran, and only the
	// inventory can answer it. It costs three more reads, so a caller who only
	// wants delivery metrics can turn it off.
	if boolArg(args, "include_inventory", true) {
		set = analysisMergeEvidence(set, a.analysisTeamInventory(ctx, subject, team))
	}
	return analyzeTeamEvidence(team, window, set), nil
}

// analysisTeamInventory normalises pipelines, schedules, and triggers into one
// shape so the inventory rules never have to know which endpoint a row came from.
// Each source is read through the bridge, so the caller sees exactly what they
// would see themselves, and a source they cannot read becomes a limitation.
func (a *App) analysisTeamInventory(ctx context.Context, subject aaamodel.Subject, team analysisSubject) analysisEvidenceSet {
	sources := []hostedMCPMonitoringInsightPath{
		{Key: "pipelines", Path: "/v1/pipelines?include_source=true"},
		{Key: "schedules", Path: "/v1/schedules"},
		{Key: "triggers", Path: "/v1/triggers"},
	}
	set := analysisEvidenceSet{Data: map[string]map[string]any{}, Sources: []string{}, Limitations: []string{}}
	items := []map[string]any{}
	for _, source := range sources {
		set.Sources = append(set.Sources, source.Path)
		payload := a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, source.Path, nil, false, false, false, "")
		if message := analysisSourceFailure(payload); message != "" {
			set.Limitations = append(set.Limitations, fmt.Sprintf("%s could not be read: %s", source.Key, message))
			continue
		}
		for _, row := range analysisResponseRows(payload) {
			if item := analysisInventoryRow(source.Key, row, team.Path); item != nil {
				items = append(items, item)
			}
		}
	}
	set.Data["inventory"] = map[string]any{"items": items}
	return set
}

// analysisInventoryRow keeps only the team's own resources plus the global ones
// it can see; another team's resources are not this team's finding.
func analysisInventoryRow(kind string, row map[string]any, teamPath string) map[string]any {
	item := map[string]any{"active": true}
	switch kind {
	case "pipelines":
		id := analysisString(row, "id")
		if id == "" {
			return nil
		}
		item["kind"] = "pipeline"
		item["id"] = id
		item["label"] = analysisLastPathSegment(id)
		item["team_path"] = analysisParentPath(id)
		item["source"] = analysisString(row, "source")
	case "schedules":
		item["kind"] = "schedule"
		item["id"] = analysisString(row, "id")
		item["label"] = firstNonEmptyString(analysisString(row, "identifier"), analysisString(row, "name"))
		item["description"] = analysisString(row, "description")
		item["team_path"] = firstNonEmptyString(analysisString(row, "run_team_path"), analysisString(row, "path"))
		item["source"] = analysisString(row, "source")
		if enabled, ok := row["enabled"].(bool); ok {
			item["active"] = enabled
		}
	case "triggers":
		item["kind"] = "trigger"
		item["id"] = analysisString(row, "repository")
		item["label"] = analysisString(row, "repository")
		item["team_path"] = analysisString(row, "team_path")
		item["source"] = analysisString(row, "source")
	default:
		return nil
	}
	if analysisString(item, "label") == "" {
		return nil
	}
	if !analysisInventoryBelongsToTeam(analysisString(item, "team_path"), teamPath) {
		return nil
	}
	return item
}

func analysisInventoryBelongsToTeam(itemPath, teamPath string) bool {
	item := strings.Trim(strings.TrimSpace(itemPath), "/")
	team := strings.Trim(strings.TrimSpace(teamPath), "/")
	if team == "" || item == "" {
		// A global resource is visible to every team, and an unscoped analysis
		// has no boundary to apply.
		return true
	}
	return item == team || strings.HasPrefix(item, team+"/") || strings.HasPrefix(team, item+"/")
}

func analysisLastPathSegment(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

func analysisParentPath(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	if index := strings.LastIndex(trimmed, "/"); index > 0 {
		return trimmed[:index]
	}
	return ""
}

// analysisResponseRows reads a list payload whether the route returns a bare
// array or wraps it in a named key.
func analysisResponseRows(payload map[string]any) []map[string]any {
	switch response := payload["response"].(type) {
	case []any:
		rows := make([]map[string]any, 0, len(response))
		for _, item := range response {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	case map[string]any:
		for _, key := range []string{"items", "pipelines", "schedules", "triggers", "results"} {
			if rows := analysisRows(response, key); len(rows) > 0 {
				return rows
			}
		}
	}
	return nil
}

func (a *App) hostedMCPAnalyzePipeline(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	pipelinePath, pipelineName := splitPipelineArg(args)
	if strings.TrimSpace(pipelineName) == "" {
		return map[string]any{
			"analysis": "pipeline",
			"ok":       false,
			"applied":  false,
			"error":    "a pipeline is required; pass pipeline as \"path/name\", or name plus path",
		}, nil
	}
	pipelineID := aaamodel.BuildPipelineID(pipelinePath, pipelineName)
	window := analysisWindowFromArgs(args)
	query := url.Values{}
	query.Set("from", window.From.UTC().Format(time.RFC3339))
	query.Set("to", window.To.UTC().Format(time.RFC3339))
	query.Set("pipelineName", pipelineName)
	if pipelinePath != "" {
		query.Set("pipelinePath", pipelinePath)
	}

	set := a.analysisEvidence(ctx, subject, query, []hostedMCPMonitoringInsightPath{
		{Key: "summary", Path: "/v1/monitoring/summary"},
		{Key: "reliability", Path: "/v1/monitoring/reliability"},
		{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
		{Key: "step_performance", Path: "/v1/monitoring/steps/performance"},
	})
	// How a pipeline behaves and how it is written are different questions, and a
	// review that answers only the first misses the secret in the YAML.
	definition, err := a.analysisPipelineDefinition(ctx, pipelinePath, pipelineName)
	if err != nil {
		set.Limitations = append(set.Limitations, "the pipeline definition could not be read: "+err.Error())
	} else {
		set.Sources = append(set.Sources, "pipeline definition "+pipelineID)
		set.Data["definition"] = map[string]any{"yaml": definition}
	}
	subjectRef := analysisSubject{Type: "pipeline", ID: pipelineID, Label: pipelineName, Path: pipelinePath}
	return analyzePipelineEvidence(subjectRef, window, set), nil
}

func (a *App) hostedMCPAnalyzeRun(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	runID := strings.TrimSpace(stringArg(args, "run_id"))
	if runID == "" {
		return map[string]any{
			"analysis": "run",
			"ok":       false,
			"applied":  false,
			"error":    "a run_id is required",
		}, nil
	}
	escaped := url.PathEscape(runID)
	set := a.analysisEvidence(ctx, subject, url.Values{}, []hostedMCPMonitoringInsightPath{
		{Key: "detail", Path: "/v1/runs/" + escaped},
		{Key: "approvals", Path: "/v1/runs/" + escaped + "/approvals"},
	})
	// Peers are what turn "it failed" into "this changed"; without them the
	// analysis can still classify the domain, it just cannot compare.
	peers := a.analysisEvidence(ctx, subject, analysisRunPeerQuery(), []hostedMCPMonitoringInsightPath{
		{Key: "peers", Path: "/v1/runs"},
	})
	set = analysisMergeEvidence(set, peers)
	set = analysisMergeEvidence(set, a.analysisRunLogEvidence(ctx, subject, escaped))

	subjectRef := analysisSubject{Type: "run", ID: runID, Label: analysisRunSubjectLabel(set, runID)}
	return analyzeRunEvidence(subjectRef, analysisWindowFromArgs(args), set), nil
}

func analysisRunPeerQuery() url.Values {
	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", analysisRunMaxPeers))
	return query
}

// analysisRunLogEvidence reads a bounded log window through the same bridge, so a
// subject allowed to read the run but not its logs loses the log signal and
// nothing else.
func (a *App) analysisRunLogEvidence(ctx context.Context, subject aaamodel.Subject, escapedRunID string) analysisEvidenceSet {
	query := url.Values{}
	query.Set("limit", "120")
	return a.analysisEvidence(ctx, subject, query, []hostedMCPMonitoringInsightPath{
		{Key: "logs", Path: "/v1/runs/" + escapedRunID + "/logs"},
	})
}

func analysisRunSubjectLabel(set analysisEvidenceSet, runID string) string {
	runInfo := analysisSubsection(set.section("detail"), "run_info")
	if pipeline := analysisRunPipelineID(runInfo); pipeline != "" {
		return pipeline
	}
	return runID
}

func analysisMergeEvidence(base analysisEvidenceSet, extra analysisEvidenceSet) analysisEvidenceSet {
	if base.Data == nil {
		base.Data = map[string]map[string]any{}
	}
	for key, value := range extra.Data {
		base.Data[key] = value
	}
	base.Sources = append(base.Sources, extra.Sources...)
	base.Limitations = append(base.Limitations, extra.Limitations...)
	return base
}

// analysisPipelineDefinition reads the stored YAML. The caller has already passed
// the pipeline.read check for this exact pipeline in tool authorization.
func (a *App) analysisPipelineDefinition(ctx context.Context, pipelinePath, pipelineName string) (string, error) {
	if a == nil || a.db == nil {
		return "", fmt.Errorf("database unavailable")
	}
	result, err := a.hostedMCPGetPipeline(ctx, map[string]any{"path": pipelinePath, "name": pipelineName})
	if err != nil {
		return "", err
	}
	return stringArg(result, "definition"), nil
}

// analysisEvidence reads every source through the permission-checked API bridge.
// A source that cannot be read becomes a limitation rather than a silent zero,
// because an unreadable category must never look like a healthy one.
func (a *App) analysisEvidence(
	ctx context.Context,
	subject aaamodel.Subject,
	query url.Values,
	specs []hostedMCPMonitoringInsightPath,
) analysisEvidenceSet {
	set := analysisEvidenceSet{Data: map[string]map[string]any{}, Sources: []string{}, Limitations: []string{}}
	for _, spec := range specs {
		path := spec.Path + "?" + query.Encode()
		set.Sources = append(set.Sources, path)
		payload := a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, path, nil, false, false, false, "")
		if message := analysisSourceFailure(payload); message != "" {
			set.Limitations = append(set.Limitations, fmt.Sprintf("%s could not be read: %s", spec.Key, message))
			continue
		}
		response, ok := payload["response"].(map[string]any)
		if !ok {
			set.Limitations = append(set.Limitations, fmt.Sprintf("%s returned no readable payload.", spec.Key))
			continue
		}
		set.Data[spec.Key] = response
	}
	return set
}

func analysisSourceFailure(payload map[string]any) string {
	if payload == nil {
		return "no response"
	}
	if message, ok := payload["error"].(string); ok && strings.TrimSpace(message) != "" {
		return message
	}
	if ok, present := payload["ok"].(bool); present && !ok {
		if text, hasText := payload["response_text"].(string); hasText && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		return fmt.Sprintf("status %v", payload["status_code"])
	}
	return ""
}

func analysisWindowFromArgs(args map[string]any) analysisWindow {
	days := intArg(args, "days", analysisDefaultWindowDays, analysisMaxWindowDays)
	if days <= 0 {
		days = analysisDefaultWindowDays
	}
	to := time.Now().UTC()
	return analysisWindow{From: to.AddDate(0, 0, -days), To: to, Days: days}
}

type analysisResolveError struct {
	result map[string]any
}

func (e *analysisResolveError) Error() string {
	message, _ := e.result["error"].(string)
	return message
}

// analysisResolveTeam matches by id, path, slug, or name and, when it cannot,
// returns the readable teams so the assistant can ask a precise question instead
// of guessing at another identifier.
func (a *App) analysisResolveTeam(ctx context.Context, subject aaamodel.Subject, args map[string]any) (analysisSubject, *analysisResolveError) {
	wanted := firstNonEmptyString(
		strings.TrimSpace(stringArg(args, "team_id")),
		strings.TrimSpace(stringArg(args, "team_path")),
		strings.TrimSpace(stringArg(args, "team")),
		strings.TrimSpace(stringArg(args, "scope")),
	)
	// "*" analyses everything the caller can see, which is what a workspace-level
	// review asks for; it is not the same as failing to name a team.
	if wanted == "*" || strings.EqualFold(wanted, "all") {
		return analysisSubject{Type: "team", ID: "", Label: "All teams", Path: ""}, nil
	}
	teams, err := a.analysisTeamDirectory(ctx, subject)
	if err != nil {
		return analysisSubject{}, &analysisResolveError{result: map[string]any{
			"ok":      false,
			"applied": false,
			"error":   "teams could not be read: " + err.Error(),
		}}
	}
	if len(teams) == 0 {
		return analysisSubject{}, &analysisResolveError{result: map[string]any{
			"ok":      false,
			"applied": false,
			"error":   "no teams are visible to the current user",
		}}
	}
	if wanted == "" {
		if len(teams) == 1 {
			return analysisSubjectFromTeam(teams[0]), nil
		}
		return analysisSubject{}, &analysisResolveError{result: map[string]any{
			"ok":              false,
			"applied":         false,
			"error":           "a team is required; pass team, team_path, or team_id",
			"available_teams": analysisTeamChoices(teams),
		}}
	}
	needle := strings.ToLower(strings.Trim(wanted, "/"))
	for _, team := range teams {
		if analysisTeamMatches(team, needle) {
			return analysisSubjectFromTeam(team), nil
		}
	}
	return analysisSubject{}, &analysisResolveError{result: map[string]any{
		"ok":              false,
		"applied":         false,
		"error":           fmt.Sprintf("no visible team matches %q", wanted),
		"available_teams": analysisTeamChoices(teams),
	}}
}

func (a *App) analysisTeamDirectory(ctx context.Context, subject aaamodel.Subject) ([]map[string]any, error) {
	payload := a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/teams?include=applications", nil, false, false, false, "")
	if message := analysisSourceFailure(payload); message != "" {
		return nil, fmt.Errorf("%s", message)
	}
	response, ok := payload["response"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("teams response was not readable")
	}
	teams := []map[string]any{}
	for _, key := range []string{"teams", "applications"} {
		for _, row := range analysisRows(response, key) {
			teams = append(teams, analysisTeamRow(row))
		}
	}
	return teams, nil
}

func analysisTeamRow(row map[string]any) map[string]any {
	item := map[string]any{
		"id":           analysisTeamID(row),
		"kind":         analysisString(row, "kind"),
		"name":         analysisString(row, "name"),
		"slug":         analysisString(row, "slug"),
		"display_name": analysisString(row, "display_name"),
		"path":         analysisString(row, "path"),
		"source":       analysisString(row, "source"),
	}
	if lastRun := analysisString(row, "last_run_at"); lastRun != "" {
		item["last_run_at"] = lastRun
	}
	return item
}

func analysisTeamID(row map[string]any) string {
	if id := analysisInt(row, "id"); id > 0 {
		return fmt.Sprintf("%d", id)
	}
	return analysisString(row, "id")
}

func analysisTeamMatches(team map[string]any, needle string) bool {
	for _, key := range []string{"id", "path", "slug", "name", "display_name"} {
		value := strings.ToLower(strings.Trim(analysisString(team, key), "/"))
		if value != "" && value == needle {
			return true
		}
	}
	return strings.Contains(strings.ToLower(analysisString(team, "display_name")), needle) ||
		strings.Contains(strings.ToLower(analysisString(team, "path")), needle)
}

func analysisTeamChoices(teams []map[string]any) []map[string]any {
	choices := make([]map[string]any, 0, len(teams))
	for index, team := range teams {
		if index >= 20 {
			break
		}
		choices = append(choices, map[string]any{
			"id":    analysisString(team, "id"),
			"path":  analysisString(team, "path"),
			"label": firstNonEmptyString(analysisString(team, "display_name"), analysisString(team, "name")),
		})
	}
	return choices
}

func analysisSubjectFromTeam(team map[string]any) analysisSubject {
	return analysisSubject{
		Type:  "team",
		ID:    analysisString(team, "id"),
		Label: firstNonEmptyString(analysisString(team, "display_name"), analysisString(team, "name"), analysisString(team, "path")),
		Path:  analysisString(team, "path"),
	}
}
