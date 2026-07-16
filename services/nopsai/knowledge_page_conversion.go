package nopsai

import (
	"bytes"
	stdhtml "html"
	"mime"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

type ExternalPageAsset struct {
	SourceBlockID   string         `json:"source_block_id"`
	SourceBlockType string         `json:"source_block_type"`
	Kind            string         `json:"kind"`
	Title           string         `json:"title,omitempty"`
	URL             string         `json:"url,omitempty"`
	MediaType       string         `json:"media_type,omitempty"`
	ContentHash     string         `json:"content_hash,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

func confluenceStorageToPromptContent(value, pageURL string) (string, []ExternalPageAsset) {
	root, err := xhtml.Parse(strings.NewReader("<div>" + strings.ReplaceAll(value, "\r\n", "\n") + "</div>"))
	if err != nil {
		return htmlToPromptText(value), nil
	}
	var builder knowledgePageMarkdownBuilder
	builder.pageURL = strings.TrimSpace(pageURL)
	for _, node := range htmlElementChildren(root) {
		builder.renderConfluenceNode(node, 0)
	}
	return builder.String(), dedupeExternalPageAssets(builder.assets)
}

type knowledgePageMarkdownBuilder struct {
	lines   []string
	assets  []ExternalPageAsset
	pageURL string
}

func (b *knowledgePageMarkdownBuilder) String() string {
	return strings.TrimSpace(strings.Join(compactMarkdownLines(b.lines), "\n"))
}

func (b *knowledgePageMarkdownBuilder) appendLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	b.lines = append(b.lines, line)
}

func (b *knowledgePageMarkdownBuilder) appendBlank() {
	if len(b.lines) == 0 || b.lines[len(b.lines)-1] == "" {
		return
	}
	b.lines = append(b.lines, "")
}

func (b *knowledgePageMarkdownBuilder) appendAsset(asset ExternalPageAsset) string {
	asset = normalizeExternalPageAsset(asset)
	if asset.SourceBlockID == "" {
		return ""
	}
	b.assets = append(b.assets, asset)
	return assetPromptPlaceholder(asset)
}

func (b *knowledgePageMarkdownBuilder) renderConfluenceNode(node *xhtml.Node, listDepth int) {
	if node == nil {
		return
	}
	if node.Type == xhtml.TextNode {
		b.appendLine(strings.TrimSpace(stdhtml.UnescapeString(node.Data)))
		return
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			b.renderConfluenceNode(child, listDepth)
		}
		return
	}
	name := strings.ToLower(node.Data)
	switch name {
	case "html", "head", "body", "div", "section", "article", "tbody", "thead":
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			b.renderConfluenceNode(child, listDepth)
		}
		if name == "div" || name == "section" || name == "article" {
			b.appendBlank()
		}
	case "p":
		b.appendLine(confluenceInlineMarkdown(node, b, true))
		b.appendBlank()
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(name[1] - '0')
		if level < 1 || level > 6 {
			level = 2
		}
		b.appendLine(strings.Repeat("#", level) + " " + confluenceInlineMarkdown(node, b, true))
		b.appendBlank()
	case "ul", "ol":
		b.renderConfluenceList(node, name == "ol", listDepth)
		b.appendBlank()
	case "blockquote":
		text := confluenceInlineMarkdown(node, b, true)
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				b.appendLine("> " + line)
			}
		}
		b.appendBlank()
	case "pre":
		text := strings.TrimSpace(textContent(node))
		if text != "" {
			b.appendLine("```\n" + text + "\n```")
			b.appendBlank()
		}
	case "code":
		b.appendLine("`" + strings.TrimSpace(textContent(node)) + "`")
	case "table":
		lines, ok := confluenceTableMarkdown(node)
		if ok {
			for _, line := range lines {
				b.appendLine(line)
			}
		} else {
			placeholder := b.appendAsset(confluenceNodeAsset(node, "table", "table", b.pageURL))
			b.appendLine(placeholder)
		}
		b.appendBlank()
	case "br":
		b.appendBlank()
	case "img", "ac:image", "ac:structured-macro", "iframe", "object", "embed", "svg":
		b.appendLine(b.appendAsset(confluenceAssetFromNode(node, b.pageURL)))
		b.appendBlank()
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			b.renderConfluenceNode(child, listDepth)
		}
		if isConfluenceBlockElement(name) {
			b.appendBlank()
		}
	}
}

func (b *knowledgePageMarkdownBuilder) renderConfluenceList(list *xhtml.Node, ordered bool, depth int) {
	for child := list.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != xhtml.ElementNode || strings.ToLower(child.Data) != "li" {
			continue
		}
		prefix := "- "
		if ordered {
			prefix = "1. "
		}
		indent := strings.Repeat("  ", depth)
		text := confluenceInlineMarkdownWithoutNestedLists(child, b)
		if text != "" {
			b.appendLine(indent + prefix + text)
		}
		for nested := child.FirstChild; nested != nil; nested = nested.NextSibling {
			if nested.Type != xhtml.ElementNode {
				continue
			}
			nestedName := strings.ToLower(nested.Data)
			if nestedName == "ul" || nestedName == "ol" {
				b.renderConfluenceList(nested, nestedName == "ol", depth+1)
			}
		}
	}
}

func confluenceInlineMarkdownWithoutNestedLists(node *xhtml.Node, builder *knowledgePageMarkdownBuilder) string {
	var clone strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			name := strings.ToLower(child.Data)
			if name == "ul" || name == "ol" {
				continue
			}
		}
		clone.WriteString(confluenceInlineMarkdown(child, builder, true))
	}
	return normalizeInlineWhitespace(clone.String())
}

func confluenceInlineMarkdown(node *xhtml.Node, builder *knowledgePageMarkdownBuilder, preserveAssets bool) string {
	if node == nil {
		return ""
	}
	if node.Type == xhtml.TextNode {
		return stdhtml.UnescapeString(node.Data)
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		var out strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			out.WriteString(confluenceInlineMarkdown(child, builder, preserveAssets))
		}
		return out.String()
	}
	name := strings.ToLower(node.Data)
	switch name {
	case "br":
		return "\n"
	case "code":
		text := normalizeInlineWhitespace(textContent(node))
		if text == "" {
			return ""
		}
		return "`" + strings.ReplaceAll(text, "`", "'") + "`"
	case "strong", "b":
		text := normalizeInlineWhitespace(confluenceChildrenInlineMarkdown(node, builder, preserveAssets))
		if text == "" {
			return ""
		}
		return "**" + text + "**"
	case "em", "i":
		text := normalizeInlineWhitespace(confluenceChildrenInlineMarkdown(node, builder, preserveAssets))
		if text == "" {
			return ""
		}
		return "*" + text + "*"
	case "a":
		text := normalizeInlineWhitespace(confluenceChildrenInlineMarkdown(node, builder, preserveAssets))
		href := strings.TrimSpace(htmlAttr(node, "href"))
		if text == "" {
			text = href
		}
		if href == "" || text == "" {
			return text
		}
		return "[" + strings.ReplaceAll(text, "]", "\\]") + "](" + href + ")"
	case "img", "ac:image", "ac:structured-macro", "iframe", "object", "embed", "svg":
		if !preserveAssets || builder == nil {
			return ""
		}
		return builder.appendAsset(confluenceAssetFromNode(node, builder.pageURL))
	default:
		return confluenceChildrenInlineMarkdown(node, builder, preserveAssets)
	}
}

func confluenceChildrenInlineMarkdown(node *xhtml.Node, builder *knowledgePageMarkdownBuilder, preserveAssets bool) string {
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(confluenceInlineMarkdown(child, builder, preserveAssets))
	}
	return out.String()
}

func confluenceTableMarkdown(table *xhtml.Node) ([]string, bool) {
	if containsConfluenceUnsupportedTableContent(table) {
		return nil, false
	}
	rows := confluenceTableRows(table)
	if len(rows) == 0 {
		return nil, false
	}
	hasHeader := false
	for cell := rows[0].FirstChild; cell != nil; cell = cell.NextSibling {
		if cell.Type == xhtml.ElementNode && strings.ToLower(cell.Data) == "th" {
			hasHeader = true
			break
		}
	}
	values := make([][]string, 0, len(rows))
	maxCols := 0
	for _, row := range rows {
		var cells []string
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type != xhtml.ElementNode {
				continue
			}
			cellName := strings.ToLower(cell.Data)
			if cellName != "td" && cellName != "th" {
				continue
			}
			cells = append(cells, normalizeInlineWhitespace(confluenceInlineMarkdown(cell, nil, false)))
		}
		if len(cells) > maxCols {
			maxCols = len(cells)
		}
		values = append(values, cells)
	}
	if maxCols == 0 {
		return nil, false
	}
	for idx := range values {
		for len(values[idx]) < maxCols {
			values[idx] = append(values[idx], "")
		}
	}
	return strings.Split(markdownTable(values, hasHeader), "\n"), true
}

func confluenceTableRows(table *xhtml.Node) []*xhtml.Node {
	var rows []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node == nil {
			return
		}
		if node.Type == xhtml.ElementNode && strings.ToLower(node.Data) == "tr" {
			rows = append(rows, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(table)
	return rows
}

func containsConfluenceUnsupportedTableContent(table *xhtml.Node) bool {
	unsupported := false
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node == nil || unsupported {
			return
		}
		if node.Type == xhtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "table", "img", "ac:image", "ac:structured-macro", "iframe", "object", "embed", "svg":
				if node != table {
					unsupported = true
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(table)
	return unsupported
}

func confluenceAssetFromNode(node *xhtml.Node, pageURL string) ExternalPageAsset {
	name := strings.ToLower(node.Data)
	switch name {
	case "ac:image":
		return confluenceNodeAsset(node, "image", firstNonEmptyString(htmlAttr(node, "ac:alt"), confluenceAttachmentName(node), "Confluence image"), pageURL)
	case "img":
		return confluenceNodeAsset(node, "image", firstNonEmptyString(htmlAttr(node, "alt"), filepath.Base(htmlAttr(node, "src")), "Image"), pageURL)
	case "ac:structured-macro":
		macroName := firstNonEmptyString(htmlAttr(node, "ac:name"), "macro")
		return confluenceNodeAsset(node, "macro", macroName, pageURL)
	case "svg":
		return confluenceNodeAsset(node, "diagram", "SVG diagram", pageURL)
	default:
		return confluenceNodeAsset(node, "embed", firstNonEmptyString(name, "embedded content"), pageURL)
	}
}

func confluenceNodeAsset(node *xhtml.Node, kind, title, pageURL string) ExternalPageAsset {
	sourceType := strings.ToLower(node.Data)
	sourceURL := firstNonEmptyString(htmlAttr(node, "src"), htmlAttr(node, "href"), confluenceURLResource(node))
	filename := confluenceAttachmentName(node)
	if title == "" {
		title = firstNonEmptyString(filename, sourceType, kind)
	}
	sourceHash := hashKnowledgeText(renderHTMLNode(node))
	return ExternalPageAsset{
		SourceBlockID:   firstNonEmptyString(htmlAttr(node, "id"), htmlAttr(node, "ac:local-id"), htmlAttr(node, "ac:macro-id"), sourceHash, filename),
		SourceBlockType: sourceType,
		Kind:            strings.TrimSpace(kind),
		Title:           strings.TrimSpace(title),
		URL:             strings.TrimSpace(sourceURL),
		MediaType:       mediaTypeFromURL(sourceURL, kind),
		ContentHash:     sourceHash,
		Metadata: map[string]any{
			"provider_block_type": sourceType,
			"page_url":            strings.TrimSpace(pageURL),
			"source_hash":         sourceHash,
			"filename":            filename,
			"macro_name":          htmlAttr(node, "ac:name"),
		},
	}
}

func confluenceAttachmentName(node *xhtml.Node) string {
	var found string
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current == nil || found != "" {
			return
		}
		if current.Type == xhtml.ElementNode {
			found = firstNonEmptyString(htmlAttr(current, "ri:filename"), htmlAttr(current, "filename"))
			if found != "" {
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(found)
}

func confluenceURLResource(node *xhtml.Node) string {
	var found string
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current == nil || found != "" {
			return
		}
		if current.Type == xhtml.ElementNode {
			found = firstNonEmptyString(htmlAttr(current, "ri:value"), htmlAttr(current, "ri:url"), htmlAttr(current, "url"))
			if found != "" {
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(found)
}

func htmlElementChildren(root *xhtml.Node) []*xhtml.Node {
	var children []*xhtml.Node
	var collect func(*xhtml.Node)
	collect = func(node *xhtml.Node) {
		if node == nil {
			return
		}
		if node.Type == xhtml.ElementNode && strings.ToLower(node.Data) == "div" {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				children = append(children, child)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(root)
	return children
}

func htmlAttr(node *xhtml.Node, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == name {
			return strings.TrimSpace(stdhtml.UnescapeString(attr.Val))
		}
	}
	return ""
}

func textContent(node *xhtml.Node) string {
	var out strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current == nil {
			return
		}
		if current.Type == xhtml.TextNode {
			out.WriteString(current.Data)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return stdhtml.UnescapeString(out.String())
}

func renderHTMLNode(node *xhtml.Node) string {
	var out bytes.Buffer
	if node != nil {
		_ = xhtml.Render(&out, node)
	}
	return out.String()
}

func isConfluenceBlockElement(name string) bool {
	switch strings.ToLower(name) {
	case "div", "section", "article", "p", "table", "tr", "ul", "ol", "li", "blockquote", "pre":
		return true
	default:
		return false
	}
}

func markdownTable(rows [][]string, firstRowHeader bool) string {
	if len(rows) == 0 {
		return ""
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return ""
	}
	header := make([]string, maxCols)
	var dataRows [][]string
	if firstRowHeader {
		copy(header, rows[0])
		dataRows = rows[1:]
	} else {
		for idx := range header {
			header[idx] = "Column " + strconv.Itoa(idx+1)
		}
		dataRows = rows
	}
	for idx := range header {
		header[idx] = markdownTableCell(firstNonEmptyString(header[idx], "Column "+strconv.Itoa(idx+1)))
	}
	var out []string
	out = append(out, "| "+strings.Join(header, " | ")+" |")
	separators := make([]string, maxCols)
	for idx := range separators {
		separators[idx] = "---"
	}
	out = append(out, "| "+strings.Join(separators, " | ")+" |")
	for _, row := range dataRows {
		cells := make([]string, maxCols)
		for idx := range cells {
			if idx < len(row) {
				cells[idx] = markdownTableCell(row[idx])
			}
		}
		out = append(out, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(out, "\n")
}

func markdownTableCell(value string) string {
	value = normalizeInlineWhitespace(strings.ReplaceAll(value, "\n", "<br>"))
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func assetPromptPlaceholder(asset ExternalPageAsset) string {
	kind := firstNonEmptyString(asset.Kind, "asset")
	title := strings.TrimSpace(asset.Title)
	if title == "" {
		title = strings.TrimSpace(asset.SourceBlockType)
	}
	if title == "" {
		title = strings.TrimSpace(asset.SourceBlockID)
	}
	if title == "" {
		return "[Asset preserved: " + kind + "]"
	}
	return "[Asset preserved: " + kind + " - " + title + "]"
}

func preservedAssetOnlyContent(assets []ExternalPageAsset) string {
	var lines []string
	for _, asset := range dedupeExternalPageAssets(assets) {
		lines = append(lines, assetPromptPlaceholder(asset))
	}
	return strings.Join(lines, "\n")
}

func dedupeExternalPageAssets(assets []ExternalPageAsset) []ExternalPageAsset {
	seen := map[string]struct{}{}
	out := make([]ExternalPageAsset, 0, len(assets))
	for _, asset := range assets {
		asset = normalizeExternalPageAsset(asset)
		key := strings.Join([]string{asset.SourceBlockID, asset.Kind, asset.URL, asset.ContentHash}, "\x00")
		if _, ok := seen[key]; ok || strings.TrimSpace(asset.SourceBlockID) == "" {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, asset)
	}
	return out
}

func normalizeExternalPageAsset(asset ExternalPageAsset) ExternalPageAsset {
	asset.SourceBlockID = strings.TrimSpace(asset.SourceBlockID)
	asset.SourceBlockType = strings.TrimSpace(asset.SourceBlockType)
	asset.Kind = strings.TrimSpace(asset.Kind)
	asset.Title = strings.TrimSpace(asset.Title)
	asset.URL = strings.TrimSpace(asset.URL)
	asset.MediaType = strings.TrimSpace(asset.MediaType)
	asset.ContentHash = strings.TrimSpace(asset.ContentHash)
	if asset.ContentHash == "" {
		asset.ContentHash = hashKnowledgeText(asset.SourceBlockID + ":" + asset.Kind + ":" + asset.URL + ":" + asset.Title)
	}
	if asset.Metadata == nil {
		asset.Metadata = map[string]any{}
	}
	return asset
}

func mediaTypeFromURL(rawURL, fallbackKind string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(rawURL)))
	if ext == "" {
		switch strings.ToLower(strings.TrimSpace(fallbackKind)) {
		case "image":
			return "image/*"
		case "pdf":
			return "application/pdf"
		case "video":
			return "video/*"
		case "audio":
			return "audio/*"
		default:
			return ""
		}
	}
	if mediaType := mime.TypeByExtension(ext); mediaType != "" {
		return mediaType
	}
	return ""
}

func normalizeInlineWhitespace(value string) string {
	value = stdhtml.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = regexp.MustCompile(`[ \t\f\v]+`).ReplaceAllString(value, " ")
	lines := strings.Split(value, "\n")
	for idx, line := range lines {
		lines[idx] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func compactMarkdownLines(lines []string) []string {
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
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
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
	value = stdhtml.UnescapeString(value)
	return strings.Join(compactMarkdownLines(strings.Split(value, "\n")), "\n")
}
