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

type DashboardSpec struct {
	Version string           `json:"version"`
	Title   string           `json:"title"`
	Blocks  []DashboardBlock `json:"blocks"`
}

type DashboardBlock struct {
	Type     string                       `json:"type"`
	Title    string                       `json:"title,omitempty"`
	Text     string                       `json:"text,omitempty"`
	Tone     string                       `json:"tone,omitempty"`
	Status   string                       `json:"status,omitempty"`
	Label    string                       `json:"label,omitempty"`
	Value    string                       `json:"value,omitempty"`
	Href     string                       `json:"href,omitempty"`
	Items    []DashboardBlockItem         `json:"items,omitempty"`
	Columns  []DashboardTableColumn       `json:"columns,omitempty"`
	Rows     []map[string]json.RawMessage `json:"rows,omitempty"`
	Progress *DashboardProgress           `json:"progress,omitempty"`
	Chart    *DashboardChart              `json:"chart,omitempty"`
}

type DashboardBlockItem struct {
	Label  string `json:"label,omitempty"`
	Value  string `json:"value,omitempty"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"`
	Tone   string `json:"tone,omitempty"`
	Href   string `json:"href,omitempty"`
}

type DashboardTableColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type DashboardProgress struct {
	Value float64 `json:"value"`
	Max   float64 `json:"max,omitempty"`
	Unit  string  `json:"unit,omitempty"`
}

type DashboardChart struct {
	Type                string                 `json:"type"`
	Unit                string                 `json:"unit,omitempty"`
	TimeWindow          *DashboardTimeWindow   `json:"time_window,omitempty"`
	AggregationInterval string                 `json:"aggregation_interval,omitempty"`
	MissingValues       string                 `json:"missing_values,omitempty"`
	Dimensions          map[string]string      `json:"dimensions,omitempty"`
	Series              []DashboardChartSeries `json:"series"`
}

type DashboardTimeWindow struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type DashboardChartSeries struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label,omitempty"`
	Team        string                 `json:"team,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Unit        string                 `json:"unit,omitempty"`
	Color       string                 `json:"color,omitempty"`
	Points      []DashboardSeriesPoint `json:"points"`
}

type DashboardSeriesPoint struct {
	Timestamp string   `json:"timestamp,omitempty"`
	Label     string   `json:"label,omitempty"`
	Value     *float64 `json:"value,omitempty"`
}
