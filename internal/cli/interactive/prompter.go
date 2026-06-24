package interactive

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

const choiceViewportSize = 10
const fallbackChoiceTerminalWidth = 120

var (
	isTerminal      = term.IsTerminal
	makeRawTerminal = term.MakeRaw
	restoreTerminal = term.Restore
	terminalSize    = term.GetSize
)

type Choice struct {
	Label       string
	Description string
	SearchText  string
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
		return p.chooseLive(label, choices)
	}
	return p.chooseLine(label, choices)
}

func (p *Prompter) canUseLiveSelector() bool {
	return p.inFile != nil &&
		p.outFile != nil &&
		isTerminal(int(p.inFile.Fd())) &&
		isTerminal(int(p.outFile.Fd()))
}

func (p *Prompter) chooseLine(label string, choices []Choice) (int, error) {
	for {
		query, err := p.Ask(label+" search", "")
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
		for index := 0; index < limit; index++ {
			_, _ = fmt.Fprintf(p.out, "%2d) %s\n", index+1, choiceText(choices[matches[index]]))
		}
		if len(matches) > limit {
			_, _ = fmt.Fprintf(p.out, "Showing %d of %d matches; enter s to search again.\n", limit, len(matches))
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

func (p *Prompter) chooseLive(label string, choices []Choice) (int, error) {
	oldState, err := makeRawTerminal(int(p.inFile.Fd()))
	if err != nil {
		return -1, err
	}
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = restoreTerminal(int(p.inFile.Fd()), oldState)
		_, _ = fmt.Fprint(p.out, "\x1b[?25h")
	}
	defer cleanup()

	_, _ = fmt.Fprint(p.out, "\x1b[?25l")
	width := fallbackChoiceTerminalWidth
	if detectedWidth, _, err := terminalSize(int(p.outFile.Fd())); err == nil && detectedWidth > 0 {
		width = detectedWidth
	}
	selectedChoice, err := runLiveChoiceSelector(p.reader, p.out, label, choices, width)
	if err != nil {
		return -1, err
	}
	text := choiceText(choices[selectedChoice])
	cleanup()
	_, _ = fmt.Fprintf(p.out, "%s: %s\n", strings.TrimSpace(label), text)
	return selectedChoice, nil
}

func runLiveChoiceSelector(reader *bufio.Reader, out io.Writer, label string, choices []Choice, width int) (int, error) {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	query := ""
	matches := matchChoices(choices, query)
	selected := 0
	offset := 0
	renderedLines := 0
	for {
		offset = ensureChoiceOffset(selected, offset, len(matches))
		if renderedLines > 0 {
			if err := clearRenderedChoices(out, renderedLines); err != nil {
				return -1, err
			}
		}
		lines, err := renderLiveChoices(out, label, choices, query, matches, selected, offset, width)
		if err != nil {
			return -1, err
		}
		renderedLines = lines
		key, err := readChoiceKey(reader)
		if err != nil {
			return -1, err
		}
		switch key.kind {
		case choiceKeyCancel:
			return -1, errors.New("selection cancelled")
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
			selected -= choiceViewportSize
			if selected < 0 {
				selected = 0
			}
		case choiceKeyPageDown:
			if len(matches) > 0 {
				selected += choiceViewportSize
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
	choiceKeyBackspace
	choiceKeyUp
	choiceKeyDown
	choiceKeyPageUp
	choiceKeyPageDown
	choiceKeyHome
	choiceKeyEnd
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
	case 3:
		return choiceKey{kind: choiceKeyCancel}, nil
	case 8, 127:
		return choiceKey{kind: choiceKeyBackspace}, nil
	case 27:
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
		return choiceKey{kind: choiceKeyCancel}, nil
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
	if err := writeChoiceLine(writer, width, "Search: %s", query); err != nil {
		return lines, err
	}
	lines++
	if err := writeChoiceLine(writer, width, "Keys: Up/Down move | PgUp/PgDn jump | Enter select | Ctrl+C cancel"); err != nil {
		return lines, err
	}
	lines++
	if len(matches) == 0 {
		if err := writeChoiceLine(writer, width, "No matches. Keep typing or backspace to widen the search."); err != nil {
			return lines, err
		}
		lines++
		for row := 0; row < choiceViewportSize; row++ {
			if err := writeChoiceLine(writer, width, ""); err != nil {
				return lines, err
			}
			lines++
		}
		return lines, nil
	}
	end := offset + choiceViewportSize
	if end > len(matches) {
		end = len(matches)
	}
	if err := writeChoiceLine(writer, width, "Showing %d-%d of %d matches", offset+1, end, len(matches)); err != nil {
		return lines, err
	}
	lines++
	if err := writeChoiceLine(writer, width, "Sel  #   Option"); err != nil {
		return lines, err
	}
	lines++
	if err := writeChoiceLine(writer, width, "---  --  ------"); err != nil {
		return lines, err
	}
	lines++
	for row := 0; row < choiceViewportSize; row++ {
		index := offset + row
		if index >= end {
			if err := writeChoiceLine(writer, width, ""); err != nil {
				return lines, err
			}
			lines++
			continue
		}
		prefix := " "
		if index == selected {
			prefix = ">"
		}
		if err := writeChoiceLine(writer, width, "%s %2d  %s", prefix, index+1, choiceText(choices[matches[index]])); err != nil {
			return lines, err
		}
		lines++
	}
	return lines, nil
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
	runes := []rune(line)
	if len(runes) <= limit {
		return line
	}
	if limit <= 3 {
		return strings.Repeat(".", limit)
	}
	return string(runes[:limit-3]) + "..."
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
	if total <= choiceViewportSize {
		return 0
	}
	if selected < offset {
		offset = selected
	}
	if selected >= offset+choiceViewportSize {
		offset = selected - choiceViewportSize + 1
	}
	maxOffset := total - choiceViewportSize
	if offset > maxOffset {
		return maxOffset
	}
	if offset < 0 {
		return 0
	}
	return offset
}

func choiceText(choice Choice) string {
	description := strings.TrimSpace(choice.Description)
	if description == "" {
		return choice.Label
	}
	return choice.Label + " - " + description
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
