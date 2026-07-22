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
	lines := make([]string, 0, height)
	lines = append(lines, screenFullBorder(width))
	lines = append(lines, screenFullLine(" "+styleAccent("NopsAI")+" "+styleDim("|")+" "+styleBold(title), width))
	for _, header := range options.Header {
		if strings.TrimSpace(header) != "" && len(lines) < 5 {
			lines = append(lines, screenFullLine(" "+formatScreenHeaderLine(header), width))
		}
	}

	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{"Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc back | Ctrl+C quit"}
	}
	leftWidth := screenLeftWidthForOptions(width, options)
	rightWidth := width - leftWidth - 7
	if rightWidth < 20 {
		rightWidth = 20
	}
	bodyHeight := height - len(lines) - len(footer) - 5
	if bodyHeight < 4 {
		bodyHeight = 4
	}
	menuTitle := strings.TrimSpace(options.LeftTitle)
	if menuTitle == "" {
		menuTitle = strings.TrimSpace(label)
	}
	detailTitle := strings.TrimSpace(options.RightTitle)
	if detailTitle == "" {
		detailTitle = "Details"
	}
	menuLines := screenMenuLines(menuTitle, choices, query, matches, selected, offset, leftWidth, bodyHeight-2)
	detailLines := screenDetailLines(choices, matches, selected, rightWidth, bodyHeight-2, options)
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	lines = append(lines, screenPaneLine(styleBold(strings.ToUpper(menuTitle)), styleBold(strings.ToUpper(detailTitle)), leftWidth, rightWidth))
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	for row := 0; row < bodyHeight; row++ {
		left := ""
		if row < len(menuLines) {
			left = menuLines[row]
		}
		right := ""
		if row < len(detailLines) {
			right = detailLines[row]
		}
		lines = append(lines, screenPaneLine(left, right, leftWidth, rightWidth))
	}
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	for _, footerLine := range footer {
		lines = append(lines, screenFullLine(" "+formatScreenFooterLine(footerLine), width))
	}
	lines = append(lines, screenFullBorder(width))
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return writeScreen(writer, lines, width)
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
	lines := make([]string, 0, height)
	lines = append(lines, screenFullBorder(width))
	lines = append(lines, screenFullLine(" "+styleAccent("NopsAI")+" "+styleDim("|")+" "+styleBold(title), width))
	for _, header := range options.Header {
		if strings.TrimSpace(header) != "" && len(lines) < 5 {
			lines = append(lines, screenFullLine(" "+formatScreenHeaderLine(header), width))
		}
	}
	if progress := fieldProgressHeaderLine(fields, selected, completed); progress != "" && len(lines) < 6 {
		lines = append(lines, screenFullLine(" "+formatScreenHeaderLine(progress), width))
	}
	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{
			"Edit: type/backspace | Next: Enter or Tab | Move: Up/Down | Submit: Ctrl+S | Back: Esc | Quit: Ctrl+C",
			"Boolean: y yes, n no, Space toggle | Multiline: Enter new line, Tab next",
		}
	}
	leftWidth := screenLeftWidthForOptions(width, options)
	rightWidth := width - leftWidth - 7
	if rightWidth < 20 {
		rightWidth = 20
	}
	bodyHeight := height - len(lines) - len(footer) - 5
	if bodyHeight < 4 {
		bodyHeight = 4
	}
	leftTitle := strings.TrimSpace(options.LeftTitle)
	if leftTitle == "" {
		leftTitle = "Steps"
	}
	rightTitle := strings.TrimSpace(options.RightTitle)
	if rightTitle == "" {
		rightTitle = "Values & Details"
	}
	leftLines := fieldSidebarLines(label, fields, selected, offset, completed, options.Sidebar, options.ActionLabel, leftWidth, bodyHeight)
	rightLines := fieldDetailPanelLines(fields, selected, completed, status, options.ActionLabel, rightWidth, bodyHeight)
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	lines = append(lines, screenPaneLine(styleBold(strings.ToUpper(leftTitle)), styleBold(strings.ToUpper(rightTitle)), leftWidth, rightWidth))
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	for row := 0; row < bodyHeight; row++ {
		left := ""
		if row < len(leftLines) {
			left = leftLines[row]
		}
		right := ""
		if row < len(rightLines) {
			right = rightLines[row]
		}
		lines = append(lines, screenPaneLine(left, right, leftWidth, rightWidth))
	}
	lines = append(lines, screenPaneBorder(width, leftWidth, rightWidth))
	for _, footerLine := range footer {
		lines = append(lines, screenFullLine(" "+formatScreenFooterLine(footerLine), width))
	}
	lines = append(lines, screenFullBorder(width))
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return writeScreen(writer, lines, width)
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
	lines := make([]string, 0, height)
	lines = append(lines, screenFullBorder(width))
	lines = append(lines, screenFullLine(" "+styleAccent("NopsAI")+" "+styleDim("|")+" "+styleBold(title), width))
	for _, header := range options.Header {
		if strings.TrimSpace(header) != "" && len(lines) < 5 {
			lines = append(lines, screenFullLine(" "+formatScreenHeaderLine(header), width))
		}
	}
	footer := options.Footer
	if len(footer) == 0 {
		footer = []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter/Esc back | Ctrl+C quit"}
	}
	bodyHeight := height - len(lines) - len(footer) - 3
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	panelWidth := textPanelWidth(width)
	end := offset + bodyHeight
	if end > len(content) {
		end = len(content)
	}
	status := fmt.Sprintf("Lines %d-%d of %d", minInt(offset+1, len(content)), end, len(content))
	if len(content) == 0 {
		status = "No output"
	}
	lines = append(lines, screenFullBorder(width))
	lines = append(lines, screenFullLine(" "+status, width))
	lines = append(lines, screenFullBorder(width))
	for row := 0; row < bodyHeight; row++ {
		text := ""
		index := offset + row
		if index < len(content) {
			text = content[index]
		}
		lines = append(lines, screenFullLine(" "+padScreenCell(text, panelWidth), width))
	}
	lines = append(lines, screenFullBorder(width))
	for _, footerLine := range footer {
		lines = append(lines, screenFullLine(" "+formatScreenFooterLine(footerLine), width))
	}
	lines = append(lines, screenFullBorder(width))
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return writeScreen(writer, lines, width)
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

func screenFullBorder(width int) string {
	if width < 2 {
		return strings.Repeat("-", width)
	}
	return styleDim("+" + strings.Repeat("-", width-2) + "+")
}

func screenFullLine(value string, width int) string {
	inner := width - 2
	if inner < 1 {
		return truncateChoiceLine(value, width+1)
	}
	return styleDim("|") + padScreenCell(value, inner) + styleDim("|")
}

func screenPaneBorder(width, leftWidth, rightWidth int) string {
	total := leftWidth + rightWidth + 7
	if total != width {
		rightWidth += width - total
		if rightWidth < 1 {
			rightWidth = 1
		}
	}
	return styleDim("+" + strings.Repeat("-", leftWidth+2) + "+" + strings.Repeat("-", rightWidth+2) + "+")
}

func screenPaneLine(left, right string, leftWidth, rightWidth int) string {
	return styleDim("|") + " " + padScreenCell(left, leftWidth) + " " + styleDim("|") + " " + padScreenCell(right, rightWidth) + " " + styleDim("|")
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

func fieldSidebarLines(label string, fields []Field, selected, offset int, completed []bool, sidebar []string, actionLabel string, width, height int) []string {
	lines := make([]string, 0, height)
	if len(sidebar) > 0 {
		lines = append(lines, sidebar...)
	} else {
		lines = append(lines,
			styleBold("Location"),
			strings.TrimSpace(label),
		)
	}
	done, needsInput := fieldProgress(fields, completed)
	stepRows := fieldSidebarStepRows(len(fields), len(lines), height)
	stepOffset := ensureChoiceOffsetRows(selected, offset, len(fields), stepRows)
	lines = append(lines,
		"",
		styleBold("Progress"),
		fmt.Sprintf("Step: %d of %d", selected+1, len(fields)),
		"Current: "+fieldDisplayLabel(fields[selected]),
		fmt.Sprintf("Done: %d of %d", done, len(fields)),
		fmt.Sprintf("Needs input: %d", needsInput),
		"",
		styleBold("Steps"),
	)
	lines = append(lines, fieldStepLines(fields, selected, stepOffset, completed, width-2, stepRows)...)
	lines = append(lines,
		"",
		styleBold("Final action"),
		fieldFinalActionLine(actionLabel, fields),
		"",
		styleBold("Navigation"),
		"Esc: back",
		"Ctrl+C: quit",
		"Up/Down: step",
		"Tab: next",
		"Ctrl+S: send/save",
	)
	if fields[selected].Multiline {
		lines = append(lines, "", styleBold("Multiline"), "Enter: new line", "Tab: next step")
	}
	for index := range lines {
		lines[index] = formatSidebarLine(lines[index])
	}
	return screenClampLines(wrapScreenLines(lines, width), width, height)
}

func fieldSidebarStepRows(total, existingLines, height int) int {
	if total <= 0 {
		return 0
	}
	reserved := existingLines + 16
	rows := height - reserved
	if rows < 4 {
		rows = 4
	}
	if rows > 12 {
		rows = 12
	}
	if rows > total {
		rows = total
	}
	return rows
}

func fieldProgressHeaderLine(fields []Field, selected int, completed []bool) string {
	if len(fields) == 0 || selected < 0 || selected >= len(fields) {
		return ""
	}
	done, needsInput := fieldProgress(fields, completed)
	return fmt.Sprintf(
		"Step: %d/%d | Current: %s | Done: %d | Needs input: %d",
		selected+1,
		len(fields),
		fieldDisplayLabel(fields[selected]),
		done,
		needsInput,
	)
}

func formatSidebarLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	if strings.Contains(value, "\x1b[") {
		return value
	}
	if strings.Contains(trimmed, ":") {
		return formatScreenLabels(value)
	}
	if visibleRuneCount(trimmed) <= 20 && !strings.Contains(trimmed, ">") {
		return styleBold(value)
	}
	return value
}

func fieldDetailPanelLines(fields []Field, selected int, completed []bool, status, actionLabel string, width, height int) []string {
	sections := []screenSection{
		{Title: fieldInputSectionTitle(fields[selected]), Lines: fieldInputLines(fields[selected])},
		{Title: "Step Details", Lines: fieldCurrentStepLines(fields[selected], selected, len(fields), completed)},
	}
	if strings.TrimSpace(fields[selected].Description) != "" {
		sections = append(sections, screenSection{Title: "Guidance", Lines: []string{strings.TrimSpace(fields[selected].Description)}})
	}
	if strings.TrimSpace(fields[selected].Example) != "" {
		sections = append(sections, screenSection{Title: "Example", Lines: strings.Split(strings.TrimSpace(fields[selected].Example), "\n")})
	}
	if strings.TrimSpace(status) != "" {
		sections = append(sections, screenSection{Title: "Validation", Lines: []string{styleWarning(strings.TrimSpace(status))}})
	}
	if selected+1 == len(fields) {
		sections = append(sections, screenSection{Title: "Submit", Lines: []string{fieldFinalActionLine(actionLabel, fields)}})
	}
	return screenClampLines(screenSectionLines(sections, width), width, height)
}

func fieldStepLines(fields []Field, selected, offset int, completed []bool, width, rowCount int) []string {
	if len(fields) == 0 {
		return nil
	}
	if rowCount <= 0 || rowCount > len(fields) {
		rowCount = len(fields)
	}
	offset = ensureChoiceOffsetRows(selected, offset, len(fields), rowCount)
	end := offset + rowCount
	if end > len(fields) {
		end = len(fields)
	}
	lines := make([]string, 0, rowCount+1)
	if offset > 0 || end < len(fields) {
		lines = append(lines, styleDim(fmt.Sprintf("Showing steps %d-%d of %d", offset+1, end, len(fields))))
	}
	for index := offset; index < end; index++ {
		field := fields[index]
		status := fieldStatusLabel(field, index, selected, completed)
		line := fmt.Sprintf("%2d. %-30s %s", index+1, fieldDisplayLabel(field), status)
		if index == selected {
			line = styleSelected(line)
		} else if fieldCompleted(index, completed) {
			line = styleOK(line)
		}
		lines = append(lines, truncateChoiceLine(line, width+1))
	}
	return lines
}

func fieldFinalActionLine(actionLabel string, fields []Field) string {
	if strings.TrimSpace(actionLabel) != "" {
		return strings.TrimSpace(actionLabel) + " with Enter or Ctrl+S"
	}
	if len(fields) == 0 {
		return "Submit"
	}
	last := fieldDisplayLabel(fields[len(fields)-1])
	if strings.TrimSpace(last) == "" {
		return "Submit"
	}
	return last + " with Enter or Ctrl+S"
}

func fieldCurrentStepLines(field Field, index, total int, completed []bool) []string {
	lines := []string{
		styleBold(fieldDisplayLabel(field)),
		fmt.Sprintf("Step: %d of %d | Status: %s", index+1, total, fieldStatusLabel(field, index, index, completed)),
		"Value: " + fieldSummary(field),
	}
	required := "Required: no"
	if field.Required {
		required = "Required: " + styleWarning("yes")
	}
	if field.Multiline {
		required += " | Input mode: multiline editor"
	}
	lines = append(lines, required)
	if strings.TrimSpace(field.Default) != "" {
		lines = append(lines, "Default: "+strings.TrimSpace(field.Default))
	}
	return lines
}

func fieldInputSectionTitle(field Field) string {
	if field.Multiline {
		return "Multiline Editor"
	}
	return "Input"
}

func fieldInputLines(field Field) []string {
	value := field.Value
	if strings.TrimSpace(value) == "" && strings.TrimSpace(field.Default) != "" {
		value = field.Default
	}
	if field.Kind == FieldBoolean {
		return []string{formatFieldBool(parseFieldBool(value))}
	}
	if strings.TrimSpace(value) == "" {
		if field.Multiline {
			return []string{styleDim("(blank multiline editor)")}
		}
		return []string{styleDim("(blank)")}
	}
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) == 0 {
		return []string{styleDim("(blank)")}
	}
	if field.Multiline {
		for index := range lines {
			lines[index] = fmt.Sprintf("%3d | %s", index+1, lines[index])
		}
	}
	return lines
}

func fieldProgress(fields []Field, completed []bool) (int, int) {
	done := 0
	needsInput := 0
	for index, field := range fields {
		if fieldCompleted(index, completed) {
			done++
			continue
		}
		if field.Required && strings.TrimSpace(field.Value) == "" {
			needsInput++
		}
	}
	return done, needsInput
}

func fieldStatusLabel(field Field, index, selected int, completed []bool) string {
	switch {
	case index == selected:
		return styleAccent("current step")
	case fieldCompleted(index, completed) && strings.TrimSpace(field.Value) != "":
		return styleOK("done")
	case fieldCompleted(index, completed):
		return styleDim("skipped")
	case strings.TrimSpace(field.Value) != "":
		return styleDim("prefilled")
	case field.Required:
		return styleWarning("needs input")
	default:
		return styleDim("optional")
	}
}

func fieldCompleted(index int, completed []bool) bool {
	return index >= 0 && index < len(completed) && completed[index]
}

func fieldDetailLines(field Field, index, total int, status string, width, height int) []string {
	metadata := []string{styleBold(fieldDisplayLabel(field))}
	if total > 0 {
		metadata = append(metadata, fmt.Sprintf("Step: %d of %d", index+1, total))
	}
	if field.Required {
		metadata = append(metadata, "Required: "+styleWarning("yes"))
	} else {
		metadata = append(metadata, "Required: no")
	}
	metadata = append(metadata, "Current: "+fieldSummary(field))
	if strings.TrimSpace(field.Default) != "" {
		metadata = append(metadata, "Default: "+strings.TrimSpace(field.Default))
	}
	sections := []screenSection{{Title: "Current Step", Lines: metadata}}
	if strings.TrimSpace(field.Description) != "" {
		sections = append(sections, screenSection{Title: "Guidance", Lines: []string{strings.TrimSpace(field.Description)}})
	}
	if strings.TrimSpace(field.Example) != "" {
		sections = append(sections, screenSection{Title: "Example", Lines: strings.Split(strings.TrimSpace(field.Example), "\n")})
	}
	if strings.TrimSpace(status) != "" {
		sections = append(sections, screenSection{Title: "Status", Lines: []string{styleWarning(strings.TrimSpace(status))}})
	}
	return screenClampLines(screenSectionLines(sections, width), width, height)
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
			out = append(out, strings.TrimSpace(section.Title))
			out = append(out, section.Lines...)
			out = append(out, "")
		}
		return wrapScreenLines(out, width)
	}
	out := make([]string, 0, len(sections)*5)
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			out = append(out, "")
		}
		title := strings.TrimSpace(section.Title)
		if title == "" {
			title = "Details"
		}
		border := "+" + strings.Repeat("-", width-2) + "+"
		out = append(out, styleDim(border))
		out = append(out, styleDim("| ")+padScreenCell(styleBold(title), width-4)+styleDim(" |"))
		out = append(out, styleDim(border))
		contentWidth := width - 4
		wrapped := wrapScreenLines(section.Lines, contentWidth)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, line := range wrapped {
			out = append(out, styleDim("| ")+padScreenCell(line, contentWidth)+styleDim(" |"))
		}
		out = append(out, styleDim(border))
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
		searchText = "type to filter"
	}
	lines := []string{"Filter: " + searchText}
	if len(matches) == 0 {
		lines = append(lines, "", "No matches.", "Backspace widens the search.")
		return screenClampLines(lines, width, height)
	}
	rowCount := height - len(lines) - 1
	if rowCount < 1 {
		rowCount = 1
	}
	if rowCount > len(matches)-offset {
		rowCount = len(matches) - offset
	}
	end := offset + rowCount
	lines = append(lines, fmt.Sprintf("Matches: %d-%d of %d", offset+1, end, len(matches)))
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
		line := fmt.Sprintf("%s %3d  %s", selector, matchIndex+1, label)
		if matchIndex == selected {
			line = styleSelected(line)
		}
		lines = append(lines, line)
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
	return styleANSI("7;1", value)
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
	header := 2
	if count := len(options.Header); count > 0 {
		if count > 3 {
			count = 3
		}
		header += count
	}
	footer := len(options.Footer)
	if len(options.Footer) == 0 {
		footer = 1
	}
	rows := height - header - footer - 9
	if rows < 3 {
		return 3
	}
	return rows
}

func liveFieldViewportSize(height int, options ScreenOptions) int {
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	header := 2
	if count := len(options.Header); count > 0 {
		if count > 3 {
			count = 3
		}
		header += count
	}
	footer := len(options.Footer)
	if footer == 0 {
		footer = 2
	}
	rows := height - header - footer - 5
	if rows < 3 {
		return 3
	}
	return rows
}

func liveTextViewportSize(height int, options ScreenOptions) int {
	if height <= 0 {
		height = fallbackChoiceTerminalHeight
	}
	header := 2
	if count := len(options.Header); count > 0 {
		if count > 3 {
			count = 3
		}
		header += count
	}
	footer := len(options.Footer)
	if footer == 0 {
		footer = 1
	}
	rows := height - header - footer - 4
	if rows < 3 {
		return 3
	}
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
