package interactive

import (
	"bufio"
	"bytes"
	"io"
	"os"
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
	if text := output.String(); !strings.Contains(text, "Enter y or n.") || !strings.Contains(text, "GET /v1/auth/providers") {
		t.Fatalf("prompt output = %q", text)
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
	if !strings.Contains(output.String(), "No matches") {
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
	if !strings.Contains(string(contents), "Route: GET /v1/auth/providers") {
		t.Fatalf("live selector output = %q", contents)
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
	if !strings.Contains(text, "Showing 2-11 of 12 matches") || !strings.Contains(text, "> 11  choice-k") || strings.Contains(text, "choice-l") {
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
	if text := output.String(); !strings.Contains(text, "Search: login") || !strings.Contains(text, ">  1  POST /v1/auth/login") || strings.Contains(text, "\x1b[2J") || strings.Contains(text, "\x1b[?1049") {
		t.Fatalf("filtered render = %q", text)
	}

	choices = nil
	for index := 0; index < 12; index++ {
		choices = append(choices, Choice{Label: "choice-" + string(rune('a'+index))})
	}
	output.Reset()
	selected, err = runLiveChoiceSelector(bufio.NewReader(strings.NewReader(strings.Repeat("\x1b[B", 11)+"\n")), &output, "Choice", choices, 80)
	if err != nil || selected != 11 {
		t.Fatalf("navigated selector = %d, %v", selected, err)
	}
	if !strings.Contains(output.String(), "Showing 3-12 of 12 matches") {
		t.Fatalf("navigation did not scroll viewport: %q", output.String())
	}

	output.Reset()
	selected, err = runLiveChoiceSelector(bufio.NewReader(strings.NewReader("\x1b[F\x1b[5~\x1b[H\x1b[6~\n")), &output, "Choice", choices, 80)
	if err != nil || selected != 10 {
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

	if _, err := runLiveChoiceSelector(bufio.NewReader(strings.NewReader(string([]byte{3}))), &bytes.Buffer{}, "Choice", choices, 80); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancel error = %v", err)
	}
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
		{"escape eof", "\x1b", choiceKeyCancel},
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
