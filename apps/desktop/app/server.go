package app

import (
	"net/url"
	"strings"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

// NormalizeServerURL accepts HTTPS, and HTTP only for loopback development.
func NormalizeServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", vaultcrypto.ErrParams
	}
	switch u.Scheme {
	case "https":
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
		default:
			return "", vaultcrypto.ErrParams
		}
	default:
		return "", vaultcrypto.ErrParams
	}
	return u.String(), nil
}

// SetDefaultServer replaces the server used for sign-in.
// A different host locks the vault. Item ciphertext is not sent to the server.
func (s *Service) SetDefaultServer(raw string) error {
	normalized, err := NormalizeServerURL(raw)
	if err != nil {
		return err
	}
	next, err := url.Parse(normalized)
	if err != nil {
		return vaultcrypto.ErrParams
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.profiles) == 0 {
		s.profiles = []Profile{{Name: next.Hostname(), URL: normalized}}
		return nil
	}
	prev, err := url.Parse(s.profiles[0].URL)
	if err == nil && prev.Hostname() != next.Hostname() {
		vaultcrypto.Wipe(s.master)
		vaultcrypto.Wipe(s.vaultKey)
		s.master = nil
		s.vaultKey = nil
		s.items = map[string]ItemBody{}
	}
	s.profiles[0].Name = next.Hostname()
	s.profiles[0].URL = normalized
	return nil
}
