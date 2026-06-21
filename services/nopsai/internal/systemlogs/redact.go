package systemlogs

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const redactionMarker = "[REDACTED]"

type Redactor struct {
	maxLineBytes int
	patterns     []*regexp.Regexp
}

func NewRedactor(maxLineBytes int) *Redactor {
	if maxLineBytes <= 0 {
		maxLineBytes = 16 * 1024
	}
	return &Redactor{
		maxLineBytes: maxLineBytes,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;]+`),
			regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|passwd|secret|client[_-]?secret)\s*[=:]\s*)[^\s,;]+`),
			regexp.MustCompile(`(?i)("(?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|passwd|secret|client[_-]?secret)"\s*:\s*")[^"]*(")`),
			regexp.MustCompile(`(?i)(postgres(?:ql)?|mysql|mongodb|redis)://[^\s/@:]+:[^\s/@]+@`),
		},
	}
}

func (r *Redactor) Redact(line string) string {
	if r == nil {
		return line
	}
	line = strings.TrimRight(line, "\r\n")
	line = r.patterns[0].ReplaceAllString(line, `${1}`+redactionMarker)
	line = r.patterns[1].ReplaceAllString(line, `${1}`+redactionMarker)
	line = r.patterns[2].ReplaceAllString(line, `${1}`+redactionMarker+`${2}`)
	line = r.patterns[3].ReplaceAllString(line, `${1}://`+redactionMarker+`@`)
	return truncateUTF8(line, r.maxLineBytes)
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	suffix := "...[truncated]"
	end := limit - len(suffix)
	if end <= 0 {
		return suffix[:limit]
	}
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + suffix
}
