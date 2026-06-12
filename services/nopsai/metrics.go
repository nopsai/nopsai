package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"nopsai/pkg/proto"
)

var pipelineRunDurationBuckets = []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 7200}

type prometheusMetricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if a == nil || a.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	body, err := a.buildPrometheusMetrics(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to build Prometheus metrics")
		http.Error(w, "failed to build metrics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (a *App) buildPrometheusMetrics(ctx context.Context) (string, error) {
	var out strings.Builder
	writeMetricHelp(&out, "nopsai_pipeline_runs_total", "Pipeline runs by status, pipeline, group, repository, and trigger source.")
	writeMetricType(&out, "nopsai_pipeline_runs_total", "counter")
	if err := a.appendPipelineRunTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_run_duration_seconds", "Pipeline run duration in seconds.")
	writeMetricType(&out, "nopsai_pipeline_run_duration_seconds", "histogram")
	if err := a.appendPipelineRunDurationHistogram(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_run_queue_duration_seconds", "Pipeline run queue duration in seconds from creation to start.")
	writeMetricType(&out, "nopsai_pipeline_run_queue_duration_seconds", "histogram")
	if err := a.appendPipelineRunIntervalHistogram(ctx, &out, "nopsai_pipeline_run_queue_duration_seconds", "pr.started_at - pr.created_at", "pr.started_at IS NOT NULL AND pr.started_at >= pr.created_at"); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_run_end_to_end_duration_seconds", "Pipeline run end-to-end duration in seconds from creation to finish.")
	writeMetricType(&out, "nopsai_pipeline_run_end_to_end_duration_seconds", "histogram")
	if err := a.appendPipelineRunIntervalHistogram(ctx, &out, "nopsai_pipeline_run_end_to_end_duration_seconds", "pr.finished_at - pr.created_at", "pr.finished_at IS NOT NULL AND pr.finished_at >= pr.created_at"); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_run_failures_total", "Failed pipeline runs by reason, pipeline, group, repository, and trigger source.")
	writeMetricType(&out, "nopsai_pipeline_run_failures_total", "counter")
	if err := a.appendPipelineRunFailureTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_reruns_total", "Pipeline reruns by pipeline, group, repository, and trigger source.")
	writeMetricType(&out, "nopsai_pipeline_reruns_total", "counter")
	if err := a.appendPipelineRerunTotals(ctx, &out); err != nil {
		return "", err
	}
	for _, metric := range []struct {
		name   string
		status string
		help   string
	}{
		{name: "nopsai_pipeline_runs_active", status: "running", help: "Currently running pipeline runs."},
		{name: "nopsai_pipeline_runs_pending", status: "pending", help: "Currently pending pipeline runs."},
		{name: "nopsai_pipeline_runs_waiting_approval", status: "waiting_approval", help: "Pipeline runs waiting for approval."},
	} {
		writeMetricHelp(&out, metric.name, metric.help)
		writeMetricType(&out, metric.name, "gauge")
		if err := a.appendPipelineRunGauge(ctx, &out, metric.name, metric.status); err != nil {
			return "", err
		}
	}
	writeMetricHelp(&out, "nopsai_pipeline_steps_total", "Pipeline steps by status, step, and pipeline.")
	writeMetricType(&out, "nopsai_pipeline_steps_total", "counter")
	if err := a.appendPipelineStepTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_pipeline_tasks_total", "Pipeline tasks by status, task, step, and pipeline.")
	writeMetricType(&out, "nopsai_pipeline_tasks_total", "counter")
	if err := a.appendPipelineTaskTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_notifications_sent_total", "Sent notification deliveries by channel and event type.")
	writeMetricType(&out, "nopsai_notifications_sent_total", "counter")
	if err := a.appendNotificationDeliveryStatusTotals(ctx, &out, "nopsai_notifications_sent_total", "sent"); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_notifications_failed_total", "Failed notification deliveries by channel, event type, and reason.")
	writeMetricType(&out, "nopsai_notifications_failed_total", "counter")
	if err := a.appendNotificationDeliveryFailureTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_notification_delivery_duration_seconds", "Notification delivery duration in seconds.")
	writeMetricType(&out, "nopsai_notification_delivery_duration_seconds", "histogram")
	if err := a.appendNotificationDeliveryDurationHistogram(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_external_trigger_invocations_total", "External trigger invocations by trigger, status, event type, and caller type.")
	writeMetricType(&out, "nopsai_external_trigger_invocations_total", "counter")
	if err := a.appendExternalTriggerInvocationTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_external_trigger_invocation_failures_total", "Failed external trigger invocations by trigger and error reason.")
	writeMetricType(&out, "nopsai_external_trigger_invocation_failures_total", "counter")
	if err := a.appendExternalTriggerInvocationFailureTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_ai_tokens_total", "AI token usage by feature, provider, model, profile, and token type.")
	writeMetricType(&out, "nopsai_ai_tokens_total", "counter")
	if err := a.appendAIUsageTokenTotals(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_approval_wait_duration_seconds", "Pending approval wait duration in seconds.")
	writeMetricType(&out, "nopsai_approval_wait_duration_seconds", "histogram")
	if err := a.appendApprovalWaitDurationHistogram(ctx, &out); err != nil {
		return "", err
	}
	writeMetricHelp(&out, "nopsai_audit_events_total", "Audit events by provider, action, and result.")
	writeMetricType(&out, "nopsai_audit_events_total", "counter")
	if err := a.appendAuditEventTotals(ctx, &out); err != nil {
		return "", err
	}
	if status, err := a.fetchDispatcherStatus(ctx); err == nil {
		appendRunnerPrometheusMetrics(&out, status)
	}
	return out.String(), nil
}

func (a *App) appendPipelineRunTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, pipelineRunMetricsQuery(""))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		labels, value, err := scanPipelineRunMetricRow(rows)
		if err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_pipeline_runs_total", labels, value)
	}
	return rows.Err()
}

func (a *App) appendPipelineRunGauge(ctx context.Context, out *strings.Builder, metricName, status string) error {
	rows, err := a.db.Query(ctx, pipelineRunMetricsQuery("WHERE LOWER(pr.status) = LOWER($1)"), status)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		labels, value, err := scanPipelineRunMetricRow(rows)
		if err != nil {
			return err
		}
		delete(labels, "status")
		writeMetricLine(out, metricName, labels, value)
	}
	return rows.Err()
}

func pipelineRunMetricsQuery(where string) string {
	if strings.TrimSpace(where) != "" {
		where = "\n" + where
	}
	return `
		SELECT
			COALESCE(pr.status, ''),
			COALESCE(pr.pipeline_path, ''),
			COALESCE(pr.pipeline_name, ''),
			COALESCE(g.name, ''),
			CASE
				WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
					THEN pr.git_repo_owner || '/' || pr.git_repo_name
				ELSE COALESCE(pr.git_repo_name, '')
			END,
			COALESCE(pr.trigger_source, ''),
			COUNT(*)::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id` + where + `
		GROUP BY 1,2,3,4,5,6
		ORDER BY 1,2,3,4,5,6`
}

func scanPipelineRunMetricRow(row interface{ Scan(dest ...any) error }) (map[string]string, float64, error) {
	var status, path, pipeline, group, repo, triggerSource string
	var value float64
	if err := row.Scan(&status, &path, &pipeline, &group, &repo, &triggerSource, &value); err != nil {
		return nil, 0, err
	}
	return map[string]string{
		"status":         normalizeMetricLabel(status),
		"pipeline":       normalizeMetricLabel(pipeline),
		"path":           normalizeMetricLabel(path),
		"group":          normalizeMetricLabel(group),
		"repo":           normalizeMetricLabel(repo),
		"trigger_source": normalizeMetricLabel(triggerSource),
	}, value, nil
}

func (a *App) appendPipelineRunDurationHistogram(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT
			COALESCE(pr.status, ''),
			COALESCE(pr.pipeline_path, ''),
			COALESCE(pr.pipeline_name, ''),
			COALESCE(g.name, ''),
			CASE
				WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
					THEN pr.git_repo_owner || '/' || pr.git_repo_name
				ELSE COALESCE(pr.git_repo_name, '')
			END,
			COALESCE(pr.trigger_source, ''),
			EXTRACT(EPOCH FROM pr.finished_at - pr.started_at)::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE pr.started_at IS NOT NULL
		  AND pr.finished_at IS NOT NULL
		  AND pr.finished_at >= pr.started_at
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type histogram struct {
		labels map[string]string
		counts []int
		count  int
		sum    float64
	}
	histograms := map[string]*histogram{}
	for rows.Next() {
		var status, path, pipeline, group, repo, triggerSource string
		var duration sql.NullFloat64
		if err := rows.Scan(&status, &path, &pipeline, &group, &repo, &triggerSource, &duration); err != nil {
			return err
		}
		if !duration.Valid {
			continue
		}
		labels := map[string]string{
			"status":         normalizeMetricLabel(status),
			"pipeline":       normalizeMetricLabel(pipeline),
			"path":           normalizeMetricLabel(path),
			"group":          normalizeMetricLabel(group),
			"repo":           normalizeMetricLabel(repo),
			"trigger_source": normalizeMetricLabel(triggerSource),
		}
		key := metricLabelKey(labels)
		item := histograms[key]
		if item == nil {
			item = &histogram{labels: labels, counts: make([]int, len(pipelineRunDurationBuckets))}
			histograms[key] = item
		}
		item.count++
		item.sum += duration.Float64
		for idx, bucket := range pipelineRunDurationBuckets {
			if duration.Float64 <= bucket {
				item.counts[idx]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keys := make([]string, 0, len(histograms))
	for key := range histograms {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := histograms[key]
		for idx, bucket := range pipelineRunDurationBuckets {
			labels := cloneMetricLabels(item.labels)
			labels["le"] = formatPrometheusFloat(bucket)
			writeMetricLine(out, "nopsai_pipeline_run_duration_seconds_bucket", labels, float64(item.counts[idx]))
		}
		infLabels := cloneMetricLabels(item.labels)
		infLabels["le"] = "+Inf"
		writeMetricLine(out, "nopsai_pipeline_run_duration_seconds_bucket", infLabels, float64(item.count))
		writeMetricLine(out, "nopsai_pipeline_run_duration_seconds_sum", item.labels, item.sum)
		writeMetricLine(out, "nopsai_pipeline_run_duration_seconds_count", item.labels, float64(item.count))
	}
	return nil
}

func (a *App) appendPipelineRunIntervalHistogram(ctx context.Context, out *strings.Builder, metricName, expression, predicate string) error {
	rows, err := a.db.Query(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(pr.status, ''),
			COALESCE(pr.pipeline_path, ''),
			COALESCE(pr.pipeline_name, ''),
			COALESCE(g.name, ''),
			CASE
				WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
					THEN pr.git_repo_owner || '/' || pr.git_repo_name
				ELSE COALESCE(pr.git_repo_name, '')
			END,
			COALESCE(pr.trigger_source, ''),
			EXTRACT(EPOCH FROM %s)::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE %s
	`, expression, predicate))
	if err != nil {
		return err
	}
	defer rows.Close()

	type histogram struct {
		labels map[string]string
		counts []int
		count  int
		sum    float64
	}
	histograms := map[string]*histogram{}
	for rows.Next() {
		var status, path, pipeline, group, repo, triggerSource string
		var duration sql.NullFloat64
		if err := rows.Scan(&status, &path, &pipeline, &group, &repo, &triggerSource, &duration); err != nil {
			return err
		}
		if !duration.Valid {
			continue
		}
		labels := map[string]string{
			"status":         normalizeMetricLabel(status),
			"pipeline":       normalizeMetricLabel(pipeline),
			"path":           normalizeMetricLabel(path),
			"group":          normalizeMetricLabel(group),
			"repo":           normalizeMetricLabel(repo),
			"trigger_source": normalizeMetricLabel(triggerSource),
		}
		key := metricLabelKey(labels)
		item := histograms[key]
		if item == nil {
			item = &histogram{labels: labels, counts: make([]int, len(pipelineRunDurationBuckets))}
			histograms[key] = item
		}
		item.count++
		item.sum += duration.Float64
		for idx, bucket := range pipelineRunDurationBuckets {
			if duration.Float64 <= bucket {
				item.counts[idx]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	keys := make([]string, 0, len(histograms))
	for key := range histograms {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := histograms[key]
		for idx, bucket := range pipelineRunDurationBuckets {
			labels := cloneMetricLabels(item.labels)
			labels["le"] = formatPrometheusFloat(bucket)
			writeMetricLine(out, metricName+"_bucket", labels, float64(item.counts[idx]))
		}
		infLabels := cloneMetricLabels(item.labels)
		infLabels["le"] = "+Inf"
		writeMetricLine(out, metricName+"_bucket", infLabels, float64(item.count))
		writeMetricLine(out, metricName+"_sum", item.labels, item.sum)
		writeMetricLine(out, metricName+"_count", item.labels, float64(item.count))
	}
	return nil
}

func (a *App) appendPipelineRunFailureTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT
			COALESCE(pr.pipeline_path, ''),
			COALESCE(pr.pipeline_name, ''),
			COALESCE(g.name, ''),
			CASE
				WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
					THEN pr.git_repo_owner || '/' || pr.git_repo_name
				ELSE COALESCE(pr.git_repo_name, '')
			END,
			COALESCE(pr.trigger_source, ''),
			COALESCE(NULLIF(pr.failure_reason, ''), 'unknown'),
			COUNT(*)::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE LOWER(pr.status) IN ('failure', 'failed')
		GROUP BY 1,2,3,4,5,6
		ORDER BY 1,2,3,4,5,6
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path, pipeline, group, repo, triggerSource, reason string
		var count float64
		if err := rows.Scan(&path, &pipeline, &group, &repo, &triggerSource, &reason, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_pipeline_run_failures_total", map[string]string{
			"pipeline":       normalizeMetricLabel(pipeline),
			"path":           normalizeMetricLabel(path),
			"group":          normalizeMetricLabel(group),
			"repo":           normalizeMetricLabel(repo),
			"trigger_source": normalizeMetricLabel(triggerSource),
			"reason":         normalizeMetricLabel(reason),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendPipelineRerunTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT
			COALESCE(pr.pipeline_path, ''),
			COALESCE(pr.pipeline_name, ''),
			COALESCE(g.name, ''),
			CASE
				WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
					THEN pr.git_repo_owner || '/' || pr.git_repo_name
				ELSE COALESCE(pr.git_repo_name, '')
			END,
			COALESCE(pr.trigger_source, ''),
			COUNT(*)::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE pr.parent_run_id IS NOT NULL
		GROUP BY 1,2,3,4,5
		ORDER BY 1,2,3,4,5
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path, pipeline, group, repo, triggerSource string
		var count float64
		if err := rows.Scan(&path, &pipeline, &group, &repo, &triggerSource, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_pipeline_reruns_total", map[string]string{
			"pipeline":       normalizeMetricLabel(pipeline),
			"path":           normalizeMetricLabel(path),
			"group":          normalizeMetricLabel(group),
			"repo":           normalizeMetricLabel(repo),
			"trigger_source": normalizeMetricLabel(triggerSource),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendPipelineStepTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(sr.status, ''), COALESCE(sr.name, ''),
		       COALESCE(pr.pipeline_path, ''), COALESCE(pr.pipeline_name, ''),
		       COUNT(*)::float8
		FROM step_runs sr
		JOIN pipeline_runs pr ON pr.run_id = sr.run_id
		GROUP BY 1,2,3,4
		ORDER BY 1,2,3,4
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status, step, path, pipeline string
		var count float64
		if err := rows.Scan(&status, &step, &path, &pipeline, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_pipeline_steps_total", map[string]string{
			"status":   normalizeMetricLabel(status),
			"step":     normalizeMetricLabel(step),
			"pipeline": normalizeMetricLabel(pipeline),
			"path":     normalizeMetricLabel(path),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendPipelineTaskTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(tr.status, ''), COALESCE(tr.step_name, ''), COALESCE(tr.task_name, ''),
		       COALESCE(pr.pipeline_path, ''), COALESCE(pr.pipeline_name, ''),
		       COUNT(*)::float8
		FROM task_runs tr
		JOIN pipeline_runs pr ON pr.run_id = tr.run_id
		GROUP BY 1,2,3,4,5
		ORDER BY 1,2,3,4,5
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status, step, task, path, pipeline string
		var count float64
		if err := rows.Scan(&status, &step, &task, &path, &pipeline, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_pipeline_tasks_total", map[string]string{
			"status":   normalizeMetricLabel(status),
			"step":     normalizeMetricLabel(step),
			"task":     normalizeMetricLabel(task),
			"pipeline": normalizeMetricLabel(pipeline),
			"path":     normalizeMetricLabel(path),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendNotificationDeliveryStatusTotals(ctx context.Context, out *strings.Builder, metricName, status string) error {
	rows, err := a.db.Query(ctx, `
		SELECT channel, event_type, COUNT(*)::float8
		FROM notification_deliveries
		WHERE LOWER(status) = LOWER($1)
		GROUP BY channel, event_type
		ORDER BY channel, event_type
	`, status)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var channel, eventType string
		var count float64
		if err := rows.Scan(&channel, &eventType, &count); err != nil {
			return err
		}
		writeMetricLine(out, metricName, map[string]string{
			"channel":    normalizeMetricLabel(channel),
			"event_type": normalizeMetricLabel(eventType),
			"result":     normalizeMetricLabel(status),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendNotificationDeliveryFailureTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT channel, event_type, COALESCE(NULLIF(error, ''), 'unknown'), COUNT(*)::float8
		FROM notification_deliveries
		WHERE LOWER(status) = 'failed'
		GROUP BY channel, event_type, COALESCE(NULLIF(error, ''), 'unknown')
		ORDER BY channel, event_type, 3
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var channel, eventType, reason string
		var count float64
		if err := rows.Scan(&channel, &eventType, &reason, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_notifications_failed_total", map[string]string{
			"channel":    normalizeMetricLabel(channel),
			"event_type": normalizeMetricLabel(eventType),
			"reason":     normalizeMetricLabel(reason),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendNotificationDeliveryDurationHistogram(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT channel, EXTRACT(EPOCH FROM sent_at - created_at)::float8
		FROM notification_deliveries
		WHERE sent_at IS NOT NULL
		  AND sent_at >= created_at
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	buckets := []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}
	type histogram struct {
		channel string
		counts  []int
		count   int
		sum     float64
	}
	histograms := map[string]*histogram{}
	for rows.Next() {
		var channel string
		var duration sql.NullFloat64
		if err := rows.Scan(&channel, &duration); err != nil {
			return err
		}
		if !duration.Valid {
			continue
		}
		channel = normalizeMetricLabel(channel)
		item := histograms[channel]
		if item == nil {
			item = &histogram{channel: channel, counts: make([]int, len(buckets))}
			histograms[channel] = item
		}
		item.count++
		item.sum += duration.Float64
		for idx, bucket := range buckets {
			if duration.Float64 <= bucket {
				item.counts[idx]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	channels := make([]string, 0, len(histograms))
	for channel := range histograms {
		channels = append(channels, channel)
	}
	sort.Strings(channels)
	for _, channel := range channels {
		item := histograms[channel]
		base := map[string]string{"channel": item.channel}
		for idx, bucket := range buckets {
			labels := cloneMetricLabels(base)
			labels["le"] = formatPrometheusFloat(bucket)
			writeMetricLine(out, "nopsai_notification_delivery_duration_seconds_bucket", labels, float64(item.counts[idx]))
		}
		infLabels := cloneMetricLabels(base)
		infLabels["le"] = "+Inf"
		writeMetricLine(out, "nopsai_notification_delivery_duration_seconds_bucket", infLabels, float64(item.count))
		writeMetricLine(out, "nopsai_notification_delivery_duration_seconds_sum", base, item.sum)
		writeMetricLine(out, "nopsai_notification_delivery_duration_seconds_count", base, float64(item.count))
	}
	return nil
}

func (a *App) appendExternalTriggerInvocationTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT trigger_id, status, event_type, caller_type, COUNT(*)::float8
		FROM external_trigger_invocations
		GROUP BY trigger_id, status, event_type, caller_type
		ORDER BY trigger_id, status, event_type, caller_type
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var triggerID, status, eventType, callerType string
		var count float64
		if err := rows.Scan(&triggerID, &status, &eventType, &callerType, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_external_trigger_invocations_total", map[string]string{
			"trigger_id":  normalizeMetricLabel(triggerID),
			"status":      normalizeMetricLabel(status),
			"event_type":  normalizeMetricLabel(eventType),
			"caller_type": normalizeMetricLabel(callerType),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendExternalTriggerInvocationFailureTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT trigger_id, COALESCE(NULLIF(error, ''), 'unknown'), COUNT(*)::float8
		FROM external_trigger_invocations
		WHERE LOWER(status) IN ('failed', 'failure')
		GROUP BY trigger_id, COALESCE(NULLIF(error, ''), 'unknown')
		ORDER BY trigger_id, 2
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var triggerID, reason string
		var count float64
		if err := rows.Scan(&triggerID, &reason, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_external_trigger_invocation_failures_total", map[string]string{
			"trigger_id": normalizeMetricLabel(triggerID),
			"reason":     normalizeMetricLabel(reason),
		}, count)
	}
	return rows.Err()
}

func (a *App) appendAIUsageTokenTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT feature, provider, model, llm_profile,
		       COALESCE(SUM(prompt_tokens), 0)::float8,
		       COALESCE(SUM(completion_tokens), 0)::float8,
		       COALESCE(SUM(total_tokens), 0)::float8
		FROM ai_usage_events
		GROUP BY feature, provider, model, llm_profile
		ORDER BY feature, provider, model, llm_profile
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var feature, provider, modelName, profile string
		var promptTokens, completionTokens, totalTokens float64
		if err := rows.Scan(&feature, &provider, &modelName, &profile, &promptTokens, &completionTokens, &totalTokens); err != nil {
			return err
		}
		base := map[string]string{
			"feature":     normalizeMetricLabel(feature),
			"provider":    normalizeMetricLabel(provider),
			"model":       normalizeMetricLabel(modelName),
			"llm_profile": normalizeMetricLabel(profile),
		}
		for tokenType, value := range map[string]float64{
			"prompt":     promptTokens,
			"completion": completionTokens,
			"total":      totalTokens,
		} {
			labels := cloneMetricLabels(base)
			labels["token_type"] = tokenType
			writeMetricLine(out, "nopsai_ai_tokens_total", labels, value)
		}
	}
	return rows.Err()
}

func (a *App) appendApprovalWaitDurationHistogram(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT approval_type, EXTRACT(EPOCH FROM NOW() - requested_at)::float8
		FROM pipeline_approvals
		WHERE status = 'pending'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	buckets := []float64{60, 300, 900, 1800, 3600, 7200, 14400, 28800, 86400}
	type histogram struct {
		approvalType string
		counts       []int
		count        int
		sum          float64
	}
	histograms := map[string]*histogram{}
	for rows.Next() {
		var approvalType string
		var duration sql.NullFloat64
		if err := rows.Scan(&approvalType, &duration); err != nil {
			return err
		}
		if !duration.Valid {
			continue
		}
		approvalType = normalizeMetricLabel(approvalType)
		item := histograms[approvalType]
		if item == nil {
			item = &histogram{approvalType: approvalType, counts: make([]int, len(buckets))}
			histograms[approvalType] = item
		}
		item.count++
		item.sum += duration.Float64
		for idx, bucket := range buckets {
			if duration.Float64 <= bucket {
				item.counts[idx]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	keys := make([]string, 0, len(histograms))
	for key := range histograms {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := histograms[key]
		base := map[string]string{"approval_type": item.approvalType}
		for idx, bucket := range buckets {
			labels := cloneMetricLabels(base)
			labels["le"] = formatPrometheusFloat(bucket)
			writeMetricLine(out, "nopsai_approval_wait_duration_seconds_bucket", labels, float64(item.counts[idx]))
		}
		infLabels := cloneMetricLabels(base)
		infLabels["le"] = "+Inf"
		writeMetricLine(out, "nopsai_approval_wait_duration_seconds_bucket", infLabels, float64(item.count))
		writeMetricLine(out, "nopsai_approval_wait_duration_seconds_sum", base, item.sum)
		writeMetricLine(out, "nopsai_approval_wait_duration_seconds_count", base, float64(item.count))
	}
	return nil
}

func (a *App) appendAuditEventTotals(ctx context.Context, out *strings.Builder) error {
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(provider, ''), COALESCE(action, ''), COALESCE(result, ''), COUNT(*)::float8
		FROM audit_logs
		GROUP BY provider, action, result
		ORDER BY provider, action, result
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var provider, action, result string
		var count float64
		if err := rows.Scan(&provider, &action, &result, &count); err != nil {
			return err
		}
		writeMetricLine(out, "nopsai_audit_events_total", map[string]string{
			"provider": normalizeMetricLabel(provider),
			"action":   normalizeMetricLabel(action),
			"result":   normalizeMetricLabel(result),
		}, count)
	}
	return rows.Err()
}

func appendRunnerPrometheusMetrics(out *strings.Builder, status interface {
	GetQueuedJobs() int32
	GetRunners() []*proto.RunnerInfo
}) {
	writeMetricHelp(out, "nopsai_runner_capacity", "Runner capacity by runner.")
	writeMetricType(out, "nopsai_runner_capacity", "gauge")
	writeMetricHelp(out, "nopsai_runner_active_jobs", "Active jobs by runner.")
	writeMetricType(out, "nopsai_runner_active_jobs", "gauge")
	writeMetricHelp(out, "nopsai_runner_inflight_jobs", "Inflight jobs by runner.")
	writeMetricType(out, "nopsai_runner_inflight_jobs", "gauge")
	writeMetricHelp(out, "nopsai_runner_queued_jobs", "Dispatcher queued jobs.")
	writeMetricType(out, "nopsai_runner_queued_jobs", "gauge")
	writeMetricHelp(out, "nopsai_runner_heartbeat_age_seconds", "Runner heartbeat age in seconds.")
	writeMetricType(out, "nopsai_runner_heartbeat_age_seconds", "gauge")

	if status == nil {
		return
	}
	writeMetricLine(out, "nopsai_runner_queued_jobs", nil, float64(status.GetQueuedJobs()))
	now := time.Now().Unix()
	for _, runner := range status.GetRunners() {
		metadata := runner.GetMetadata()
		labels := map[string]string{
			"runner_id": normalizeMetricLabel(runner.GetRunnerId()),
			"runtime":   normalizeMetricLabel(runnerRuntime(metadata)),
			"namespace": normalizeMetricLabel(firstMonitoringText(metadata["kubernetes_namespace"], metadata["namespace"])),
			"node":      normalizeMetricLabel(firstMonitoringText(metadata["kubernetes_node"], metadata["node"], metadata["hostname"])),
		}
		writeMetricLine(out, "nopsai_runner_capacity", labels, float64(runner.GetCapacity()))
		writeMetricLine(out, "nopsai_runner_active_jobs", labels, float64(runner.GetActiveJobs()))
		writeMetricLine(out, "nopsai_runner_inflight_jobs", labels, float64(runner.GetInflightJobs()))
		heartbeat := runner.GetLastHeartbeatUnix()
		age := float64(0)
		if heartbeat > 0 && now >= heartbeat {
			age = float64(now - heartbeat)
		}
		writeMetricLine(out, "nopsai_runner_heartbeat_age_seconds", labels, age)
	}
}

func writeMetricHelp(out *strings.Builder, name, help string) {
	out.WriteString("# HELP ")
	out.WriteString(name)
	out.WriteByte(' ')
	out.WriteString(strings.ReplaceAll(help, "\n", " "))
	out.WriteByte('\n')
}

func writeMetricType(out *strings.Builder, name, metricType string) {
	out.WriteString("# TYPE ")
	out.WriteString(name)
	out.WriteByte(' ')
	out.WriteString(metricType)
	out.WriteByte('\n')
}

func writeMetricLine(out *strings.Builder, name string, labels map[string]string, value float64) {
	out.WriteString(name)
	if len(labels) > 0 {
		out.WriteByte('{')
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for idx, key := range keys {
			if idx > 0 {
				out.WriteByte(',')
			}
			out.WriteString(key)
			out.WriteString("=\"")
			out.WriteString(escapePrometheusLabel(labels[key]))
			out.WriteByte('"')
		}
		out.WriteByte('}')
	}
	out.WriteByte(' ')
	out.WriteString(formatPrometheusFloat(value))
	out.WriteByte('\n')
}

func normalizeMetricLabel(value string) string {
	return strings.TrimSpace(value)
}

func escapePrometheusLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatPrometheusFloat(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%g", value)
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func metricLabelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\xff")
}
