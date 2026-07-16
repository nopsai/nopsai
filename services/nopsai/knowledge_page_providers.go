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
	ID         string              `json:"id"`
	Title      string              `json:"title"`
	URL        string              `json:"url"`
	Text       string              `json:"text"`
	Assets     []ExternalPageAsset `json:"assets,omitempty"`
	ModifiedAt *time.Time          `json:"modified_at,omitempty"`
	Hash       string              `json:"hash"`
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
	lines, assets, err := p.getNotionBlockContent(ctx, connection, token, pageID, 0)
	if err != nil {
		return ExternalPage{}, err
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" && len(assets) > 0 {
		text = preservedAssetOnlyContent(assets)
	}
	if text == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageUnavailable, http.StatusNotFound, "Notion page has no prompt-friendly text.")
	}
	modified := parseProviderTime(page.LastEditedTime)
	return ExternalPage{
		ID:         page.ID,
		Title:      firstNonEmptyString(notionPageTitle(page), "Untitled page"),
		URL:        page.URL,
		Text:       text,
		Assets:     assets,
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

func (p notionKnowledgeProvider) getNotionBlockContent(ctx context.Context, connection Connection, token, blockID string, depth int) ([]string, []ExternalPageAsset, error) {
	if depth > 6 {
		return nil, nil, nil
	}
	var lines []string
	var assets []ExternalPageAsset
	blocks, err := p.getNotionChildBlocks(ctx, connection, token, blockID)
	if err != nil {
		return nil, nil, err
	}
	for _, block := range blocks {
		if block.Type == "table" {
			tableText, tableAssets, err := p.getNotionTableMarkdown(ctx, connection, token, block.ID, block.Table, depth+1)
			if err != nil {
				return nil, nil, err
			}
			if tableText != "" {
				lines = append(lines, tableText)
			}
			assets = append(assets, tableAssets...)
			continue
		}
		if line := notionBlockText(block); line != "" {
			lines = append(lines, line)
		}
		if asset, ok := notionBlockAsset(block); ok {
			assets = append(assets, asset)
			lines = append(lines, assetPromptPlaceholder(asset))
		}
		if block.HasChildren {
			childLines, childAssets, err := p.getNotionBlockContent(ctx, connection, token, block.ID, depth+1)
			if err != nil {
				return nil, nil, err
			}
			lines = append(lines, childLines...)
			assets = append(assets, childAssets...)
		}
		if len(strings.Join(lines, "\n")) > 1_000_000 {
			return nil, nil, newKnowledgeProviderError(knowledgeProviderErrorPageTooLarge, http.StatusRequestEntityTooLarge, "Provider page is too large to use as Knowledge Context.")
		}
	}
	return lines, dedupeExternalPageAssets(assets), nil
}

func (p notionKnowledgeProvider) getNotionChildBlocks(ctx context.Context, connection Connection, token, blockID string) ([]notionBlockObject, error) {
	var blocks []notionBlockObject
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
		blocks = append(blocks, payload.Results...)
		if !payload.HasMore || payload.NextCursor == "" {
			break
		}
		cursor = payload.NextCursor
	}
	return blocks, nil
}

func (p notionKnowledgeProvider) getNotionTableMarkdown(ctx context.Context, connection Connection, token, tableID string, table notionTableBlock, depth int) (string, []ExternalPageAsset, error) {
	if depth > 6 {
		return "", nil, nil
	}
	rows, err := p.getNotionChildBlocks(ctx, connection, token, tableID)
	if err != nil {
		return "", nil, err
	}
	cells := make([][]string, 0, len(rows))
	var assets []ExternalPageAsset
	for _, row := range rows {
		if row.Type != "table_row" {
			if asset, ok := notionUnsupportedBlockAsset(row); ok {
				assets = append(assets, asset)
			}
			continue
		}
		values := make([]string, 0, len(row.TableRow.Cells))
		for _, cell := range row.TableRow.Cells {
			values = append(values, richTextPlain(cell))
		}
		cells = append(cells, values)
	}
	if len(cells) == 0 {
		asset := ExternalPageAsset{
			SourceBlockID:   tableID,
			SourceBlockType: "table",
			Kind:            "table",
			Title:           "Empty Notion table",
			ContentHash:     hashKnowledgeText(tableID + ":empty-table"),
			Metadata: map[string]any{
				"reason":             "empty_table",
				"notion_table_width": table.TableWidth,
			},
		}
		return assetPromptPlaceholder(asset), []ExternalPageAsset{asset}, nil
	}
	return markdownTable(cells, table.HasColumnHeader), assets, nil
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
	ID               string              `json:"id"`
	Type             string              `json:"type"`
	HasChildren      bool                `json:"has_children"`
	Paragraph        notionTextBlock     `json:"paragraph"`
	Heading1         notionTextBlock     `json:"heading_1"`
	Heading2         notionTextBlock     `json:"heading_2"`
	Heading3         notionTextBlock     `json:"heading_3"`
	BulletedListItem notionTextBlock     `json:"bulleted_list_item"`
	NumberedListItem notionTextBlock     `json:"numbered_list_item"`
	ToDo             notionTextBlock     `json:"to_do"`
	Toggle           notionTextBlock     `json:"toggle"`
	Quote            notionTextBlock     `json:"quote"`
	Callout          notionTextBlock     `json:"callout"`
	Code             notionCodeBlock     `json:"code"`
	Table            notionTableBlock    `json:"table"`
	TableRow         notionTableRowBlock `json:"table_row"`
	Image            notionFileBlock     `json:"image"`
	File             notionFileBlock     `json:"file"`
	PDF              notionFileBlock     `json:"pdf"`
	Video            notionFileBlock     `json:"video"`
	Audio            notionFileBlock     `json:"audio"`
	Embed            notionURLBlock      `json:"embed"`
	Bookmark         notionURLBlock      `json:"bookmark"`
	LinkPreview      notionURLBlock      `json:"link_preview"`
	ChildPage        notionTitleBlock    `json:"child_page"`
}

type notionTextBlock struct {
	RichText []notionRichText `json:"rich_text"`
}

type notionCodeBlock struct {
	RichText []notionRichText `json:"rich_text"`
	Language string           `json:"language"`
}

type notionTableBlock struct {
	TableWidth      int  `json:"table_width"`
	HasColumnHeader bool `json:"has_column_header"`
	HasRowHeader    bool `json:"has_row_header"`
}

type notionTableRowBlock struct {
	Cells [][]notionRichText `json:"cells"`
}

type notionFileBlock struct {
	Type     string           `json:"type"`
	Caption  []notionRichText `json:"caption"`
	External *struct {
		URL string `json:"url"`
	} `json:"external"`
	File *struct {
		URL string `json:"url"`
	} `json:"file"`
}

type notionURLBlock struct {
	URL     string           `json:"url"`
	Caption []notionRichText `json:"caption"`
}

type notionTitleBlock struct {
	Title string `json:"title"`
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

func notionBlockAsset(block notionBlockObject) (ExternalPageAsset, bool) {
	switch block.Type {
	case "image":
		return notionFileAsset(block, "image", block.Image), true
	case "file":
		return notionFileAsset(block, "file", block.File), true
	case "pdf":
		return notionFileAsset(block, "pdf", block.PDF), true
	case "video":
		return notionFileAsset(block, "video", block.Video), true
	case "audio":
		return notionFileAsset(block, "audio", block.Audio), true
	case "embed":
		return notionURLAsset(block, "embed", block.Embed), true
	case "bookmark":
		return notionURLAsset(block, "bookmark", block.Bookmark), true
	case "link_preview":
		return notionURLAsset(block, "link_preview", block.LinkPreview), true
	case "child_page":
		title := strings.TrimSpace(block.ChildPage.Title)
		return ExternalPageAsset{
			SourceBlockID:   block.ID,
			SourceBlockType: block.Type,
			Kind:            "linked_page",
			Title:           firstNonEmptyString(title, "Linked Notion page"),
			ContentHash:     hashKnowledgeText(block.ID + ":" + block.Type + ":" + title),
			Metadata:        map[string]any{"notion_block_type": block.Type},
		}, true
	default:
		return notionUnsupportedBlockAsset(block)
	}
}

func notionUnsupportedBlockAsset(block notionBlockObject) (ExternalPageAsset, bool) {
	switch block.Type {
	case "", "paragraph", "heading_1", "heading_2", "heading_3", "bulleted_list_item", "numbered_list_item", "to_do", "toggle", "quote", "callout", "code", "table", "table_row", "column_list", "column", "divider", "breadcrumb", "unsupported":
		if block.Type != "unsupported" {
			return ExternalPageAsset{}, false
		}
	}
	return ExternalPageAsset{
		SourceBlockID:   block.ID,
		SourceBlockType: block.Type,
		Kind:            "unsupported",
		Title:           firstNonEmptyString(block.Type, "Unsupported Notion block"),
		ContentHash:     hashKnowledgeText(block.ID + ":" + block.Type),
		Metadata:        map[string]any{"notion_block_type": block.Type},
	}, true
}

func notionFileAsset(block notionBlockObject, kind string, file notionFileBlock) ExternalPageAsset {
	sourceURL := ""
	if file.External != nil {
		sourceURL = strings.TrimSpace(file.External.URL)
	}
	if sourceURL == "" && file.File != nil {
		sourceURL = strings.TrimSpace(file.File.URL)
	}
	title := richTextPlain(file.Caption)
	if title == "" {
		title = strings.TrimSpace(kind)
	}
	return ExternalPageAsset{
		SourceBlockID:   block.ID,
		SourceBlockType: block.Type,
		Kind:            kind,
		Title:           title,
		URL:             sourceURL,
		MediaType:       mediaTypeFromURL(sourceURL, kind),
		ContentHash:     hashKnowledgeText(block.ID + ":" + kind + ":" + sourceURL),
		Metadata: map[string]any{
			"notion_block_type": block.Type,
			"notion_file_type":  strings.TrimSpace(file.Type),
		},
	}
}

func notionURLAsset(block notionBlockObject, kind string, value notionURLBlock) ExternalPageAsset {
	sourceURL := strings.TrimSpace(value.URL)
	title := richTextPlain(value.Caption)
	if title == "" {
		title = strings.TrimSpace(kind)
	}
	return ExternalPageAsset{
		SourceBlockID:   block.ID,
		SourceBlockType: block.Type,
		Kind:            kind,
		Title:           title,
		URL:             sourceURL,
		MediaType:       mediaTypeFromURL(sourceURL, kind),
		ContentHash:     hashKnowledgeText(block.ID + ":" + kind + ":" + sourceURL),
		Metadata:        map[string]any{"notion_block_type": block.Type},
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
	pageURL := absoluteProviderURL(base, firstNonEmptyString(payload.Links["webui"], payload.Links["self"]))
	text, assets := confluenceStorageToPromptContent(payload.Body.Storage.Value, pageURL)
	text = strings.TrimSpace(text)
	if text == "" && len(assets) > 0 {
		text = preservedAssetOnlyContent(assets)
	}
	if text == "" {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageUnavailable, http.StatusNotFound, "Confluence page has no prompt-friendly text.")
	}
	if len(text) > 1_000_000 {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorPageTooLarge, http.StatusRequestEntityTooLarge, "Provider page is too large to use as Knowledge Context.")
	}
	return ExternalPage{
		ID:         firstNonEmptyString(payload.ID, pageID),
		Title:      firstNonEmptyString(strings.TrimSpace(payload.Title), "Untitled page"),
		URL:        pageURL,
		Text:       text,
		Assets:     assets,
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
