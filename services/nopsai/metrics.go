package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
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
