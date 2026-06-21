package nopsai

import (
	"encoding/json"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseDocumentSpecAcceptsSupportedBlocks(t *testing.T) {
	content := `{"version":"1","title":"Release report","metadata":[{"label":"Run","value":"run-1"}],"sections":[{"title":"Summary","blocks":[{"type":"paragraph","text":"Complete"},{"type":"bullet_list","items":["One"]},{"type":"numbered_list","items":["First"]},{"type":"table","table":{"columns":["Name","Value"],"rows":[["API","42"]]}},{"type":"callout","tone":"success","title":"Result","text":"Passed"}]}]}`
	spec, err := parseDocumentSpec(content)
	if err != nil {
		t.Fatalf("parseDocumentSpec() error = %v", err)
	}
	if spec.Title != "Release report" || len(spec.Sections[0].Blocks) != 5 {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestParseDocumentSpecRejectsUnknownFieldsAndInvalidTables(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "unknown field", content: `{"version":"1","title":"Report","unknown":true,"sections":[{"title":"Summary","blocks":[{"type":"paragraph","text":"Done"}]}]}`, want: "unknown field"},
		{name: "row width", content: `{"version":"1","title":"Report","sections":[{"title":"Summary","blocks":[{"type":"table","table":{"columns":["One","Two"],"rows":[["only one"]]}}]}]}`, want: "expected 2"},
		{name: "mixed block fields", content: `{"version":"1","title":"Report","sections":[{"title":"Summary","blocks":[{"type":"paragraph","text":"Done","items":["bad"]}]}]}`, want: "another block type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseDocumentSpec(test.content)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseSpreadsheetSpecValidatesTypedRows(t *testing.T) {
	content := `{"version":"1","title":"Metrics","sheets":[{"name":"Summary","columns":[{"key":"service","header":"Service","number_format":"text"},{"key":"availability","header":"Availability","number_format":"percent"},{"key":"healthy","header":"Healthy","number_format":"boolean"}],"rows":[{"service":"API","availability":0.999,"healthy":true}],"freeze_header":true,"auto_filter":true}]}`
	spec, err := parseSpreadsheetSpec(content)
	if err != nil {
		t.Fatalf("parseSpreadsheetSpec() error = %v", err)
	}
	if len(spec.Sheets) != 1 || string(spec.Sheets[0].Rows[0]["availability"]) != "0.999" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestParseSpreadsheetSpecRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "unknown column", content: `{"version":"1","sheets":[{"name":"One","columns":[{"key":"name","header":"Name"}],"rows":[{"other":"bad"}]}]}`, want: "unknown column"},
		{name: "object cell", content: `{"version":"1","sheets":[{"name":"One","columns":[{"key":"name","header":"Name"}],"rows":[{"name":{"nested":true}}]}]}`, want: "JSON scalars"},
		{name: "format", content: `{"version":"1","sheets":[{"name":"One","columns":[{"key":"name","header":"Name","number_format":"formula"}],"rows":[]}]}`, want: "unsupported number_format"},
		{name: "duplicate sheet", content: `{"version":"1","sheets":[{"name":"One","columns":[{"key":"name","header":"Name"}],"rows":[]},{"name":"one","columns":[{"key":"name","header":"Name"}],"rows":[]}]}`, want: "duplicated"},
		{name: "normalized duplicate sheet", content: `{"version":"1","sheets":[{"name":"One/Two","columns":[{"key":"name","header":"Name"}],"rows":[]},{"name":"One:Two","columns":[{"key":"name","header":"Name"}],"rows":[]}]}`, want: "normalization"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpreadsheetSpec(test.content)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSpreadsheetSpecRejectsOversizedCell(t *testing.T) {
	value, _ := json.Marshal(strings.Repeat("x", maxSpreadsheetCellBytes+1))
	spec := models.SpreadsheetSpec{Version: "1", Sheets: []models.SpreadsheetSheet{{
		Name: "Data", Columns: []models.SpreadsheetColumn{{Key: "value", Header: "Value"}},
		Rows: []map[string]json.RawMessage{{"value": value}},
	}}}
	if err := validateSpreadsheetSpec(spec); err == nil || !strings.Contains(err.Error(), "cell limit") {
		t.Fatalf("error = %v, want cell limit", err)
	}
}
