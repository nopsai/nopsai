package interactive

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestPrompterAskRequiredConfirmAndChoose(t *testing.T) {
	var output bytes.Buffer
	prompter := NewPrompter(strings.NewReader("\nvalue\nmaybe\ny\napi\n2\n"), &output)
	if value, err := prompter.Ask("Optional", "default"); err != nil || value != "default" {
		t.Fatalf("Ask() = %q, %v", value, err)
	}
	if value, err := prompter.AskRequired("Required", ""); err != nil || value != "value" {
		t.Fatalf("AskRequired() = %q, %v", value, err)
	}
	if ok, err := prompter.Confirm("Continue", false); err != nil || !ok {
		t.Fatalf("Confirm() = %v, %v", ok, err)
	}
	selected, err := prompter.Choose("Route", []Choice{
		{Label: "GET /healthz", SearchText: "platform"},
		{Label: "GET /v1/auth/providers", SearchText: "api public auth"},
	})
	if err != nil || selected != 1 {
		t.Fatalf("Choose() = %d, %v", selected, err)
	}
	if text := output.String(); !strings.Contains(text, "Enter y or n.") || !strings.Contains(text, "No.  Option") || !strings.Contains(text, "GET /v1/auth/providers") {
		t.Fatalf("prompt output = %q", text)
	}
}

func TestPrompterAskSeparatesPromptWhenInputDoesNotEcho(t *testing.T) {
	var output bytes.Buffer
	prompter := NewPrompter(strings.NewReader("value\n"), &output)
	if value, err := prompter.Ask("Value", ""); err != nil || value != "value" {
		t.Fatalf("Ask() = %q, %v", value, err)
	}
	if text := output.String(); !strings.Contains(text, "Value: \n") {
		t.Fatalf("prompt was not separated from following output: %q", text)
	}
}

func TestPrompterChooseCanSearchAgainAndReportsEOF(t *testing.T) {
	var output bytes.Buffer
	prompter := NewPrompter(strings.NewReader("missing\napi\ns\nhealth\n1\n"), &output)
	selected, err := prompter.Choose("Route", []Choice{
		{Label: "GET /healthz", SearchText: "platform health"},
		{Label: "GET /v1/auth/providers", SearchText: "api public auth"},
	})
	if err != nil || selected != 0 {
		t.Fatalf("Choose() = %d, %v", selected, err)
	}
	if !strings.Contains(output.String(), "No matches") || !strings.Contains(output.String(), "Type search terms") {
		t.Fatalf("missing search feedback in %q", output.String())
	}

	if _, err := NewPrompter(strings.NewReader(""), &bytes.Buffer{}).Ask("Value", ""); err != io.EOF {
		t.Fatalf("empty prompt error = %v, want EOF", err)
	}
}

func TestPrompterChooseUsesLiveSelectorOnTerminal(t *testing.T) {
	originalIsTerminal := isTerminal
	originalMakeRaw := makeRawTerminal
	originalRestore := restoreTerminal
	originalTerminalSize := terminalSize
	isTerminal = func(int) bool { return true }
	makeRawTerminal = func(int) (*term.State, error) { return nil, nil }
	restoreTerminal = func(int, *term.State) error { return nil }
	terminalSize = func(int) (int, int, error) { return 80, 24, nil }
	defer func() {
		isTerminal = originalIsTerminal
		makeRawTerminal = originalMakeRaw
		restoreTerminal = originalRestore
		terminalSize = originalTerminalSize
	}()

	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer inputReader.Close()
	if _, err := inputWriter.WriteString("providers\n"); err != nil {
		t.Fatal(err)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "selector-output-*")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()

	prompter := NewPrompter(inputReader, output)
	selected, err := prompter.Choose("Route", []Choice{
		{Label: "GET /healthz", SearchText: "platform"},
		{Label: "GET /v1/auth/providers", SearchText: "api public auth"},
	})
	if err != nil || selected != 1 {
		t.Fatalf("Choose() live = %d, %v", selected, err)
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "\x1b[2J\x1b[H") || !strings.Contains(text, "GET /v1/auth/providers") {
		t.Fatalf("live selector output = %q", contents)
	}
	if strings.Contains(text, "\x1b[?1049") {
		t.Fatalf("live selector should not switch to the alternate screen buffer: %q", contents)
	}
	if strings.Contains(text, "Route: GET /v1/auth/providers") {
		t.Fatalf("live selector should not leave a selected transcript line: %q", contents)
	}
}

func TestLiveChoiceViewportAndRendering(t *testing.T) {
	if offset := ensureChoiceOffset(0, 0, 25); offset != 0 {
		t.Fatalf("initial offset = %d", offset)
	}
	if offset := ensureChoiceOffset(10, 0, 25); offset != 1 {
		t.Fatalf("offset after moving past first page = %d", offset)
	}
	if offset := ensureChoiceOffset(24, 1, 25); offset != 15 {
		t.Fatalf("end offset = %d", offset)
	}
	choices := make([]Choice, 0, 12)
	for index := 0; index < 12; index++ {
		choices = append(choices, Choice{Label: "choice-" + string(rune('a'+index))})
	}
	matches := matchChoices(choices, "")
	var output bytes.Buffer
	if _, err := renderLiveChoices(&output, "Pick", choices, "", matches, 10, ensureChoiceOffset(10, 0, len(matches)), 80); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Matches: 2-11 of 12") || !strings.Contains(text, ">     11  choice-k") || strings.Contains(text, "choice-l") {
		t.Fatalf("rendered viewport = %q", text)
	}
	if strings.Contains(text, "\x1b[2J") || strings.Contains(text, "\x1b[?1049") {
		t.Fatalf("selector used destructive screen control: %q", text)
	}
	if !strings.Contains(text, "Pick\r\nSearch:") {
		t.Fatalf("selector must use CRLF in raw mode: %q", text)
	}

	output.Reset()
	longChoice := []Choice{{Label: "0123456789abcdefghijklmnopqrstuvwxyz"}}
	if _, err := renderLiveChoices(&output, "Pick", longChoice, "", []int{0}, 0, 0, 24); err != nil {
		t.Fatal(err)
	}
	if text := output.String(); strings.Contains(text, "abcdefghijklmnopqrstuvwxyz") || !strings.Contains(text, "...") {
		t.Fatalf("long choice was not truncated: %q", text)
	}

	output.Reset()
	shortChoices := []Choice{{Label: "docker-compose"}, {Label: "kubernetes"}}
	if lines, err := renderLiveChoices(&output, "Install target", shortChoices, "", []int{0, 1}, 0, 0, 80); err != nil {
		t.Fatal(err)
	} else if lines != 8 {
		t.Fatalf("short selector rendered %d lines, want compact 8:\n%s", lines, output.String())
	}
}

func TestChoiceSearchIgnoresDisplayOnlyDescriptions(t *testing.T) {
	choices := []Choice{
		{Label: "GET /healthz", Description: "domain=platform"},
		{Label: "GET /v1/mcp", Description: "domain=mcp"},
	}
	matches := matchChoices(choices, "domain")
	if len(matches) != 0 {
		t.Fatalf("description-only term matched choices: %#v", matches)
	}
	matches = matchChoices(choices, "mcp")
	if len(matches) != 1 || matches[0] != 1 {
		t.Fatalf("label term matches = %#v", matches)
	}
}

func TestChoiceTableAlignsDetails(t *testing.T) {
	choices := []Choice{
		{Label: "GET /healthz", Description: "domain=platform"},
		{Label: "POST /v1/auth/login", Description: "domain=auth, public"},
	}
	lines := choiceTableLines(choices, []int{0, 1}, 1, 0, 2, 80, true)
	if len(lines) != 4 {
		t.Fatalf("table lines = %#v", lines)
	}
	detailsColumn := strings.Index(lines[0], "Details")
	if detailsColumn < 0 {
		t.Fatalf("missing details header: %#v", lines)
	}
	for _, line := range lines[2:] {
		if index := strings.Index(line, "domain="); index != detailsColumn {
			t.Fatalf("details column = %d, want %d in %q", index, detailsColumn, line)
		}
	}
	if !strings.HasPrefix(lines[3], ">") {
		t.Fatalf("selected row was not marked: %#v", lines)
	}
}

func TestRunLiveChoiceSelectorFiltersAndNavigates(t *testing.T) {
	choices := []Choice{
		{Label: "GET /healthz", SearchText: "platform"},
		{Label: "GET /v1/auth/providers", SearchText: "api public auth"},
		{Label: "POST /v1/auth/login", SearchText: "api public auth"},
	}
	var output bytes.Buffer
	selected, err := runLiveChoiceSelector(bufio.NewReader(strings.NewReader("login\n")), &output, "Route", choices, 80)
	if err != nil || selected != 2 {
		t.Fatalf("filtered selector = %d, %v", selected, err)
	}
	if text := output.String(); !strings.Contains(text, "Search: login") || !strings.Contains(text, "> POST /v1/auth/login") || !strings.Contains(text, "Guide:") || !strings.Contains(text, "\x1b[2J") || strings.Contains(text, "+---") || strings.Contains(text, "DETAILS") {
		t.Fatalf("filtered render = %q", text)
	}

	choices = nil
	for index := 0; index < 12; index++ {
		choices = append(choices, Choice{Label: "choice-" + string(rune('a'+index))})
	}
	output.Reset()
	selected, err = runLiveChoiceSelectorWithOptions(bufio.NewReader(strings.NewReader(strings.Repeat("\x1b[B", 11)+"\n")), &output, "Choice", choices, 80, 16, ScreenOptions{})
	if err != nil || selected != 11 {
		t.Fatalf("navigated selector = %d, %v", selected, err)
	}
	if !strings.Contains(output.String(), "choice-k") {
		t.Fatalf("navigation did not scroll viewport: %q", output.String())
	}

	output.Reset()
	selected, err = runLiveChoiceSelector(bufio.NewReader(strings.NewReader("\x1b[F\x1b[5~\x1b[H\x1b[6~\n")), &output, "Choice", choices, 80)
	if err != nil || selected != 11 {
		t.Fatalf("jump selector = %d, %v", selected, err)
	}

	output.Reset()
	selected, err = runLiveChoiceSelector(bufio.NewReader(strings.NewReader("zzz\b\b\b\n")), &output, "Choice", choices, 80)
	if err != nil || selected != 0 {
		t.Fatalf("no-match recovery selector = %d, %v", selected, err)
	}
	if !strings.Contains(output.String(), "No matches.") {
		t.Fatalf("no-match state was not rendered: %q", output.String())
	}

	if _, err := runLiveChoiceSelector(bufio.NewReader(strings.NewReader("\x1b")), &bytes.Buffer{}, "Choice", choices, 80); !errors.Is(err, ErrBack) {
		t.Fatalf("escape error = %v", err)
	}
	if _, err := runLiveChoiceSelector(bufio.NewReader(strings.NewReader(string([]byte{3}))), &bytes.Buffer{}, "Choice", choices, 80); !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestZenChoiceBlockCapsMenuRowsAndPinsGuide(t *testing.T) {
	choices := make([]Choice, 0, 25)
	matches := make([]int, 0, 25)
	for index := 0; index < 25; index++ {
		choices = append(choices, Choice{Label: "choice-" + string(rune('a'+index))})
		matches = append(matches, index)
	}

	longBlock := zenChoiceBlock("Choice", choices, "", matches, 0, 0, 80, 32, ScreenOptions{})
	longText := strings.Join(longBlock, "\n")
	if !strings.Contains(longText, "choice-t") || strings.Contains(longText, "choice-u") {
		t.Fatalf("long menu was not capped at 20 visible rows: %q", longText)
	}

	shortBlock := zenChoiceBlock("Choice", choices[:2], "", []int{0, 1}, 0, 0, 80, 32, ScreenOptions{})
	if separatorIndex(longBlock) != separatorIndex(shortBlock) {
		t.Fatalf("guide separator moved: long=%d short=%d", separatorIndex(longBlock), separatorIndex(shortBlock))
	}

	scrolledBlock := zenChoiceBlock("Choice", choices, "", matches, 20, 20, 80, 32, ScreenOptions{})
	scrolledText := strings.Join(scrolledBlock, "\n")
	if !strings.Contains(scrolledText, "choice-u") || strings.Contains(scrolledText, "> choice-a") {
		t.Fatalf("scrolled menu did not honor offset: %q", scrolledText)
	}
}

func TestZenChoiceBlockShowsLongLabelsAndBreadcrumbWithoutRightSideParameters(t *testing.T) {
	longLabel := "POST    /v1/admin/service-accounts/{serviceAccountID}/tokens/{tokenID}/rotate"
	choices := []Choice{{
		Label: longLabel,
	}}
	block := zenChoiceBlock("API", choices, "", []int{0}, 0, 0, 112, 32, ScreenOptions{Breadcrumb: []string{"Home", "API"}})
	text := strings.Join(block, "\n")
	for _, want := range []string{"Home > API >", longLabel} {
		if !strings.Contains(text, want) {
			t.Fatalf("choice block missing %q: %q", want, text)
		}
	}
	for _, unwanted := range []string{"Parameters", "- attach bearer token", "- service", "- token"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("choice block rendered right-side parameter %q: %q", unwanted, text)
		}
	}
	if strings.Contains(text, "/rotate...") {
		t.Fatalf("long menu label was truncated: %q", text)
	}
}

func TestZenChoiceBlockWidthUsesWideTerminals(t *testing.T) {
	if got := zenChoiceBlockWidth(180); got < 160 {
		t.Fatalf("zenChoiceBlockWidth(180) = %d; want at least 160", got)
	}
}

func TestZenAnchoredFieldBlockKeepsMenuGeometryAndParameters(t *testing.T) {
	choices := []Choice{{Label: "GET     /internal/v1/runtime-config/{service}"}}
	choiceBlock := zenChoiceBlock("API", choices, "", []int{0}, 0, 0, 112, 32, ScreenOptions{Breadcrumb: []string{"Home", "API", "Catalog"}})
	fields := []Field{
		{Name: "query.action", Label: "Required query: action", Value: "allow", Required: true},
		{Name: "query.resource_def", Label: "Required query: resource_def", Value: "project", Required: true},
		{
			Name:        "query.resource_id",
			Label:       "Required query: resource_id",
			Description: "The unique identifier of the resource.",
			Example:     "prj_123abc",
			Required:    true,
		},
		{Name: "auth", Label: "Attach bearer token", Value: "yes", Kind: FieldBoolean},
		{Name: "send", Label: "Send request", Value: "yes", Kind: FieldBoolean},
	}
	fieldBlock := zenAnchoredFieldBlock("Runtime config", fields, 2, 0, []bool{true, true, false, false, false}, "", "Send request", 112, 32, ScreenOptions{Breadcrumb: []string{"Home", "API", "Catalog"}})
	text := strings.Join(fieldBlock, "\n")
	for _, want := range []string{
		"Home > API > Catalog >",
		"Parameters",
		"✓ 1. query: action",
		"✓ 2. query: resource_def",
		"> 3. query: resource_id",
		"Required",
		"Value: " + styleBlink("|"),
		"○ 4. attach bearer token",
		"○ 5. send request",
		styleBold("Guide:"),
		"        The unique identifier of the resource.",
		styleBold("Example:"),
		"        prj_123abc",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("anchored field block missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "- attach bearer token") {
		t.Fatalf("anchored field block rendered right-side parameter list: %q", text)
	}
	if strings.Contains(text, "Value: (blank)") {
		t.Fatalf("anchored field block rendered blank instead of cursor: %q", text)
	}
	if separatorIndex(choiceBlock) != separatorIndex(fieldBlock) {
		t.Fatalf("field detail separator moved: choice=%d field=%d", separatorIndex(choiceBlock), separatorIndex(fieldBlock))
	}
}

func separatorIndex(lines []string) int {
	for index, line := range lines {
		if strings.Contains(line, "____") {
			return index
		}
	}
	return -1
}

func TestReadChoiceKeyParsesNavigation(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want choiceKeyKind
	}{
		{"down", "\x1b[B", choiceKeyDown},
		{"up", "\x1b[A", choiceKeyUp},
		{"page down", "\x1b[6~", choiceKeyPageDown},
		{"page up", "\x1b[5~", choiceKeyPageUp},
		{"home csi", "\x1b[H", choiceKeyHome},
		{"end csi", "\x1b[F", choiceKeyEnd},
		{"home ss3", "\x1bOH", choiceKeyHome},
		{"end ss3", "\x1bOF", choiceKeyEnd},
		{"home tilde", "\x1b[1~", choiceKeyHome},
		{"end tilde", "\x1b[4~", choiceKeyEnd},
		{"escape back", "\x1b", choiceKeyBack},
		{"unknown escape", "\x1bx", choiceKeyUnknown},
		{"unknown csi", "\x1b[Z", choiceKeyUnknown},
		{"unknown tilde", "\x1b[5x", choiceKeyUnknown},
		{"enter", "\n", choiceKeyEnter},
		{"backspace", string([]byte{127}), choiceKeyBackspace},
		{"cancel", string([]byte{3}), choiceKeyCancel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, err := readChoiceKey(bufio.NewReader(strings.NewReader(test.in)))
			if err != nil || key.kind != test.want {
				t.Fatalf("readChoiceKey() = %#v, %v; want %v", key, err, test.want)
			}
		})
	}
}

func TestRunLiveFieldEditorEditsAndSubmits(t *testing.T) {
	fields := []Field{
		{Name: "version", Label: "Version", Value: "2.7.0", Required: true, Description: "Install version"},
		{Name: "output", Label: "Output", Value: "nopsai-install", Required: true, Description: "Install output directory"},
		{Name: "run", Label: "Run", Value: "no", Kind: FieldBoolean, Description: "Start after generating files"},
	}
	var output bytes.Buffer
	edited, err := runLiveFieldEditor(bufio.NewReader(strings.NewReader("\nprod\ny"+string([]byte{19}))), &output, "Install", fields, 100, 40, ScreenOptions{Title: "Install"})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, field := range edited {
		values[field.Name] = field.Value
	}
	if values["version"] != "2.7.0" || values["output"] != "prod" || values["run"] != "yes" {
		t.Fatalf("edited fields = %#v", values)
	}
	if text := output.String(); !strings.Contains(text, "Home > Install >") ||
		!strings.Contains(text, "Parameters") ||
		!strings.Contains(text, "✓ 1. version") ||
		!strings.Contains(text, "> 2. output") ||
		!strings.Contains(text, "Value: prod") ||
		!strings.Contains(text, "> 3. run") ||
		!strings.Contains(text, "Value: yes") ||
		!strings.Contains(text, styleBold("Guide:")) ||
		!strings.Contains(text, "        Install output directory") ||
		strings.Contains(text, "Run?") ||
		strings.Contains(text, "> Yes") ||
		strings.Contains(text, "+---") ||
		strings.Contains(text, "VALUES & DETAILS") ||
		strings.Contains(text, "Action:") ||
		strings.Contains(text, "Step:") {
		t.Fatalf("form output = %q", text)
	}
}

func TestRunLiveFieldEditorSupportsMultilineInput(t *testing.T) {
	fields := []Field{
		{Name: "body", Label: "Payload source: paste", Required: true, Multiline: true, Description: "Paste JSON content."},
	}
	var output bytes.Buffer
	edited, err := runLiveFieldEditor(bufio.NewReader(strings.NewReader("{\n}"+string([]byte{19}))), &output, "API Request", fields, 100, 36, ScreenOptions{Title: "API Request"})
	if err != nil {
		t.Fatal(err)
	}
	if edited[0].Value != "{\n}" {
		t.Fatalf("multiline value = %q", edited[0].Value)
	}
	text := output.String()
	if !strings.Contains(text, "Home > API Request >") ||
		!strings.Contains(text, "Parameters") ||
		!strings.Contains(text, "> 1. payload source: paste") ||
		!strings.Contains(text, "Value:") ||
		!strings.Contains(text, "      {") ||
		!strings.Contains(text, "      }") ||
		!strings.Contains(text, styleBold("Guide:")) ||
		!strings.Contains(text, "        Paste JSON content.") ||
		strings.Contains(text, "Value: 2 lines") ||
		strings.Contains(text, "Input mode: multiline editor") {
		t.Fatalf("multiline form output = %q", text)
	}
}

func TestRunLiveFieldEditorSkipsBlankOptionalMultilineWithEnter(t *testing.T) {
	fields := []Field{
		{Name: "query.extra", Label: "Additional query values", Multiline: true, Description: "Optional query assignments."},
		{Name: "auth", Label: "Attach bearer token", Value: "yes", Kind: FieldBoolean, Description: "Attach auth."},
	}
	var output bytes.Buffer
	edited, err := runLiveFieldEditor(bufio.NewReader(strings.NewReader("\n"+string([]byte{19}))), &output, "API Request", fields, 100, 36, ScreenOptions{Title: "API Request"})
	if err != nil {
		t.Fatal(err)
	}
	if edited[0].Value != "" || edited[1].Value != "yes" {
		t.Fatalf("edited fields = %#v", edited)
	}
	text := output.String()
	if !strings.Contains(text, "✓ 1. additional query values") ||
		!strings.Contains(text, "> 2. attach bearer token") ||
		strings.Contains(text, "<blank line>") {
		t.Fatalf("optional multiline skip output = %q", text)
	}
}

func TestRunLiveTextViewerScrollsAndBacksOut(t *testing.T) {
	content := []string{"Result"}
	for index := 0; index < 30; index++ {
		content = append(content, "line-"+strconv.Itoa(index))
	}
	var output bytes.Buffer
	err := runLiveTextViewer(bufio.NewReader(strings.NewReader("\x1b[6~\x1b")), &output, "Result", content, 90, 18, ScreenOptions{})
	if !errors.Is(err, ErrBack) {
		t.Fatalf("viewer error = %v", err)
	}
	if text := output.String(); !strings.Contains(text, "Home > Result >") ||
		!strings.Contains(text, "(11-20 of 31)") ||
		!strings.Contains(text, "Result") ||
		!strings.Contains(text, "line-") ||
		strings.Contains(text, "Lines") ||
		!strings.Contains(text, "\x1b[2J") {
		t.Fatalf("viewer output = %q", text)
	}
}
