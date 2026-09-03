package maintenance

import (
	"regexp"
	"strings"
)

var (
	sensitiveText     = regexp.MustCompile(`(?i)((?:token|password|secret|api[_-]?key)\s*[=:]\s*)[^\s,;]+`)
	authorizationText = regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s,;]+`)
)

func truncate(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func redact(value string) string {
	value = sensitiveText.ReplaceAllString(value, `${1}[REDACTED]`)
	return authorizationText.ReplaceAllString(value, `${1}[REDACTED]`)
}

func cleanOutput(value string, limit int) string {
	return truncate(strings.TrimSpace(redact(value)), limit)
}
