package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/llmclient"
	"nopsai/pkg/models"
)

func TestNormalizePipelineFinalOutputContentValidatesJSON(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("json", "<final_output>\n```json\n{\"ok\": true}\n```\n</final_output>")
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if content != `{"ok": true}` {
		t.Fatalf("content = %q, want JSON without fence", content)
	}

	if _, err := normalizePipelineFinalOutputContent("json", "<final_output>{not-json</final_output>"); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestCancelActivePipelineFinalOutputGenerationSignalsRegisteredContext(t *testing.T) {
	app := &App{}
	generationCtx, release := app.registerPipelineFinalOutputGeneration(context.Background(), "output-1")
	defer release()

	app.cancelActivePipelineFinalOutputGeneration("output-1")

	select {
	case <-generationCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("generation context was not cancelled")
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDocumentSpec(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("html", `<final_output>{"version":"1","title":"Report","sections":[{"title":"Summary","blocks":[{"type":"paragraph","text":"Everything passed."}]}]}</final_output>`)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if !strings.Contains(content, `"title": "Report"`) || strings.Contains(content, "<h1") {
		t.Fatalf("normalized DocumentSpec = %s", content)
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDashboardSpec(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("dashboard", `<final_output>{"version":"1","title":"Ops Dashboard","blocks":[{"type":"status","label":"Health","status":"success","value":"Green"},{"type":"table","columns":[{"key":"name","label":"Name"}],"rows":[{"name":"api"}]}]}</final_output>`)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if !strings.Contains(content, `"title": "Ops Dashboard"`) || strings.Contains(content, "<table") {
		t.Fatalf("normalized DashboardSpec = %s", content)
	}

	if _, err := normalizePipelineFinalOutputContent("dashboard", `<final_output>{"version":"1","title":"Bad","blocks":[{"type":"link","href":"javascript:alert(1)","text":"Bad"}]}</final_output>`); err == nil {
		t.Fatal("expected unsafe dashboard link to fail")
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsIntentGeneratedDashboardSpec(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Pipeline Run Dashboard",
  "blocks": [
    {
      "type": "text",
      "title": "Run Status",
      "text": "Pipeline completed successfully."
    },
    {
      "type": "list",
      "title": "Recent Issues",
      "items": [
        { "text": "No critical issues detected" },
        { "text": "All steps completed successfully" }
      ]
    },
    {
      "type": "chart",
      "title": "Error Count",
      "chart": {
        "type": "bar",
        "series": [
          {
            "key": "errors",
            "label": "Errors",
            "points": [
              { "timestamp": "2026-07-16T19:12:36Z", "value": 0 }
            ]
          }
        ]
      }
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if !strings.Contains(content, `"type": "list"`) || !strings.Contains(content, `"text": "No critical issues detected"`) {
		t.Fatalf("normalized DashboardSpec = %s", content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardSectionsAlias(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Service Metrics",
  "sections": [
    {
      "title": "Current service metrics",
      "blocks": [
        {
          "type": "status",
          "label": "Health",
          "status": "success",
          "value": "Green"
        },
        {
          "type": "table",
          "title": "Task durations",
          "columns": [
            { "key": "task", "label": "Task" },
            { "key": "duration", "label": "Duration" }
          ],
          "rows": [
            { "task": "collect", "duration": "12s" },
            { "task": "publish", "duration": "5s" }
          ]
        }
      ]
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if strings.Contains(content, `"sections"`) || !strings.Contains(content, `"blocks"`) || !strings.Contains(content, `"Task durations"`) {
		t.Fatalf("normalized DashboardSpec = %s", content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardWidgetsAlias(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Service Metrics",
  "widgets": [
    {
      "type": "chart",
      "title": "Requests by outcome",
      "chart": {
        "type": "bar",
        "series": [
          {
            "key": "requests",
            "label": "Requests",
            "points": [
              { "label": "success", "value": 1240 },
              { "label": "errors", "value": 10 }
            ]
          }
        ]
      }
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if strings.Contains(content, `"widgets"`) || !strings.Contains(content, `"blocks"`) || !strings.Contains(content, `"type": "bar"`) {
		t.Fatalf("normalized DashboardSpec = %s", content)
	}
}

func TestNormalizePipelineFinalOutputContentFlattensDashboardNestedBlockAliases(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Service Metrics",
  "blocks": [
    {
      "title": "Image build section",
      "blocks": [
        {
          "type": "table",
          "title": "Built images",
          "columns": [
            { "key": "image", "label": "Image" },
            { "key": "version", "label": "Version" }
          ],
          "rows": [
            { "image": "nopsai-dashboard", "version": "latest" }
          ]
        }
      ]
    },
    {
      "title": "Warnings",
      "widgets": [
        {
          "type": "callout",
          "tone": "warning",
          "text": "Images have vulnerabilities and cannot run in production."
        }
      ]
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if len(spec.Blocks) != 2 {
		t.Fatalf("blocks = %#v\n%s", spec.Blocks, content)
	}
	if spec.Blocks[0].Type != "table" || spec.Blocks[0].Title != "Built images" {
		t.Fatalf("first flattened block = %#v\n%s", spec.Blocks[0], content)
	}
	if spec.Blocks[1].Type != "callout" || spec.Blocks[1].Tone != "warning" {
		t.Fatalf("second flattened block = %#v\n%s", spec.Blocks[1], content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardKeyAliases(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Service Metrics",
  "blocks": [
    {
      "type": "properties",
      "title": "Service facts",
      "properties": [
        { "key": "Service", "value": "checkout-api" },
        { "key": "Errors", "value": 10 }
      ]
    },
    {
      "type": "chart",
      "key": "Requests",
      "chart": {
        "type": "bar",
        "series": [
          {
            "key": "requests",
            "points": [
              { "key": "success", "value": 1240 },
              { "key": "errors", "value": 10 }
            ]
          }
        ]
      }
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if got := spec.Blocks[0].Items[0].Label; got != "Service" {
		t.Fatalf("properties item label = %q, want Service\n%s", got, content)
	}
	if got := spec.Blocks[0].Items[1].Value; got != "10" {
		t.Fatalf("numeric properties value = %q, want 10\n%s", got, content)
	}
	if got := spec.Blocks[1].Label; got != "Requests" {
		t.Fatalf("block key alias label = %q, want Requests\n%s", got, content)
	}
	if got := spec.Blocks[1].Chart.Series[0].Points[0].Label; got != "success" {
		t.Fatalf("chart point key alias label = %q, want success\n%s", got, content)
	}
}

func TestNormalizePipelineFinalOutputContentDefaultsMissingDashboardTitle(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "blocks": [
    {
      "type": "text",
      "title": "Service Summary",
      "text": "Checkout API is healthy."
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if spec.Title != "Service Summary" {
		t.Fatalf("defaulted dashboard title = %q, want Service Summary\n%s", spec.Title, content)
	}
}

func TestNormalizePipelineFinalOutputContentDefaultsDashboardVersion(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing",
			raw: `<final_output>{
  "title": "Service Summary",
  "blocks": [{ "type": "text", "text": "Checkout API is healthy." }]
}</final_output>`,
		},
		{
			name: "numeric",
			raw: `<final_output>{
  "version": 1,
  "title": "Service Summary",
  "blocks": [{ "type": "text", "text": "Checkout API is healthy." }]
}</final_output>`,
		},
		{
			name: "string-one-dot-zero",
			raw: `<final_output>{
  "version": "1.0",
  "title": "Service Summary",
  "blocks": [{ "type": "text", "text": "Checkout API is healthy." }]
}</final_output>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := normalizePipelineFinalOutputContent("dashboard", tt.raw)
			if err != nil {
				t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
			}
			var spec models.DashboardSpec
			if err := json.Unmarshal([]byte(content), &spec); err != nil {
				t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
			}
			if spec.Version != "1" {
				t.Fatalf("defaulted dashboard version = %q, want 1\n%s", spec.Version, content)
			}
		})
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardTextAliases(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "blocks": [
    {
      "type": "callout",
      "tone": "warning",
      "title": "Follow-up",
      "body": "Keep watching error rate during the next deploy window."
    },
    {
      "type": "text",
      "title": "Summary",
      "description": "Checkout API is healthy."
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if got := spec.Blocks[0].Text; got != "Keep watching error rate during the next deploy window." {
		t.Fatalf("callout body alias text = %q\n%s", got, content)
	}
	if got := spec.Blocks[1].Text; got != "Checkout API is healthy." {
		t.Fatalf("text description alias text = %q\n%s", got, content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardRootAliasesAndScalarItems(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Release report",
  "callout": {
    "tone": "warning",
    "body": "Images contain vulnerabilities and cannot run in production."
  },
  "body": "Git repositories were updated with a proper changelog.",
  "blocks": [
    {
      "type": "properties",
      "title": "Readiness facts",
      "properties": {
        "git_changelog_updated": true,
        "risk": "vulnerable images"
      }
    },
    {
      "type": "properties",
      "title": "Readiness list",
      "items": [
        "Images built: 4",
        "Production ready: false"
      ]
    },
    {
      "type": "list",
      "title": "Next actions",
      "items": [
        "Patch vulnerable images",
        { "body": "Provide missing environment configuration" }
      ]
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if len(spec.Blocks) != 5 {
		t.Fatalf("blocks = %#v\n%s", spec.Blocks, content)
	}
	if spec.Blocks[0].Type != "callout" || spec.Blocks[0].Text == "" {
		t.Fatalf("root callout alias not normalized: %#v\n%s", spec.Blocks[0], content)
	}
	if spec.Blocks[1].Type != "text" || !strings.Contains(spec.Blocks[1].Text, "changelog") {
		t.Fatalf("root body alias not normalized: %#v\n%s", spec.Blocks[1], content)
	}
	if got := spec.Blocks[2].Items[0].Label; got != "git_changelog_updated" {
		t.Fatalf("property object label = %q\n%s", got, content)
	}
	if got := spec.Blocks[2].Items[0].Value; got != "true" {
		t.Fatalf("property bool value = %q\n%s", got, content)
	}
	if got := spec.Blocks[3].Items[0].Label; got != "Images built" {
		t.Fatalf("property scalar label = %q\n%s", got, content)
	}
	if got := spec.Blocks[3].Items[0].Value; got != "4" {
		t.Fatalf("property scalar value = %q\n%s", got, content)
	}
	if got := spec.Blocks[4].Items[1].Text; got != "Provide missing environment configuration" {
		t.Fatalf("list item body alias = %q\n%s", got, content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsDashboardChartAliases(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Build duration",
  "blocks": [
    {
      "type_name": "status",
      "label": "Production readiness",
      "value": "blocked",
      "status": "critical"
    },
    {
      "title": "Build duration metrics",
      "chartType": "bar",
      "series": [
        {
          "key": "duration",
          "label": "Build duration",
          "points": [
            { "label": "nopsai-dashboard", "value": 24 },
            { "label": "git-sample", "value": 55 }
          ]
        }
      ]
    },
    {
      "type": "chart",
      "title": "Timeline",
      "chart": {
        "chartType": "line",
        "series": [
          {
            "key": "timeline",
            "points": [
              { "timestamp": "2026-07-17T10:00:00Z", "value": 1 }
            ]
          }
        ]
      }
    },
    {
      "title": "Build data alias",
      "chartType": "bar",
      "data": [
        { "label": "app-finance", "value": 60 },
        { "label": "seed-static", "value": 12 }
      ]
    },
    {
      "type": "chart",
      "title": "Chart points alias",
      "chart": {
        "chartType": "bar",
        "points": [
          { "label": "nopsai-dashboard", "value": 24 }
        ]
      }
    },
    {
      "type": "bar",
      "title": "Block chart type alias",
      "data": [
        { "label": "app-finance", "value": 60 },
        { "label": "seed-static", "value": 12 }
      ]
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if spec.Blocks[0].Type != "status" {
		t.Fatalf("type_name alias type = %q\n%s", spec.Blocks[0].Type, content)
	}
	if spec.Blocks[1].Type != "chart" || spec.Blocks[1].Chart == nil || spec.Blocks[1].Chart.Type != "bar" {
		t.Fatalf("block-level chart aliases not normalized: %#v\n%s", spec.Blocks[1], content)
	}
	if got := spec.Blocks[1].Chart.Series[0].Points[0].Label; got != "nopsai-dashboard" {
		t.Fatalf("chart point label = %q\n%s", got, content)
	}
	if spec.Blocks[2].Chart == nil || spec.Blocks[2].Chart.Type != "line" {
		t.Fatalf("chart.chartType alias not normalized: %#v\n%s", spec.Blocks[2], content)
	}
	if spec.Blocks[3].Chart == nil || len(spec.Blocks[3].Chart.Series) != 1 || len(spec.Blocks[3].Chart.Series[0].Points) != 2 {
		t.Fatalf("block data alias not normalized: %#v\n%s", spec.Blocks[3], content)
	}
	if spec.Blocks[4].Chart == nil || len(spec.Blocks[4].Chart.Series) != 1 || len(spec.Blocks[4].Chart.Series[0].Points) != 1 {
		t.Fatalf("chart points alias not normalized: %#v\n%s", spec.Blocks[4], content)
	}
	if spec.Blocks[5].Type != "chart" || spec.Blocks[5].Chart == nil || spec.Blocks[5].Chart.Type != "bar" || len(spec.Blocks[5].Chart.Series) != 1 {
		t.Fatalf("block chart type alias not normalized: %#v\n%s", spec.Blocks[5], content)
	}

	rootRaw := `<final_output>{
  "version": "1",
  "title": "Root chart",
  "chartType": "bar",
  "points": [
    { "label": "seed-static", "value": 12 }
  ]
}</final_output>`
	content, err = normalizePipelineFinalOutputContent("dashboard", rootRaw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent(root chart) error = %v", err)
	}
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(root chart DashboardSpec) error = %v\n%s", err, content)
	}
	if len(spec.Blocks) != 1 || spec.Blocks[0].Type != "chart" || spec.Blocks[0].Chart == nil || spec.Blocks[0].Chart.Type != "bar" {
		t.Fatalf("root chart aliases not normalized: %#v\n%s", spec.Blocks, content)
	}

	emptySeriesRaw := `<final_output>{
  "version": "1",
  "title": "Empty chart",
  "blocks": [
    {
      "type": "chart",
      "title": "Metrics pending",
      "chart": {
        "type": "bar",
        "series": [
          { "key": "duration", "label": "Build duration", "points": [] }
        ]
      }
    }
  ]
}</final_output>`
	if _, err := normalizePipelineFinalOutputContent("dashboard", emptySeriesRaw); err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent(empty series) error = %v", err)
	}
}

func TestNormalizePipelineFinalOutputContentInfersDashboardChartPointsFromTables(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Build duration",
  "blocks": [
    {
      "type": "chart",
      "title": "Build Duration by Image",
      "chart": {
        "type": "bar",
        "series": [
          { "key": "duration_sec", "label": "Seconds", "points": null }
        ]
      }
    },
    {
      "type": "table",
      "columns": [
        { "key": "image", "label": "Image Name" },
        { "key": "tag", "label": "Tag" },
        { "key": "duration", "label": "Build Duration" }
      ],
      "rows": [
        { "image": "nopsai-dashboard", "tag": "latest", "duration": "24s" },
        { "image": "git-sample", "tag": "dev", "duration": "55s" }
      ]
    },
    {
      "type": "table",
      "columns": [
        { "key": "image", "label": "Image Name" },
        { "key": "duration", "label": "Duration (s)" }
      ],
      "rows": [
        { "image": "app-finance", "duration": 60 },
        { "image": "seed-static", "duration": 12 }
      ]
    },
    {
      "type": "chart",
      "title": "Duration comparison",
      "chart": {
        "type": "bar",
        "series": [
          { "key": "duration", "label": "Seconds", "points": [] }
        ]
      }
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	firstPoints := spec.Blocks[0].Chart.Series[0].Points
	if len(firstPoints) != 2 || firstPoints[0].Label != "nopsai-dashboard" || firstPoints[0].Value == nil || *firstPoints[0].Value != 24 {
		t.Fatalf("first chart points = %#v\n%s", firstPoints, content)
	}
	secondPoints := spec.Blocks[3].Chart.Series[0].Points
	if len(secondPoints) != 2 || secondPoints[0].Label != "app-finance" || secondPoints[0].Value == nil || *secondPoints[0].Value != 60 {
		t.Fatalf("second chart points = %#v\n%s", secondPoints, content)
	}

	raw = `<final_output>{
  "version": "1",
  "title": "Production readiness",
  "blocks": [
    {
      "type": "chart",
      "title": "Production readiness status",
      "chart": {
        "type": "bar",
        "series": [
          { "key": "readiness", "label": "Ready", "points": null }
        ]
      }
    },
    {
      "type": "table",
      "columns": [
        { "key": "image", "label": "Image Name" },
        { "key": "tag", "label": "Tag" },
        { "key": "prod_ready", "label": "Prod Ready" }
      ],
      "rows": [
        { "image": "nopsai-dashboard", "tag": "latest", "prod_ready": "No" },
        { "image": "seed-static", "tag": "3.4.5", "prod_ready": "No" }
      ]
    }
  ]
}</final_output>`

	content, err = normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() identifier table error = %v", err)
	}
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(identifier table DashboardSpec) error = %v\n%s", err, content)
	}
	if points := spec.Blocks[0].Chart.Series[0].Points; len(points) != 0 {
		t.Fatalf("identifier-looking tag column inferred chart points = %#v\n%s", points, content)
	}
}

func TestNormalizePipelineFinalOutputContentAcceptsCommonLLMDashboardAliases(t *testing.T) {
	raw := `<final_output>{
  "version": "1",
  "title": "Release readiness",
  "entryKey": "release-readiness",
  "blocks": [
    {
      "type": "status",
      "label": "Production readiness",
      "status": "failure"
    },
    {
      "type": "status",
      "label": "Production ready",
      "status": "False",
      "value": { "value": false }
    },
    {
      "type": "chart",
      "title": "Build timeline",
      "unit": "s",
      "chart": {
        "type": "timeline",
        "series": {
          "key": "duration",
          "label": "Duration",
          "points": [
            { "label": "nopsai-dashboard", "value": "24s" }
          ]
        }
      }
    },
    {
      "type": "chart",
      "title": "Readiness donut",
      "chart": {
        "shape": "doughnut",
        "series": {
          "readiness": {
            "Production Ready": 0,
            "Blocked from Production": 4
          }
        }
      }
    },
    {
      "type": "metric",
      "label": "Total build time",
      "value": 151,
      "unit": "s",
      "shape": "card"
    },
    {
      "type": "properties",
      "title": "Generated values",
      "items": [
        { "label": "Average build time", "value": { "value": 37.75 }, "unit": "s" },
        { "label": "Changelog updated", "value": { "value": true } }
      ]
    },
    {
      "type": "properties",
      "title": "Property map",
      "properties": {
        "Production ready": { "ready": false }
      }
    }
  ]
}</final_output>`

	content, err := normalizePipelineFinalOutputContent("dashboard", raw)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if strings.Contains(content, "entryKey") {
		t.Fatalf("root metadata alias was not removed:\n%s", content)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal(normalized DashboardSpec) error = %v\n%s", err, content)
	}
	if spec.Blocks[0].Status != "critical" || spec.Blocks[0].Value != "failure" {
		t.Fatalf("failure status alias = %#v\n%s", spec.Blocks[0], content)
	}
	if spec.Blocks[1].Status != "critical" || spec.Blocks[1].Value != "false" {
		t.Fatalf("boolean status alias = %#v\n%s", spec.Blocks[1], content)
	}
	if spec.Blocks[2].Chart == nil || spec.Blocks[2].Chart.Type != "line" || spec.Blocks[2].Chart.Unit != "s" || len(spec.Blocks[2].Chart.Series) != 1 {
		t.Fatalf("timeline chart alias = %#v\n%s", spec.Blocks[2], content)
	}
	if points := spec.Blocks[2].Chart.Series[0].Points; len(points) != 1 || points[0].Value == nil || *points[0].Value != 24 {
		t.Fatalf("series object points = %#v\n%s", points, content)
	}
	if spec.Blocks[3].Chart == nil || spec.Blocks[3].Chart.Type != "donut" || len(spec.Blocks[3].Chart.Series) != 1 {
		t.Fatalf("doughnut/chart series map alias = %#v\n%s", spec.Blocks[3], content)
	}
	if points := spec.Blocks[3].Chart.Series[0].Points; len(points) != 2 {
		t.Fatalf("readiness points = %#v\n%s", points, content)
	}
	if spec.Blocks[4].Type != "status" || spec.Blocks[4].Value != "151s" {
		t.Fatalf("metric block alias = %#v\n%s", spec.Blocks[4], content)
	}
	if got := spec.Blocks[5].Items[0].Value; got != "37.75s" {
		t.Fatalf("object value with unit = %q\n%s", got, content)
	}
	if got := spec.Blocks[5].Items[1].Value; got != "true" {
		t.Fatalf("object boolean value = %q\n%s", got, content)
	}
	if got := spec.Blocks[6].Items[0].Value; got != "false" {
		t.Fatalf("property map object value = %q\n%s", got, content)
	}
	var normalizedRoot struct {
		Blocks []map[string]json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(content), &normalizedRoot); err != nil {
		t.Fatalf("json.Unmarshal(normalized root) error = %v\n%s", err, content)
	}
	for blockIndex, block := range normalizedRoot.Blocks {
		if _, hasShape := block["shape"]; hasShape {
			t.Fatalf("block %d leaked shape alias:\n%s", blockIndex, content)
		}
		if _, hasUnit := block["unit"]; hasUnit {
			t.Fatalf("block %d leaked unit alias:\n%s", blockIndex, content)
		}
	}
	if strings.Contains(content, `"type": "metric"`) {
		t.Fatalf("metric type alias leaked into normalized content:\n%s", content)
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDashboardChartSpec(t *testing.T) {
	spec := dashboardSeriesSpec("Ops Dashboard", "Latency", []models.DashboardChartSeries{
		{
			Key:         "api.p95",
			Label:       "API p95",
			Team:        "platform",
			Environment: "prod",
			Color:       "#2563eb",
			Points: []models.DashboardSeriesPoint{
				dashboardPoint("2026-07-15T10:00:00Z", "", 120),
				dashboardPoint("2026-07-15T10:05:00Z", "", 130),
			},
		},
	})
	raw, err := marshalFinalOutputSpec(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	content, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+raw+"</final_output>")
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() chart error = %v", err)
	}
	if !strings.Contains(content, `"type": "series"`) || !strings.Contains(content, `"aggregation_interval": "5m"`) {
		t.Fatalf("normalized chart DashboardSpec = %s", content)
	}

	badScript := strings.Replace(raw, "API p95", "<script>alert(1)</script>", 1)
	if _, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+badScript+"</final_output>"); err == nil {
		t.Fatal("expected executable chart content to fail")
	}

	badPieSeries := strings.Replace(raw, `"type": "line"`, `"type": "pie"`, 1)
	if _, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+badPieSeries+"</final_output>"); err == nil {
		t.Fatal("expected pie series block to fail")
	}
}

func TestDashboardFinalOutputGuidanceStaysIntentDriven(t *testing.T) {
	guidance := dashboardFinalOutputFormatGuidance("auto")
	for _, want := range []string{
		"Translate the user's dashboard intent into a useful dashboard",
		"Choose the dashboard structure dynamically",
		"If the user did not name a visualization, choose by data shape",
		"table for repeated records",
		"bar chart for categorical counts/durations/rankings",
		"line or area chart for time series",
		"recent pipeline history",
		"Use only evidence present in the run context",
		"Keep content scoped to the user's dashboard request",
		"Copy non-secret operational labels from emitted evidence exactly",
		"Available DashboardSpec blocks are status, text, callout, list, properties, table, progress, link, chart, and series",
		"Include a non-empty title",
		"Use one flat top-level blocks array",
		"do not put nested blocks or widgets inside a block",
		"Use text for text and callout block bodies",
		"Use label for display labels; key is only for table columns and chart series identifiers",
		"Charts need type, series, and points",
		"will be retried if it does not match the DashboardSpec contract",
		"auto means choose the smallest useful presentation",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("dashboard guidance missing %q:\n%s", want, guidance)
		}
	}
	for _, staticFragment := range []string{
		"Supported block types are",
		`"blocks":[`,
		"Built Images",
		"For requests about \"how many\", use status",
	} {
		if strings.Contains(guidance, staticFragment) {
			t.Fatalf("dashboard guidance contains static fragment %q:\n%s", staticFragment, guidance)
		}
	}
}

func TestDashboardFinalOutputGuidanceDefinesPresetShapes(t *testing.T) {
	tests := []struct {
		preset string
		wants  []string
	}{
		{
			preset: "report",
			wants: []string{
				"report means a narrative operator report",
				"executive summary",
				"what changed",
				"Tables are allowed only as compact supporting evidence after the narrative",
				"do not make a table the primary or first block",
			},
		},
		{
			preset: "table",
			wants: []string{
				"table means a row-and-column output",
				"Make one table the primary block",
				"stable keys for repeated records",
				"Avoid charts or long narrative",
			},
		},
		{
			preset: "status",
			wants: []string{
				"status means current health or readiness",
				"Start with one status block or callout",
				"properties, progress, or a short list",
			},
		},
		{
			preset: "timeline",
			wants: []string{
				"timeline means chronological order",
				"series line or area chart for timestamped numeric data",
				"sorted oldest-to-newest",
			},
		},
		{
			preset: "comparison",
			wants: []string{
				"comparison means side-by-side differences",
				"comparison table or properties blocks",
				"most important difference, winner, or risk",
			},
		},
		{
			preset: "metrics",
			wants: []string{
				"metrics means numbers first",
				"include units and ratios",
				"bar charts for categorical metrics or line/area/series charts for trends",
			},
		},
		{
			preset: "mixed",
			wants: []string{
				"mixed means a cohesive multi-block digest",
				"headline properties/status, charts, risk callouts, tables, and next-action lists",
				"without duplicating the same facts",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.preset, func(t *testing.T) {
			guidance := dashboardFinalOutputFormatGuidance(test.preset)
			for _, want := range test.wants {
				if !strings.Contains(guidance, want) {
					t.Fatalf("dashboard %s guidance missing %q:\n%s", test.preset, want, guidance)
				}
			}
		})
	}
}

func TestPipelineFinalOutputGuidanceDefinesSeriesModeShape(t *testing.T) {
	guidance := pipelineFinalOutputFormatGuidance(pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard", Name: "Build timeline"},
		Dashboard:              models.DashboardOutputTarget{Mode: "series", Preset: "timeline"},
	})
	for _, want := range []string{
		"timeline means chronological order",
		"Publication mode guidance",
		"Because dashboard publish mode is series",
		"include at least one chart or series block",
		"chart.series[].points",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("series dashboard guidance missing %q:\n%s", want, guidance)
		}
	}
}

func TestPipelineFinalOutputLogEvidenceSummaryExtractsImagesAndDurations(t *testing.T) {
	logs := []string{
		"Preparing agent container agent-dashboard-sample with image ghcr.io/nopsai/nopsai-agent:dev",
		`2026-07-16T20:31:50Z {"level":"info","image":"python:3.11-slim","message":"Image found locally"}`,
		`2026-07-16T20:31:50Z {"level":"info","message":"status=success action=\"echo \"we built 4 different docker images, nopsai-dashboard:latest, git-sample:dev, app-finance:prod, seed-static:3.4.5\"\" output=\"we built 4 different docker images, nopsai-dashboard:latest, git-sample:dev, app-finance:prod, seed-static:3.4.5\nthis is how each image took to be built, 24s, 55s, 60s, 12s\nThese images have vulnerabilities and can not be run in production\nmy dog name is leone, and I have a car\""}`,
	}
	summary := strings.Join(pipelineFinalOutputLogEvidenceSummary(logs), "\n")
	for _, want := range []string{
		"image_count: 4",
		"image: nopsai-dashboard | version: latest | build_duration: 24s",
		"image: git-sample | version: dev | build_duration: 55s",
		"image: app-finance | version: prod | build_duration: 60s",
		"image: seed-static | version: 3.4.5 | build_duration: 12s",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("log evidence summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "dog") || strings.Contains(summary, "car") ||
		strings.Contains(summary, "nopsai-agent") || strings.Contains(summary, "python") {
		t.Fatalf("log evidence summary included incidental or operational details:\n%s", summary)
	}
}

func TestPipelineFinalOutputLogEvidenceExtractsKubernetesDirectScriptOutput(t *testing.T) {
	output := strings.Join([]string{
		"we built 4 different docker images, nopsai-dashboard:latest, git-sample:dev, app-finance:prod, seed-static:3.4.5",
		"this is how each image took to be built, 24s, 55s, 60s, 12s",
		"These images have vulnerabilities and can not be run in production",
		"some environments are not provided for images, so it might fails during running",
		"git repositories are updated with proper changelog",
		"my dog name is leone, and I have a car",
		`dashboard_evidence={"git_changelog_updated":true,"images":[{"build_duration_seconds":24,"environment":"dashboard","has_vulnerabilities":true,"missing_environment":false,"name":"nopsai-dashboard","production_ready":false,"tag":"latest"},{"build_duration_seconds":55,"environment":"development","has_vulnerabilities":true,"missing_environment":true,"name":"git-sample","production_ready":false,"tag":"dev"},{"build_duration_seconds":60,"environment":"production","has_vulnerabilities":true,"missing_environment":true,"name":"app-finance","production_ready":false,"tag":"prod"},{"build_duration_seconds":12,"environment":"static-assets","has_vulnerabilities":true,"missing_environment":false,"name":"seed-static","production_ready":false,"tag":"3.4.5"}],"images_built":4,"readiness_summary":{"blocked_from_production":4,"missing_runtime_configuration":2,"production_ready":0,"runtime_configuration_present":2,"vulnerable_images":4},"subject":"docker image build and production readiness"}`,
	}, "\n")
	payload, err := json.Marshal(map[string]string{
		"level":   "info",
		"runid":   "3e349032-4918-4f47-8a9f-349600932e57",
		"message": "status=success action=\"python - <<'PY'\nimport json\nprint(\"dashboard_evidence=...\")\nPY\" output=\"" + output + "\"",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	logs := []string{
		`Preparing agent container agent-dashboard-sample with image ghcr.io/nopsai/nopsai-agent:dev`,
		`2026-07-18T09:38:55Z {"level":"info","image":"python:3.11-slim","message":"Creating new Kubernetes pod for step"}`,
		"2026-07-18T09:38:57Z " + string(payload),
	}

	evidence := buildPipelineFinalOutputLogEvidence(logs)
	if len(evidence.Lines) < 7 {
		t.Fatalf("emitted evidence lines = %#v", evidence.Lines)
	}
	if len(evidence.Structured) != 1 || !strings.Contains(evidence.Structured[0], `"images_built":4`) {
		t.Fatalf("structured evidence = %#v", evidence.Structured)
	}
	summary := strings.Join(pipelineFinalOutputLogEvidenceSummaryFromEvidence(evidence), "\n")
	for _, want := range []string{
		"image_count: 4",
		"image: nopsai-dashboard | version: latest | build_duration: 24s",
		"image: app-finance | version: prod | build_duration: 60s",
		"image: seed-static | version: 3.4.5 | build_duration: 12s",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("log evidence summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "python") || strings.Contains(summary, "nopsai-agent") {
		t.Fatalf("log evidence summary included runtime metadata:\n%s", summary)
	}
}

func TestPipelineFinalOutputCommandOutputLinesKeepsTrailingEmbeddedJSONQuotes(t *testing.T) {
	message := `status=success action="python - <<'PY'
print("dashboard_evidence=...")
PY" output="first line
dashboard_evidence={"images_built":4,"readiness_summary":{"blocked_from_production":4,"production_ready":0}}"`

	lines := pipelineFinalOutputCommandOutputLines(message)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}
	if lines[1] != `dashboard_evidence={"images_built":4,"readiness_summary":{"blocked_from_production":4,"production_ready":0}}` {
		t.Fatalf("structured line = %q", lines[1])
	}
	evidence := pipelineFinalOutputStructuredEvidenceFromLines(lines)
	if len(evidence) != 1 || !strings.Contains(evidence[0], `"images_built":4`) {
		t.Fatalf("structured evidence = %#v", evidence)
	}
}

func TestWritePipelineFinalOutputCurrentLogEvidenceMarksLogsAuthoritative(t *testing.T) {
	logs := []string{
		`2026-07-16T20:31:50Z {"level":"info","message":"status=success action=\"echo \"we built 4 different docker images, nopsai-dashboard:latest\"\" output=\"we built 4 different docker images, nopsai-dashboard:latest\nthis is how each image took to be built, 24s\ndashboard_evidence={\\\"images_built\\\":1,\\\"readiness_summary\\\":{\\\"production_ready\\\":0,\\\"blocked_from_production\\\":1}}\""}`,
	}
	var builder strings.Builder
	writePipelineFinalOutputCurrentLogEvidence(&builder, logs, buildPipelineFinalOutputLogEvidence(logs))
	out := builder.String()
	for _, want := range []string{
		"Current run emitted evidence (authoritative for business facts)",
		"before operational runner logs, metadata, history, or configured runtime/container fields",
		"Extracted facts from current log lines",
		"image_count: 1",
		"image: nopsai-dashboard | version: latest | build_duration: 24s",
		"Structured emitted evidence",
		`dashboard_evidence={"images_built":1,"readiness_summary":{"production_ready":0,"blocked_from_production":1}}`,
		"Emitted step output lines",
		"Raw operational log excerpt",
		"we built 4 different docker images, nopsai-dashboard:latest",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("current log evidence context missing %q:\n%s", want, out)
		}
	}
}

func TestGroundPipelineFinalDashboardOutputContentPreservesValidOutputIntent(t *testing.T) {
	logs := []string{
		"Preparing agent container agent-dashboard-sample with image ghcr.io/nopsai/nopsai-agent:dev",
		`2026-07-16T20:31:50Z {"level":"info","message":"status=success action=\"echo \"we built 4 different docker images, nopsai-dashboard:latest, git-sample:dev, app-finance:prod, seed-static:3.4.5\"\" output=\"we built 4 different docker images, nopsai-dashboard:latest, git-sample:dev, app-finance:prod, seed-static:3.4.5\nthis is how each image took to be built, 24s, 55s, 60s, 12s\nThese images have vulnerabilities and can not be run in production\n some environments are not provided for images, so it might fails during running\ngit repositories are updated with proper changelog\nmy dog name is leone, and I have a car\""}`,
	}
	inventoryContent := `{
  "version": "1",
  "title": "Image inventory table",
  "blocks": [
    {
      "type": "table",
      "title": "Inventory",
      "columns": [
        { "key": "image", "label": "Image" },
        { "key": "version", "label": "Version" },
        { "key": "duration", "label": "Duration" },
        { "key": "ready", "label": "Ready" }
      ],
      "rows": [
        { "image": "nopsai-dashboard", "version": "latest", "duration": "24s", "ready": false }
      ]
    }
  ]
}`
	content, err := groundPipelineFinalDashboardOutputContent(inventoryContent, pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard"},
		Prompt:                 "Show how many images were built, which version each image used, and how long each image build took.",
	}, buildPipelineFinalOutputLogEvidence(logs))
	if err != nil {
		t.Fatalf("groundPipelineFinalDashboardOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatalf("json.Unmarshal grounded content error = %v\n%s", err, content)
	}
	if spec.Title != "Image inventory table" {
		t.Fatalf("grounding changed output title to %q\n%s", spec.Title, content)
	}
	if len(spec.Blocks) != 1 || spec.Blocks[0].Title != "Inventory" {
		t.Fatalf("grounding changed output blocks: %#v\n%s", spec.Blocks, content)
	}
	if strings.Contains(content, "Image Build Summary") || strings.Contains(content, "Built Images") {
		t.Fatalf("grounding replaced output with generic image summary:\n%s", content)
	}
}

func TestGroundPipelineFinalDashboardOutputContentFillsCircularChartPointsFromStructuredEvidence(t *testing.T) {
	evidence := pipelineFinalOutputLogEvidence{
		Structured: []string{
			`dashboard_evidence={"images_built":4,"readiness_summary":{"blocked_from_production":4,"missing_runtime_configuration":2,"production_ready":0,"runtime_configuration_present":2,"vulnerable_images":4}}`,
		},
	}
	content := `{
  "version": "1",
  "title": "Production Readiness Dashboard",
  "blocks": [
    {
      "type": "chart",
      "label": "Production Readiness",
      "chart": {
        "type": "donut",
        "series": [
          { "key": "production_ready", "label": "production ready", "points": null },
          { "key": "blocked_from_production", "label": "blocked from production", "points": null }
        ]
      }
    },
    {
      "type": "chart",
      "label": "Runtime Configuration Coverage",
      "chart": {
        "type": "pie",
        "series": [
          { "key": "missing_configuration", "label": "missing configuration", "points": null },
          { "key": "configuration_present", "label": "configuration present", "points": null }
        ]
      }
    }
  ]
}`
	grounded, err := groundPipelineFinalDashboardOutputContent(content, pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard"},
		Prompt:                 "Create a compact circular-chart dashboard output from emitted JSON evidence.",
	}, evidence)
	if err != nil {
		t.Fatalf("groundPipelineFinalDashboardOutputContent() error = %v", err)
	}
	if strings.Contains(grounded, `"points":null`) {
		t.Fatalf("grounded dashboard still contains null points:\n%s", grounded)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(grounded), &spec); err != nil {
		t.Fatalf("json.Unmarshal grounded content error = %v\n%s", err, grounded)
	}
	readinessPoints := spec.Blocks[0].Chart.Series[0].Points
	if len(readinessPoints) != 2 || readinessPoints[0].Label != "Production Ready" || readinessPoints[0].Value == nil || *readinessPoints[0].Value != 0 ||
		readinessPoints[1].Label != "Blocked From Production" || readinessPoints[1].Value == nil || *readinessPoints[1].Value != 4 {
		t.Fatalf("readiness points = %#v\n%s", readinessPoints, grounded)
	}
	configPoints := spec.Blocks[1].Chart.Series[0].Points
	if len(configPoints) != 2 || configPoints[0].Label != "Missing Configuration" || configPoints[0].Value == nil || *configPoints[0].Value != 2 ||
		configPoints[1].Label != "Configuration Present" || configPoints[1].Value == nil || *configPoints[1].Value != 2 {
		t.Fatalf("configuration points = %#v\n%s", configPoints, grounded)
	}
}

func TestGroundPipelineFinalDashboardOutputContentCorrectsBuildDurationMetrics(t *testing.T) {
	evidence := pipelineFinalOutputLogEvidence{
		Structured: []string{
			`dashboard_evidence={"images":[{"build_duration_seconds":24,"name":"nopsai-dashboard","tag":"latest"},{"build_duration_seconds":55,"name":"git-sample","tag":"dev"},{"build_duration_seconds":60,"name":"app-finance","tag":"prod"},{"build_duration_seconds":12,"name":"seed-static","tag":"3.4.5"}],"readiness_summary":{"blocked_from_production":4,"production_ready":0}}`,
		},
	}
	content := `{
  "version": "1",
  "title": "Build duration metrics",
  "blocks": [
    {
      "type": "properties",
      "items": [
        { "label": "Total Build Time", "value": "149s" },
        { "label": "Average Build Time", "value": "37.25s" },
        { "label": "Fastest Image", "value": "N/A" },
        { "label": "Slowest Image", "value": "N/A" }
      ]
    },
    {
      "type": "chart",
      "label": "Build Duration by Image",
      "chart": {
        "type": "bar",
        "series": [
          {
            "key": "duration",
            "label": "Seconds",
            "points": [
              { "label": "nopsai-dashboard", "value": 1 }
            ]
          }
        ]
      }
    }
  ]
}`
	grounded, err := groundPipelineFinalDashboardOutputContent(content, pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard", Name: "Build duration metrics"},
		Prompt:                 "Publish numeric build duration metrics for each image.",
		Dashboard:              models.DashboardOutputTarget{EntryKey: "build-duration-metrics", Preset: "metrics"},
	}, evidence)
	if err != nil {
		t.Fatalf("groundPipelineFinalDashboardOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(grounded), &spec); err != nil {
		t.Fatalf("json.Unmarshal grounded content error = %v\n%s", err, grounded)
	}
	items := spec.Blocks[0].Items
	wantValues := []string{"151s", "37.75s", "seed-static:3.4.5 (12s)", "app-finance:prod (60s)"}
	for index, want := range wantValues {
		if items[index].Value != want {
			t.Fatalf("items[%d].Value = %q, want %q\n%s", index, items[index].Value, want, grounded)
		}
	}
	points := spec.Blocks[1].Chart.Series[0].Points
	if len(points) != 4 || points[0].Value == nil || *points[0].Value != 24 || points[3].Value == nil || *points[3].Value != 12 {
		t.Fatalf("points = %#v\n%s", points, grounded)
	}
}

func TestGroundPipelineFinalDashboardOutputContentBuildsOperationsOverviewFromEvidence(t *testing.T) {
	evidence := pipelineFinalOutputLogEvidence{
		Structured: []string{
			`dashboard_evidence={"git_changelog_updated":true,"images":[{"build_duration_seconds":24,"environment":"dashboard","has_vulnerabilities":true,"missing_environment":false,"name":"nopsai-dashboard","production_ready":false,"tag":"latest"},{"build_duration_seconds":55,"environment":"development","has_vulnerabilities":true,"missing_environment":true,"name":"git-sample","production_ready":false,"tag":"dev"},{"build_duration_seconds":60,"environment":"production","has_vulnerabilities":true,"missing_environment":true,"name":"app-finance","production_ready":false,"tag":"prod"},{"build_duration_seconds":12,"environment":"static-assets","has_vulnerabilities":true,"missing_environment":false,"name":"seed-static","production_ready":false,"tag":"3.4.5"}],"images_built":4,"operational_risk":"Images contain vulnerabilities and missing environment configuration can make runtime execution fail.","readiness_summary":{"blocked_from_production":4,"missing_runtime_configuration":2,"production_ready":0,"runtime_configuration_present":2,"vulnerable_images":4}}`,
		},
	}
	content := `{
  "version": "1",
  "title": "Unstructured Digest",
  "blocks": [{ "type": "text", "text": "placeholder" }]
}`
	grounded, err := groundPipelineFinalDashboardOutputContent(content, pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard", Name: "Mixed operations digest"},
		Prompt:                 "Create a mixed dashboard digest with a short summary, key metrics, risks, and next actions.",
		Dashboard:              models.DashboardOutputTarget{EntryKey: "mixed-operations-digest", Preset: "mixed"},
	}, evidence)
	if err != nil {
		t.Fatalf("groundPipelineFinalDashboardOutputContent() error = %v", err)
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(grounded), &spec); err != nil {
		t.Fatalf("json.Unmarshal grounded content error = %v\n%s", err, grounded)
	}
	if spec.Title != "Docker Image Operations Overview" {
		t.Fatalf("title = %q", spec.Title)
	}
	if len(spec.Blocks) != 7 {
		t.Fatalf("blocks = %d, want composed overview\n%s", len(spec.Blocks), grounded)
	}
	metrics := spec.Blocks[0].Items
	wantValues := []string{"4", "151s", "0 / 4", "2 / 4"}
	for index, want := range wantValues {
		if metrics[index].Value != want {
			t.Fatalf("metrics[%d].Value = %q, want %q\n%s", index, metrics[index].Value, want, grounded)
		}
	}
	if spec.Blocks[1].Chart == nil || len(spec.Blocks[1].Chart.Series[0].Points) != 4 {
		t.Fatalf("build duration chart not grounded: %#v", spec.Blocks[1])
	}
	if spec.Blocks[2].Chart.Series[0].Points[0].Label != "Production Ready" ||
		*spec.Blocks[2].Chart.Series[0].Points[0].Value != 0 ||
		spec.Blocks[3].Chart.Series[0].Points[0].Label != "Configuration Present" ||
		*spec.Blocks[3].Chart.Series[0].Points[0].Value != 2 {
		t.Fatalf("readiness charts = %#v / %#v", spec.Blocks[2].Chart.Series, spec.Blocks[3].Chart.Series)
	}
	if len(spec.Blocks[5].Rows) != 4 || string(spec.Blocks[5].Rows[2]["image"]) != `"app-finance:prod"` {
		t.Fatalf("matrix rows = %#v", spec.Blocks[5].Rows)
	}
	if strings.Contains(grounded, "dog") || strings.Contains(grounded, "car") {
		t.Fatalf("overview included noise:\n%s", grounded)
	}
}

func TestBuildPipelineFinalOutputPromptGuidesEvidenceFromLogs(t *testing.T) {
	prompt := buildPipelineFinalOutputPrompt(
		"Current run emitted evidence (authoritative for business facts)\n- {\"status\":\"success\",\"error_count\":5}\n",
		pipelineFinalOutputRecord{
			PipelineRunFinalOutput: models.PipelineRunFinalOutput{
				Name: "dashboard-widgets",
				Type: "dashboard",
			},
			Dashboard: models.DashboardOutputTarget{
				Ref:      "team-1/ops-dashboard",
				Section:  "service-metrics",
				EntryKey: "web-api-status",
				Mode:     "replace",
				Preset:   "status",
			},
			Prompt: "Show service health.",
		},
	)
	for _, want := range []string{
		"Treat emitted current-run step output, including structured JSON, NDJSON, and plain-language log lines",
		"If emitted step output contains values that answer the user instruction, copy those values exactly",
		"Do copy non-secret operational labels from emitted evidence exactly",
		"Do not substitute configured container images, runner/runtime images, LLM/agent metadata, operational log image-pull lines, or recent-history values",
		"For intent-level dashboard requests, infer the dashboard structure",
		"do not add generic run metadata",
		"Use run history, step, and task duration metadata for operational run timing only",
		"Prefer operationally relevant subjects over incidental personal/noise lines",
		"do not infer or invent",
		"Dashboard section: service-metrics",
		"Dashboard entry key: web-api-status",
		"Dashboard preset: status",
		`{"status":"success","error_count":5}`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("final output prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestWritePipelineFinalOutputRunHistoryIncludesEvidenceForDashboardPrompts(t *testing.T) {
	finishedAt := time.Date(2026, 7, 16, 18, 0, 45, 0, time.UTC)
	var builder strings.Builder
	writePipelineFinalOutputRunHistory(&builder, []models.RunListItem{{
		RunID:           "run-previous",
		PipelineName:    "image-build",
		PipelinePath:    "platform",
		PipelineVersion: "1.2.3",
		Status:          "failure",
		StartedAt:       finishedAt.Add(-45 * time.Second),
		FinishedAt:      finishedAt,
		Duration:        "45s",
		FailureReason:   "build timed out",
	}})
	out := builder.String()
	for _, want := range []string{
		"Recent pipeline history",
		"run-previous",
		"pipeline: platform/image-build",
		"version: 1.2.3",
		"status: failure",
		"duration: 45s",
		"finished_at: 2026-07-16T18:00:45Z",
		"failure_reason: build timed out",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("history context missing %q:\n%s", want, out)
		}
	}
}

func TestResolvePipelineFinalOutputProfileNameUsesItemOutputPipelineDefault(t *testing.T) {
	pipeline := models.Pipeline{
		LLMProfile: "pipeline-profile",
		Output:     models.PipelineOutput{LLMProfile: "output-profile"},
	}
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{LLMProfile: "item-profile"}); got != "item-profile" {
		t.Fatalf("item profile = %q, want item-profile", got)
	}
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "output-profile" {
		t.Fatalf("output profile = %q, want output-profile", got)
	}
	pipeline.Output.LLMProfile = ""
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "pipeline-profile" {
		t.Fatalf("pipeline profile = %q, want pipeline-profile", got)
	}
	pipeline.LLMProfile = ""
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "default" {
		t.Fatalf("default profile = %q, want default", got)
	}
}

func TestPipelineFinalOutputMatchesRunStatus(t *testing.T) {
	tests := []struct {
		name      string
		when      string
		runStatus string
		want      bool
	}{
		{name: "default always on success", runStatus: "success", want: true},
		{name: "default always on failure", runStatus: "failure", want: true},
		{name: "success matches success", when: "success", runStatus: "success", want: true},
		{name: "success skips failure", when: "success", runStatus: "failure", want: false},
		{name: "failure skips success", when: "failure", runStatus: "success", want: false},
		{name: "failure matches rejected", when: "failure", runStatus: "rejected", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pipelineFinalOutputMatchesRunStatus(tt.when, tt.runStatus); got != tt.want {
				t.Fatalf("pipelineFinalOutputMatchesRunStatus(%q, %q) = %v, want %v", tt.when, tt.runStatus, got, tt.want)
			}
		})
	}
}

func TestPipelineFinalOutputLLMClientForcesZeroTemperature(t *testing.T) {
	var temperature *float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			fmt.Fprint(w, `{"models":[{"type":"llm","key":"report-model","loaded_instances":[{"id":"report-model"}]}]}`)
		case "/api/v1/chat":
			var request struct {
				Temperature *float64 `json:"temperature"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			temperature = request.Temperature
			fmt.Fprint(w, `{"output":[{"type":"message","content":"<final_output>report</final_output>"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configuredTemperature := 0.8
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "reporting",
		LLMProfiles: map[string]config.LLMProfile{
			"reporting": {
				Provider:    config.LLMProviderLMStudio,
				BaseURL:     server.URL,
				Model:       "report-model",
				Temperature: &configuredTemperature,
			},
		},
	}}
	client, err := app.pipelineFinalOutputLLMClient(t.Context(), "reporting", "")
	if err != nil {
		t.Fatalf("pipelineFinalOutputLLMClient() error = %v", err)
	}
	if _, err := client.CompleteWithSystem(t.Context(), pipelineFinalOutputSystemInstruction, "Build report."); err != nil {
		t.Fatalf("CompleteWithSystem() error = %v", err)
	}
	if temperature == nil || *temperature != 0 {
		t.Fatalf("temperature = %#v, want 0", temperature)
	}
}

func TestPipelineFinalOutputAttemptUsageReportsIncludesRejectedRetry(t *testing.T) {
	output := pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{
			ID:   "output-1",
			Name: "Executive summary",
			Type: "markdown",
		},
	}
	result := pipelineFinalOutputGenerationResult{
		Attempts: []pipelineFinalOutputGenerationAttempt{
			{
				Completion: llmclient.Completion{Usage: llmclient.Usage{
					Provider:         "openai",
					Model:            "gpt-test",
					Profile:          "reporting",
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				}},
			},
			{
				Completion: llmclient.Completion{Usage: llmclient.Usage{
					Provider:     "openai",
					Model:        "gpt-test",
					Profile:      "reporting",
					PromptTokens: 12,
					TotalTokens:  18,
				}},
				ContractValid: true,
			},
			{},
		},
	}
	reports := pipelineFinalOutputAttemptUsageReports(output, result)
	if len(reports) != 2 {
		t.Fatalf("reports = %#v", reports)
	}
	if reports[0].Metadata["attempt"] != 1 ||
		reports[0].Metadata["retry"] != false ||
		reports[0].Metadata["contract_valid"] != false ||
		reports[1].Metadata["attempt"] != 2 ||
		reports[1].Metadata["retry"] != true ||
		reports[1].Metadata["contract_valid"] != true {
		t.Fatalf("reports = %#v", reports)
	}
}

func TestPipelineFinalOutputSchemaIncludesGenerationAndRenderAuditConstraints(t *testing.T) {
	combined := strings.Join(pipelineFinalOutputSchemaStatements, "\n")
	for _, expected := range []string{
		"generation_attempts INTEGER NOT NULL DEFAULT 0",
		"contract_violations INTEGER NOT NULL DEFAULT 0",
		"pipeline_run_outputs_generation_audit_check",
		"contract_violations <= generation_attempts",
		"render_attempts INTEGER NOT NULL DEFAULT 0",
		"render_failures INTEGER NOT NULL DEFAULT 0",
		"pipeline_run_outputs_render_audit_check",
		"render_failures <= render_attempts",
		"status IN ('pending', 'generating', 'success', 'failure', 'cancelled')",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("schema statements missing %q", expected)
		}
	}
}

func TestRenderPipelineFinalOutputDownloadFormats(t *testing.T) {
	pdfConverter := &recordingPDFConverter{payload: []byte("%PDF-1.7\nrendered")}
	pdfBytes, pdfType, pdfName, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Comparison Report",
		Type:    "pdf",
		Status:  finalOutputStatusSuccess,
		Content: `{"version":"1","title":"Security Issue Details","sections":[{"title":"Findings","blocks":[{"type":"table","table":{"columns":["Severity","Issue Type"],"rows":[["Critical","Disclosure"]]}},{"type":"paragraph","text":"Everything passed."}]}]}`,
	}, pdfConverter)
	if err != nil {
		t.Fatalf("render pdf error = %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.7")) {
		t.Fatalf("pdf bytes missing header: %q", string(pdfBytes[:8]))
	}
	if !bytes.Contains(pdfConverter.documentHTML, []byte("<table>")) || !bytes.Contains(pdfConverter.documentHTML, []byte("Disclosure")) {
		t.Fatalf("PDF HTML does not contain a real table: %s", pdfConverter.documentHTML)
	}
	if pdfType != "application/pdf" || pdfName != "comparison-report.pdf" {
		t.Fatalf("pdf metadata = %q %q", pdfType, pdfName)
	}

	xlsxBytes, xlsxType, xlsxName, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Data Table",
		Type:    "excel",
		Status:  finalOutputStatusSuccess,
		Content: `{"version":"1","title":"Data","sheets":[{"name":"Results","columns":[{"key":"name","header":"Name","width":24,"number_format":"text"},{"key":"value","header":"Value","number_format":"integer"}],"rows":[{"name":"API","value":42}],"freeze_header":true,"auto_filter":true}]}`,
	}, nil)
	if err != nil {
		t.Fatalf("render xlsx error = %v", err)
	}
	if xlsxType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || xlsxName != "data-table.xlsx" {
		t.Fatalf("xlsx metadata = %q %q", xlsxType, xlsxName)
	}
	if !bytes.HasPrefix(xlsxBytes, []byte("PK")) {
		t.Fatal("xlsx archive header missing")
	}
}

func TestRenderPipelineFinalOutputDownloadRejectsUnreadyOutput(t *testing.T) {
	_, _, _, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Summary",
		Type:    "markdown",
		Status:  finalOutputStatusRunning,
		Content: "still running",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("render error = %v, want not ready", err)
	}
}

type recordingPDFConverter struct {
	documentHTML []byte
	payload      []byte
}

func (c *recordingPDFConverter) ConvertHTML(_ context.Context, documentHTML []byte, _ string) ([]byte, error) {
	c.documentHTML = append([]byte(nil), documentHTML...)
	return c.payload, nil
}
