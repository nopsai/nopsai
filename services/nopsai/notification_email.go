package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	runquery "nopsai/services/nopsai/internal/runs"
)

const (
	pipelineNotificationLogLimit        = 5
	pipelineNotificationLogQueryLimit   = 60
	pipelineNotificationLogLineMaxRunes = 480
)

var (
	notificationANSISequencePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	notificationBearerPattern       = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	notificationSecretPattern       = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|authorization|api[_-]?key)\b(\s*["']?\s*[:=]\s*["']?|\s+)([^,\s;"']+)`)
)

type pipelineNotificationStep struct {
	Name       string
	Status     string
	TaskTotal  int
	TaskPassed int
}

type pipelineNotificationLogEntry struct {
	Text string
	Step string
	Task string
}

type pipelineNotificationBranding struct {
	Name       string
	PublicURL  string
	LogoURL    string
	WebsiteURL string
	SupportURL string
	Address    string
}

type pipelineNotificationProgress struct {
	Total   int
	Passed  int
	Failed  int
	Running int
	Pending int
	Skipped int
}

type pipelineNotificationMailView struct {
	PreviewText      string
	Brand            pipelineNotificationBranding
	StatusLabel      string
	StatusBackground template.CSS
	StatusForeground template.CSS
	Headline         string
	Summary          string
	Pipeline         string
	Progress         pipelineNotificationProgress
	ProgressLabel    string
	FailureLocation  string
	FailureReason    string
	Steps            []pipelineNotificationStepView
	Logs             []pipelineNotificationLogEntry
	RunID            string
	GroupPath        string
	Repository       string
	Branch           string
	Commit           string
	Trigger          string
	Duration         string
	StartedAt        string
	RunURL           string
	RepositoryURL    string
	CommitURL        string
	WebsiteDisplay   string
	SupportDisplay   string
}

type pipelineNotificationStepView struct {
	Name        string
	Status      string
	TaskSummary string
	Color       template.CSS
	Background  template.CSS
}

func (a *App) enrichPipelineNotificationContext(ctx context.Context, notificationCtx *pipelineNotificationContext, eventType string) error {
	if notificationCtx == nil {
		return nil
	}
	tasksByStep, taskErr := runquery.LoadTaskDetailsByStep(ctx, a.db, notificationCtx.RunID)
	childRuns, childErr := runquery.LoadChildRuns(ctx, a.db, notificationCtx.RunID)

	var stepDetails []models.StepDetail
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(notificationCtx.PipelineDefinitionYAML), &pipeline); err == nil {
		resolvedPipeline := pipeline
		if resolved, resolveErr := a.resolveStepIncludes(&resolvedPipeline); resolveErr == nil && resolved != nil {
			resolvedPipeline = *resolved
		}
		stepDetails = runquery.BuildStepDetailsForRun(models.RunListItem{Status: notificationCtx.Status}, pipeline, resolvedPipeline, tasksByStep, childRuns, nil, nil)
	}
	if len(stepDetails) == 0 {
		stepDetails = fallbackPipelineNotificationSteps(notificationCtx.Status, tasksByStep)
	}
	notificationCtx.Steps = summarizePipelineNotificationSteps(stepDetails)

	if normalizeNotificationEventType(eventType) == "failure" || runquery.NormalizeRunDetailStatus(notificationCtx.Status) == "failure" {
		logs, err := a.loadPipelineNotificationLogExcerpt(ctx, notificationCtx.RunID)
		if err == nil {
			notificationCtx.LogExcerpt = logs
		}
	}
	notificationCtx.FailureStep, notificationCtx.FailureTask = pipelineNotificationFailureLocation(notificationCtx.LogExcerpt, stepDetails, tasksByStep)

	switch {
	case taskErr != nil:
		return taskErr
	case childErr != nil:
		return childErr
	default:
		return nil
	}
}

func fallbackPipelineNotificationSteps(runStatus string, tasksByStep map[string][]models.TaskDetail) []models.StepDetail {
	names := make([]string, 0, len(tasksByStep))
	for name := range tasksByStep {
		names = append(names, name)
	}
	sort.Strings(names)
	steps := make([]models.StepDetail, 0, len(names))
	for _, name := range names {
		tasks := tasksByStep[name]
		status := runquery.FinalizeRunDetailStepStatus(runquery.DeriveRunDetailStepStatus(tasks, nil), tasks, runStatus)
		steps = append(steps, models.StepDetail{Name: name, Status: status, Tasks: tasks})
	}
	return steps
}

func summarizePipelineNotificationSteps(stepDetails []models.StepDetail) []pipelineNotificationStep {
	steps := make([]pipelineNotificationStep, 0, len(stepDetails))
	for _, step := range stepDetails {
		summary := pipelineNotificationStep{
			Name:      step.Name,
			Status:    runquery.NormalizeRunDetailStatus(step.Status),
			TaskTotal: len(step.Tasks),
		}
		for _, task := range step.Tasks {
			if runquery.NormalizeRunDetailStatus(task.Status) == "success" {
				summary.TaskPassed++
			}
		}
		steps = append(steps, summary)
	}
	return steps
}

func pipelineNotificationFailureLocation(logs []pipelineNotificationLogEntry, steps []models.StepDetail, tasksByStep map[string][]models.TaskDetail) (string, string) {
	for _, entry := range logs {
		if entry.Step != "" || entry.Task != "" {
			return entry.Step, entry.Task
		}
	}
	for _, step := range steps {
		for _, task := range step.Tasks {
			if runquery.NormalizeRunDetailStatus(task.Status) == "failure" {
				return step.Name, task.TaskName
			}
		}
	}
	var latest models.TaskDetail
	for _, tasks := range tasksByStep {
		for _, task := range tasks {
			if task.StartedAt.IsZero() || !task.FinishedAt.IsZero() {
				continue
			}
			if latest.StartedAt.IsZero() || task.StartedAt.After(latest.StartedAt) {
				latest = task
			}
		}
	}
	if !latest.StartedAt.IsZero() {
		return latest.StepName, latest.TaskName
	}
	for _, step := range steps {
		if runquery.NormalizeRunDetailStatus(step.Status) == "failure" {
			return step.Name, ""
		}
	}
	return "", ""
}

func (a *App) loadPipelineNotificationLogExcerpt(ctx context.Context, runID string) ([]pipelineNotificationLogEntry, error) {
	rows, err := a.db.Query(ctx, `
		SELECT line
		FROM pipeline_run_logs
		WHERE run_id = $1::uuid
		  AND line ~* '(error|fail|fatal|panic|exception|exit code|denied|timeout)'
		ORDER BY id DESC
		LIMIT $2
	`, runID, pipelineNotificationLogQueryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]pipelineNotificationLogEntry, 0, pipelineNotificationLogLimit)
	seen := make(map[string]struct{}, pipelineNotificationLogLimit)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		entry, ok := parsePipelineNotificationLogEntry(raw)
		if !ok {
			continue
		}
		key := strings.ToLower(strings.Join([]string{entry.Step, entry.Task, entry.Text}, "|"))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, entry)
		if len(entries) == pipelineNotificationLogLimit {
			break
		}
	}
	return entries, rows.Err()
}

func parsePipelineNotificationLogEntry(raw string) (pipelineNotificationLogEntry, bool) {
	cleaned := strings.TrimSpace(notificationANSISequencePattern.ReplaceAllString(raw, ""))
	if cleaned == "" {
		return pipelineNotificationLogEntry{}, false
	}
	if objectStart := strings.Index(cleaned, "{"); objectStart >= 0 {
		var payload map[string]any
		if json.Unmarshal([]byte(cleaned[objectStart:]), &payload) == nil {
			level := strings.ToLower(notificationLogString(payload, "level"))
			status := strings.ToLower(notificationLogString(payload, "status"))
			if level != "warn" && level != "error" && level != "fatal" && level != "panic" && status != "failure" {
				return pipelineNotificationLogEntry{}, false
			}
			message := notificationLogString(payload, "message")
			detail := notificationLogString(payload, "error")
			text := message
			if detail != "" && !strings.Contains(strings.ToLower(message), strings.ToLower(detail)) {
				if text != "" {
					text += ": "
				}
				text += detail
			}
			if text == "" {
				return pipelineNotificationLogEntry{}, false
			}
			return pipelineNotificationLogEntry{
				Text: sanitizePipelineNotificationLogText(text),
				Step: notificationLogString(payload, "step"),
				Task: notificationLogString(payload, "task"),
			}, true
		}
	}
	lower := strings.ToLower(cleaned)
	if !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") && !strings.Contains(lower, "fatal") &&
		!strings.Contains(lower, "panic") && !strings.Contains(lower, "exception") && !strings.Contains(lower, "exit code") &&
		!strings.Contains(lower, "denied") && !strings.Contains(lower, "timeout") {
		return pipelineNotificationLogEntry{}, false
	}
	return pipelineNotificationLogEntry{Text: sanitizePipelineNotificationLogText(cleaned)}, true
}

func notificationLogString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func sanitizePipelineNotificationLogText(value string) string {
	value = notificationBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = notificationSecretPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > pipelineNotificationLogLineMaxRunes {
		value = string(runes[:pipelineNotificationLogLineMaxRunes]) + "..."
	}
	return value
}

func (a *App) renderPipelineNotificationMail(notificationCtx pipelineNotificationContext, eventType string) (notificationMailMessage, error) {
	branding := a.pipelineNotificationBranding()
	progress := summarizePipelineNotificationProgress(notificationCtx.Steps)
	view := buildPipelineNotificationMailView(notificationCtx, eventType, branding, progress)

	var htmlBody bytes.Buffer
	if err := pipelineNotificationHTMLTemplate.Execute(&htmlBody, view); err != nil {
		return notificationMailMessage{}, fmt.Errorf("failed to render pipeline notification email: %w", err)
	}
	return notificationMailMessage{
		FromName: branding.Name,
		Subject:  pipelineNotificationSubject(notificationCtx, eventType),
		TextBody: buildPipelineNotificationTextBody(view),
		HTMLBody: htmlBody.String(),
	}, nil
}

func (a *App) pipelineNotificationBranding() pipelineNotificationBranding {
	branding := pipelineNotificationBranding{Name: "NopsAI"}
	if a == nil || a.cfg == nil {
		return branding
	}
	cfg := a.getConfigSnapshot()
	publicURL := normalizeNotificationHTTPURL(cfg.PublicURL)
	websiteURL := normalizeNotificationHTTPURL(cfg.NotificationMailWebsiteURL)
	if websiteURL == "" {
		websiteURL = publicURL
	}
	logoURL := normalizeNotificationHTTPURL(cfg.NotificationMailLogoURL)
	if logoURL == "" && publicURL != "" {
		logoURL = strings.TrimRight(publicURL, "/") + "/brand/nopsai-logo-light.png"
	}
	return pipelineNotificationBranding{
		Name:       branding.Name,
		PublicURL:  publicURL,
		LogoURL:    logoURL,
		WebsiteURL: websiteURL,
		SupportURL: normalizeNotificationHTTPURL(cfg.NotificationMailSupportURL),
		Address:    strings.TrimSpace(cfg.NotificationMailFooterAddress),
	}
}

func normalizeNotificationHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.User != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func buildPipelineNotificationMailView(notificationCtx pipelineNotificationContext, eventType string, branding pipelineNotificationBranding, progress pipelineNotificationProgress) pipelineNotificationMailView {
	statusLabel, headline, statusBackground, statusForeground := pipelineNotificationPresentation(notificationCtx.Status, eventType)
	pipeline := pipelineNotificationDisplayName(notificationCtx)
	failureLocation := pipelineNotificationFailureLocationLabel(notificationCtx.FailureStep, notificationCtx.FailureTask)
	repository := strings.Trim(strings.Join([]string{notificationCtx.RepoOwner, notificationCtx.RepoName}, "/"), "/")
	repositoryURL := normalizeNotificationHTTPURL(notificationCtx.RepoURL)
	commitURL := normalizeNotificationHTTPURL(notificationCtx.GitCommitURL)
	runURL := pipelineNotificationRunURL(branding.PublicURL, notificationCtx.GroupID, notificationCtx.RunID)

	summary := fmt.Sprintf("%s is %s.", pipeline, strings.ToLower(statusLabel))
	if failureLocation != "" && statusLabel == "FAILED" {
		summary = fmt.Sprintf("Failure detected in %s after %s.", failureLocation, firstNonEmptyString(notificationCtx.Duration, "an unknown duration"))
	} else if notificationCtx.Duration != "" {
		summary = fmt.Sprintf("%s after %s.", headline, notificationCtx.Duration)
	}

	steps := make([]pipelineNotificationStepView, 0, len(notificationCtx.Steps))
	for _, step := range notificationCtx.Steps {
		color, background := pipelineNotificationStepColors(step.Status)
		taskSummary := "No task records"
		if step.TaskTotal > 0 {
			taskSummary = fmt.Sprintf("%d of %d tasks passed", step.TaskPassed, step.TaskTotal)
		}
		steps = append(steps, pipelineNotificationStepView{
			Name:        step.Name,
			Status:      strings.ToUpper(strings.ReplaceAll(step.Status, "_", " ")),
			TaskSummary: taskSummary,
			Color:       color,
			Background:  background,
		})
	}

	commit := strings.TrimSpace(notificationCtx.GitCommitSHA)
	if len(commit) > 12 {
		commit = commit[:12]
	}
	startedAt := ""
	if !notificationCtx.StartedAt.IsZero() {
		startedAt = notificationCtx.StartedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	progressLabel := ""
	if progress.Total > 0 {
		progressLabel = fmt.Sprintf("%d of %d steps passed", progress.Passed, progress.Total)
	}
	preview := strings.TrimSpace(strings.Join([]string{headline, pipeline, failureLocation, progressLabel}, " - "))

	return pipelineNotificationMailView{
		PreviewText:      preview,
		Brand:            branding,
		StatusLabel:      statusLabel,
		StatusBackground: statusBackground,
		StatusForeground: statusForeground,
		Headline:         headline,
		Summary:          summary,
		Pipeline:         pipeline,
		Progress:         progress,
		ProgressLabel:    progressLabel,
		FailureLocation:  failureLocation,
		FailureReason:    strings.TrimSpace(notificationCtx.FailureReason),
		Steps:            steps,
		Logs:             notificationCtx.LogExcerpt,
		RunID:            notificationCtx.RunID,
		GroupPath:        notificationCtx.GroupPath,
		Repository:       repository,
		Branch:           notificationBranchLabel(notificationCtx.GitRef),
		Commit:           commit,
		Trigger:          strings.ReplaceAll(strings.TrimSpace(notificationCtx.TriggerSource), "_", " "),
		Duration:         notificationCtx.Duration,
		StartedAt:        startedAt,
		RunURL:           runURL,
		RepositoryURL:    repositoryURL,
		CommitURL:        commitURL,
		WebsiteDisplay:   notificationURLDisplay(branding.WebsiteURL),
		SupportDisplay:   notificationURLDisplay(branding.SupportURL),
	}
}

func pipelineNotificationPresentation(status, eventType string) (string, string, template.CSS, template.CSS) {
	normalized := runquery.NormalizeRunDetailStatus(firstNonEmptyString(status, eventType))
	switch normalized {
	case "success":
		return "SUCCEEDED", "Pipeline succeeded", template.CSS("#16794b"), template.CSS("#ffffff")
	case "failure":
		return "FAILED", "Pipeline failed", template.CSS("#b42318"), template.CSS("#ffffff")
	case "cancelled":
		return "CANCELLED", "Pipeline cancelled", template.CSS("#b54708"), template.CSS("#ffffff")
	case "waiting_approval":
		return "ACTION REQUIRED", "Pipeline needs approval", template.CSS("#b54708"), template.CSS("#ffffff")
	case "rejected":
		return "REJECTED", "Pipeline approval rejected", template.CSS("#b42318"), template.CSS("#ffffff")
	case "running":
		return "RUNNING", "Pipeline is running", template.CSS("#175cd3"), template.CSS("#ffffff")
	default:
		return strings.ToUpper(strings.ReplaceAll(normalized, "_", " ")), "Pipeline update", template.CSS("#344054"), template.CSS("#ffffff")
	}
}

func pipelineNotificationStepColors(status string) (template.CSS, template.CSS) {
	switch runquery.NormalizeRunDetailStatus(status) {
	case "success":
		return template.CSS("#067647"), template.CSS("#ecfdf3")
	case "failure", "rejected":
		return template.CSS("#b42318"), template.CSS("#fef3f2")
	case "running":
		return template.CSS("#175cd3"), template.CSS("#eff8ff")
	case "cancelled":
		return template.CSS("#b54708"), template.CSS("#fffaeb")
	default:
		return template.CSS("#475467"), template.CSS("#f2f4f7")
	}
}

func summarizePipelineNotificationProgress(steps []pipelineNotificationStep) pipelineNotificationProgress {
	progress := pipelineNotificationProgress{Total: len(steps)}
	for _, step := range steps {
		switch runquery.NormalizeRunDetailStatus(step.Status) {
		case "success":
			progress.Passed++
		case "failure", "rejected", "failure (ignored)":
			progress.Failed++
		case "running", "waiting_approval":
			progress.Running++
		case "skipped", "cancelled":
			progress.Skipped++
		default:
			progress.Pending++
		}
	}
	return progress
}

func pipelineNotificationDisplayName(notificationCtx pipelineNotificationContext) string {
	name := strings.Trim(strings.TrimSpace(notificationCtx.PipelineName), "/")
	pipelinePath := strings.Trim(strings.TrimSpace(notificationCtx.PipelinePath), "/")
	switch {
	case pipelinePath == "":
		return firstNonEmptyString(name, "pipeline")
	case name == "" || pipelinePath == name || strings.HasSuffix(pipelinePath, "/"+name):
		return pipelinePath
	default:
		return pipelinePath + "/" + name
	}
}

func pipelineNotificationFailureLocationLabel(step, task string) string {
	step = strings.TrimSpace(step)
	task = strings.TrimSpace(task)
	switch {
	case step != "" && task != "":
		return step + " / " + task
	case step != "":
		return step
	case task != "":
		return task
	default:
		return ""
	}
}

func pipelineNotificationRunURL(publicURL string, groupID int, runID string) string {
	publicURL = normalizeNotificationHTTPURL(publicURL)
	if publicURL == "" || strings.TrimSpace(runID) == "" {
		return ""
	}
	target, err := url.Parse(publicURL + "/#/pipelineruns/main")
	if err != nil {
		return ""
	}
	fragment, err := url.Parse(target.Fragment)
	if err != nil {
		return ""
	}
	query := fragment.Query()
	if groupID > 0 {
		query.Set("group", fmt.Sprintf("%d", groupID))
	}
	query.Set("run", runID)
	fragment.RawQuery = query.Encode()
	target.Fragment = fragment.String()
	return target.String()
}

func notificationURLDisplay(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	display := parsed.Host + strings.TrimRight(parsed.Path, "/")
	return strings.TrimSpace(display)
}

func buildPipelineNotificationTextBody(view pipelineNotificationMailView) string {
	var lines []string
	lines = append(lines, view.StatusLabel+": "+view.Pipeline, view.Summary)
	if view.ProgressLabel != "" {
		lines = append(lines, "", "Progress: "+view.ProgressLabel)
	}
	if view.FailureLocation != "" {
		lines = append(lines, "Failure location: "+view.FailureLocation)
	}
	if view.FailureReason != "" {
		lines = append(lines, "Failure reason: "+view.FailureReason)
	}
	if len(view.Steps) > 0 {
		lines = append(lines, "", "Steps:")
		for _, step := range view.Steps {
			lines = append(lines, fmt.Sprintf("- %s: %s (%s)", step.Name, step.Status, step.TaskSummary))
		}
	}
	if len(view.Logs) > 0 {
		lines = append(lines, "", "Relevant errors:")
		for _, entry := range view.Logs {
			location := pipelineNotificationFailureLocationLabel(entry.Step, entry.Task)
			if location != "" {
				lines = append(lines, fmt.Sprintf("- [%s] %s", location, entry.Text))
			} else {
				lines = append(lines, "- "+entry.Text)
			}
		}
	}
	lines = append(lines, "", "Run details:")
	for _, item := range []struct{ label, value string }{
		{"Run ID", view.RunID},
		{"Group", view.GroupPath},
		{"Repository", view.Repository},
		{"Branch", view.Branch},
		{"Commit", view.Commit},
		{"Trigger", view.Trigger},
		{"Started", view.StartedAt},
		{"Duration", view.Duration},
	} {
		if item.value != "" {
			lines = append(lines, item.label+": "+item.value)
		}
	}
	for _, item := range []struct{ label, value string }{
		{"View run", view.RunURL},
		{"Repository", view.RepositoryURL},
		{"Commit", view.CommitURL},
		{"Website", view.Brand.WebsiteURL},
		{"Support", view.Brand.SupportURL},
	} {
		if item.value != "" {
			lines = append(lines, item.label+": "+item.value)
		}
	}
	lines = append(lines, "", "Sent automatically by "+view.Brand.Name+".")
	if view.Brand.Address != "" {
		lines = append(lines, view.Brand.Address)
	}
	return strings.Join(lines, "\n")
}

var pipelineNotificationHTMLTemplate = template.Must(template.New("pipeline-notification").Funcs(template.FuncMap{
	"add": func(values ...int) int {
		total := 0
		for _, value := range values {
			total += value
		}
		return total
	},
}).Parse(`<!doctype html>
<html>
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
    <title>{{.Headline}}</title>
  </head>
  <body style="margin:0;padding:0;background:#f2f4f7;color:#101828;font-family:Inter,Segoe UI,Arial,sans-serif;">
    <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.PreviewText}}</div>
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f2f4f7;">
      <tr>
        <td align="center" style="padding:32px 12px;">
          <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:680px;background:#ffffff;border:1px solid #e4e7ec;border-radius:16px;overflow:hidden;box-shadow:0 4px 14px rgba(16,24,40,.06);">
            <tr>
              <td style="padding:20px 28px;border-bottom:1px solid #eaecf0;">
                {{if .Brand.LogoURL}}<img src="{{.Brand.LogoURL}}" width="112" alt="{{.Brand.Name}}" style="display:block;width:112px;height:auto;border:0;">{{else}}<div style="font-size:23px;line-height:28px;font-weight:800;letter-spacing:-.5px;color:#101828;">{{.Brand.Name}}</div>{{end}}
              </td>
            </tr>
            <tr>
              <td style="padding:28px;background:{{.StatusBackground}};color:{{.StatusForeground}};">
                <div style="font-size:12px;line-height:18px;font-weight:800;letter-spacing:1.3px;">{{.StatusLabel}}</div>
                <h1 style="margin:7px 0 5px;font-size:28px;line-height:35px;font-weight:750;">{{.Headline}}</h1>
                <div style="font-size:17px;line-height:25px;font-weight:650;">{{.Pipeline}}</div>
                <p style="margin:10px 0 0;font-size:14px;line-height:22px;opacity:.92;">{{.Summary}}</p>
              </td>
            </tr>
            <tr>
              <td style="padding:26px 28px 6px;">
                {{if .ProgressLabel}}
                <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
                  <tr>
                    <td width="25%" style="padding:0 5px 0 0;"><div style="padding:13px 10px;background:#f9fafb;border:1px solid #eaecf0;border-radius:10px;text-align:center;"><div style="font-size:22px;font-weight:750;">{{.Progress.Total}}</div><div style="font-size:11px;color:#667085;">TOTAL STEPS</div></div></td>
                    <td width="25%" style="padding:0 5px;"><div style="padding:13px 10px;background:#ecfdf3;border:1px solid #abefc6;border-radius:10px;text-align:center;"><div style="font-size:22px;font-weight:750;color:#067647;">{{.Progress.Passed}}</div><div style="font-size:11px;color:#067647;">PASSED</div></div></td>
                    <td width="25%" style="padding:0 5px;"><div style="padding:13px 10px;background:#fef3f2;border:1px solid #fecdca;border-radius:10px;text-align:center;"><div style="font-size:22px;font-weight:750;color:#b42318;">{{.Progress.Failed}}</div><div style="font-size:11px;color:#b42318;">FAILED</div></div></td>
                    <td width="25%" style="padding:0 0 0 5px;"><div style="padding:13px 10px;background:#f2f4f7;border:1px solid #eaecf0;border-radius:10px;text-align:center;"><div style="font-size:22px;font-weight:750;color:#475467;">{{add .Progress.Running .Progress.Pending .Progress.Skipped}}</div><div style="font-size:11px;color:#667085;">REMAINING</div></div></td>
                  </tr>
                </table>
                {{end}}
                {{if .FailureLocation}}
                <div style="margin-top:20px;padding:16px 18px;background:#fff7ed;border:1px solid #fed7aa;border-left:4px solid #f97316;border-radius:10px;">
                  <div style="font-size:11px;line-height:16px;font-weight:800;letter-spacing:.7px;color:#9a3412;">FAILURE LOCATION</div>
                  <div style="margin-top:4px;font-size:16px;line-height:23px;font-weight:700;color:#7c2d12;">{{.FailureLocation}}</div>
                  {{if .FailureReason}}<div style="margin-top:7px;font-size:13px;line-height:20px;color:#9a3412;">{{.FailureReason}}</div>{{end}}
                </div>
                {{end}}
              </td>
            </tr>
            {{if .Steps}}
            <tr>
              <td style="padding:18px 28px 4px;">
                <h2 style="margin:0 0 10px;font-size:16px;line-height:24px;">Pipeline steps</h2>
                <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="border:1px solid #eaecf0;border-radius:10px;overflow:hidden;">
                  {{range .Steps}}<tr>
                    <td style="padding:12px 14px;border-bottom:1px solid #eaecf0;">
                      <div style="font-size:14px;line-height:20px;font-weight:650;">{{.Name}}</div>
                      <div style="font-size:12px;line-height:18px;color:#667085;">{{.TaskSummary}}</div>
                    </td>
                    <td align="right" style="padding:12px 14px;border-bottom:1px solid #eaecf0;">
                      <span style="display:inline-block;padding:4px 9px;border-radius:999px;background:{{.Background}};color:{{.Color}};font-size:11px;line-height:16px;font-weight:800;">{{.Status}}</span>
                    </td>
                  </tr>{{end}}
                </table>
              </td>
            </tr>
            {{end}}
            {{if .Logs}}
            <tr>
              <td style="padding:20px 28px 4px;">
                <h2 style="margin:0 0 10px;font-size:16px;line-height:24px;">Relevant errors</h2>
                <div style="padding:14px 16px;background:#101828;border-radius:10px;color:#d0d5dd;font-family:SFMono-Regular,Consolas,Liberation Mono,monospace;font-size:12px;line-height:19px;">
                  {{range .Logs}}<div style="margin:0 0 9px;word-break:break-word;">{{if or .Step .Task}}<span style="color:#fdb022;">[{{if .Step}}{{.Step}}{{end}}{{if and .Step .Task}} / {{end}}{{if .Task}}{{.Task}}{{end}}]</span> {{end}}{{.Text}}</div>{{end}}
                </div>
                <div style="margin-top:7px;font-size:11px;line-height:17px;color:#667085;">Only a short, redacted excerpt is included. Open the run for complete logs.</div>
              </td>
            </tr>
            {{end}}
            <tr>
              <td style="padding:20px 28px 8px;">
                <h2 style="margin:0 0 10px;font-size:16px;line-height:24px;">Run details</h2>
                <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="font-size:13px;line-height:20px;">
                  <tr><td style="padding:5px 12px 5px 0;color:#667085;">Run ID</td><td style="padding:5px 0;font-family:SFMono-Regular,Consolas,monospace;word-break:break-all;">{{.RunID}}</td></tr>
                  {{if .GroupPath}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Group</td><td style="padding:5px 0;">{{.GroupPath}}</td></tr>{{end}}
                  {{if .Repository}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Repository</td><td style="padding:5px 0;">{{.Repository}}</td></tr>{{end}}
                  {{if .Branch}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Branch</td><td style="padding:5px 0;">{{.Branch}}</td></tr>{{end}}
                  {{if .Commit}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Commit</td><td style="padding:5px 0;font-family:SFMono-Regular,Consolas,monospace;">{{.Commit}}</td></tr>{{end}}
                  {{if .Trigger}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Trigger</td><td style="padding:5px 0;text-transform:capitalize;">{{.Trigger}}</td></tr>{{end}}
                  {{if .StartedAt}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Started</td><td style="padding:5px 0;">{{.StartedAt}}</td></tr>{{end}}
                  {{if .Duration}}<tr><td style="padding:5px 12px 5px 0;color:#667085;">Duration</td><td style="padding:5px 0;">{{.Duration}}</td></tr>{{end}}
                </table>
              </td>
            </tr>
            {{if or .RunURL .RepositoryURL .CommitURL}}
            <tr>
              <td style="padding:18px 28px 28px;">
                {{if .RunURL}}<a href="{{.RunURL}}" style="display:inline-block;margin:0 8px 8px 0;padding:11px 17px;border-radius:8px;background:#175cd3;color:#ffffff;text-decoration:none;font-size:13px;font-weight:700;">View run</a>{{end}}
                {{if .RepositoryURL}}<a href="{{.RepositoryURL}}" style="display:inline-block;margin:0 8px 8px 0;padding:10px 16px;border:1px solid #d0d5dd;border-radius:8px;background:#ffffff;color:#344054;text-decoration:none;font-size:13px;font-weight:700;">Repository</a>{{end}}
                {{if .CommitURL}}<a href="{{.CommitURL}}" style="display:inline-block;margin:0 0 8px;padding:10px 16px;border:1px solid #d0d5dd;border-radius:8px;background:#ffffff;color:#344054;text-decoration:none;font-size:13px;font-weight:700;">Commit</a>{{end}}
              </td>
            </tr>
            {{end}}
            <tr>
              <td align="center" style="padding:24px 28px;background:#f9fafb;border-top:1px solid #eaecf0;color:#667085;font-size:12px;line-height:19px;">
                {{if .Brand.LogoURL}}<img src="{{.Brand.LogoURL}}" width="76" alt="{{.Brand.Name}}" style="display:block;margin:0 auto 10px;width:76px;height:auto;border:0;">{{else}}<div style="margin-bottom:8px;font-size:17px;font-weight:800;color:#344054;">{{.Brand.Name}}</div>{{end}}
                <div>Automated pipeline notification from {{.Brand.Name}}</div>
                {{if .Brand.Address}}<div>{{.Brand.Address}}</div>{{end}}
                {{if or .Brand.WebsiteURL .Brand.SupportURL}}<div style="margin-top:6px;">{{if .Brand.WebsiteURL}}<a href="{{.Brand.WebsiteURL}}" style="color:#175cd3;text-decoration:none;">{{.WebsiteDisplay}}</a>{{end}}{{if and .Brand.WebsiteURL .Brand.SupportURL}} &nbsp;|&nbsp; {{end}}{{if .Brand.SupportURL}}<a href="{{.Brand.SupportURL}}" style="color:#175cd3;text-decoration:none;">Support</a>{{end}}</div>{{end}}
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`))
