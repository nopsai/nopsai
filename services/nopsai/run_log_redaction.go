package nopsai

import "regexp"

const (
	runLogRedactionMarker     = "[REDACTED]"
	runLogSensitiveKeyPattern = `authorization|api[_-]?(?:key|token)|access[_-]?token|refresh[_-]?token|id[_-]?token|token|password|passwd|secret|client[_-]?secret|private[_-]?key|credential(?:s)?`
)

var runLogRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;]+`),
	regexp.MustCompile(`(?i)((?:` + runLogSensitiveKeyPattern + `)\s*[=:]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)("(?:` + runLogSensitiveKeyPattern + `)"\s*:\s*")[^"]*(")`),
	regexp.MustCompile(`(?i)(\\"(?:` + runLogSensitiveKeyPattern + `)\\"\s*:\s*\\")[^\\"]*(\\")`),
	regexp.MustCompile(`(?i)(postgres(?:ql)?|mysql|mongodb|redis)://[^\s/@:]+:[^\s/@]+@`),
}

func redactRunLogLine(line string) string {
	if line == "" {
		return line
	}
	redacted := line
	redacted = runLogRedactionPatterns[0].ReplaceAllString(redacted, `${1}`+runLogRedactionMarker)
	redacted = runLogRedactionPatterns[1].ReplaceAllString(redacted, `${1}`+runLogRedactionMarker)
	redacted = runLogRedactionPatterns[2].ReplaceAllString(redacted, `${1}`+runLogRedactionMarker+`${2}`)
	redacted = runLogRedactionPatterns[3].ReplaceAllString(redacted, `${1}`+runLogRedactionMarker+`${2}`)
	redacted = runLogRedactionPatterns[4].ReplaceAllString(redacted, `${1}://`+runLogRedactionMarker+`@`)
	return redacted
}
