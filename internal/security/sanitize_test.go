package security

import (
	"strings"
	"testing"
)

func TestRedactAndTruncate(t *testing.T) {
	if got := Redact("password=secret authorization: Bearer abc"); strings.Contains(got, "secret") || strings.Contains(got, "abc") {
		t.Fatalf("sensitive value leaked: %s", got)
	}
	if got := Truncate("abcdef", 3); got != "abc..." {
		t.Fatalf("unexpected truncation: %q", got)
	}
}
