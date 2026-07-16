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
	if err := decodeStrictFinalOutputJSON(content, &spec); err != nil {
		return spec, fmt.Errorf("invalid DashboardSpec: %w", err)
	}
	if err := validateDashboardSpec(spec); err != nil {
		return spec, fmt.Errorf("invalid DashboardSpec: %w", err)
	}
	return spec, nil
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
