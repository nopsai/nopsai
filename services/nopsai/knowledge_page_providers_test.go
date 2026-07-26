package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKnowledgeProviderURLParsing(t *testing.T) {
	notionID, err := parseNotionPageID("https://www.notion.so/acme/Repository-Guardrails-1234567890abcdef1234567890abcdef?pvs=4")
	if err != nil {
		t.Fatalf("parseNotionPageID() error = %v", err)
	}
	if notionID != "12345678-90ab-cdef-1234-567890abcdef" {
		t.Fatalf("notion id = %q", notionID)
	}
	appNotionID, err := parseNotionPageID("https://app.notion.com/p/e213be946be88257b03681b18977a26b?v=25f3be946be8822490368883dc04de13")
	if err != nil {
		t.Fatalf("parseNotionPageID(app.notion.com) error = %v", err)
	}
	if appNotionID != "e213be94-6be8-8257-b036-81b18977a26b" {
		t.Fatalf("app notion id = %q", appNotionID)
	}
	confluenceID, err := parseConfluencePageID("https://acme.atlassian.net/wiki/spaces/SEC/pages/123456/Repository+Guardrails")
	if err != nil {
		t.Fatalf("parseConfluencePageID() error = %v", err)
	}
	if confluenceID != "123456" {
		t.Fatalf("confluence id = %q", confluenceID)
	}
}

func TestNotionBaseURLNormalizesWebPageURLsToAPI(t *testing.T) {
	for _, baseURL := range []string{
		"",
		"https://www.notion.so/acme/Repository-Guardrails-1234567890abcdef1234567890abcdef?pvs=4",
		"https://app.notion.com/p/e213be946be88257b03681b18977a26b?v=25f3be946be8822490368883dc04de13",
		"https://api.notion.com",
	} {
		connection := knowledgeConnectionRecord{
			knowledgeConnectionListItem: knowledgeConnectionListItem{BaseURL: baseURL},
		}
		if got := notionBaseURL(connection); got != "https://api.notion.com" {
			t.Fatalf("notionBaseURL(%q) = %q, want API base URL", baseURL, got)
		}
	}
}

func TestConfluenceProviderFetchesPromptFriendlyPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch {
		case strings.Contains(r.URL.Path, "/rest/api/content/search"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":    "123",
					"title": "Repository Guardrails",
					"_links": map[string]string{
						"webui": "/wiki/spaces/SEC/pages/123/Repository+Guardrails",
					},
					"version": map[string]string{"when": "2026-07-15T10:00:00Z"},
				}},
			})
		case strings.Contains(r.URL.Path, "/rest/api/content/123"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "123",
				"title": "Repository Guardrails",
				"_links": map[string]string{
					"webui": "/wiki/spaces/SEC/pages/123/Repository+Guardrails",
				},
				"version": map[string]string{"when": "2026-07-15T10:00:00Z"},
				"body": map[string]any{"storage": map[string]string{
					"value": `<h1>Repository Guardrails</h1>
						<p>Require signed commits.</p>
						<ul><li>Block secrets</li></ul>
						<table><tr><th>Check</th><th>Status</th></tr><tr><td>Secrets</td><td>Blocked</td></tr></table>
						<ac:image><ri:attachment ri:filename="deployment-flow.png" /></ac:image>
						<ac:structured-macro ac:name="chart" ac:macro-id="macro-1" />`,
				}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &App{
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{"credential://system/knowledge/confluence": "token"},
	}
	provider, err := app.knowledgePageProvider(knowledgeConnectionProviderConfluence)
	if err != nil {
		t.Fatalf("knowledgePageProvider() error = %v", err)
	}
	connection := knowledgeConnectionRecord{
		knowledgeConnectionListItem: knowledgeConnectionListItem{
			ID:       "security/confluence",
			Team:     "security",
			Name:     "confluence",
			Provider: knowledgeConnectionProviderConfluence,
			BaseURL:  server.URL,
		},
		credentialRef: "credential://system/knowledge/confluence",
	}
	result, err := provider.SearchPages(context.Background(), connection, "guardrails", "")
	if err != nil {
		t.Fatalf("SearchPages() error = %v", err)
	}
	if len(result.Pages) != 1 || result.Pages[0].ID != "123" {
		t.Fatalf("SearchPages() = %#v", result)
	}
	page, err := provider.GetPage(context.Background(), connection, "123")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if page.Title != "Repository Guardrails" || !strings.Contains(page.Text, "Require signed commits.") || page.Hash == "" {
		t.Fatalf("page = %#v", page)
	}
	if !strings.Contains(page.Text, "| Check | Status |") || !strings.Contains(page.Text, "[Asset preserved: image - deployment-flow.png]") {
		t.Fatalf("page text did not include converted table and asset placeholder:\n%s", page.Text)
	}
	if len(page.Assets) != 2 {
		t.Fatalf("assets = %#v, want image and macro", page.Assets)
	}
	if page.Assets[0].Kind != "image" && page.Assets[1].Kind != "image" {
		t.Fatalf("assets missing image: %#v", page.Assets)
	}
}

func TestNotionProviderFetchesPageBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer notion-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":               "12345678-90ab-cdef-1234-567890abcdef",
					"url":              "https://notion.test/page",
					"last_edited_time": "2026-07-15T10:00:00Z",
					"properties": map[string]any{"Name": map[string]any{
						"type":  "title",
						"title": []map[string]string{{"plain_text": "Repository Guardrails"}},
					}},
				}},
			})
		case "/v1/pages/12345678-90ab-cdef-1234-567890abcdef":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "12345678-90ab-cdef-1234-567890abcdef",
				"url":              "https://notion.test/page",
				"last_edited_time": "2026-07-15T10:00:00Z",
				"properties": map[string]any{"Name": map[string]any{
					"type":  "title",
					"title": []map[string]string{{"plain_text": "Repository Guardrails"}},
				}},
			})
		case "/v1/blocks/12345678-90ab-cdef-1234-567890abcdef/children":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"id":        "block-1",
						"type":      "heading_1",
						"heading_1": map[string]any{"rich_text": []map[string]string{{"plain_text": "Repository Guardrails"}}},
					},
					{
						"id":        "block-2",
						"type":      "paragraph",
						"paragraph": map[string]any{"rich_text": []map[string]string{{"plain_text": "Require reviewed changes."}}},
					},
					{
						"id":           "block-table",
						"type":         "table",
						"has_children": true,
						"table": map[string]any{
							"table_width":       2,
							"has_column_header": true,
						},
					},
					{
						"id":   "block-image",
						"type": "image",
						"image": map[string]any{
							"type":     "external",
							"caption":  []map[string]string{{"plain_text": "Deployment flow"}},
							"external": map[string]string{"url": "https://assets.test/deployment-flow.png"},
						},
					},
				},
			})
		case "/v1/blocks/block-table/children":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"id":   "row-1",
						"type": "table_row",
						"table_row": map[string]any{"cells": []any{
							[]map[string]string{{"plain_text": "Check"}},
							[]map[string]string{{"plain_text": "Status"}},
						}},
					},
					{
						"id":   "row-2",
						"type": "table_row",
						"table_row": map[string]any{"cells": []any{
							[]map[string]string{{"plain_text": "Review"}},
							[]map[string]string{{"plain_text": "Required"}},
						}},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &App{
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{"credential://system/knowledge/notion": "notion-token"},
	}
	provider, err := app.knowledgePageProvider(knowledgeConnectionProviderNotion)
	if err != nil {
		t.Fatalf("knowledgePageProvider() error = %v", err)
	}
	connection := knowledgeConnectionRecord{
		knowledgeConnectionListItem: knowledgeConnectionListItem{
			ID:       "security/notion",
			Team:     "security",
			Name:     "notion",
			Provider: knowledgeConnectionProviderNotion,
			BaseURL:  server.URL,
		},
		credentialRef: "credential://system/knowledge/notion",
	}
	result, err := provider.SearchPages(context.Background(), connection, "guardrails", "")
	if err != nil {
		t.Fatalf("SearchPages() error = %v", err)
	}
	if len(result.Pages) != 1 || result.Pages[0].Title != "Repository Guardrails" {
		t.Fatalf("SearchPages() = %#v", result)
	}
	page, err := provider.GetPage(context.Background(), connection, "12345678-90ab-cdef-1234-567890abcdef")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if page.Title != "Repository Guardrails" || !strings.Contains(page.Text, "Require reviewed changes.") || page.Hash == "" {
		t.Fatalf("page = %#v", page)
	}
	if !strings.Contains(page.Text, "| Check | Status |") || !strings.Contains(page.Text, "[Asset preserved: image - Deployment flow]") {
		t.Fatalf("page text did not include converted table and asset placeholder:\n%s", page.Text)
	}
	if len(page.Assets) != 1 || page.Assets[0].Kind != "image" || page.Assets[0].MediaType != "image/png" {
		t.Fatalf("assets = %#v, want preserved image with inferred media type", page.Assets)
	}
}
