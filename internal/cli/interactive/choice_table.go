package interactive

import (
	"fmt"
	"strconv"
	"strings"
)

const minChoiceLabelColumnWidth = 14
const minChoiceDetailColumnWidth = 16

func choiceTableLines(choices []Choice, matches []int, selected, offset, rowCount, width int, includeSelector bool) []string {
	if width <= 0 {
		width = fallbackChoiceTerminalWidth
	}
	if rowCount < 0 {
		rowCount = 0
	}
	end := offset + rowCount
	if end > len(matches) {
		end = len(matches)
	}
	numberWidth := choiceNumberColumnWidth(len(matches))
	showDetails := choiceTableShowsDetails(choices, matches, offset, end)
	labelWidth := choiceLabelColumnWidth(choices, matches, offset, end, width, numberWidth, showDetails, includeSelector)
	lines := []string{
		choiceTableHeader(numberWidth, labelWidth, showDetails, includeSelector),
		choiceTableSeparator(numberWidth, labelWidth, showDetails, includeSelector),
	}
	for row := 0; row < rowCount; row++ {
		index := offset + row
		if index >= end {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, choiceTableRow(index+1, choices[matches[index]], index == selected, numberWidth, labelWidth, showDetails, includeSelector))
	}
	return lines
}

func choiceTableHeader(numberWidth, labelWidth int, showDetails, includeSelector bool) string {
	label := "Option"
	if showDetails {
		label = padChoiceCell(label, labelWidth)
	}
	line := fmt.Sprintf("%*s  %s", numberWidth, "No.", label)
	if includeSelector {
		line = fmt.Sprintf("%-3s  %s", "Sel", line)
	}
	if showDetails {
		line += "  Details"
	}
	return strings.TrimRight(line, " ")
}

func choiceTableSeparator(numberWidth, labelWidth int, showDetails, includeSelector bool) string {
	label := strings.Repeat("-", len("Option"))
	if showDetails {
		label = strings.Repeat("-", labelWidth)
	}
	line := fmt.Sprintf("%*s  %s", numberWidth, strings.Repeat("-", numberWidth), label)
	if includeSelector {
		line = fmt.Sprintf("%-3s  %s", strings.Repeat("-", len("Sel")), line)
	}
	if showDetails {
		line += "  " + strings.Repeat("-", len("Details"))
	}
	return strings.TrimRight(line, " ")
}

func choiceTableRow(number int, choice Choice, selected bool, numberWidth, labelWidth int, showDetails, includeSelector bool) string {
	label, details := choiceDisplay(choice)
	label = truncateChoiceCell(label, labelWidth)
	if showDetails {
		label = padChoiceCell(label, labelWidth)
	}
	line := fmt.Sprintf("%*d  %s", numberWidth, number, label)
	if includeSelector {
		selector := ""
		if selected {
			selector = ">"
		}
		line = fmt.Sprintf("%-3s  %s", selector, line)
	}
	if showDetails {
		line += "  " + details
	}
	return strings.TrimRight(line, " ")
}

func choiceNumberColumnWidth(total int) int {
	width := len("No.")
	if digits := len(strconv.Itoa(total)); digits > width {
		width = digits
	}
	return width
}

func choiceTableShowsDetails(choices []Choice, matches []int, offset, end int) bool {
	for index := offset; index < end; index++ {
		if index >= 0 && index < len(matches) {
			_, details := choiceDisplay(choices[matches[index]])
			if details != "" {
				return true
			}
		}
	}
	return false
}

func choiceLabelColumnWidth(choices []Choice, matches []int, offset, end, width, numberWidth int, showDetails, includeSelector bool) int {
	labelWidth := len("Option")
	for index := offset; index < end; index++ {
		if index < 0 || index >= len(matches) {
			continue
		}
		label, _ := choiceDisplay(choices[matches[index]])
		if length := runeCount(label); length > labelWidth {
			labelWidth = length
		}
	}
	if showDetails && labelWidth < minChoiceLabelColumnWidth {
		labelWidth = minChoiceLabelColumnWidth
	}
	reserved := numberWidth + len("  ")
	if includeSelector {
		reserved += len("Sel") + len("  ")
	}
	if showDetails {
		reserved += len("  ") + minChoiceDetailColumnWidth
	}
	available := width - 1 - reserved
	if available >= len("Option") && labelWidth > available {
		labelWidth = available
	}
	if showDetails && labelWidth < minChoiceLabelColumnWidth {
		labelWidth = minChoiceLabelColumnWidth
	}
	if labelWidth < len("Option") {
		labelWidth = len("Option")
	}
	return labelWidth
}

func padChoiceCell(value string, width int) string {
	padding := width - runeCount(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func truncateChoiceCell(value string, width int) string {
	value = strings.TrimSpace(value)
	if width <= 0 {
		return ""
	}
	if runeCount(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return truncateVisible(value, width-3) + "..."
}

func choiceText(choice Choice) string {
	label, details := choiceDisplay(choice)
	if details == "" {
		return label
	}
	return label + " - " + details
}

func choiceDisplay(choice Choice) (string, string) {
	return strings.TrimSpace(choice.Label), strings.TrimSpace(choice.Description)
}

func runeCount(value string) int {
	return visibleRuneCount(value)
}
