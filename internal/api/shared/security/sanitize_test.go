package security

import "testing"

func TestRedactAndTruncate(t *testing.T) {
	value := Redact("token=abc Authorization: Bearer secret password=hidden")
	if value == "" || contains(value, "abc") || contains(value, "secret") || contains(value, "hidden") {
		t.Fatalf("credential leaked: %q", value)
	}
	if got := Truncate("abcdefgh", 3); got != "abc..." {
		t.Fatalf("unexpected truncation: %q", got)
	}
}
func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
