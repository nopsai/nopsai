package nopsai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nopsai/pkg/models"
)

const (
	maxDocumentSections     = 100
	maxDocumentBlocks       = 500
	maxDocumentTableColumns = 30
	maxDocumentTableRows    = 5000
	maxDocumentTextBytes    = 2 << 20
	maxSpreadsheetSheets    = 20
	maxSpreadsheetColumns   = 100
	maxSpreadsheetRows      = 10000
	maxSpreadsheetCellBytes = 32767
	maxDashboardBlocks      = 200
	maxDashboardTableCols   = 30
	maxDashboardTableRows   = 5000
	maxDashboardTextBytes   = 2 << 20
	maxDashboardChartSeries = 20
	maxDashboardChartPoints = 10000
)

var spreadsheetColumnKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var dashboardSeriesKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var dashboardColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)
var dashboardHTMLPattern = regexp.MustCompile(`(?i)<\s*/?\s*[a-z][^>]*>`)
var dashboardExecutableContentPattern = regexp.MustCompile(`(?i)(javascript:|data:text/html|<\s*(script|style|iframe|form|object|embed|link|meta)\b|on[a-z]+\s*=|style\s*=)`)

var supportedSpreadsheetNumberFormats = map[string]struct{}{
	"": {}, "text": {}, "integer": {}, "decimal": {}, "percent": {},
	"currency_usd": {}, "currency_eur": {}, "date": {}, "datetime": {}, "boolean": {},
}

var supportedDashboardTones = map[string]struct{}{
	"": {}, "neutral": {}, "info": {}, "success": {}, "warning": {}, "critical": {},
}

var supportedDashboardStatuses = map[string]struct{}{
	"": {}, "neutral": {}, "info": {}, "success": {}, "warning": {}, "critical": {}, "pending": {}, "running": {},
}

var supportedDashboardChartTypes = map[string]struct{}{
	"line": {}, "bar": {}, "area": {}, "pie": {}, "donut": {},
}

var supportedDashboardMissingValues = map[string]struct{}{
	"": {}, "gap": {}, "zero": {}, "null": {}, "previous": {},
}

func parseDocumentSpec(content string) (models.DocumentSpec, error) {
	var spec models.DocumentSpec
	if err := decodeStrictFinalOutputJSON(content, &spec); err != nil {
		return spec, fmt.Errorf("invalid DocumentSpec: %w", err)
	}
	if err := validateDocumentSpec(spec); err != nil {
		return spec, fmt.Errorf("invalid DocumentSpec: %w", err)
	}
	return spec, nil
}

func validateDocumentSpec(spec models.DocumentSpec) error {
	if spec.Version != models.FinalOutputSpecVersion {
		return fmt.Errorf("version must be %q", models.FinalOutputSpecVersion)
	}
	if strings.TrimSpace(spec.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if len(spec.Sections) == 0 || len(spec.Sections) > maxDocumentSections {
		return fmt.Errorf("sections must contain between 1 and %d entries", maxDocumentSections)
	}
	totalBlocks := 0
	totalBytes := len(spec.Title) + len(spec.Subtitle)
	for index, item := range spec.Metadata {
		if strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Value) == "" {
			return fmt.Errorf("metadata[%d] requires label and value", index)
		}
		totalBytes += len(item.Label) + len(item.Value)
	}
	for sectionIndex, section := range spec.Sections {
		if strings.TrimSpace(section.Title) == "" {
			return fmt.Errorf("sections[%d].title is required", sectionIndex)
		}
		if len(section.Blocks) == 0 {
			return fmt.Errorf("sections[%d].blocks must not be empty", sectionIndex)
		}
		totalBytes += len(section.Title)
		totalBlocks += len(section.Blocks)
		for blockIndex, block := range section.Blocks {
			if err := validateDocumentBlock(block); err != nil {
				return fmt.Errorf("sections[%d].blocks[%d]: %w", sectionIndex, blockIndex, err)
			}
			totalBytes += documentBlockTextBytes(block)
		}
	}
	if totalBlocks > maxDocumentBlocks {
		return fmt.Errorf("document exceeds %d blocks", maxDocumentBlocks)
	}
	if totalBytes > maxDocumentTextBytes {
		return fmt.Errorf("document exceeds %d text bytes", maxDocumentTextBytes)
	}
	return nil
}

func validateDocumentBlock(block models.DocumentBlock) error {
	switch block.Type {
	case "paragraph":
		if strings.TrimSpace(block.Text) == "" {
			return fmt.Errorf("paragraph text is required")
		}
		if len(block.Items) > 0 || block.Table != nil || block.Title != "" || block.Tone != "" {
			return fmt.Errorf("paragraph contains fields for another block type")
		}
	case "bullet_list", "numbered_list":
		if len(block.Items) == 0 {
			return fmt.Errorf("%s items must not be empty", block.Type)
		}
		for index, item := range block.Items {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("items[%d] must not be empty", index)
			}
		}
		if block.Text != "" || block.Table != nil || block.Title != "" || block.Tone != "" {
			return fmt.Errorf("%s contains fields for another block type", block.Type)
		}
	case "table":
		if block.Table == nil {
			return fmt.Errorf("table is required")
		}
		if block.Text != "" || len(block.Items) > 0 || block.Title != "" || block.Tone != "" {
			return fmt.Errorf("table contains fields for another block type")
		}
		return validateDocumentTable(*block.Table)
	case "callout":
		if strings.TrimSpace(block.Text) == "" {
			return fmt.Errorf("callout text is required")
		}
		switch block.Tone {
		case "", "info", "success", "warning", "critical":
		default:
			return fmt.Errorf("unsupported callout tone %q", block.Tone)
		}
		if len(block.Items) > 0 || block.Table != nil {
			return fmt.Errorf("callout contains fields for another block type")
		}
	default:
		return fmt.Errorf("unsupported block type %q", block.Type)
	}
	return nil
}

func validateDocumentTable(table models.DocumentTable) error {
	if len(table.Columns) == 0 || len(table.Columns) > maxDocumentTableColumns {
		return fmt.Errorf("table columns must contain between 1 and %d entries", maxDocumentTableColumns)
	}
	if len(table.Rows) > maxDocumentTableRows {
		return fmt.Errorf("table exceeds %d rows", maxDocumentTableRows)
	}
	for index, column := range table.Columns {
		if strings.TrimSpace(column) == "" {
			return fmt.Errorf("columns[%d] must not be empty", index)
		}
	}
	for index, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return fmt.Errorf("rows[%d] has %d cells, expected %d", index, len(row), len(table.Columns))
		}
	}
	return nil
}

func documentBlockTextBytes(block models.DocumentBlock) int {
	total := len(block.Text) + len(block.Title) + len(block.Tone)
	for _, item := range block.Items {
		total += len(item)
	}
	if block.Table != nil {
		for _, column := range block.Table.Columns {
			total += len(column)
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row {
				total += len(cell)
			}
		}
	}
	return total
}

func parseSpreadsheetSpec(content string) (models.SpreadsheetSpec, error) {
	var spec models.SpreadsheetSpec
	if err := decodeStrictFinalOutputJSON(content, &spec); err != nil {
		return spec, fmt.Errorf("invalid SpreadsheetSpec: %w", err)
	}
	if err := validateSpreadsheetSpec(spec); err != nil {
		return spec, fmt.Errorf("invalid SpreadsheetSpec: %w", err)
	}
	return spec, nil
}

func parseDashboardSpec(content string) (models.DashboardSpec, error) {
	var spec models.DashboardSpec
	normalized, err := normalizeDashboardSpecAliases(content)
	if err != nil {
		return spec, fmt.Errorf("invalid DashboardSpec: %w", err)
	}
	if err := decodeStrictFinalOutputJSON(normalized, &spec); err != nil {
		return spec, fmt.Errorf("invalid DashboardSpec: %w", err)
	}
	if err := validateDashboardSpec(spec); err != nil {
		return spec, fmt.Errorf("invalid DashboardSpec: %w", err)
	}
	return spec, nil
}

func normalizeDashboardSpecAliases(content string) (string, error) {
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values are not allowed")
		}
		return "", err
	}
	if root == nil {
		return content, nil
	}
	changed := false
	versionChanged, err := normalizeDashboardVersionAlias(root)
	if err != nil {
		return "", err
	}
	changed = changed || versionChanged
	var blocks json.RawMessage
	var ok bool
	if existingBlocks, hasBlocks := root["blocks"]; hasBlocks {
		blocks = existingBlocks
		ok = true
	} else if sections, hasSections := root["sections"]; hasSections {
		blocks, ok, err = dashboardBlocksFromSectionAliases(sections)
		if err != nil {
			return "", err
		}
		delete(root, "sections")
		changed = true
	} else if widgets, hasWidgets := root["widgets"]; hasWidgets {
		blocks = widgets
		ok = true
		delete(root, "widgets")
		changed = true
	}
	if !ok {
		return content, nil
	}
	blocks, changedBlocks, err := normalizeDashboardBlocksAliases(blocks)
	if err != nil {
		return "", err
	}
	changed = changed || changedBlocks
	if dashboardTitleIsEmpty(root["title"]) {
		title := dashboardTitleFromBlocks(blocks)
		if title == "" {
			title = "Dashboard output"
		}
		titlePayload, err := json.Marshal(title)
		if err != nil {
			return "", err
		}
		root["title"] = titlePayload
		changed = true
	}
	if !changed {
		return content, nil
	}
	root["blocks"] = blocks
	normalized, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func normalizeDashboardVersionAlias(root map[string]json.RawMessage) (bool, error) {
	raw, ok := root["version"]
	trimmed := bytes.TrimSpace(raw)
	if !ok || len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		root["version"] = json.RawMessage(`"1"`)
		return true, nil
	}
	var version string
	if err := json.Unmarshal(trimmed, &version); err == nil {
		switch strings.TrimSpace(strings.ToLower(version)) {
		case "1":
			return false, nil
		case "", "1.0", "v1":
			root["version"] = json.RawMessage(`"1"`)
			return true, nil
		default:
			return false, nil
		}
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return false, nil
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return false, fmt.Errorf("version contains multiple JSON values")
		}
		return false, err
	}
	if number.String() == "1" || number.String() == "1.0" {
		root["version"] = json.RawMessage(`"1"`)
		return true, nil
	}
	return false, nil
}

func dashboardTitleIsEmpty(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	var title string
	if err := json.Unmarshal(trimmed, &title); err != nil {
		return false
	}
	return strings.TrimSpace(title) == ""
}

func dashboardTitleFromBlocks(raw json.RawMessage) string {
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	for _, block := range blocks {
		for _, field := range []string{"title", "label"} {
			var value string
			if err := json.Unmarshal(block[field], &value); err == nil && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func normalizeDashboardBlocksAliases(raw json.RawMessage) (json.RawMessage, bool, error) {
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false, fmt.Errorf("blocks must be an array: %w", err)
	}
	changed := false
	normalizedBlocks := make([]json.RawMessage, 0, len(blocks))
	for index, block := range blocks {
		if nested, ok, err := dashboardBlocksFromNestedBlockAlias(block, fmt.Sprintf("blocks[%d]", index)); err != nil {
			return nil, false, err
		} else if ok {
			nestedPayload, err := json.Marshal(nested)
			if err != nil {
				return nil, false, err
			}
			normalizedNested, _, err := normalizeDashboardBlocksAliases(nestedPayload)
			if err != nil {
				return nil, false, err
			}
			var nestedBlocks []json.RawMessage
			if err := json.Unmarshal(normalizedNested, &nestedBlocks); err != nil {
				return nil, false, err
			}
			normalizedBlocks = append(normalizedBlocks, nestedBlocks...)
			changed = true
			continue
		}
		normalized, blockChanged, err := normalizeDashboardBlockAliases(block, fmt.Sprintf("blocks[%d]", index))
		if err != nil {
			return nil, false, err
		}
		if blockChanged {
			block = normalized
			changed = true
		}
		normalizedBlocks = append(normalizedBlocks, block)
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(normalizedBlocks)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func dashboardBlocksFromNestedBlockAlias(raw json.RawMessage, path string) ([]json.RawMessage, bool, error) {
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, false, nil
	}
	var nestedRaw json.RawMessage
	var field string
	if rawBlocks, hasBlocks := block["blocks"]; hasBlocks {
		nestedRaw = rawBlocks
		field = "blocks"
	} else if rawWidgets, hasWidgets := block["widgets"]; hasWidgets {
		nestedRaw = rawWidgets
		field = "widgets"
	} else {
		return nil, false, nil
	}
	var nested []json.RawMessage
	if err := json.Unmarshal(nestedRaw, &nested); err != nil {
		return nil, false, fmt.Errorf("%s.%s must be an array: %w", path, field, err)
	}
	return nested, true, nil
}

func normalizeDashboardBlockAliases(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return raw, false, nil
	}
	changed := false
	keyChanged, err := normalizeDashboardKeyAlias(block)
	if err != nil {
		return nil, false, fmt.Errorf("%s.key: %w", path, err)
	}
	changed = changed || keyChanged
	for _, field := range []string{"title", "text", "status", "label", "value", "href"} {
		normalized, fieldChanged, err := dashboardStringAliasRaw(block[field])
		if err != nil {
			return nil, false, fmt.Errorf("%s.%s: %w", path, field, err)
		}
		if fieldChanged {
			block[field] = normalized
			changed = true
		}
	}
	textChanged, err := normalizeDashboardTextAlias(block)
	if err != nil {
		return nil, false, fmt.Errorf("%s.text: %w", path, err)
	}
	changed = changed || textChanged
	if rawItems, hasItems := block["items"]; hasItems {
		items, itemsChanged, err := normalizeDashboardItemsAlias(rawItems, path+".items")
		if err != nil {
			return nil, false, err
		}
		if itemsChanged {
			block["items"] = items
			changed = true
		}
	} else if rawProperties, hasProperties := block["properties"]; hasProperties {
		items, err := dashboardItemsFromPropertiesAlias(rawProperties, path+".properties")
		if err != nil {
			return nil, false, err
		}
		block["items"] = items
		delete(block, "properties")
		changed = true
	}
	if rawChart, hasChart := block["chart"]; hasChart {
		chart, chartChanged, err := normalizeDashboardChartAliases(rawChart, path+".chart")
		if err != nil {
			return nil, false, err
		}
		if chartChanged {
			block["chart"] = chart
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(block)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func dashboardItemsFromPropertiesAlias(raw json.RawMessage, path string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s must be an array or object", path)
	}
	if trimmed[0] == '[' {
		items, _, err := normalizeDashboardItemsAlias(raw, path)
		return items, err
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(raw, &properties); err != nil {
		return nil, fmt.Errorf("%s must be an array or object: %w", path, err)
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		item := map[string]json.RawMessage{}
		if bytes.HasPrefix(bytes.TrimSpace(properties[key]), []byte("{")) {
			if err := json.Unmarshal(properties[key], &item); err != nil {
				return nil, fmt.Errorf("%s.%s must be a scalar or item object: %w", path, key, err)
			}
		} else {
			value, converted, err := dashboardStringAliasRaw(properties[key])
			if err != nil || !converted {
				return nil, fmt.Errorf("%s.%s must be a scalar or item object", path, key)
			}
			item["value"] = value
		}
		if _, hasLabel := item["label"]; !hasLabel {
			label, _ := json.Marshal(key)
			item["label"] = label
		}
		normalized, changed, err := normalizeDashboardItemObject(item)
		if err != nil {
			return nil, err
		}
		if changed {
			item = normalized
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		items = append(items, payload)
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func normalizeDashboardItemsAlias(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, fmt.Errorf("%s must be an array: %w", path, err)
	}
	changed := false
	for index, item := range items {
		normalized, itemChanged, err := normalizeDashboardItemObject(item)
		if err != nil {
			return nil, false, fmt.Errorf("%s[%d]: %w", path, index, err)
		}
		if itemChanged {
			items[index] = normalized
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func normalizeDashboardItemObject(item map[string]json.RawMessage) (map[string]json.RawMessage, bool, error) {
	changed := false
	keyChanged, err := normalizeDashboardKeyAlias(item)
	if err != nil {
		return nil, false, fmt.Errorf("key: %w", err)
	}
	changed = changed || keyChanged
	for _, field := range []string{"label", "value", "text", "status", "tone", "href"} {
		normalized, fieldChanged, err := dashboardStringAliasRaw(item[field])
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", field, err)
		}
		if fieldChanged {
			item[field] = normalized
			changed = true
		}
	}
	return item, changed, nil
}

func normalizeDashboardTextAlias(fields map[string]json.RawMessage) (bool, error) {
	changed := false
	hasText := !dashboardTitleIsEmpty(fields["text"])
	for _, alias := range []string{"message", "description", "content", "summary"} {
		raw, ok := fields[alias]
		if !ok {
			continue
		}
		if !hasText {
			normalized, converted, err := dashboardStringAliasRaw(raw)
			if err != nil {
				return false, err
			}
			if !converted {
				normalized = raw
			}
			fields["text"] = normalized
			hasText = true
		}
		delete(fields, alias)
		changed = true
	}
	return changed, nil
}

func normalizeDashboardKeyAlias(fields map[string]json.RawMessage) (bool, error) {
	raw, ok := fields["key"]
	if !ok {
		return false, nil
	}
	if _, hasLabel := fields["label"]; !hasLabel {
		normalized, converted, err := dashboardStringAliasRaw(raw)
		if err != nil {
			return false, err
		}
		if !converted {
			normalized = raw
		}
		fields["label"] = normalized
	}
	delete(fields, "key")
	return true, nil
}

func normalizeDashboardChartAliases(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	var chart map[string]json.RawMessage
	if err := json.Unmarshal(raw, &chart); err != nil {
		return raw, false, nil
	}
	changed := false
	for _, field := range []string{"type", "unit", "aggregation_interval", "missing_values"} {
		normalized, fieldChanged, err := dashboardStringAliasRaw(chart[field])
		if err != nil {
			return nil, false, fmt.Errorf("%s.%s: %w", path, field, err)
		}
		if fieldChanged {
			chart[field] = normalized
			changed = true
		}
	}
	if rawSeries, hasSeries := chart["series"]; hasSeries {
		series, seriesChanged, err := normalizeDashboardChartSeriesAliases(rawSeries, path+".series")
		if err != nil {
			return nil, false, err
		}
		if seriesChanged {
			chart["series"] = series
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(chart)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func normalizeDashboardChartSeriesAliases(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	var series []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &series); err != nil {
		return nil, false, fmt.Errorf("%s must be an array: %w", path, err)
	}
	changed := false
	for seriesIndex, item := range series {
		for _, field := range []string{"key", "label", "team", "environment", "unit", "color"} {
			normalized, fieldChanged, err := dashboardStringAliasRaw(item[field])
			if err != nil {
				return nil, false, fmt.Errorf("%s[%d].%s: %w", path, seriesIndex, field, err)
			}
			if fieldChanged {
				item[field] = normalized
				changed = true
			}
		}
		if rawPoints, hasPoints := item["points"]; hasPoints {
			points, pointsChanged, err := normalizeDashboardChartPointAliases(rawPoints, fmt.Sprintf("%s[%d].points", path, seriesIndex))
			if err != nil {
				return nil, false, err
			}
			if pointsChanged {
				item["points"] = points
				changed = true
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(series)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func normalizeDashboardChartPointAliases(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	var points []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &points); err != nil {
		return nil, false, fmt.Errorf("%s must be an array: %w", path, err)
	}
	changed := false
	for pointIndex, point := range points {
		keyChanged, err := normalizeDashboardKeyAlias(point)
		if err != nil {
			return nil, false, fmt.Errorf("%s[%d].key: %w", path, pointIndex, err)
		}
		changed = changed || keyChanged
		for _, field := range []string{"timestamp", "label"} {
			normalized, fieldChanged, err := dashboardStringAliasRaw(point[field])
			if err != nil {
				return nil, false, fmt.Errorf("%s[%d].%s: %w", path, pointIndex, field, err)
			}
			if fieldChanged {
				point[field] = normalized
				changed = true
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	payload, err := json.Marshal(points)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func dashboardStringAliasRaw(raw json.RawMessage) (json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' {
		return raw, false, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`""`), true, nil
	}
	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		var value bool
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return nil, false, err
		}
		payload, _ := json.Marshal(strconv.FormatBool(value))
		return payload, true, nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return raw, false, nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return nil, false, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, false, fmt.Errorf("multiple JSON values are not allowed")
		}
		return nil, false, err
	}
	payload, _ := json.Marshal(number.String())
	return payload, true, nil
}

func dashboardBlocksFromSectionAliases(raw json.RawMessage) (json.RawMessage, bool, error) {
	var sections []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, false, fmt.Errorf("sections must be an array: %w", err)
	}
	blocks := make([]json.RawMessage, 0, len(sections))
	for sectionIndex, section := range sections {
		var sectionBlocks []json.RawMessage
		if rawBlocks, hasBlocks := section["blocks"]; hasBlocks {
			if err := json.Unmarshal(rawBlocks, &sectionBlocks); err != nil {
				return nil, false, fmt.Errorf("sections[%d].blocks must be an array: %w", sectionIndex, err)
			}
		} else if rawWidgets, hasWidgets := section["widgets"]; hasWidgets {
			if err := json.Unmarshal(rawWidgets, &sectionBlocks); err != nil {
				return nil, false, fmt.Errorf("sections[%d].widgets must be an array: %w", sectionIndex, err)
			}
		}
		blocks = append(blocks, sectionBlocks...)
	}
	payload, err := json.Marshal(blocks)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func validateDashboardSpec(spec models.DashboardSpec) error {
	if spec.Version != models.FinalOutputSpecVersion {
		return fmt.Errorf("version must be %q", models.FinalOutputSpecVersion)
	}
	if strings.TrimSpace(spec.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if err := validateDashboardSafeText("title", spec.Title); err != nil {
		return err
	}
	if len(spec.Blocks) == 0 || len(spec.Blocks) > maxDashboardBlocks {
		return fmt.Errorf("blocks must contain between 1 and %d entries", maxDashboardBlocks)
	}
	totalBytes := len(spec.Title)
	for blockIndex, block := range spec.Blocks {
		if err := validateDashboardBlock(block); err != nil {
			return fmt.Errorf("blocks[%d]: %w", blockIndex, err)
		}
		totalBytes += dashboardBlockTextBytes(block)
	}
	if totalBytes > maxDashboardTextBytes {
		return fmt.Errorf("dashboard exceeds %d text bytes", maxDashboardTextBytes)
	}
	return nil
}

func validateDashboardBlock(block models.DashboardBlock) error {
	if err := validateDashboardSafeText("title", block.Title); err != nil {
		return err
	}
	if err := validateDashboardSafeText("text", block.Text); err != nil {
		return err
	}
	if err := validateDashboardSafeText("label", block.Label); err != nil {
		return err
	}
	if err := validateDashboardSafeText("value", block.Value); err != nil {
		return err
	}
	if _, ok := supportedDashboardTones[block.Tone]; !ok {
		return fmt.Errorf("unsupported tone %q", block.Tone)
	}
	switch block.Type {
	case "status":
		if strings.TrimSpace(firstNonEmptyString(block.Label, block.Title)) == "" {
			return fmt.Errorf("status requires label or title")
		}
		if strings.TrimSpace(firstNonEmptyString(block.Status, block.Value, block.Text)) == "" {
			return fmt.Errorf("status requires status, value, or text")
		}
		if _, ok := supportedDashboardStatuses[block.Status]; !ok {
			return fmt.Errorf("unsupported status %q", block.Status)
		}
	case "text":
		if strings.TrimSpace(block.Text) == "" {
			return fmt.Errorf("text block requires text")
		}
	case "callout":
		if strings.TrimSpace(block.Text) == "" {
			return fmt.Errorf("callout requires text")
		}
	case "list":
		if len(block.Items) == 0 {
			return fmt.Errorf("list requires items")
		}
		for index, item := range block.Items {
			if strings.TrimSpace(firstNonEmptyString(item.Text, item.Label)) == "" {
				return fmt.Errorf("items[%d] requires text or label", index)
			}
			if err := validateDashboardSafeText(fmt.Sprintf("items[%d].label", index), item.Label); err != nil {
				return err
			}
			if err := validateDashboardSafeText(fmt.Sprintf("items[%d].value", index), item.Value); err != nil {
				return err
			}
			if err := validateDashboardSafeText(fmt.Sprintf("items[%d].text", index), item.Text); err != nil {
				return err
			}
			if strings.TrimSpace(item.Href) != "" {
				if err := validateDashboardItemHref(item.Href); err != nil {
					return fmt.Errorf("items[%d].href: %w", index, err)
				}
			}
			if _, ok := supportedDashboardTones[item.Tone]; !ok {
				return fmt.Errorf("items[%d] has unsupported tone %q", index, item.Tone)
			}
		}
	case "properties":
		if len(block.Items) == 0 {
			return fmt.Errorf("properties requires items")
		}
		for index, item := range block.Items {
			if strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Value) == "" {
				return fmt.Errorf("items[%d] requires label and value", index)
			}
			if err := validateDashboardSafeText(fmt.Sprintf("items[%d].label", index), item.Label); err != nil {
				return err
			}
			if err := validateDashboardSafeText(fmt.Sprintf("items[%d].value", index), item.Value); err != nil {
				return err
			}
		}
	case "table":
		if err := validateDashboardTable(block); err != nil {
			return err
		}
	case "progress":
		if block.Progress == nil {
			return fmt.Errorf("progress requires progress")
		}
		if block.Progress.Value < 0 || block.Progress.Max < 0 {
			return fmt.Errorf("progress values must be non-negative")
		}
		if block.Progress.Max > 0 && block.Progress.Value > block.Progress.Max {
			return fmt.Errorf("progress value cannot exceed max")
		}
	case "link":
		if strings.TrimSpace(firstNonEmptyString(block.Text, block.Title, block.Label)) == "" {
			return fmt.Errorf("link requires text, title, or label")
		}
		if err := validateDashboardItemHref(block.Href); err != nil {
			return fmt.Errorf("href: %w", err)
		}
	case "chart", "series":
		if block.Chart == nil {
			return fmt.Errorf("%s requires chart", block.Type)
		}
		if err := validateDashboardChart(*block.Chart, block.Type); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported block type %q", block.Type)
	}
	return nil
}

func validateDashboardTable(block models.DashboardBlock) error {
	if len(block.Columns) == 0 || len(block.Columns) > maxDashboardTableCols {
		return fmt.Errorf("table columns must contain between 1 and %d entries", maxDashboardTableCols)
	}
	if len(block.Rows) > maxDashboardTableRows {
		return fmt.Errorf("table exceeds %d rows", maxDashboardTableRows)
	}
	columnKeys := map[string]struct{}{}
	for columnIndex, column := range block.Columns {
		if !spreadsheetColumnKeyPattern.MatchString(column.Key) {
			return fmt.Errorf("columns[%d].key is invalid", columnIndex)
		}
		if _, exists := columnKeys[column.Key]; exists {
			return fmt.Errorf("column key %q is duplicated", column.Key)
		}
		if strings.TrimSpace(column.Label) == "" {
			return fmt.Errorf("columns[%d].label is required", columnIndex)
		}
		if err := validateDashboardSafeText(fmt.Sprintf("columns[%d].label", columnIndex), column.Label); err != nil {
			return err
		}
		columnKeys[column.Key] = struct{}{}
	}
	for rowIndex, row := range block.Rows {
		for key, raw := range row {
			if _, exists := columnKeys[key]; !exists {
				return fmt.Errorf("rows[%d] contains unknown column %q", rowIndex, key)
			}
			if err := validateSpreadsheetCell(raw); err != nil {
				return fmt.Errorf("rows[%d].%s: %w", rowIndex, key, err)
			}
			if err := validateDashboardRawScalarText(fmt.Sprintf("rows[%d].%s", rowIndex, key), raw); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDashboardItemHref(raw string) error {
	href := strings.TrimSpace(raw)
	if href == "" {
		return fmt.Errorf("is required")
	}
	if err := validateDashboardSafeText("href", href); err != nil {
		return err
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return err
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "" {
		if strings.TrimSpace(parsed.Host) != "" {
			return fmt.Errorf("protocol-relative links are not supported")
		}
		return nil
	}
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only http, https, and relative links are supported")
	}
	return nil
}

func validateDashboardChart(chart models.DashboardChart, blockType string) error {
	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	if _, ok := supportedDashboardChartTypes[chartType]; !ok {
		return fmt.Errorf("chart.type must be one of line, bar, area, pie, or donut")
	}
	if blockType == "series" {
		switch chartType {
		case "line", "bar", "area":
		default:
			return fmt.Errorf("series blocks support line, bar, and area charts")
		}
	}
	if len(chart.Series) == 0 || len(chart.Series) > maxDashboardChartSeries {
		return fmt.Errorf("chart.series must contain between 1 and %d entries", maxDashboardChartSeries)
	}
	if _, ok := supportedDashboardMissingValues[strings.ToLower(strings.TrimSpace(chart.MissingValues))]; !ok {
		return fmt.Errorf("chart.missing_values is unsupported")
	}
	if strings.TrimSpace(chart.AggregationInterval) != "" {
		duration, err := parseDashboardDuration(chart.AggregationInterval)
		if err != nil || duration <= 0 {
			return fmt.Errorf("chart.aggregation_interval must be a positive duration")
		}
	}
	if chart.TimeWindow != nil {
		if err := validateDashboardTimeBoundary("chart.time_window.from", chart.TimeWindow.From); err != nil {
			return err
		}
		if err := validateDashboardTimeBoundary("chart.time_window.to", chart.TimeWindow.To); err != nil {
			return err
		}
	}
	for key, value := range chart.Dimensions {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("chart.dimensions contains an empty key")
		}
		if err := validateDashboardSafeText("chart.dimensions."+key, value); err != nil {
			return err
		}
	}
	totalPoints := 0
	seriesKeys := map[string]struct{}{}
	for seriesIndex, series := range chart.Series {
		key := strings.TrimSpace(series.Key)
		if !dashboardSeriesKeyPattern.MatchString(key) {
			return fmt.Errorf("chart.series[%d].key is invalid", seriesIndex)
		}
		if _, exists := seriesKeys[key]; exists {
			return fmt.Errorf("chart.series key %q is duplicated", key)
		}
		seriesKeys[key] = struct{}{}
		if err := validateDashboardSafeText(fmt.Sprintf("chart.series[%d].label", seriesIndex), series.Label); err != nil {
			return err
		}
		if err := validateDashboardSafeText(fmt.Sprintf("chart.series[%d].team", seriesIndex), series.Team); err != nil {
			return err
		}
		if err := validateDashboardSafeText(fmt.Sprintf("chart.series[%d].environment", seriesIndex), series.Environment); err != nil {
			return err
		}
		if color := strings.TrimSpace(series.Color); color != "" && !dashboardColorPattern.MatchString(color) {
			return fmt.Errorf("chart.series[%d].color must be a hex color", seriesIndex)
		}
		if len(series.Points) == 0 {
			return fmt.Errorf("chart.series[%d].points must not be empty", seriesIndex)
		}
		totalPoints += len(series.Points)
		if totalPoints > maxDashboardChartPoints {
			return fmt.Errorf("chart exceeds %d points", maxDashboardChartPoints)
		}
		for pointIndex, point := range series.Points {
			if err := validateDashboardPoint(chartType, seriesIndex, pointIndex, point); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDashboardPoint(chartType string, seriesIndex, pointIndex int, point models.DashboardSeriesPoint) error {
	if err := validateDashboardSafeText(fmt.Sprintf("chart.series[%d].points[%d].label", seriesIndex, pointIndex), point.Label); err != nil {
		return err
	}
	switch chartType {
	case "pie", "donut":
		if strings.TrimSpace(point.Label) == "" {
			return fmt.Errorf("chart.series[%d].points[%d].label is required for %s charts", seriesIndex, pointIndex, chartType)
		}
	default:
		if strings.TrimSpace(point.Timestamp) != "" {
			if err := validateDashboardTimeBoundary(fmt.Sprintf("chart.series[%d].points[%d].timestamp", seriesIndex, pointIndex), point.Timestamp); err != nil {
				return err
			}
		} else if strings.TrimSpace(point.Label) == "" {
			return fmt.Errorf("chart.series[%d].points[%d] requires timestamp or label", seriesIndex, pointIndex)
		}
	}
	if point.Value != nil && (math.IsInf(*point.Value, 0) || math.IsNaN(*point.Value)) {
		return fmt.Errorf("chart.series[%d].points[%d].value must be finite", seriesIndex, pointIndex)
	}
	return nil
}

func validateDashboardTimeBoundary(path, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if strings.EqualFold(value, "now") {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(value), "now-") || strings.HasPrefix(strings.ToLower(value), "now+") {
		_, err := parseDashboardDuration(value[4:])
		if err != nil {
			return fmt.Errorf("%s must be RFC3339, YYYY-MM-DD, now, or now+/-duration", path)
		}
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return nil
	}
	return fmt.Errorf("%s must be RFC3339, YYYY-MM-DD, now, or now+/-duration", path)
}

func parseDashboardDuration(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func validateDashboardSafeText(path, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if dashboardExecutableContentPattern.MatchString(value) || dashboardHTMLPattern.MatchString(value) {
		return fmt.Errorf("%s contains unsupported HTML, CSS, or executable content", path)
	}
	return nil
}

func validateDashboardRawScalarText(path string, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil
	}
	return validateDashboardSafeText(path, value)
}

func dashboardBlockTextBytes(block models.DashboardBlock) int {
	total := len(block.Type) + len(block.Title) + len(block.Text) + len(block.Tone) + len(block.Status) + len(block.Label) + len(block.Value) + len(block.Href)
	for _, item := range block.Items {
		total += len(item.Label) + len(item.Value) + len(item.Text) + len(item.Status) + len(item.Tone) + len(item.Href)
	}
	for _, column := range block.Columns {
		total += len(column.Key) + len(column.Label)
	}
	for _, row := range block.Rows {
		for key, raw := range row {
			total += len(key) + len(raw)
		}
	}
	if block.Progress != nil {
		total += len(block.Progress.Unit)
	}
	if block.Chart != nil {
		total += len(block.Chart.Type) + len(block.Chart.Unit) + len(block.Chart.AggregationInterval) + len(block.Chart.MissingValues)
		if block.Chart.TimeWindow != nil {
			total += len(block.Chart.TimeWindow.From) + len(block.Chart.TimeWindow.To)
		}
		for key, value := range block.Chart.Dimensions {
			total += len(key) + len(value)
		}
		for _, series := range block.Chart.Series {
			total += len(series.Key) + len(series.Label) + len(series.Team) + len(series.Environment) + len(series.Unit) + len(series.Color)
			for _, point := range series.Points {
				total += len(point.Timestamp) + len(point.Label)
			}
		}
	}
	return total
}

func validateSpreadsheetSpec(spec models.SpreadsheetSpec) error {
	if spec.Version != models.FinalOutputSpecVersion {
		return fmt.Errorf("version must be %q", models.FinalOutputSpecVersion)
	}
	if len(spec.Sheets) == 0 || len(spec.Sheets) > maxSpreadsheetSheets {
		return fmt.Errorf("sheets must contain between 1 and %d entries", maxSpreadsheetSheets)
	}
	sheetNames := map[string]struct{}{}
	effectiveSheetNames := map[string]struct{}{}
	for sheetIndex, sheet := range spec.Sheets {
		name := strings.TrimSpace(sheet.Name)
		if name == "" {
			return fmt.Errorf("sheets[%d].name is required", sheetIndex)
		}
		nameKey := strings.ToLower(name)
		if _, exists := sheetNames[nameKey]; exists {
			return fmt.Errorf("sheet name %q is duplicated", name)
		}
		sheetNames[nameKey] = struct{}{}
		effectiveName := strings.ToLower(safeSheetName(name))
		if _, exists := effectiveSheetNames[effectiveName]; exists {
			return fmt.Errorf("sheet name %q conflicts after Excel name normalization", name)
		}
		effectiveSheetNames[effectiveName] = struct{}{}
		if len(sheet.Columns) == 0 || len(sheet.Columns) > maxSpreadsheetColumns {
			return fmt.Errorf("sheets[%d].columns must contain between 1 and %d entries", sheetIndex, maxSpreadsheetColumns)
		}
		if len(sheet.Rows) > maxSpreadsheetRows {
			return fmt.Errorf("sheets[%d] exceeds %d rows", sheetIndex, maxSpreadsheetRows)
		}
		columnKeys := map[string]struct{}{}
		for columnIndex, column := range sheet.Columns {
			if !spreadsheetColumnKeyPattern.MatchString(column.Key) {
				return fmt.Errorf("sheets[%d].columns[%d].key is invalid", sheetIndex, columnIndex)
			}
			if _, exists := columnKeys[column.Key]; exists {
				return fmt.Errorf("sheets[%d] column key %q is duplicated", sheetIndex, column.Key)
			}
			columnKeys[column.Key] = struct{}{}
			if strings.TrimSpace(column.Header) == "" {
				return fmt.Errorf("sheets[%d].columns[%d].header is required", sheetIndex, columnIndex)
			}
			if column.Width < 0 || column.Width > 100 {
				return fmt.Errorf("sheets[%d].columns[%d].width must be between 0 and 100", sheetIndex, columnIndex)
			}
			if _, supported := supportedSpreadsheetNumberFormats[column.NumberFormat]; !supported {
				return fmt.Errorf("sheets[%d].columns[%d] has unsupported number_format %q", sheetIndex, columnIndex, column.NumberFormat)
			}
		}
		for rowIndex, row := range sheet.Rows {
			for key, raw := range row {
				if _, exists := columnKeys[key]; !exists {
					return fmt.Errorf("sheets[%d].rows[%d] contains unknown column %q", sheetIndex, rowIndex, key)
				}
				if err := validateSpreadsheetCell(raw); err != nil {
					return fmt.Errorf("sheets[%d].rows[%d].%s: %w", sheetIndex, rowIndex, key, err)
				}
			}
		}
	}
	return nil
}

func validateSpreadsheetCell(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("cell value is empty")
	}
	if bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		return nil
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return fmt.Errorf("invalid string: %w", err)
		}
		if !utf8.ValidString(value) || len(value) > maxSpreadsheetCellBytes {
			return fmt.Errorf("string exceeds the Excel cell limit")
		}
		return nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return fmt.Errorf("cell values must be JSON scalars")
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("cell values must be strings, numbers, booleans, or null")
	}
	return nil
}

func decodeStrictFinalOutputJSON(content string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func marshalFinalOutputSpec(value any) (string, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func sortedSpreadsheetRowKeys(row map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
