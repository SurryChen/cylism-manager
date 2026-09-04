package cluster

import "testing"

func TestShellEscapeProtectsSingleQuotes(t *testing.T) {
	if got := shellEscape("worker'node"); got != "'worker'\\''node'" {
		t.Fatalf("shellEscape() = %q", got)
	}
}

func TestNodeJoinProgressHandlerConstructorCopiesEncryptionKey(t *testing.T) {
	key := []byte("secret")
	h := NewNodeJoinProgressHandler(nil, key, nil)
	key[0] = 'X'
	if string(h.encKey) != "secret" {
		t.Fatalf("constructor retained mutable key: %q", h.encKey)
	}
}
