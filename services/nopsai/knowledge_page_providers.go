package nopsai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/credentials"
)

type Connection = knowledgeConnectionRecord

type KnowledgePageProvider interface {
	TestConnection(ctx context.Context, connection Connection) error
	SearchPages(ctx context.Context, connection Connection, query string, cursor string) (PageSearchResult, error)
	GetPage(ctx context.Context, connection Connection, pageID string) (ExternalPage, error)
	ResolvePage(ctx context.Context, connection Connection, pageURL string) (ExternalPage, error)
}

type PageSearchResult struct {
	Pages      []ExternalPageSummary `json:"pages"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type ExternalPageSummary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	URL        string     `json:"url,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	Snippet    string     `json:"snippet,omitempty"`
}

type ExternalPage struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	Text       string     `json:"text"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	Hash       string     `json:"hash"`
}

type knowledgeProviderErrorKind string

const (
	knowledgeProviderErrorAuthentication  knowledgeProviderErrorKind = "authentication_required"
	knowledgeProviderErrorPermission      knowledgeProviderErrorKind = "permission_denied"
	knowledgeProviderErrorUnavailable     knowledgeProviderErrorKind = "provider_unavailable"
	knowledgeProviderErrorDisabled        knowledgeProviderErrorKind = "connection_disabled"
	knowledgeProviderErrorPageUnavailable knowledgeProviderErrorKind = "page_unavailable"
	knowledgeProviderErrorPageTooLarge    knowledgeProviderErrorKind = "page_too_large"
	knowledgeProviderErrorInvalidRequest  knowledgeProviderErrorKind = "invalid_request"
)

type knowledgeProviderError struct {
	Kind       knowledgeProviderErrorKind
	StatusCode int
	Message    string
}

func (e knowledgeProviderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func newKnowledgeProviderError(kind knowledgeProviderErrorKind, statusCode int, message string) error {
	return knowledgeProviderError{Kind: kind, StatusCode: statusCode, Message: strings.TrimSpace(message)}
}

func knowledgeProviderErrorStatus(err error) string {
	var providerErr knowledgeProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case knowledgeProviderErrorAuthentication:
			return knowledgeConnectionStatusAuthenticationRequired
		case knowledgeProviderErrorPermission:
			return knowledgeConnectionStatusPermissionDenied
		case knowledgeProviderErrorPageUnavailable:
			return "page_unavailable"
		case knowledgeProviderErrorDisabled:
			return knowledgeConnectionStatusDisabled
		default:
			return knowledgeConnectionStatusProviderUnavailable
		}
	}
	return knowledgeConnectionStatusProviderUnavailable
}

func (a *App) knowledgePageProvider(provider string) (KnowledgePageProvider, error) {
	client := http.DefaultClient
	if a != nil && a.httpClient != nil {
		client = a.httpClient
	}
	common := knowledgeHTTPProvider{
		client: client,
		resolveToken: func(ctx context.Context, connection Connection, operation string) (string, error) {
			return a.resolveKnowledgeConnectionCredential(ctx, connection, operation)
		},
	}
	normalized, err := normalizeKnowledgeConnectionProvider(provider)
	if err != nil {
		return nil, err
	}
	switch normalized {
	case knowledgeConnectionProviderNotion:
		return notionKnowledgeProvider{knowledgeHTTPProvider: common}, nil
	case knowledgeConnectionProviderConfluence, knowledgeConnectionProviderWiki:
		return confluenceKnowledgeProvider{knowledgeHTTPProvider: common}, nil
	default:
		return nil, fmt.Errorf("unsupported knowledge provider %q", provider)
	}
}

func (a *App) resolveKnowledgeConnectionCredential(ctx context.Context, connection Connection, operation string) (string, error) {
	if strings.TrimSpace(connection.credentialRef) == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorAuthentication, http.StatusUnauthorized, "Authentication is not configured.")
	}
	value, err := a.resolveCredentialText(ctx, connection.credentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "knowledge_connection." + strings.ToLower(strings.TrimSpace(operation)),
		SubjectType:     grantResourceKnowledgeConnection,
		SubjectID:       connection.ID,
	})
	if err != nil {
		return "", newKnowledgeProviderError(knowledgeProviderErrorAuthentication, http.StatusUnauthorized, "Knowledge provider credential could not be resolved.")
	}
	if strings.TrimSpace(value) == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorAuthentication, http.StatusUnauthorized, "Knowledge provider credential is empty.")
	}
	return value, nil
}

type knowledgeHTTPProvider struct {
	client       *http.Client
	resolveToken func(context.Context, Connection, string) (string, error)
}

func (p knowledgeHTTPProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return http.DefaultClient
}

func (p knowledgeHTTPProvider) token(ctx context.Context, connection Connection, operation string) (string, error) {
	if p.resolveToken == nil {
		return "", newKnowledgeProviderError(knowledgeProviderErrorAuthentication, http.StatusUnauthorized, "Knowledge provider credential resolver is unavailable.")
	}
	return p.resolveToken(ctx, connection, operation)
}

type notionKnowledgeProvider struct {
	knowledgeHTTPProvider
}

func (p notionKnowledgeProvider) TestConnection(ctx context.Context, connection Connection) error {
	_, err := p.SearchPages(ctx, connection, "", "")
	return err
}

func (p notionKnowledgeProvider) SearchPages(ctx context.Context, connection Connection, query string, cursor string) (PageSearchResult, error) {
	token, err := p.token(ctx, connection, "search")
	if err != nil {
		return PageSearchResult{}, err
	}
	body := map[string]any{
		"page_size": 10,
		"filter": map[string]string{
			"property": "object",
			"value":    "page",
		},
	}
	if strings.TrimSpace(query) != "" {
		body["query"] = strings.TrimSpace(query)
	}
	if strings.TrimSpace(cursor) != "" {
		body["start_cursor"] = strings.TrimSpace(cursor)
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notionBaseURL(connection)+"/v1/search", bytes.NewReader(raw))
	if err != nil {
		return PageSearchResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", notionVersion(connection))
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return PageSearchResult{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Notion search request failed.")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return PageSearchResult{}, providerHTTPError(resp, "Notion search request failed.")
	}
	var payload struct {
		Results    []notionPageObject `json:"results"`
		HasMore    bool               `json:"has_more"`
		NextCursor string             `json:"next_cursor"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return PageSearchResult{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, resp.StatusCode, "Notion search response could not be decoded.")
	}
	result := PageSearchResult{NextCursor: payload.NextCursor}
	if !payload.HasMore {
		result.NextCursor = ""
	}
	for _, item := range payload.Results {
		modified := parseProviderTime(item.LastEditedTime)
		result.Pages = append(result.Pages, ExternalPageSummary{
			ID:         item.ID,
			Title:      firstNonEmptyString(notionPageTitle(item), "Untitled page"),
			URL:        item.URL,
			ModifiedAt: modified,
		})
	}
	return result, nil
}

func (p notionKnowledgeProvider) ResolvePage(ctx context.Context, connection Connection, pageURL string) (ExternalPage, error) {
	pageID, err := parseNotionPageID(pageURL)
	if err != nil {
		return ExternalPage{}, err
	}
	return p.GetPage(ctx, connection, pageID)
}

func (p notionKnowledgeProvider) GetPage(ctx context.Context, connection Connection, pageID string) (ExternalPage, error) {
	pageID = normalizeNotionPageID(pageID)
	if pageID == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Notion page ID is required.")
	}
	token, err := p.token(ctx, connection, "get_page")
	if err != nil {
		return ExternalPage{}, err
	}
	page, err := p.getNotionPageMetadata(ctx, connection, token, pageID)
	if err != nil {
		return ExternalPage{}, err
	}
	lines, err := p.getNotionBlockText(ctx, connection, token, pageID, 0)
	if err != nil {
		return ExternalPage{}, err
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageUnavailable, http.StatusNotFound, "Notion page has no prompt-friendly text.")
	}
	modified := parseProviderTime(page.LastEditedTime)
	return ExternalPage{
		ID:         page.ID,
		Title:      firstNonEmptyString(notionPageTitle(page), "Untitled page"),
		URL:        page.URL,
		Text:       text,
		ModifiedAt: modified,
		Hash:       hashKnowledgeText(text),
	}, nil
}

func (p notionKnowledgeProvider) getNotionPageMetadata(ctx context.Context, connection Connection, token, pageID string) (notionPageObject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, notionBaseURL(connection)+"/v1/pages/"+url.PathEscape(pageID), nil)
	if err != nil {
		return notionPageObject{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionVersion(connection))
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return notionPageObject{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Notion page request failed.")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return notionPageObject{}, providerHTTPError(resp, "Notion page request failed.")
	}
	var page notionPageObject
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&page); err != nil {
		return notionPageObject{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, resp.StatusCode, "Notion page response could not be decoded.")
	}
	return page, nil
}

func (p notionKnowledgeProvider) getNotionBlockText(ctx context.Context, connection Connection, token, blockID string, depth int) ([]string, error) {
	if depth > 6 {
		return nil, nil
	}
	var lines []string
	cursor := ""
	for {
		endpoint := notionBaseURL(connection) + "/v1/blocks/" + url.PathEscape(blockID) + "/children?page_size=100"
		if cursor != "" {
			endpoint += "&start_cursor=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Notion-Version", notionVersion(connection))
		resp, err := p.httpClient().Do(req)
		if err != nil {
			return nil, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Notion block request failed.")
		}
		if resp.StatusCode >= 300 {
			err := providerHTTPError(resp, "Notion block request failed.")
			resp.Body.Close()
			return nil, err
		}
		var payload struct {
			Results    []notionBlockObject `json:"results"`
			HasMore    bool                `json:"has_more"`
			NextCursor string              `json:"next_cursor"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, resp.StatusCode, "Notion block response could not be decoded.")
		}
		resp.Body.Close()
		for _, block := range payload.Results {
			if line := notionBlockText(block); line != "" {
				lines = append(lines, line)
			}
			if block.HasChildren {
				childLines, err := p.getNotionBlockText(ctx, connection, token, block.ID, depth+1)
				if err != nil {
					return nil, err
				}
				lines = append(lines, childLines...)
			}
			if len(strings.Join(lines, "\n")) > 1_000_000 {
				return nil, newKnowledgeProviderError(knowledgeProviderErrorPageTooLarge, http.StatusRequestEntityTooLarge, "Provider page is too large to use as Knowledge Context.")
			}
		}
		if !payload.HasMore || payload.NextCursor == "" {
			break
		}
		cursor = payload.NextCursor
	}
	return lines, nil
}

type notionPageObject struct {
	ID             string                        `json:"id"`
	URL            string                        `json:"url"`
	LastEditedTime string                        `json:"last_edited_time"`
	Properties     map[string]notionPageProperty `json:"properties"`
}

type notionPageProperty struct {
	Type     string           `json:"type"`
	Title    []notionRichText `json:"title"`
	RichText []notionRichText `json:"rich_text"`
}

type notionBlockObject struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	HasChildren      bool            `json:"has_children"`
	Paragraph        notionTextBlock `json:"paragraph"`
	Heading1         notionTextBlock `json:"heading_1"`
	Heading2         notionTextBlock `json:"heading_2"`
	Heading3         notionTextBlock `json:"heading_3"`
	BulletedListItem notionTextBlock `json:"bulleted_list_item"`
	NumberedListItem notionTextBlock `json:"numbered_list_item"`
	ToDo             notionTextBlock `json:"to_do"`
	Toggle           notionTextBlock `json:"toggle"`
	Quote            notionTextBlock `json:"quote"`
	Callout          notionTextBlock `json:"callout"`
	Code             notionCodeBlock `json:"code"`
}

type notionTextBlock struct {
	RichText []notionRichText `json:"rich_text"`
}

type notionCodeBlock struct {
	RichText []notionRichText `json:"rich_text"`
	Language string           `json:"language"`
}

type notionRichText struct {
	PlainText string `json:"plain_text"`
}

func notionBaseURL(connection Connection) string {
	base := strings.TrimRight(strings.TrimSpace(connection.BaseURL), "/")
	if base == "" {
		return "https://api.notion.com"
	}
	if parsed, err := url.Parse(base); err == nil {
		host := strings.ToLower(parsed.Hostname())
		switch {
		case host == "api.notion.com":
			return "https://api.notion.com"
		case host == "notion.so" || strings.HasSuffix(host, ".notion.so"):
			return "https://api.notion.com"
		case host == "notion.com" || strings.HasSuffix(host, ".notion.com"):
			return "https://api.notion.com"
		}
	}
	return base
}

func notionVersion(connection Connection) string {
	if value, ok := connection.Config["notion_version"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "2022-06-28"
}

func notionPageTitle(page notionPageObject) string {
	for _, prop := range page.Properties {
		if prop.Type == "title" && len(prop.Title) > 0 {
			return richTextPlain(prop.Title)
		}
	}
	return ""
}

func notionBlockText(block notionBlockObject) string {
	switch block.Type {
	case "paragraph":
		return richTextPlain(block.Paragraph.RichText)
	case "heading_1":
		return "# " + richTextPlain(block.Heading1.RichText)
	case "heading_2":
		return "## " + richTextPlain(block.Heading2.RichText)
	case "heading_3":
		return "### " + richTextPlain(block.Heading3.RichText)
	case "bulleted_list_item":
		return "- " + richTextPlain(block.BulletedListItem.RichText)
	case "numbered_list_item":
		return "1. " + richTextPlain(block.NumberedListItem.RichText)
	case "to_do":
		return "- [ ] " + richTextPlain(block.ToDo.RichText)
	case "toggle":
		return richTextPlain(block.Toggle.RichText)
	case "quote":
		return "> " + richTextPlain(block.Quote.RichText)
	case "callout":
		return richTextPlain(block.Callout.RichText)
	case "code":
		text := richTextPlain(block.Code.RichText)
		if text == "" {
			return ""
		}
		return "```" + strings.TrimSpace(block.Code.Language) + "\n" + text + "\n```"
	default:
		return ""
	}
}

func richTextPlain(values []notionRichText) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.PlainText) != "" {
			parts = append(parts, value.PlainText)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

var notionPageIDPattern = regexp.MustCompile(`(?i)([0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[?#/]|$)`)
var notionURLPattern = regexp.MustCompile(`https?://[^\s)]+`)

func parseNotionPageID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Notion page URL is required.")
	}
	if id := normalizeNotionPageID(value); len(strings.ReplaceAll(id, "-", "")) == 32 {
		return id, nil
	}
	for _, candidate := range notionURLCandidates(value) {
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		if id := firstNotionPageIDMatch(parsed.Path); id != "" {
			return normalizeNotionPageID(id), nil
		}
	}
	if id := firstNotionPageIDMatch(value); id != "" {
		return normalizeNotionPageID(id), nil
	}
	return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Could not find a Notion page ID in the URL.")
}

func notionURLCandidates(value string) []string {
	var candidates []string
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		candidates = append(candidates, value)
	}
	for _, match := range notionURLPattern.FindAllString(value, -1) {
		seen := false
		for _, candidate := range candidates {
			if candidate == match {
				seen = true
				break
			}
		}
		if !seen {
			candidates = append(candidates, match)
		}
	}
	return candidates
}

func firstNotionPageIDMatch(value string) string {
	matches := notionPageIDPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[0][1]
}

func normalizeNotionPageID(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.Trim(value, "/")
	value = strings.ReplaceAll(value, "-", "")
	if len(value) != 32 {
		return strings.TrimSpace(raw)
	}
	return value[0:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}

type confluenceKnowledgeProvider struct {
	knowledgeHTTPProvider
}

func (p confluenceKnowledgeProvider) TestConnection(ctx context.Context, connection Connection) error {
	_, err := p.SearchPages(ctx, connection, "", "")
	return err
}

func (p confluenceKnowledgeProvider) SearchPages(ctx context.Context, connection Connection, query string, cursor string) (PageSearchResult, error) {
	token, err := p.token(ctx, connection, "search")
	if err != nil {
		return PageSearchResult{}, err
	}
	base, err := confluenceBaseURL(connection)
	if err != nil {
		return PageSearchResult{}, err
	}
	cql := "type=page"
	if strings.TrimSpace(query) != "" {
		cql += ` AND text ~ "` + strings.ReplaceAll(strings.TrimSpace(query), `"`, `\"`) + `"`
	}
	endpoint := base + "/rest/api/content/search?limit=10&expand=version&_links=true&cql=" + url.QueryEscape(cql)
	if strings.TrimSpace(cursor) != "" {
		endpoint += "&start=" + url.QueryEscape(strings.TrimSpace(cursor))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PageSearchResult{}, err
	}
	p.setConfluenceAuth(req, connection, token)
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return PageSearchResult{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Confluence search request failed.")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return PageSearchResult{}, providerHTTPError(resp, "Confluence search request failed.")
	}
	var payload struct {
		Results []struct {
			ID      string            `json:"id"`
			Title   string            `json:"title"`
			Links   map[string]string `json:"_links"`
			Version struct {
				When string `json:"when"`
			} `json:"version"`
		} `json:"results"`
		Size  int               `json:"size"`
		Start int               `json:"start"`
		Limit int               `json:"limit"`
		Links map[string]string `json:"_links"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return PageSearchResult{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, resp.StatusCode, "Confluence search response could not be decoded.")
	}
	result := PageSearchResult{}
	for _, item := range payload.Results {
		result.Pages = append(result.Pages, ExternalPageSummary{
			ID:         item.ID,
			Title:      firstNonEmptyString(strings.TrimSpace(item.Title), "Untitled page"),
			URL:        absoluteProviderURL(base, firstNonEmptyString(item.Links["webui"], item.Links["self"])),
			ModifiedAt: parseProviderTime(item.Version.When),
		})
	}
	if next := payload.Links["next"]; next != "" {
		if parsed, err := url.Parse(next); err == nil {
			result.NextCursor = parsed.Query().Get("start")
		}
	}
	return result, nil
}

func (p confluenceKnowledgeProvider) ResolvePage(ctx context.Context, connection Connection, pageURL string) (ExternalPage, error) {
	pageID, err := parseConfluencePageID(pageURL)
	if err != nil {
		return ExternalPage{}, err
	}
	return p.GetPage(ctx, connection, pageID)
}

func (p confluenceKnowledgeProvider) GetPage(ctx context.Context, connection Connection, pageID string) (ExternalPage, error) {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Confluence page ID is required.")
	}
	token, err := p.token(ctx, connection, "get_page")
	if err != nil {
		return ExternalPage{}, err
	}
	base, err := confluenceBaseURL(connection)
	if err != nil {
		return ExternalPage{}, err
	}
	endpoint := base + "/rest/api/content/" + url.PathEscape(pageID) + "?expand=body.storage,version,history,_links"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ExternalPage{}, err
	}
	p.setConfluenceAuth(req, connection, token)
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Confluence page request failed.")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ExternalPage{}, providerHTTPError(resp, "Confluence page request failed.")
	}
	var payload struct {
		ID    string            `json:"id"`
		Title string            `json:"title"`
		Links map[string]string `json:"_links"`
		Body  struct {
			Storage struct {
				Value string `json:"value"`
			} `json:"storage"`
		} `json:"body"`
		Version struct {
			When string `json:"when"`
		} `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, resp.StatusCode, "Confluence page response could not be decoded.")
	}
	text := strings.TrimSpace(htmlToPromptText(payload.Body.Storage.Value))
	if text == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageUnavailable, http.StatusNotFound, "Confluence page has no prompt-friendly text.")
	}
	if len(text) > 1_000_000 {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageTooLarge, http.StatusRequestEntityTooLarge, "Provider page is too large to use as Knowledge Context.")
	}
	return ExternalPage{
		ID:         firstNonEmptyString(payload.ID, pageID),
		Title:      firstNonEmptyString(strings.TrimSpace(payload.Title), "Untitled page"),
		URL:        absoluteProviderURL(base, firstNonEmptyString(payload.Links["webui"], payload.Links["self"])),
		Text:       text,
		ModifiedAt: parseProviderTime(payload.Version.When),
		Hash:       hashKnowledgeText(text),
	}, nil
}

func (p confluenceKnowledgeProvider) setConfluenceAuth(req *http.Request, connection Connection, token string) {
	authType := strings.ToLower(strings.TrimSpace(knowledgeProviderStringMapValue(connection.Config, "auth_type")))
	username := strings.TrimSpace(knowledgeProviderStringMapValue(connection.Config, "username"))
	if authType == "basic" || username != "" || strings.Count(token, ":") == 1 {
		if username == "" && strings.Count(token, ":") == 1 {
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(token)))
			return
		}
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+token)))
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func confluenceBaseURL(connection Connection) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(connection.BaseURL), "/")
	if base == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Confluence base URL is required.")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Confluence base URL is invalid.")
	}
	if strings.HasSuffix(parsed.Path, "/wiki") {
		return base, nil
	}
	return base, nil
}

var confluencePagePathPattern = regexp.MustCompile(`/pages/([0-9]+)(?:/|$)`)

func parseConfluencePageID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Confluence page URL is required.")
	}
	if regexp.MustCompile(`^[0-9]+$`).MatchString(value) {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err == nil {
		if pageID := parsed.Query().Get("pageId"); pageID != "" {
			return pageID, nil
		}
	}
	matches := confluencePagePathPattern.FindStringSubmatch(value)
	if len(matches) > 1 {
		return matches[1], nil
	}
	return "", newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, http.StatusBadRequest, "Could not find a Confluence page ID in the URL.")
}

func providerHTTPError(resp *http.Response, fallback string) error {
	status := resp.StatusCode
	kind := knowledgeProviderErrorUnavailable
	switch status {
	case http.StatusUnauthorized:
		kind = knowledgeProviderErrorAuthentication
	case http.StatusForbidden:
		kind = knowledgeProviderErrorPermission
	case http.StatusNotFound:
		kind = knowledgeProviderErrorPageUnavailable
	case http.StatusRequestEntityTooLarge:
		kind = knowledgeProviderErrorPageTooLarge
	case http.StatusBadRequest:
		kind = knowledgeProviderErrorInvalidRequest
	}
	message := strings.TrimSpace(fallback)
	var payload map[string]any
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if len(raw) > 0 && json.Unmarshal(raw, &payload) == nil {
		for _, key := range []string{"message", "error", "error_description"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				message = strings.TrimSpace(value)
				break
			}
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return newKnowledgeProviderError(kind, status, message)
}

func parseProviderTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func hashKnowledgeText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func absoluteProviderURL(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	parsedBase, err := url.Parse(base)
	if err != nil {
		return value
	}
	parsedValue, err := url.Parse(value)
	if err != nil {
		return value
	}
	return parsedBase.ResolveReference(parsedValue).String()
}

func htmlToPromptText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	replacements := []struct{ old, new string }{
		{"</p>", "\n\n"},
		{"</div>", "\n"},
		{"</li>", "\n"},
		{"<br>", "\n"},
		{"<br/>", "\n"},
		{"<br />", "\n"},
		{"</h1>", "\n\n"},
		{"</h2>", "\n\n"},
		{"</h3>", "\n\n"},
		{"</tr>", "\n"},
		{"</table>", "\n"},
	}
	for _, replacement := range replacements {
		value = strings.ReplaceAll(value, replacement.old, replacement.new)
	}
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	value = tagPattern.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func knowledgeProviderStringMapValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}
