package nopsai

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"nopsai/pkg/models"
)

func buildPipelineFinalOutputXLSX(fallbackSheetName, content string) ([]byte, error) {
	spec, err := parseSpreadsheetSpec(content)
	if err != nil {
		spec = legacySpreadsheetSpec(fallbackSheetName, content)
	}
	workbook := excelize.NewFile()
	defer workbook.Close()

	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1D4ED8"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "AEBBD0", Style: 1}, {Type: "right", Color: "AEBBD0", Style: 1},
			{Type: "top", Color: "AEBBD0", Style: 1}, {Type: "bottom", Color: "AEBBD0", Style: 1},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create spreadsheet header style: %w", err)
	}

	for sheetIndex, sheet := range spec.Sheets {
		name := safeSheetName(sheet.Name)
		if sheetIndex == 0 {
			if err := workbook.SetSheetName("Sheet1", name); err != nil {
				return nil, fmt.Errorf("name spreadsheet sheet: %w", err)
			}
		} else if _, err := workbook.NewSheet(name); err != nil {
			return nil, fmt.Errorf("create spreadsheet sheet %q: %w", name, err)
		}
		if err := renderSpreadsheetSheet(workbook, name, sheet, headerStyle); err != nil {
			return nil, err
		}
	}
	workbook.SetActiveSheet(0)
	if err := workbook.SetDocProps(&excelize.DocProperties{Title: spec.Title, Creator: "NopsAI"}); err != nil {
		return nil, fmt.Errorf("set spreadsheet properties: %w", err)
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write spreadsheet: %w", err)
	}
	return buffer.Bytes(), nil
}

func renderSpreadsheetSheet(workbook *excelize.File, name string, sheet models.SpreadsheetSheet, headerStyle int) error {
	for columnIndex, column := range sheet.Columns {
		cell, _ := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err := workbook.SetCellValue(name, cell, column.Header); err != nil {
			return fmt.Errorf("write spreadsheet header: %w", err)
		}
		columnName, _ := excelize.ColumnNumberToName(columnIndex + 1)
		width := column.Width
		if width == 0 {
			width = estimatedSpreadsheetColumnWidth(column, sheet.Rows)
		}
		if err := workbook.SetColWidth(name, columnName, columnName, width); err != nil {
			return fmt.Errorf("set spreadsheet column width: %w", err)
		}
		style, err := spreadsheetColumnStyle(workbook, column.NumberFormat)
		if err != nil {
			return err
		}
		if style != 0 && len(sheet.Rows) > 0 {
			lastRow := len(sheet.Rows) + 1
			if err := workbook.SetCellStyle(name, columnName+"2", columnName+strconv.Itoa(lastRow), style); err != nil {
				return fmt.Errorf("set spreadsheet column style: %w", err)
			}
		}
	}
	lastColumn, _ := excelize.ColumnNumberToName(len(sheet.Columns))
	if err := workbook.SetCellStyle(name, "A1", lastColumn+"1", headerStyle); err != nil {
		return fmt.Errorf("style spreadsheet header: %w", err)
	}
	if err := workbook.SetRowHeight(name, 1, 24); err != nil {
		return fmt.Errorf("size spreadsheet header: %w", err)
	}
	for rowIndex, row := range sheet.Rows {
		for columnIndex, column := range sheet.Columns {
			raw, exists := row[column.Key]
			if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err := setSpreadsheetCell(workbook, name, cell, raw, column.NumberFormat); err != nil {
				return fmt.Errorf("write spreadsheet cell %s!%s: %w", name, cell, err)
			}
		}
	}
	if sheet.FreezeHeader {
		if err := workbook.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
			return fmt.Errorf("freeze spreadsheet header: %w", err)
		}
	}
	if sheet.AutoFilter {
		if err := workbook.AutoFilter(name, "A1:"+lastColumn+strconv.Itoa(len(sheet.Rows)+1), nil); err != nil {
			return fmt.Errorf("add spreadsheet filter: %w", err)
		}
	}
	return nil
}

func setSpreadsheetCell(workbook *excelize.File, sheet, cell string, raw json.RawMessage, numberFormat string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		if numberFormat == "date" || numberFormat == "datetime" {
			if parsed, ok := parseSpreadsheetTime(value, numberFormat); ok {
				return workbook.SetCellValue(sheet, cell, parsed)
			}
		}
		return workbook.SetCellStr(sheet, cell, value)
	}
	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		return workbook.SetCellBool(sheet, cell, bytes.Equal(trimmed, []byte("true")))
	}
	value, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil {
		return err
	}
	return workbook.SetCellFloat(sheet, cell, value, -1, 64)
}

func parseSpreadsheetTime(value, numberFormat string) (time.Time, bool) {
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	if numberFormat == "date" {
		layouts = []string{"2006-01-02", time.RFC3339}
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func spreadsheetColumnStyle(workbook *excelize.File, numberFormat string) (int, error) {
	format := ""
	switch numberFormat {
	case "text":
		format = "@"
	case "integer":
		format = "#,##0"
	case "decimal":
		format = "#,##0.00"
	case "percent":
		format = "0.00%"
	case "currency_usd":
		format = `$#,##0.00;[Red]-$#,##0.00`
	case "currency_eur":
		format = `€#,##0.00;[Red]-€#,##0.00`
	case "date":
		format = "yyyy-mm-dd"
	case "datetime":
		format = "yyyy-mm-dd hh:mm:ss"
	case "boolean":
		format = "General"
	}
	if format == "" {
		return 0, nil
	}
	style, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: &format, Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return 0, fmt.Errorf("create spreadsheet number format %q: %w", numberFormat, err)
	}
	return style, nil
}

func estimatedSpreadsheetColumnWidth(column models.SpreadsheetColumn, rows []map[string]json.RawMessage) float64 {
	width := len([]rune(column.Header)) + 2
	for _, row := range rows {
		raw := row[column.Key]
		value := strings.Trim(string(raw), `"`)
		if current := len([]rune(value)) + 2; current > width {
			width = current
		}
	}
	if width < 10 {
		width = 10
	}
	if width > 50 {
		width = 50
	}
	return float64(width)
}

func legacySpreadsheetSpec(sheetName, content string) models.SpreadsheetSpec {
	rows := legacySpreadsheetRows(content)
	if len(rows) == 0 {
		rows = [][]string{{content}}
	}
	columnCount := 1
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	columns := make([]models.SpreadsheetColumn, columnCount)
	for index := range columns {
		columns[index] = models.SpreadsheetColumn{Key: fmt.Sprintf("column_%d", index+1), Header: fmt.Sprintf("Column %d", index+1)}
	}
	dataStart := 0
	if len(rows) > 1 {
		dataStart = 1
		for index, value := range rows[0] {
			if strings.TrimSpace(value) != "" {
				columns[index].Header = value
			}
		}
	}
	dataRows := make([]map[string]json.RawMessage, 0, len(rows)-dataStart)
	for _, values := range rows[dataStart:] {
		row := map[string]json.RawMessage{}
		for index, value := range values {
			if index >= len(columns) {
				break
			}
			payload, _ := json.Marshal(value)
			row[columns[index].Key] = payload
		}
		dataRows = append(dataRows, row)
	}
	return models.SpreadsheetSpec{Version: models.FinalOutputSpecVersion, Title: sheetName, Sheets: []models.SpreadsheetSheet{{Name: safeSheetName(sheetName), Columns: columns, Rows: dataRows, FreezeHeader: true, AutoFilter: true}}}
}

func legacySpreadsheetRows(content string) [][]string {
	rows := [][]string{}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "|") {
			cells := strings.Split(strings.Trim(line, "|"), "|")
			separator := true
			for index := range cells {
				cells[index] = strings.TrimSpace(cells[index])
				if strings.Trim(cells[index], " :-") != "" {
					separator = false
				}
			}
			if !separator {
				rows = append(rows, cells)
			}
			continue
		}
		parsed, err := csv.NewReader(strings.NewReader(line)).Read()
		if err == nil && len(parsed) > 1 {
			rows = append(rows, parsed)
		} else {
			rows = append(rows, []string{line})
		}
	}
	return rows
}

func safeSheetName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "'")
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
