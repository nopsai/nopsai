package nopsai

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderPipelineFinalOutputDocumentHTMLUsesRealLayoutAndEscapesContent(t *testing.T) {
	longCell := strings.Repeat("complete detail ", 40)
	content := `{"version":"1","title":"<Release>","sections":[{"title":"Findings","blocks":[{"type":"table","table":{"columns":["Status","Detail"],"rows":[["Passed",` + quotedJSON(longCell) + `]]}},{"type":"callout","tone":"warning","title":"Review","text":"Check <input>"}]}]}`
	payload, err := renderPipelineFinalOutputDocumentHTML("fallback", content)
	if err != nil {
		t.Fatalf("renderPipelineFinalOutputDocumentHTML() error = %v", err)
	}
	for _, expected := range [][]byte{[]byte("<table>"), []byte("<thead>"), []byte("complete detail"), []byte("&lt;Release&gt;"), []byte("Check &lt;input&gt;")} {
		if !bytes.Contains(payload, expected) {
			t.Fatalf("HTML missing %q: %s", expected, payload)
		}
	}
	if bytes.Contains(payload, []byte("<Release>")) || !bytes.Contains(payload, []byte(longCell)) {
		t.Fatalf("HTML escaped or truncated incorrectly: %s", payload)
	}
}

func TestRenderPipelineFinalOutputDocumentHTMLSupportsLegacyMarkdown(t *testing.T) {
	payload, err := renderPipelineFinalOutputDocumentHTML("Legacy", "## Summary\n\n| Name | Value |\n| --- | --- |\n| API | 42 |\n\n<script>bad()</script>")
	if err != nil {
		t.Fatalf("render legacy Markdown error = %v", err)
	}
	if !bytes.Contains(payload, []byte("<table>")) || bytes.Contains(payload, []byte("<script>")) {
		t.Fatalf("legacy HTML = %s", payload)
	}
}

func quotedJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
