package nopsai

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBuildPipelineFinalOutputXLSXRendersTypedMultiSheetWorkbook(t *testing.T) {
	content := `{"version":"1","title":"Operations","sheets":[{"name":"Summary","columns":[{"key":"service","header":"Service","width":22,"number_format":"text"},{"key":"availability","header":"Availability","number_format":"percent"},{"key":"active","header":"Active","number_format":"boolean"}],"rows":[{"service":"=not-a-formula","availability":0.995,"active":true}],"freeze_header":true,"auto_filter":true},{"name":"Costs","columns":[{"key":"amount","header":"Amount","number_format":"currency_eur"}],"rows":[{"amount":1234.5}]}]}`
	payload, err := buildPipelineFinalOutputXLSX("fallback", content)
	if err != nil {
		t.Fatalf("buildPipelineFinalOutputXLSX() error = %v", err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer workbook.Close()
	if got := workbook.GetSheetList(); len(got) != 2 || got[0] != "Summary" || got[1] != "Costs" {
		t.Fatalf("sheets = %#v", got)
	}
	if value, _ := workbook.GetCellValue("Summary", "A2"); value != "=not-a-formula" {
		t.Fatalf("A2 = %q", value)
	}
	if formula, _ := workbook.GetCellFormula("Summary", "A2"); formula != "" {
		t.Fatalf("A2 formula = %q", formula)
	}
	if value, _ := workbook.GetCellValue("Summary", "B2"); value != "99.50%" {
		t.Fatalf("B2 = %q", value)
	}
	if value, _ := workbook.GetCellValue("Costs", "A2"); !strings.Contains(value, "1,234.50") {
		t.Fatalf("Costs!A2 = %q", value)
	}
}

func TestBuildPipelineFinalOutputXLSXSupportsLegacyTables(t *testing.T) {
	payload, err := buildPipelineFinalOutputXLSX("Legacy", "| Name | Value |\n| --- | --- |\n| API | 42 |")
	if err != nil {
		t.Fatalf("build legacy workbook error = %v", err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer workbook.Close()
	if value, _ := workbook.GetCellValue("Legacy", "A2"); value != "API" {
		t.Fatalf("Legacy!A2 = %q", value)
	}
}

func TestBuildPipelineFinalOutputXLSXSupportsFormattedEmptySheets(t *testing.T) {
	payload, err := buildPipelineFinalOutputXLSX("Empty", `{"version":"1","sheets":[{"name":"Empty","columns":[{"key":"value","header":"Value","number_format":"integer"}],"rows":[],"auto_filter":true}]}`)
	if err != nil {
		t.Fatalf("build empty workbook error = %v", err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer workbook.Close()
	if value, _ := workbook.GetCellValue("Empty", "A1"); value != "Value" {
		t.Fatalf("Empty!A1 = %q", value)
	}
}
