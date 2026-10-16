//go:build !fido2

package fido2

// Devices reports that hmac-secret is not linked into this binary.
func Devices() ([]Info, error) {
	return nil, ErrUnavailable
}

// MakeCredential reports that hmac-secret is not linked into this binary.
func MakeCredential(path, pin string) ([]byte, error) {
	return nil, ErrUnavailable
}

// HMACSecret reports that hmac-secret is not linked into this binary.
func HMACSecret(path, pin string, credentialID, salt []byte) ([]byte, error) {
	return nil, ErrUnavailable
}

// Explain describes the stub failure for the desktop window.
func Explain(err error) string {
	return "This desktop build cannot talk to a security key. Rebuild with -tags fido2."
}

// PINFailure is false when this binary cannot talk to a key.
func PINFailure(error) bool { return false }
