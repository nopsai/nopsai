package nopsai

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"nopsai/pkg/models"
)

var outputFileNameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)
var (
	markdownLinkPattern          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	markdownOrderedListPattern   = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
	markdownUnorderedListPattern = regexp.MustCompile(`^[-*+]\s+(.+)$`)
)

type pdfLine struct {
	Text    string
	Font    string
	Size    int
	Leading int
}

func renderPipelineFinalOutputDownload(output models.PipelineRunFinalOutput) ([]byte, string, string, error) {
	if strings.TrimSpace(output.Status) != finalOutputStatusSuccess {
		return nil, "", "", fmt.Errorf("final output is not ready")
	}
	content := strings.TrimSpace(output.Content)
	if content == "" {
		return nil, "", "", fmt.Errorf("final output is empty")
	}
	outputType := normalizePipelineFinalOutputType(output.Type)
	fileName := pipelineFinalOutputFileName(output.Name, outputType)
	switch outputType {
	case "markdown":
		return []byte(content), "text/markdown; charset=utf-8", fileName, nil
	case "json":
		return []byte(content), "application/json; charset=utf-8", fileName, nil
	case "html":
		return []byte(content), "text/html; charset=utf-8", fileName, nil
	case "pdf":
		return buildSimplePDF(output.Name, content), "application/pdf", fileName, nil
	case "excel":
		payload, err := buildSimpleXLSX(output.Name, content)
		if err != nil {
			return nil, "", "", err
		}
		return payload, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", fileName, nil
	default:
		return []byte(content), "text/plain; charset=utf-8", pipelineFinalOutputFileName(output.Name, "txt"), nil
	}
}

func pipelineFinalOutputFileName(name, outputType string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = strings.ReplaceAll(base, " ", "-")
	base = outputFileNameUnsafe.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-_.")
	if base == "" {
		base = "final-output"
	}
	ext := pipelineFinalOutputExtension(outputType)
	return base + "." + ext
}

func pipelineFinalOutputExtension(outputType string) string {
	switch normalizePipelineFinalOutputType(outputType) {
	case "markdown":
		return "md"
	case "pdf":
		return "pdf"
	case "excel":
		return "xlsx"
	case "json":
		return "json"
	case "html":
		return "html"
	default:
		return strings.Trim(strings.TrimSpace(outputType), ".")
	}
}

func buildSimplePDF(title, content string) []byte {
	lines := markdownPDFLines(title, content)
	pages := paginatePDFLines(lines)
	if len(pages) == 0 {
		pages = [][]pdfLine{{{Text: "", Font: "F1", Size: 10, Leading: 12}}}
	}

	objects := []string{}
	objects = append(objects, "<< /Type /Catalog /Pages 2 0 R >>")

	pageObjectIDs := make([]int, 0, len(pages))
	contentObjectIDs := make([]int, 0, len(pages))
	nextObjectID := 3
	for range pages {
		pageObjectIDs = append(pageObjectIDs, nextObjectID)
		contentObjectIDs = append(contentObjectIDs, nextObjectID+1)
		nextObjectID += 2
	}
	regularFontObjectID := nextObjectID
	boldFontObjectID := nextObjectID + 1
	monoFontObjectID := nextObjectID + 2

	kids := make([]string, 0, len(pageObjectIDs))
	for _, id := range pageObjectIDs {
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
	}
	objects = append(objects, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageObjectIDs)))

	for idx, pageLines := range pages {
		pageID := pageObjectIDs[idx]
		contentID := contentObjectIDs[idx]
		for len(objects) < pageID-1 {
			objects = append(objects, "")
		}
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R /F3 %d 0 R >> >> /Contents %d 0 R >>", regularFontObjectID, boldFontObjectID, monoFontObjectID, contentID))

		stream := pdfTextStream(pageLines)
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for idx, object := range objects {
		objectID := idx + 1
		offsets[objectID] = out.Len()
		out.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", objectID, object))
	}
	xrefOffset := out.Len()
	out.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	out.WriteString("0000000000 65535 f \n")
	for objectID := 1; objectID <= len(objects); objectID++ {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[objectID]))
	}
	out.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset))
	return out.Bytes()
}

func pdfTextStream(lines []pdfLine) string {
	var builder strings.Builder
	builder.WriteString("BT\n")
	y := 760
	for _, line := range lines {
		leading := line.Leading
		if leading <= 0 {
			leading = line.Size + 4
		}
		if strings.TrimSpace(line.Text) == "" {
			y -= leading
			continue
		}
		font := line.Font
		if font == "" {
			font = "F1"
		}
		size := line.Size
		if size <= 0 {
			size = 10
		}
		builder.WriteString(fmt.Sprintf("/%s %d Tf\n1 0 0 1 50 %d Tm\n", font, size, y))
		builder.WriteString("(" + escapePDFString(line.Text) + ") Tj\n")
		y -= leading
	}
	builder.WriteString("ET")
	return builder.String()
}

func markdownPDFLines(title, content string) []pdfLine {
	lines := []pdfLine{}
	appendWrappedPDFLine(&lines, strings.TrimSpace(title), "F2", 18, 22)
	lines = append(lines, pdfLine{Leading: 12})

	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for idx := 0; idx < len(rawLines); {
		raw := strings.TrimRight(rawLines[idx], " \t")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			lines = append(lines, pdfLine{Leading: 10})
			idx++
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			idx++
			for idx < len(rawLines) && !strings.HasPrefix(strings.TrimSpace(rawLines[idx]), "```") {
				appendWrappedPDFLine(&lines, strings.TrimRight(rawLines[idx], " \t"), "F3", 9, 12)
				idx++
			}
			if idx < len(rawLines) {
				idx++
			}
			continue
		}
		if markdownTableLine(trimmed) {
			rows := [][]string{}
			for idx < len(rawLines) && markdownTableLine(strings.TrimSpace(rawLines[idx])) {
				cells := splitMarkdownTableLine(strings.TrimSpace(rawLines[idx]))
				if len(cells) > 0 && !markdownSeparatorRow(cells) {
					rows = append(rows, cells)
				}
				idx++
			}
			appendPDFTable(&lines, rows)
			continue
		}
		if strings.Trim(trimmed, "-*_ ") == "" {
			lines = append(lines, pdfLine{Leading: 8})
			idx++
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			text := strings.TrimSpace(trimmed[level:])
			if text != "" {
				size := 16
				leading := 20
				if level == 2 {
					size = 14
					leading = 18
				} else if level >= 3 {
					size = 12
					leading = 16
				}
				appendWrappedPDFLine(&lines, stripMarkdownInline(text), "F2", size, leading)
			}
			idx++
			continue
		}
		if match := markdownUnorderedListPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			appendWrappedPDFLine(&lines, "- "+stripMarkdownInline(match[1]), "F1", 10, 14)
			idx++
			continue
		}
		if match := markdownOrderedListPattern.FindStringSubmatch(trimmed); len(match) == 3 {
			appendWrappedPDFLine(&lines, match[1]+". "+stripMarkdownInline(match[2]), "F1", 10, 14)
			idx++
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			appendWrappedPDFLine(&lines, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")), "F1", 10, 14)
			idx++
			continue
		}
		appendWrappedPDFLine(&lines, stripMarkdownInline(trimmed), "F1", 10, 14)
		idx++
	}
	return lines
}

func markdownTableLine(line string) bool {
	cells := splitMarkdownTableLine(line)
	return strings.Contains(line, "|") && len(cells) > 1
}

func appendPDFTable(lines *[]pdfLine, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, 0)
	for _, row := range rows {
		for idx, cell := range row {
			if idx >= 6 {
				break
			}
			for len(widths) <= idx {
				widths = append(widths, 0)
			}
			clean := stripMarkdownInline(cell)
			if len(clean) > widths[idx] {
				widths[idx] = len(clean)
			}
		}
	}
	for idx, width := range widths {
		if width > 24 {
			widths[idx] = 24
		}
	}
	for rowIdx, row := range rows {
		cells := make([]string, 0, len(widths))
		for colIdx := range widths {
			cell := ""
			if colIdx < len(row) {
				cell = truncatePDFCell(stripMarkdownInline(row[colIdx]), widths[colIdx])
			}
			cells = append(cells, padRight(cell, widths[colIdx]))
		}
		font := "F3"
		if rowIdx == 0 {
			font = "F2"
		}
		appendWrappedPDFLine(lines, strings.Join(cells, "  "), font, 9, 12)
	}
	*lines = append(*lines, pdfLine{Leading: 8})
}

func stripMarkdownInline(value string) string {
	value = markdownLinkPattern.ReplaceAllString(value, "$1")
	for _, marker := range []string{"**", "__", "`", "*", "_"} {
		value = strings.ReplaceAll(value, marker, "")
	}
	return strings.TrimSpace(value)
}

func appendWrappedPDFLine(lines *[]pdfLine, text, font string, size, leading int) {
	width := 92
	if size >= 16 {
		width = 58
	} else if size >= 14 {
		width = 66
	} else if size >= 12 {
		width = 76
	} else if font == "F3" {
		width = 82
	}
	for _, line := range wrapPDFText(text, width) {
		*lines = append(*lines, pdfLine{Text: line, Font: font, Size: size, Leading: leading})
	}
}

func wrapPDFText(content string, width int) []string {
	if width <= 0 {
		return []string{strings.TrimSpace(content)}
	}
	line := strings.TrimSpace(content)
	if line == "" {
		return []string{""}
	}
	out := []string{}
	for len([]rune(line)) > width {
		runes := []rune(line)
		cut := width
		for cut > width/2 && runes[cut] != ' ' {
			cut--
		}
		if cut <= width/2 {
			cut = width
		}
		out = append(out, strings.TrimSpace(string(runes[:cut])))
		line = strings.TrimSpace(string(runes[cut:]))
	}
	out = append(out, line)
	return out
}

func paginatePDFLines(lines []pdfLine) [][]pdfLine {
	pages := [][]pdfLine{}
	current := []pdfLine{}
	y := 760
	for _, line := range lines {
		leading := line.Leading
		if leading <= 0 {
			leading = line.Size + 4
		}
		if len(current) > 0 && y-leading < 50 {
			pages = append(pages, current)
			current = []pdfLine{}
			y = 760
		}
		current = append(current, line)
		y -= leading
	}
	if len(current) > 0 {
		pages = append(pages, current)
	}
	return pages
}

func truncatePDFCell(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "."
}

func padRight(value string, width int) string {
	for len([]rune(value)) < width {
		value += " "
	}
	return value
}

func escapePDFString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "(", "\\(")
	value = strings.ReplaceAll(value, ")", "\\)")
	return value
}

func wrapPDFLines(content string, width int) []string {
	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := []string{}
	for _, raw := range rawLines {
		line := strings.TrimRight(raw, " \t")
		if line == "" {
			out = append(out, "")
			continue
		}
		for len([]rune(line)) > width {
			runes := []rune(line)
			cut := width
			for cut > width/2 && runes[cut] != ' ' {
				cut--
			}
			if cut <= width/2 {
				cut = width
			}
			out = append(out, strings.TrimSpace(string(runes[:cut])))
			line = strings.TrimSpace(string(runes[cut:]))
		}
		out = append(out, line)
	}
	return out
}

func chunkStrings(values []string, size int) [][]string {
	if size <= 0 {
		return nil
	}
	chunks := [][]string{}
	for len(values) > 0 {
		end := size
		if len(values) < end {
			end = len(values)
		}
		chunks = append(chunks, values[:end])
		values = values[end:]
	}
	return chunks
}

func buildSimpleXLSX(sheetName, content string) ([]byte, error) {
	rows := tableRowsFromOutput(content)
	if len(rows) == 0 {
		rows = [][]string{{content}}
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        xlsxContentTypesXML,
		"_rels/.rels":                xlsxRootRelsXML,
		"xl/workbook.xml":            xlsxWorkbookXML(safeSheetName(sheetName)),
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRelsXML,
		"xl/worksheets/sheet1.xml":   xlsxWorksheetXML(rows),
		"xl/styles.xml":              xlsxStylesXML,
		"docProps/core.xml":          xlsxCoreXML,
		"docProps/app.xml":           xlsxAppXML,
	}
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func tableRowsFromOutput(content string) [][]string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	rows := [][]string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "|") {
			cells := splitMarkdownTableLine(line)
			if len(cells) == 0 || markdownSeparatorRow(cells) {
				continue
			}
			rows = append(rows, cells)
			continue
		}
		if strings.Contains(line, ",") {
			rows = append(rows, splitCSVLikeLine(line))
			continue
		}
		rows = append(rows, []string{line})
	}
	if len(rows) > 500 {
		rows = rows[:500]
	}
	return rows
}

func splitMarkdownTableLine(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}

func markdownSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		trimmed := strings.Trim(cell, " :-")
		if trimmed != "" {
			return false
		}
	}
	return true
}

func splitCSVLikeLine(line string) []string {
	parts := strings.Split(line, ",")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.Trim(strings.TrimSpace(part), `"`))
	}
	return cells
}

func xlsxWorksheetXML(rows [][]string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	builder.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIdx, row := range rows {
		rowNumber := rowIdx + 1
		builder.WriteString(`<row r="` + strconv.Itoa(rowNumber) + `">`)
		for colIdx, cell := range row {
			if colIdx >= 50 {
				break
			}
			ref := xlsxColumnName(colIdx+1) + strconv.Itoa(rowNumber)
			builder.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t>`)
			builder.WriteString(html.EscapeString(cell))
			builder.WriteString(`</t></is></c>`)
		}
		builder.WriteString(`</row>`)
	}
	builder.WriteString(`</sheetData></worksheet>`)
	return builder.String()
}

func xlsxColumnName(index int) string {
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

func safeSheetName(name string) string {
	name = strings.TrimSpace(name)
	for _, bad := range []string{":", "\\", "/", "?", "*", "[", "]"} {
		name = strings.ReplaceAll(name, bad, " ")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Final Output"
	}
	runes := []rune(name)
	if len(runes) > 31 {
		name = string(runes[:31])
	}
	return name
}

func xlsxWorkbookXML(sheetName string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + html.EscapeString(sheetName) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`
}

const xlsxContentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`
const xlsxRootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`
const xlsxWorkbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
const xlsxStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="1"><fill><patternFill patternType="none"/></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf/></cellStyleXfs><cellXfs count="1"><xf xfId="0"/></cellXfs></styleSheet>`
const xlsxCoreXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:creator>Nopsai</dc:creator></cp:coreProperties>`
const xlsxAppXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Nopsai</Application></Properties>`
