package registry

import "github.com/cylism/cylism-manager/internal/crypto"

const (
	AuthTypeAnonymous = "anonymous"
	AuthTypeBasic     = "basic"
	AuthTypeToken     = "token"
)

// EncryptCredential keeps Registry credentials encrypted before they enter a
// persisted model. Callers intentionally map errors to their API-specific
// operator messages.
func EncryptCredential(key []byte, credential string) (string, error) {
	return crypto.Encrypt(key, credential)
}

// DecryptCredential resolves a persisted Registry credential only at the
// boundary that must use it for a Registry request or generated node config.
func DecryptCredential(key []byte, encrypted string) (string, error) {
	return crypto.Decrypt(key, encrypted)
}

func CredentialConfigured(encrypted string) bool {
	return encrypted != ""
}
