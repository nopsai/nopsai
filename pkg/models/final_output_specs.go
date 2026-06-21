package models

import "encoding/json"

const FinalOutputSpecVersion = "1"

type DocumentSpec struct {
	Version  string             `json:"version"`
	Title    string             `json:"title"`
	Subtitle string             `json:"subtitle,omitempty"`
	Metadata []DocumentMetadata `json:"metadata,omitempty"`
	Sections []DocumentSection  `json:"sections"`
}

type DocumentMetadata struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type DocumentSection struct {
	Title  string          `json:"title"`
	Blocks []DocumentBlock `json:"blocks"`
}

type DocumentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	Title string         `json:"title,omitempty"`
	Tone  string         `json:"tone,omitempty"`
	Items []string       `json:"items,omitempty"`
	Table *DocumentTable `json:"table,omitempty"`
}

type DocumentTable struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type SpreadsheetSpec struct {
	Version string             `json:"version"`
	Title   string             `json:"title,omitempty"`
	Sheets  []SpreadsheetSheet `json:"sheets"`
}

type SpreadsheetSheet struct {
	Name         string                       `json:"name"`
	Columns      []SpreadsheetColumn          `json:"columns"`
	Rows         []map[string]json.RawMessage `json:"rows"`
	FreezeHeader bool                         `json:"freeze_header,omitempty"`
	AutoFilter   bool                         `json:"auto_filter,omitempty"`
}

type SpreadsheetColumn struct {
	Key          string  `json:"key"`
	Header       string  `json:"header"`
	Width        float64 `json:"width,omitempty"`
	NumberFormat string  `json:"number_format,omitempty"`
}
