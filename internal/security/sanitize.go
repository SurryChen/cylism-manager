package security

import (
	"regexp"
	"strings"
)

var sensitive = regexp.MustCompile(`(?i)(password|passwd|token|secret|credential|authorization)\s*[:=]\s*([^\s,;]+)`)
var authorization = regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s,;]+`)

func Truncate(value string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func Redact(value string) string {
	value = authorization.ReplaceAllString(value, "$1[REDACTED]")
	return sensitive.ReplaceAllString(value, "$1=[REDACTED]")
}

func TokenDisplay(value string, edge int) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == ':' {
			return r
		}
		return -1
	}, value)
	if edge < 1 || len(value) <= edge*2 {
		return value
	}
	return value[:edge] + "..." + value[len(value)-edge:]
}
