package nopsai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
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
)

var spreadsheetColumnKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

var supportedSpreadsheetNumberFormats = map[string]struct{}{
	"": {}, "text": {}, "integer": {}, "decimal": {}, "percent": {},
	"currency_usd": {}, "currency_eur": {}, "date": {}, "datetime": {}, "boolean": {},
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
