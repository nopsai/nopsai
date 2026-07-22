package interactive

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const choiceViewportSize = 10
const zenChoiceViewportSize = 20
const fallbackChoiceTerminalWidth = 120
const fallbackChoiceTerminalHeight = 30
const ansiReset = "\x1b[0m"

var (
	isTerminal      = term.IsTerminal
	makeRawTerminal = term.MakeRaw
	restoreTerminal = term.Restore
	terminalSize    = term.GetSize
)

var (
	ErrBack      = errors.New("interactive screen back")
	ErrCancelled = errors.New("interactive session cancelled")
)

type Choice struct {
	Label       string
	Description string
	SearchText  string
}

type ScreenOptions struct {
	Title       string
	Breadcrumb  []string
	Header      []string
	Footer      []string
	Sidebar     []string
	LeftTitle   string
	RightTitle  string
	LeftWidth   int
	ActionLabel string
	DetailTitle func(index int, choice Choice) string
	Detail      func(index int, choice Choice) []string
}

type FieldKind string

const (
	FieldText    FieldKind = "text"
	FieldBoolean FieldKind = "boolean"
)

type Field struct {
	Name        string
	Label       string
	Value       string
	Default     string
	Required    bool
	Kind        FieldKind
	Multiline   bool
	Description string
	Example     string
}

type screenSection struct {
	Title string
	Lines []string
}

type Prompter struct {
	reader  *bufio.Reader
	out     io.Writer
	inFile  *os.File
	outFile *os.File
}

func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	prompter := &Prompter{reader: bufio.NewReader(in), out: out}
	if file, ok := in.(*os.File); ok {
		prompter.inFile = file
	}
	if file, ok := out.(*os.File); ok {
		prompter.outFile = file
	}
	return prompter
}

func (p *Prompter) Ask(label, defaultValue string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", errors.New("prompt label is required")
	}
	if defaultValue != "" {
		_, _ = fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
	} else {
		_, _ = fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.reader.ReadString('\n')
	if !p.promptEchoesToOutput() {
		_, _ = fmt.Fprintln(p.out)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		line = defaultValue
	}
	if err != nil && errors.Is(err, io.EOF) && line == "" {
		return "", io.EOF
	}
	return line, nil
}

func (p *Prompter) AskRequired(label, defaultValue string) (string, error) {
	for {
		value, err := p.Ask(label, defaultValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return value, nil
		}
		_, _ = fmt.Fprintln(p.out, "A value is required.")
	}
}

func (p *Prompter) Confirm(label string, defaultValue bool) (bool, error) {
	defaultText := "y"
	if !defaultValue {
		defaultText = "n"
	}
	for {
		value, err := p.Ask(label+" (y/n)", defaultText)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			_, _ = fmt.Fprintln(p.out, "Enter y or n.")
		}
	}
}

func (p *Prompter) Choose(label string, choices []Choice) (int, error) {
	if len(choices) == 0 {
		return -1, errors.New("no choices available")
	}
	if p.canUseLiveSelector() {
		return p.chooseLive(label, choices, ScreenOptions{})
	}
	return p.chooseLine(label, choices)
}

func (p *Prompter) ChooseScreen(label string, choices []Choice, options ScreenOptions) (int, error) {
	if len(choices) == 0 {
		return -1, errors.New("no choices available")
	}
	if p.canUseLiveSelector() {
		return p.chooseLive(label, choices, options)
	}
	return p.chooseLine(label, choices)
}

func (p *Prompter) CanUseLiveSelector() bool {
	return p.canUseLiveSelector()
}

func (p *Prompter) canUseLiveSelector() bool {
	return p.inFile != nil &&
		p.outFile != nil &&
		isTerminal(int(p.inFile.Fd())) &&
		isTerminal(int(p.outFile.Fd()))
}

func (p *Prompter) promptEchoesToOutput() bool {
	return p.inFile != nil &&
		p.outFile != nil &&
		isTerminal(int(p.inFile.Fd())) &&
		isTerminal(int(p.outFile.Fd()))
}

func (p *Prompter) chooseLine(label string, choices []Choice) (int, error) {
	label = strings.TrimSpace(label)
	for {
		_, _ = fmt.Fprintln(p.out, label)
		_, _ = fmt.Fprintln(p.out, "Type search terms, or press Enter to list every option.")
		query, err := p.Ask("Search", "")
		if err != nil {
			return -1, err
		}
		matches := matchChoices(choices, query)
		if len(matches) == 0 {
			_, _ = fmt.Fprintln(p.out, "No matches. Try another search.")
			continue
		}
		limit := len(matches)
		if limit > choiceViewportSize {
			limit = choiceViewportSize
		}
		_, _ = fmt.Fprintf(p.out, "Matches: 1-%d of %d\n", limit, len(matches))
		for _, line := range choiceTableLines(choices, matches, -1, 0, limit, fallbackChoiceTerminalWidth, false) {
			_, _ = fmt.Fprintln(p.out, line)
		}
		if len(matches) > limit {
			_, _ = fmt.Fprintf(p.out, "Showing first %d of %d matches. Enter s to search again.\n", limit, len(matches))
		}
		for {
			raw, err := p.Ask("Select number", "1")
			if err != nil {
				return -1, err
			}
			raw = strings.TrimSpace(strings.ToLower(raw))
			if raw == "s" || raw == "search" {
				break
			}
			selected, err := strconv.Atoi(raw)
			if err != nil || selected < 1 || selected > limit {
				_, _ = fmt.Fprintf(p.out, "Enter a number from 1 to %d, or s to search again.\n", limit)
				continue
			}
			return matches[selected-1], nil
		}
	}
}

func (p *Prompter) chooseLive(label string, choices []Choice, options ScreenOptions) (int, error) {
	var selectedChoice int
	err := p.withLiveScreen(func(width, height int) error {
		choice, err := runLiveChoiceSelectorWithOptions(p.reader, p.out, label, choices, width, height, options)
		if err != nil {
			return err
		}
		selectedChoice = choice
		return nil
	})
	if err != nil {
		return -1, err
	}
	return selectedChoice, nil
}

func (p *Prompter) withLiveScreen(run func(width, height int) error) error {
	oldState, err := makeRawTerminal(int(p.inFile.Fd()))
	if err != nil {
		return err
	}
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = restoreTerminal(int(p.inFile.Fd()), oldState)
		_, _ = fmt.Fprint(p.out, "\x1b[?25h\x1b[?1049l")
	}
	defer cleanup()

	_, _ = fmt.Fprint(p.out, "\x1b[?1049h\x1b[?25l")
	width := fallbackChoiceTerminalWidth
	height := fallbackChoiceTerminalHeight
	if detectedWidth, detectedHeight, err := terminalSize(int(p.outFile.Fd())); err == nil {
		if detectedWidth > 0 {
			width = detectedWidth
		}
		if detectedHeight > 0 {
			height = detectedHeight
		}
	}
	return run(width, height)
}

func (p *Prompter) EditFieldsScreen(label string, fields []Field, options ScreenOptions) ([]Field, error) {
	if len(fields) == 0 {
		return nil, errors.New("no fields available")
	}
	if !p.canUseLiveSelector() {
		return p.editFieldsLine(fields)
	}
	var resolved []Field
	err := p.withLiveScreen(func(width, height int) error {
		var err error
		resolved, err = runLiveFieldEditor(p.reader, p.out, label, fields, width, height, options)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (p *Prompter) editFieldsLine(fields []Field) ([]Field, error) {
	resolved := cloneFields(fields)
	for index := range resolved {
		field := &resolved[index]
		defaultValue := field.Value
		if defaultValue == "" {
			defaultValue = field.Default
		}
		if field.Kind == FieldBoolean {
			value, err := p.Confirm(field.Label, parseFieldBool(defaultValue))
			if err != nil {
				return nil, err
			}
			field.Value = formatFieldBool(value)
			continue
		}
		var (
			value string
			err   error
		)
		if field.Required {
			value, err = p.AskRequired(field.Label, defaultValue)
		} else {
			value, err = p.Ask(field.Label, defaultValue)
		}
		if err != nil {
			return nil, err
		}
		field.Value = value
	}
	return resolved, nil
}

func (p *Prompter) ShowTextScreen(label string, content []string, options ScreenOptions) error {
	if !p.canUseLiveSelector() {
		for _, line := range content {
			if _, err := fmt.Fprintln(p.out, line); err != nil {
				return err
			}
		}
		return nil
	}
	return p.withLiveScreen(func(width, height int) error {
		return runLiveTextViewer(p.reader, p.out, label, content, width, height, options)
	})
}

func runLiveFieldEditor(reader *bufio.Reader, out io.Writer, label string, fields []Field, width, height int, options ScreenOptions) ([]Field, error) {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	fields = cloneFields(fields)
	for index := range fields {
		if fields[index].Kind == "" {
			fields[index].Kind = FieldText
		}
		if fields[index].Value == "" && fields[index].Default != "" {
			fields[index].Value = fields[index].Default
		}
		if fields[index].Kind == FieldBoolean {
			fields[index].Value = formatFieldBool(parseFieldBool(fields[index].Value))
		}
	}
	selected := 0
	offset := 0
	touched := make([]bool, len(fields))
	completed := make([]bool, len(fields))
	status := ""
	for {
		viewport := liveFieldViewportSize(height, options)
		offset = ensureChoiceOffsetRows(selected, offset, len(fields), viewport)
		if err := renderFieldScreen(out, label, fields, selected, offset, completed, status, width, height, options); err != nil {
			return nil, err
		}
		key, err := readChoiceKey(reader)
		if err != nil {
			return nil, err
		}
		status = ""
		switch key.kind {
		case choiceKeyBack:
			return nil, ErrBack
		case choiceKeyCancel:
			return nil, ErrCancelled
		case choiceKeySubmit:
			if missing := firstMissingRequiredField(fields); missing >= 0 {
				selected = missing
				status = "Required field: " + fieldDisplayLabel(fields[missing])
				continue
			}
			return fields, nil
		case choiceKeyEnter:
			if fields[selected].Multiline {
				if !touched[selected] {
					fields[selected].Value = ""
					touched[selected] = true
				}
				fields[selected].Value += "\n"
				continue
			}
			if fields[selected].Required && strings.TrimSpace(fields[selected].Value) == "" {
				status = "Required field: " + fieldDisplayLabel(fields[selected])
				continue
			}
			completed[selected] = true
			if selected+1 >= len(fields) {
				if missing := firstMissingRequiredField(fields); missing >= 0 {
					selected = missing
					status = "Required field: " + fieldDisplayLabel(fields[missing])
					continue
				}
				return fields, nil
			}
			selected++
		case choiceKeyTab:
			if fields[selected].Required && strings.TrimSpace(fields[selected].Value) == "" {
				status = "Required field: " + fieldDisplayLabel(fields[selected])
				continue
			}
			completed[selected] = true
			if selected+1 >= len(fields) {
				if missing := firstMissingRequiredField(fields); missing >= 0 {
					selected = missing
					status = "Required field: " + fieldDisplayLabel(fields[missing])
					continue
				}
				return fields, nil
			}
			selected++
		case choiceKeyUp:
			if selected > 0 {
				selected--
			}
		case choiceKeyDown:
			if selected+1 < len(fields) {
				selected++
			}
		case choiceKeyPageUp:
			selected -= viewport
			if selected < 0 {
				selected = 0
			}
		case choiceKeyPageDown:
			selected += viewport
			if selected >= len(fields) {
				selected = len(fields) - 1
			}
		case choiceKeyHome:
			selected = 0
		case choiceKeyEnd:
			selected = len(fields) - 1
		case choiceKeyBackspace:
			if fields[selected].Kind == FieldBoolean {
				fields[selected].Value = "no"
				touched[selected] = true
				continue
			}
			if !touched[selected] {
				fields[selected].Value = ""
				touched[selected] = true
				continue
			}
			runes := []rune(fields[selected].Value)
			if len(runes) > 0 {
				fields[selected].Value = string(runes[:len(runes)-1])
			}
		case choiceKeyRune:
			if fields[selected].Kind == FieldBoolean {
				switch strings.ToLower(string(key.value)) {
				case " ", "t":
					fields[selected].Value = formatFieldBool(!parseFieldBool(fields[selected].Value))
					touched[selected] = true
				case "y":
					fields[selected].Value = "yes"
					touched[selected] = true
				case "n":
					fields[selected].Value = "no"
					touched[selected] = true
				}
				continue
			}
			if !touched[selected] {
				fields[selected].Value = ""
				touched[selected] = true
			}
			fields[selected].Value += string(key.value)
		}
	}
}

func runLiveTextViewer(reader *bufio.Reader, out io.Writer, label string, content []string, width, height int, options ScreenOptions) error {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	offset := 0
	for {
		viewport := liveTextViewportSize(height, options)
		lines := wrapScreenLines(content, textPanelWidth(width))
		if len(lines) <= viewport {
			offset = 0
		} else if offset > len(lines)-viewport {
			offset = len(lines) - viewport
		}
		if err := renderTextScreen(out, label, lines, offset, width, height, options); err != nil {
			return err
		}
		key, err := readChoiceKey(reader)
		if err != nil {
			return err
		}
		switch key.kind {
		case choiceKeyBack:
			return ErrBack
		case choiceKeyEnter:
			return nil
		case choiceKeyCancel:
			return ErrCancelled
		case choiceKeyUp:
			if offset > 0 {
				offset--
			}
		case choiceKeyDown:
			if offset+viewport < len(lines) {
				offset++
			}
		case choiceKeyPageUp:
			offset -= viewport
			if offset < 0 {
				offset = 0
			}
		case choiceKeyPageDown:
			offset += viewport
			if offset+viewport > len(lines) {
				offset = len(lines) - viewport
				if offset < 0 {
					offset = 0
				}
			}
		case choiceKeyHome:
			offset = 0
		case choiceKeyEnd:
			offset = len(lines) - viewport
			if offset < 0 {
				offset = 0
			}
		case choiceKeyRune:
			switch strings.ToLower(string(key.value)) {
			case "q":
				return ErrBack
			}
		}
	}
}

func runLiveChoiceSelector(reader *bufio.Reader, out io.Writer, label string, choices []Choice, width int) (int, error) {
	return runLiveChoiceSelectorWithOptions(reader, out, label, choices, width, fallbackChoiceTerminalHeight, ScreenOptions{})
}

func runLiveChoiceSelectorWithOptions(reader *bufio.Reader, out io.Writer, label string, choices []Choice, width, height int, options ScreenOptions) (int, error) {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	query := ""
	matches := matchChoices(choices, query)
	selected := 0
	offset := 0
	for {
		viewport := liveChoiceViewportSize(height, options)
		offset = ensureChoiceOffsetRows(selected, offset, len(matches), viewport)
		if err := renderChoiceScreen(out, label, choices, query, matches, selected, offset, width, height, options); err != nil {
			return -1, err
		}
		key, err := readChoiceKey(reader)
		if err != nil {
			return -1, err
		}
		switch key.kind {
		case choiceKeyBack:
			return -1, ErrBack
		case choiceKeyCancel:
			return -1, ErrCancelled
		case choiceKeyEnter:
			if len(matches) == 0 {
				continue
			}
			return matches[selected], nil
		case choiceKeyUp:
			if selected > 0 {
				selected--
			}
		case choiceKeyDown:
			if selected+1 < len(matches) {
				selected++
			}
		case choiceKeyPageUp:
			selected -= viewport
			if selected < 0 {
				selected = 0
			}
		case choiceKeyPageDown:
			if len(matches) > 0 {
				selected += viewport
				if selected >= len(matches) {
					selected = len(matches) - 1
				}
			}
		case choiceKeyHome:
			selected = 0
		case choiceKeyEnd:
			if len(matches) > 0 {
				selected = len(matches) - 1
			}
		case choiceKeyBackspace:
			if query != "" {
				runes := []rune(query)
				query = string(runes[:len(runes)-1])
				matches = matchChoices(choices, query)
				selected = 0
				offset = 0
			}
		case choiceKeyRune:
			query += string(key.value)
			matches = matchChoices(choices, query)
			selected = 0
			offset = 0
		}
		if len(matches) == 0 {
			selected = 0
			offset = 0
		}
	}
}

type choiceKeyKind int

const (
	choiceKeyUnknown choiceKeyKind = iota
	choiceKeyRune
	choiceKeyEnter
	choiceKeyTab
	choiceKeySubmit
	choiceKeyBackspace
	choiceKeyUp
	choiceKeyDown
	choiceKeyPageUp
	choiceKeyPageDown
	choiceKeyHome
	choiceKeyEnd
	choiceKeyBack
	choiceKeyCancel
)

type choiceKey struct {
	kind  choiceKeyKind
	value rune
}

func readChoiceKey(reader *bufio.Reader) (choiceKey, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return choiceKey{}, err
	}
	switch value {
	case '\r', '\n':
		return choiceKey{kind: choiceKeyEnter}, nil
	case 9:
		return choiceKey{kind: choiceKeyTab}, nil
	case 19:
		return choiceKey{kind: choiceKeySubmit}, nil
	case 3:
		return choiceKey{kind: choiceKeyCancel}, nil
	case 8, 127:
		return choiceKey{kind: choiceKeyBackspace}, nil
	case 27:
		if reader.Buffered() == 0 {
			return choiceKey{kind: choiceKeyBack}, nil
		}
		return readEscapeChoiceKey(reader)
	default:
		if value >= 32 && value != 127 {
			return choiceKey{kind: choiceKeyRune, value: rune(value)}, nil
		}
		return choiceKey{kind: choiceKeyUnknown}, nil
	}
}

func readEscapeChoiceKey(reader *bufio.Reader) (choiceKey, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return choiceKey{kind: choiceKeyBack}, nil
	}
	switch value {
	case '[':
		return readCSIChoiceKey(reader)
	case 'O':
		value, err := reader.ReadByte()
		if err != nil {
			return choiceKey{}, err
		}
		switch value {
		case 'H':
			return choiceKey{kind: choiceKeyHome}, nil
		case 'F':
			return choiceKey{kind: choiceKeyEnd}, nil
		default:
			return choiceKey{kind: choiceKeyUnknown}, nil
		}
	default:
		return choiceKey{kind: choiceKeyUnknown}, nil
	}
}

func readCSIChoiceKey(reader *bufio.Reader) (choiceKey, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return choiceKey{}, err
	}
	switch value {
	case 'A':
		return choiceKey{kind: choiceKeyUp}, nil
	case 'B':
		return choiceKey{kind: choiceKeyDown}, nil
	case 'H':
		return choiceKey{kind: choiceKeyHome}, nil
	case 'F':
		return choiceKey{kind: choiceKeyEnd}, nil
	case '1', '4', '5', '6', '7', '8':
		if tilde, err := reader.ReadByte(); err != nil {
			return choiceKey{}, err
		} else if tilde != '~' {
			return choiceKey{kind: choiceKeyUnknown}, nil
		}
		switch value {
		case '1', '7':
			return choiceKey{kind: choiceKeyHome}, nil
		case '4', '8':
			return choiceKey{kind: choiceKeyEnd}, nil
		case '5':
			return choiceKey{kind: choiceKeyPageUp}, nil
		case '6':
			return choiceKey{kind: choiceKeyPageDown}, nil
		}
	}
	return choiceKey{kind: choiceKeyUnknown}, nil
}

func renderLiveChoices(writer io.Writer, label string, choices []Choice, query string, matches []int, selected, offset, width int) (int, error) {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	lines := 0
	if err := writeChoiceLine(writer, width, "%s", strings.TrimSpace(label)); err != nil {
		return lines, err
	}
	lines++
	searchText := query
	if searchText == "" {
		searchText = "type to filter"
	}
	if err := writeChoiceLine(writer, width, "Search: %s", searchText); err != nil {
		return lines, err
	}
	lines++
	if err := writeChoiceLine(writer, width, "Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc back | Ctrl+C quit"); err != nil {
		return lines, err
	}
	lines++
	if len(matches) == 0 {
		if err := writeChoiceLine(writer, width, "No matches for %q.", query); err != nil {
			return lines, err
		}
		lines++
		if err := writeChoiceLine(writer, width, "Keep typing, or use Backspace to widen the search."); err != nil {
			return lines, err
		}
		lines++
		return lines, nil
	}
	rowCount := choiceViewportSize
	if remaining := len(matches) - offset; remaining < rowCount {
		rowCount = remaining
	}
	end := offset + rowCount
	if end > len(matches) {
		end = len(matches)
	}
	if err := writeChoiceLine(writer, width, "Matches: %d-%d of %d", offset+1, end, len(matches)); err != nil {
		return lines, err
	}
	lines++
	for _, line := range choiceTableLines(choices, matches, selected, offset, rowCount, width, true) {
		if err := writeChoiceLine(writer, width, "%s", line); err != nil {
			return lines, err
		}
		lines++
	}
	return lines, nil
}

func renderChoiceScreen(writer io.Writer, label string, choices []Choice, query string, matches []int, selected, offset, width, height int, options ScreenOptions) error {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	if width < 60 || height < 16 {
		_, _ = fmt.Fprint(writer, "\x1b[2J\x1b[H")
		_, err := renderLiveChoices(writer, label, choices, query, matches, selected, offset, width)
		return err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = strings.TrimSpace(label)
	}
	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{"Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc back | Ctrl+C quit"}
	}
	footer = zenStableFooterLines()
	bodyHeight := zenBodyHeight(height, footer)
	blockWidth := zenChoiceBlockWidth(width)
	body := zenChoiceBlock(title, choices, query, matches, selected, offset, blockWidth, bodyHeight, options)
	return writeZenScreen(writer, title, options.Header, body, footer, width, height, blockWidth, true)
}

func renderFieldScreen(writer io.Writer, label string, fields []Field, selected, offset int, completed []bool, status string, width, height int, options ScreenOptions) error {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	if width < 60 || height < 16 {
		return renderCompactFieldScreen(writer, label, fields, selected, status, width)
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = strings.TrimSpace(label)
	}
	header := append([]string(nil), options.Header...)
	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{
			"Edit: type/backspace | Next: Enter or Tab | Move: Up/Down | Submit: Ctrl+S | Back: Esc | Quit: Ctrl+C",
			"Boolean: y yes, n no, Space toggle | Multiline: Enter new line, Tab next",
		}
	}
	footer = zenStableFooterLines()
	bodyHeight := zenBodyHeight(height, footer)
	blockWidth := zenChoiceBlockWidth(width)
	body := zenAnchoredFieldBlock(label, fields, selected, offset, completed, status, options.ActionLabel, blockWidth, bodyHeight, options)
	return writeZenScreen(writer, title, header, body, footer, width, height, blockWidth, true)
}

func renderCompactFieldScreen(writer io.Writer, label string, fields []Field, selected int, status string, width int) error {
	if _, err := fmt.Fprint(writer, "\x1b[2J\x1b[H"); err != nil {
		return err
	}
	if err := writeChoiceLine(writer, width, "%s", label); err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		if err := writeChoiceLine(writer, width, "Status: %s", status); err != nil {
			return err
		}
	}
	for index, field := range fields {
		prefix := " "
		if index == selected {
			prefix = ">"
		}
		if err := writeChoiceLine(writer, width, "%s %s: %s", prefix, fieldDisplayLabel(field), fieldSummary(field)); err != nil {
			return err
		}
	}
	return writeChoiceLine(writer, width, "Keys: type edit | Enter next/save | Esc back | Ctrl+C quit")
}

func renderTextScreen(writer io.Writer, label string, content []string, offset, width, height int, options ScreenOptions) error {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	if width < 50 || height < 12 {
		return renderCompactTextScreen(writer, label, content, offset, width, height)
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = strings.TrimSpace(label)
	}
	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter/Esc back | Ctrl+C quit"}
	}
	footer = zenStableFooterLines()
	bodyHeight := zenBodyHeight(height, footer)
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	blockWidth := zenChoiceBlockWidth(width)
	body := zenTextBlock(title, content, offset, blockWidth, bodyHeight, options)
	return writeZenScreen(writer, title, options.Header, body, footer, width, height, blockWidth, true)
}

func renderCompactTextScreen(writer io.Writer, label string, content []string, offset, width, height int) error {
	if _, err := fmt.Fprint(writer, "\x1b[2J\x1b[H"); err != nil {
		return err
	}
	if err := writeChoiceLine(writer, width, "%s", label); err != nil {
		return err
	}
	rowCount := height - 3
	if rowCount < 1 {
		rowCount = 1
	}
	for row := 0; row < rowCount; row++ {
		index := offset + row
		if index >= len(content) {
			break
		}
		if err := writeChoiceLine(writer, width, "%s", content[index]); err != nil {
			return err
		}
	}
	return writeChoiceLine(writer, width, "Keys: scroll | Enter/Esc back | Ctrl+C quit")
}

func zenBodyHeight(height int, footer []string) int {
	bodyHeight := height - 1 - len(footer)
	if bodyHeight < 1 {
		return 1
	}
	return bodyHeight
}

func zenBlockWidth(width int) int {
	blockWidth := width - 12
	if blockWidth > 56 {
		blockWidth = 56
	}
	if blockWidth < 38 {
		blockWidth = width - 4
	}
	if blockWidth < 20 {
		blockWidth = 20
	}
	return blockWidth
}

func zenChoiceBlockWidth(width int) int {
	blockWidth := width - 8
	if blockWidth > 160 {
		blockWidth = 160
	}
	if blockWidth < 56 {
		blockWidth = width - 4
	}
	if blockWidth < 20 {
		blockWidth = 20
	}
	return blockWidth
}

func zenStableFooterLines() []string {
	return []string{
		"Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc quit | Ctrl+C quit",
		"Quick: nopsai api call --interactive | nopsai api request GET /v1/auth/me | nopsai platform release --interactive",
	}
}

func writeZenScreen(writer io.Writer, title string, headers []string, body []string, footer []string, width, height, blockWidth int, center bool) error {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	lines := make([]string, 0, height)
	lines = append(lines, screenFullLine(zenHeaderLine(title, headers), width))
	bodyRows := height - 1 - len(footer)
	if bodyRows < 0 {
		bodyRows = 0
	}
	body = screenClipLines(body, blockWidth, bodyRows)
	topPad := 0
	if center && bodyRows > len(body) {
		topPad = (bodyRows - len(body)) / 2
	}
	for row := 0; row < topPad && len(lines) < height-len(footer); row++ {
		lines = append(lines, "")
	}
	for _, line := range body {
		if len(lines) >= height-len(footer) {
			break
		}
		lines = append(lines, zenBlockLine(line, width, blockWidth))
	}
	for len(lines) < height-len(footer) {
		lines = append(lines, "")
	}
	for _, footerLine := range footer {
		lines = append(lines, screenFullLine(" "+zenFooterLine(footerLine), width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return writeScreen(writer, lines, width)
}

func zenHeaderLine(title string, headers []string) string {
	parts := []string{styleBold("NopsAI CLI"), "Home"}
	for _, header := range headers {
		for _, part := range strings.Split(header, "|") {
			part = strings.TrimSpace(part)
			if zenHeaderPartAllowed(part) {
				parts = append(parts, formatScreenHeaderLine(part))
			}
		}
	}
	_ = title
	return strings.Join(parts, " "+styleDim("|")+" ")
}

func zenHeaderPartAllowed(part string) bool {
	label, _, ok := strings.Cut(strings.TrimSpace(part), ":")
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "version", "context", "api", "user", "token", "health", "warning":
		return true
	default:
		return false
	}
}

func zenFooterLine(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "Keys:"))
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return styleDim(strings.Join(parts, "  |  "))
}

func zenBlockLine(value string, width, blockWidth int) string {
	if blockWidth > width-2 {
		blockWidth = width - 2
	}
	if blockWidth < 1 {
		blockWidth = 1
	}
	indent := (width - blockWidth) / 2
	if indent < 0 {
		indent = 0
	}
	return strings.Repeat(" ", indent) + padScreenCell(value, blockWidth)
}

func zenChoiceBlock(label string, choices []Choice, query string, matches []int, selected, offset, width, height int, options ScreenOptions) []string {
	if height < 6 {
		height = 6
	}
	guide := zenChoiceGuideLines(choices, matches, selected, width, options)
	rowCount, guideRows := zenChoiceLayoutRows(height)
	title := zenChoiceTitle(label, query, len(matches), offset, rowCount, options)
	lines := []string{title, ""}
	if len(matches) == 0 {
		lines = append(lines, "No matches.")
		if rowCount > 1 {
			lines = append(lines, styleDim("Backspace widens the search."))
		}
	} else {
		for row := 0; row < rowCount; row++ {
			matchIndex := offset + row
			if matchIndex >= len(matches) {
				break
			}
			prefix := "  "
			if matchIndex == selected {
				prefix = "> "
			}
			choiceLabel, _ := choiceDisplay(choices[matches[matchIndex]])
			line := prefix + choiceLabel
			if matchIndex == selected {
				line = styleSelected(line)
			}
			lines = append(lines, truncateChoiceLine(line, width+1))
		}
	}
	for len(lines) < rowCount+2 {
		lines = append(lines, "")
	}
	if guideRows > 0 {
		lines = append(lines, "", strings.Repeat("_", minInt(width, 34)))
		lines = append(lines, zenChoiceGuideRows(guide, nil, width, guideRows)...)
	}
	return screenClipLines(lines, width, height)
}

func zenChoiceLayoutRows(height int) (int, int) {
	if height < 1 {
		return 1, 0
	}
	guideRows := 8
	if height < zenChoiceViewportSize+4+guideRows {
		guideRows = height - zenChoiceViewportSize - 4
	}
	if guideRows < 0 {
		guideRows = 0
	}
	rowCount := minInt(zenChoiceViewportSize, height-guideRows-4)
	if height >= zenChoiceViewportSize+4 && rowCount < zenChoiceViewportSize {
		rowCount = zenChoiceViewportSize
		guideRows = height - rowCount - 4
	}
	if rowCount < 1 {
		rowCount = 1
		guideRows = height - rowCount - 4
	}
	if guideRows < 0 {
		guideRows = 0
	}
	if guideRows > 8 {
		guideRows = 8
	}
	return rowCount, guideRows
}

func zenTextBlock(label string, content []string, offset, width, height int, options ScreenOptions) []string {
	if height < 6 {
		height = 6
	}
	rowCount, guideRows := zenTextLayoutRows(height)
	title := zenTextTitle(label, len(content), offset, rowCount, options)
	lines := []string{title, "", "Result"}
	if len(content) == 0 {
		lines = append(lines, "No output.")
	} else {
		for row := 0; row < rowCount; row++ {
			index := offset + row
			if index >= len(content) {
				break
			}
			lines = append(lines, truncateChoiceLine(content[index], width+1))
		}
	}
	for len(lines) < rowCount+3 {
		lines = append(lines, "")
	}
	if guideRows > 0 {
		lines = append(lines, "", strings.Repeat("_", minInt(width, 34)))
		lines = append(lines, fixedScreenRows(zenTextGuideLines(len(content), rowCount, width), width, guideRows)...)
	}
	return screenClipLines(lines, width, height)
}

func zenTextLayoutRows(height int) (int, int) {
	if height < 1 {
		return 1, 0
	}
	guideRows := 8
	if height < zenChoiceViewportSize+5+guideRows {
		guideRows = height - zenChoiceViewportSize - 5
	}
	if guideRows < 0 {
		guideRows = 0
	}
	rowCount := minInt(zenChoiceViewportSize, height-guideRows-5)
	if height >= zenChoiceViewportSize+5 && rowCount < zenChoiceViewportSize {
		rowCount = zenChoiceViewportSize
		guideRows = height - rowCount - 5
	}
	if rowCount < 1 {
		rowCount = 1
		guideRows = height - rowCount - 5
	}
	if guideRows < 0 {
		guideRows = 0
	}
	if guideRows > 8 {
		guideRows = 8
	}
	return rowCount, guideRows
}

func zenTextTitle(label string, total, offset, rowCount int, options ScreenOptions) string {
	title := zenChoiceBreadcrumbTitle(label, options)
	if total <= 0 || rowCount <= 0 || total <= rowCount {
		return title
	}
	end := offset + rowCount
	if end > total {
		end = total
	}
	return fmt.Sprintf("%s (%d-%d of %d)", title, offset+1, end, total)
}

func zenTextGuideLines(total, rowCount, width int) []string {
	if total > rowCount {
		return zenLabeledParagraph("Guide", "Up/Down scroll one line. PgUp/PgDn jump. Enter or Esc returns.", width)
	}
	return zenLabeledParagraph("Guide", "Press Enter or Esc to return.", width)
}

func zenChoiceTitle(label, query string, matchCount, offset, rowCount int, options ScreenOptions) string {
	title := zenChoiceBreadcrumbTitle(label, options)
	if queryText := strings.TrimSpace(query); queryText != "" {
		title += " | Search: " + queryText
	}
	if matchCount <= 0 || rowCount <= 0 {
		return title
	}
	end := offset + rowCount
	if end > matchCount {
		end = matchCount
	}
	if matchCount > rowCount || offset > 0 {
		title += fmt.Sprintf(" (%d-%d of %d)", offset+1, end, matchCount)
	}
	return title
}

func zenChoiceBreadcrumbTitle(label string, options ScreenOptions) string {
	crumbs := make([]string, 0, len(options.Breadcrumb)+2)
	for _, crumb := range options.Breadcrumb {
		crumb = strings.TrimSpace(crumb)
		if crumb != "" {
			crumbs = append(crumbs, crumb)
		}
	}
	if len(crumbs) == 0 {
		title := strings.TrimSpace(label)
		if title == "" {
			title = strings.TrimSpace(options.Title)
		}
		if strings.EqualFold(title, "home") || strings.EqualFold(title, "nopsai home") {
			return "Home"
		}
		if title == "" {
			return "Home"
		}
		crumbs = append(crumbs, "Home", title)
	}
	if len(crumbs) == 1 {
		return crumbs[0]
	}
	return strings.Join(crumbs, " > ") + " >"
}

func zenChoiceGuideLines(choices []Choice, matches []int, selected, width int, options ScreenOptions) []string {
	if len(matches) == 0 {
		return zenLabeledParagraph("Guide", "Type to filter choices, or use Backspace to widen the search.", width)
	}
	choiceIndex := matches[selected]
	choice := choices[choiceIndex]
	rawDetails := []string(nil)
	if options.Detail != nil {
		rawDetails = options.Detail(choiceIndex, choice)
	}
	guide := zenDetailGuide(rawDetails)
	if _, description := choiceDisplay(choice); description != "" {
		if guide == "" {
			guide = description
		}
	}
	if guide == "" {
		guide = "Press Enter to select this item."
	}
	raw := zenLabeledParagraph("Guide", guide, width)
	if examples := zenDetailExamples(rawDetails); len(examples) > 0 {
		if len(raw) > 0 {
			raw = append(raw, "")
		}
		raw = append(raw, zenExampleLines(examples, width)...)
	}
	return raw
}

func zenChoiceGuideRows(guide, parameters []string, width, rows int) []string {
	if rows <= 0 {
		return nil
	}
	if len(parameters) == 0 || width < 72 {
		return fixedScreenRows(guide, width, rows)
	}
	paramWidth := width / 3
	if paramWidth < 24 {
		paramWidth = 24
	}
	if paramWidth > 36 {
		paramWidth = 36
	}
	guideWidth := width - paramWidth - 3
	if guideWidth < 28 {
		return fixedScreenRows(append(guide, append([]string{""}, parameters...)...), width, rows)
	}
	left := fixedScreenRows(guide, guideWidth, rows)
	right := fixedScreenRows(parameters, paramWidth, rows)
	lines := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		lines = append(lines, padScreenCell(left[row], guideWidth)+"   "+right[row])
	}
	return lines
}

func fixedScreenRows(lines []string, width, rows int) []string {
	lines = screenClipLines(wrapScreenLinesPreserve(lines, width), width, rows)
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

func zenAnchoredFieldBlock(label string, fields []Field, selected, offset int, completed []bool, status, actionLabel string, width, height int, options ScreenOptions) []string {
	if len(fields) == 0 {
		return nil
	}
	rowCount, guideRows := zenChoiceLayoutRows(height)
	title := zenChoiceBreadcrumbTitle(label, options)
	lines := []string{title, ""}
	fieldRows := zenFieldMenuRows(fields, selected, offset, completed, width, rowCount)
	lines = append(lines, fieldRows...)
	for len(lines) < rowCount+2 {
		lines = append(lines, "")
	}
	if guideRows > 0 {
		field := fields[selected]
		guide := zenFieldGuideLines(field, status, width)
		lines = append(lines, "", strings.Repeat("_", minInt(width, 34)))
		lines = append(lines, zenChoiceGuideRows(guide, nil, width, guideRows)...)
	}
	_ = options
	_ = actionLabel
	return screenClipLines(lines, width, height)
}

func zenFieldMenuRows(fields []Field, selected, offset int, completed []bool, width, rowCount int) []string {
	if rowCount <= 0 {
		return nil
	}
	rows := make([]string, 0, rowCount)
	rows = append(rows, "Parameters")
	for index := offset; index < len(fields) && len(rows) < rowCount; index++ {
		fieldRows := zenFieldParameterRows(fields[index], index, selected, completed, width)
		for _, row := range fieldRows {
			if len(rows) >= rowCount {
				break
			}
			rows = append(rows, row)
		}
	}
	return fixedScreenRows(rows, width, rowCount)
}

func zenFieldParameterRows(field Field, index, selected int, completed []bool, width int) []string {
	label := zenFieldParameterLabel(field)
	prefix := "○"
	if fieldCompleted(index, completed) {
		prefix = "✓"
	}
	if index == selected {
		prefix = "  >"
	}
	row := fmt.Sprintf("%s %d. %s", prefix, index+1, label)
	if index != selected {
		return []string{truncateChoiceLine(row, width+1)}
	}
	row = zenFieldSelectedParameterRow(row, field, width)
	return []string{
		truncateChoiceLine(styleSelected(row), width+1),
		zenFieldSelectedValueRow(field, width),
	}
}

func zenFieldSelectedParameterRow(row string, field Field, width int) string {
	if !field.Required {
		return row
	}
	required := "Required"
	column := width / 2
	if column < 36 {
		column = 36
	}
	spaces := column - visibleRuneCount(row)
	if spaces < 2 {
		spaces = 2
	}
	return row + strings.Repeat(" ", spaces) + required
}

func zenFieldSelectedValueRow(field Field, width int) string {
	value := zenFieldInlineValue(field)
	prefix := "Value: "
	indent := width / 2
	if indent < 8 {
		indent = 8
	}
	row := strings.Repeat(" ", indent) + prefix + value
	return truncateChoiceLine(row, width+1)
}

func zenFieldInlineValue(field Field) string {
	if strings.TrimSpace(field.Value) == "" && strings.TrimSpace(field.Default) == "" {
		return styleBlink("|")
	}
	if field.Multiline {
		value := field.Value
		if strings.TrimSpace(value) == "" && strings.TrimSpace(field.Default) != "" {
			value = field.Default
		}
		lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
		if len(lines) > 1 {
			return fmt.Sprintf("%d lines", len(lines))
		}
		if len(lines) == 1 && strings.TrimSpace(lines[0]) != "" {
			return strings.TrimSpace(lines[0])
		}
		return styleBlink("|")
	}
	return fieldSummary(field)
}

func zenFieldParameterLabel(field Field) string {
	name := strings.TrimSpace(field.Name)
	switch {
	case strings.HasPrefix(name, "path."):
		return "path: " + strings.TrimPrefix(name, "path.")
	case strings.HasPrefix(name, "query."):
		key := strings.TrimPrefix(name, "query.")
		if key == "extra" {
			return "additional query values"
		}
		return "query: " + key
	case name == "body.file":
		return "payload file"
	case name == "body.raw":
		return "payload editor"
	case name == "contentType":
		return "payload media type"
	case name == "accept":
		return "response format"
	case name == "auth":
		return "attach bearer token"
	case name == "send":
		return "send request"
	default:
		return strings.ToLower(strings.TrimSuffix(fieldDisplayLabel(field), "?"))
	}
}

func zenFieldGuideLines(field Field, status string, width int) []string {
	lines := make([]string, 0)
	if strings.TrimSpace(field.Description) != "" {
		lines = append(lines, zenLabeledParagraph("Guide", strings.TrimSpace(field.Description), width)...)
	}
	if strings.TrimSpace(field.Example) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, zenExampleLines(strings.Split(strings.TrimSpace(field.Example), "\n"), width)...)
	}
	if strings.TrimSpace(status) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, zenLabeledParagraph("Validation", strings.TrimSpace(status), width)...)
	}
	if len(lines) == 0 {
		lines = zenLabeledParagraph("Guide", "Press Enter to continue.", width)
	}
	return lines
}

func zenLabeledParagraph(label, value string, width int) []string {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	indent := "        "
	contentWidth := width - visibleRuneCount(indent)
	if contentWidth < 12 {
		contentWidth = width
		indent = ""
	}
	parts := wrapScreenLines(strings.Split(value, "\n"), contentWidth)
	out := make([]string, 0, len(parts)+1)
	if label != "" {
		out = append(out, styleBold(label+":"))
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+part)
	}
	return out
}

func zenExampleLines(examples []string, width int) []string {
	cleaned := make([]string, 0, len(examples))
	for _, example := range examples {
		example = strings.TrimSpace(example)
		if example != "" {
			cleaned = append(cleaned, example)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	if len(cleaned) == 1 {
		return zenLabeledParagraph("Example", cleaned[0], width)
	}
	out := []string{styleBold("Example:")}
	for _, example := range cleaned {
		out = append(out, zenIndentedContentLines(example, width)...)
	}
	return out
}

func zenIndentedContentLines(value string, width int) []string {
	indent := "        "
	contentWidth := width - visibleRuneCount(indent)
	if contentWidth < 12 {
		contentWidth = width
		indent = ""
	}
	parts := wrapScreenLines(strings.Split(strings.TrimSpace(value), "\n"), contentWidth)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+part)
	}
	return out
}

func zenDetailGuide(lines []string) string {
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if guide, ok := cutGuideValue(trimmed); ok {
			return guide
		}
		if strings.EqualFold(trimmed, "Guide") && index+1 < len(lines) {
			return strings.TrimSpace(lines[index+1])
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.EqualFold(trimmed, "Example") && !strings.HasPrefix(strings.ToLower(trimmed), "example:") {
			return trimmed
		}
	}
	return ""
}

func cutGuideValue(line string) (string, bool) {
	prefix := "guide:"
	if strings.HasPrefix(strings.ToLower(line), prefix) {
		return strings.TrimSpace(line[len(prefix):]), true
	}
	return "", false
}

func zenDetailExamples(lines []string) []string {
	examples := make([]string, 0)
	capturing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if capturing {
				break
			}
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "example:") {
			value := strings.TrimSpace(trimmed[len("example:"):])
			if value != "" {
				examples = append(examples, value)
			}
			capturing = true
			continue
		}
		if strings.EqualFold(trimmed, "Example") || strings.EqualFold(trimmed, "Examples") {
			capturing = true
			continue
		}
		if capturing {
			examples = append(examples, trimmed)
		}
	}
	return examples
}

func screenFullBorder(width int) string {
	if width < 2 {
		return strings.Repeat("-", width)
	}
	return styleDim(strings.Repeat("-", width))
}

func screenFullLine(value string, width int) string {
	inner := width - 1
	if inner < 1 {
		return truncateChoiceLine(value, width+1)
	}
	return " " + padScreenCell(value, inner)
}

func screenPaneLine(left, right string, leftWidth, rightWidth int) string {
	return " " + padScreenCell(left, leftWidth) + " " + styleDim("|") + " " + padScreenCell(right, rightWidth)
}

func screenLeftWidth(width int) int {
	left := width / 3
	if left < 28 {
		left = 28
	}
	if left > 44 {
		left = 44
	}
	if width-left-3 < 28 {
		left = width - 31
	}
	if left < 20 {
		left = 20
	}
	return left
}

func screenLeftWidthForOptions(width int, options ScreenOptions) int {
	if options.LeftWidth > 0 {
		left := options.LeftWidth
		if left < 20 {
			left = 20
		}
		if left > width-32 {
			left = width - 32
		}
		if left < 20 {
			left = 20
		}
		return left
	}
	return screenLeftWidth(width)
}

func screenTitleLine(title string, headers []string) string {
	parts := []string{styleBold("NopsAI CLI") + " " + styleAccent("("+strings.TrimSpace(title)+")")}
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		parts = append(parts, formatScreenHeaderLine(header))
	}
	return strings.Join(parts, " "+styleDim("|")+" ")
}

func fieldCompleted(index int, completed []bool) bool {
	return index >= 0 && index < len(completed) && completed[index]
}

func fieldDisplayLabel(field Field) string {
	if strings.TrimSpace(field.Label) != "" {
		return strings.TrimSpace(field.Label)
	}
	return strings.TrimSpace(field.Name)
}

func fieldSummary(field Field) string {
	value := field.Value
	if strings.TrimSpace(value) == "" && strings.TrimSpace(field.Default) != "" {
		value = field.Default
	}
	if field.Kind == FieldBoolean {
		return formatFieldBool(parseFieldBool(value))
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "(blank)"
	}
	return value
}

func screenSectionLines(sections []screenSection, width int) []string {
	if width < 12 {
		out := make([]string, 0)
		for _, section := range sections {
			out = append(out, styleAccent(strings.ToUpper(strings.TrimSpace(section.Title))))
			out = append(out, section.Lines...)
			out = append(out, "")
		}
		return wrapScreenLines(out, width)
	}
	out := make([]string, 0, len(sections)*4)
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			out = append(out, "")
		}
		title := strings.TrimSpace(section.Title)
		if title == "" {
			title = "Details"
		}
		out = append(out, styleAccent(strings.ToUpper(title)))
		contentWidth := width - 2
		wrapped := wrapScreenLinesPreserve(section.Lines, contentWidth)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, line := range wrapped {
			out = append(out, line)
		}
	}
	return out
}

func firstMissingRequiredField(fields []Field) int {
	for index, field := range fields {
		if field.Required && strings.TrimSpace(field.Value) == "" {
			return index
		}
	}
	return -1
}

func cloneFields(fields []Field) []Field {
	clone := make([]Field, len(fields))
	copy(clone, fields)
	return clone
}

func parseFieldBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func formatFieldBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func screenMenuLines(label string, choices []Choice, query string, matches []int, selected, offset, width, height int) []string {
	searchText := query
	if searchText == "" {
		searchText = "Type to filter..."
	}
	lines := []string{styleDim(searchText), ""}
	if len(matches) == 0 {
		lines = append(lines, "", "No matches.", "Backspace widens the search.")
		return screenClampLines(lines, width, height)
	}
	rowCount := height - len(lines)
	if rowCount < 1 {
		rowCount = 1
	}
	if rowCount > len(matches)-offset {
		rowCount = len(matches) - offset
	}
	for row := 0; row < rowCount; row++ {
		matchIndex := offset + row
		if matchIndex >= len(matches) {
			break
		}
		selector := " "
		if matchIndex == selected {
			selector = ">"
		}
		label, _ := choiceDisplay(choices[matches[matchIndex]])
		line := fmt.Sprintf("%s %-3d %s", selector, matchIndex+1, label)
		if matchIndex == selected {
			line = styleSelected(line)
		}
		lines = append(lines, line)
	}
	if offset+rowCount < len(matches) {
		lines = append(lines, styleDim(fmt.Sprintf("... and %d more", len(matches)-offset-rowCount)))
	}
	return screenClampLines(lines, width, height)
}

func screenDetailLines(choices []Choice, matches []int, selected, width, height int, options ScreenOptions) []string {
	if len(matches) == 0 {
		return screenClampLines([]string{"No selection", "", "Type to filter the left pane."}, width, height)
	}
	choiceIndex := matches[selected]
	choice := choices[choiceIndex]
	title := choiceDisplayTitle(choice)
	if options.DetailTitle != nil {
		if custom := strings.TrimSpace(options.DetailTitle(choiceIndex, choice)); custom != "" {
			title = custom
		}
	}
	selection := []string{styleBold(title)}
	if _, description := choiceDisplay(choice); description != "" {
		selection = append(selection, description)
	}
	sections := []screenSection{{Title: "Selection", Lines: selection}}
	if options.Detail != nil {
		if custom := options.Detail(choiceIndex, choice); len(custom) > 0 {
			sections = append(sections, screenSection{Title: "Details", Lines: custom})
		}
	}
	return screenClampLines(screenSectionLines(sections, width), width, height)
}

func choiceDisplayTitle(choice Choice) string {
	label, _ := choiceDisplay(choice)
	return label
}

func wrapScreenLines(lines []string, width int) []string {
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			wrapped = append(wrapped, "")
			continue
		}
		current := ""
		for _, word := range words {
			if runeCount(word) > width {
				if current != "" {
					wrapped = append(wrapped, current)
					current = ""
				}
				wrapped = append(wrapped, splitLongScreenWord(word, width)...)
				continue
			}
			if current == "" {
				current = word
				continue
			}
			if runeCount(current)+1+runeCount(word) > width {
				wrapped = append(wrapped, current)
				current = word
				continue
			}
			current += " " + word
		}
		if current != "" {
			wrapped = append(wrapped, current)
		}
	}
	return wrapped
}

func wrapScreenLinesPreserve(lines []string, width int) []string {
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		prefixWidth := 0
		for prefixWidth < len(line) && line[prefixWidth] == ' ' {
			prefixWidth++
		}
		prefix := line[:prefixWidth]
		content := strings.TrimLeft(line, " ")
		contentWidth := width - visibleRuneCount(prefix)
		if contentWidth < 8 {
			contentWidth = width
			prefix = ""
		}
		for index, part := range wrapScreenLines([]string{content}, contentWidth) {
			if index == 0 {
				wrapped = append(wrapped, prefix+part)
				continue
			}
			wrapped = append(wrapped, prefix+part)
		}
	}
	return wrapped
}

func splitLongScreenWord(word string, width int) []string {
	if width <= 0 {
		return []string{word}
	}
	runes := []rune(word)
	parts := make([]string, 0, len(runes)/width+1)
	for len(runes) > width {
		parts = append(parts, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}
	return parts
}

func screenClampLines(lines []string, width, height int) []string {
	out := make([]string, 0, height)
	for _, line := range lines {
		if len(out) >= height {
			break
		}
		out = append(out, truncateChoiceLine(line, width+1))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func screenClipLines(lines []string, width, height int) []string {
	out := make([]string, 0, height)
	for _, line := range lines {
		if len(out) >= height {
			break
		}
		out = append(out, truncateChoiceLine(line, width+1))
	}
	return out
}

func padScreenCell(value string, width int) string {
	value = truncateChoiceLine(value, width+1)
	padding := width - visibleRuneCount(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func writeScreen(writer io.Writer, lines []string, width int) error {
	if _, err := fmt.Fprint(writer, "\x1b[2J\x1b[H"); err != nil {
		return err
	}
	for index, line := range lines {
		line = truncateChoiceLine(line, width+1)
		if index+1 == len(lines) {
			if _, err := fmt.Fprint(writer, line); err != nil {
				return err
			}
			break
		}
		if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
			return err
		}
	}
	return nil
}

func writeChoiceLine(writer io.Writer, width int, format string, args ...any) error {
	line := fmt.Sprintf(format, args...)
	line = truncateChoiceLine(line, width)
	_, err := fmt.Fprintf(writer, "%s\r\n", line)
	return err
}

func truncateChoiceLine(line string, width int) string {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	limit := width - 1
	if limit < 1 {
		limit = 1
	}
	if visibleRuneCount(line) <= limit {
		return line
	}
	if limit <= 3 {
		return strings.Repeat(".", limit)
	}
	return truncateVisible(line, limit-3) + "..."
}

func styleBold(value string) string {
	return styleANSI("1", value)
}

func styleDim(value string) string {
	return styleANSI("2", value)
}

func styleAccent(value string) string {
	return styleANSI("1;36", value)
}

func styleSelected(value string) string {
	return styleBold(value)
}

func styleWarning(value string) string {
	return styleANSI("1;33", value)
}

func styleOK(value string) string {
	return styleANSI("1;32", value)
}

func styleError(value string) string {
	return styleANSI("1;31", value)
}

func styleBlink(value string) string {
	return styleANSI("5", value)
}

func formatScreenFooterLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return formatScreenLabels(value)
}

func formatScreenHeaderLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return formatScreenLabels(value)
}

func formatScreenLabels(value string) string {
	parts := strings.Split(value, " | ")
	if len(parts) > 1 {
		for index, part := range parts {
			if label, rest, ok := strings.Cut(part, ":"); ok && len(label) <= 18 {
				parts[index] = styleBold(label+":") + rest
			}
		}
		return strings.Join(parts, " "+styleDim("|")+" ")
	}
	if label, rest, ok := strings.Cut(value, ":"); ok && len(label) <= 18 {
		return styleBold(label+":") + rest
	}
	return value
}

func styleANSI(code, value string) string {
	if value == "" {
		return ""
	}
	return "\x1b[" + code + "m" + value + ansiReset
}

func visibleRuneCount(value string) int {
	count := 0
	for index := 0; index < len(value); {
		if value[index] == 0x1b {
			next := skipANSI(value, index)
			if next > index {
				index = next
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(value[index:])
		count++
		index += size
	}
	return count
}

func truncateVisible(value string, visibleLimit int) string {
	if visibleLimit <= 0 {
		return ""
	}
	var builder strings.Builder
	visible := 0
	styled := false
	for index := 0; index < len(value) && visible < visibleLimit; {
		if value[index] == 0x1b {
			next := skipANSI(value, index)
			if next > index {
				builder.WriteString(value[index:next])
				styled = true
				index = next
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(value[index:])
		builder.WriteString(value[index : index+size])
		visible++
		index += size
	}
	if styled {
		builder.WriteString(ansiReset)
	}
	return builder.String()
}

func skipANSI(value string, index int) int {
	if index+1 >= len(value) || value[index] != 0x1b || value[index+1] != '[' {
		return index
	}
	for cursor := index + 2; cursor < len(value); cursor++ {
		if value[cursor] >= 0x40 && value[cursor] <= 0x7e {
			return cursor + 1
		}
	}
	return index
}

func clearRenderedChoices(writer io.Writer, lines int) error {
	if lines <= 0 {
		return nil
	}
	if _, err := fmt.Fprintf(writer, "\x1b[%dA", lines); err != nil {
		return err
	}
	for line := 0; line < lines; line++ {
		if _, err := fmt.Fprint(writer, "\r\x1b[2K"); err != nil {
			return err
		}
		if line+1 < lines {
			if _, err := fmt.Fprint(writer, "\x1b[1B"); err != nil {
				return err
			}
		}
	}
	if lines > 1 {
		if _, err := fmt.Fprintf(writer, "\x1b[%dA", lines-1); err != nil {
			return err
		}
	}
	return nil
}

func ensureChoiceOffset(selected, offset, total int) int {
	return ensureChoiceOffsetRows(selected, offset, total, choiceViewportSize)
}

func ensureChoiceOffsetRows(selected, offset, total, rowCount int) int {
	if rowCount <= 0 {
		rowCount = choiceViewportSize
	}
	if total <= 0 || total <= rowCount {
		return 0
	}
	if selected < offset {
		offset = selected
	}
	if selected >= offset+rowCount {
		offset = selected - rowCount + 1
	}
	maxOffset := total - rowCount
	if offset > maxOffset {
		return maxOffset
	}
	if offset < 0 {
		return 0
	}
	return offset
}

func liveChoiceViewportSize(height int, options ScreenOptions) int {
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	rows, _ := zenChoiceLayoutRows(zenBodyHeight(height, zenStableFooterLines()))
	if rows < 3 {
		return 3
	}
	_ = options
	return rows
}

func liveFieldViewportSize(height int, options ScreenOptions) int {
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	rows, _ := zenChoiceLayoutRows(zenBodyHeight(height, zenStableFooterLines()))
	if rows < 3 {
		return 3
	}
	_ = options
	return rows
}

func liveTextViewportSize(height int, options ScreenOptions) int {
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	rows, _ := zenTextLayoutRows(zenBodyHeight(height, zenStableFooterLines()))
	if rows < 3 {
		return 3
	}
	_ = options
	return rows
}

func textPanelWidth(width int) int {
	panel := width - 4
	if panel < 1 {
		return 1
	}
	return panel
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func matchChoices(choices []Choice, query string) []int {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	matches := make([]int, 0, len(choices))
	for index, choice := range choices {
		haystack := strings.ToLower(strings.TrimSpace(choice.Label + " " + choice.SearchText))
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, index)
		}
	}
	return matches
}
