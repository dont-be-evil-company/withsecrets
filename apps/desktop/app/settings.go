package app

import (
	"encoding/base32"
	"strings"
	"time"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

// DesktopSettings is the local window preference file. It is not vault content.
type DesktopSettings struct {
	Theme                 string `json:"theme"`
	IdleLockMinutes       int    `json:"idleLockMinutes"`
	ClipboardClearSeconds int    `json:"clipboardClearSeconds"`
	CloseToTray           bool   `json:"closeToTray"`
}

// DefaultSettings is the first-run window behavior.
func DefaultSettings() DesktopSettings {
	return DesktopSettings{
		Theme:                 "dark",
		IdleLockMinutes:       5,
		ClipboardClearSeconds: 30,
		CloseToTray:           true,
	}
}

// NormalizeSettings rejects unknown themes and out-of-range timers.
func NormalizeSettings(in DesktopSettings) (DesktopSettings, error) {
	switch in.Theme {
	case "dark", "light":
	default:
		return DesktopSettings{}, vaultcrypto.ErrParams
	}
	if in.IdleLockMinutes < 0 || in.IdleLockMinutes > 240 {
		return DesktopSettings{}, vaultcrypto.ErrParams
	}
	if in.ClipboardClearSeconds < 0 || in.ClipboardClearSeconds > 300 {
		return DesktopSettings{}, vaultcrypto.ErrParams
	}
	return in, nil
}

// Settings returns a copy of the window preferences.
func (s *Service) Settings() DesktopSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// ApplySettings stores window preferences. Zero idle lock disables the timer.
func (s *Service) ApplySettings(in DesktopSettings) error {
	normalized, err := NormalizeSettings(in)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = normalized
	s.lastActivity = time.Now()
	return nil
}

// Touch records that a person used the vault, so idle lock starts over.
func (s *Service) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
}

// IdleExpired reports that the unlocked vault should lock.
func (s *Service) IdleExpired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) != 32 || s.settings.IdleLockMinutes <= 0 {
		return false
	}
	return now.Sub(s.lastActivity) >= time.Duration(s.settings.IdleLockMinutes)*time.Minute
}

// CurrentTOTP returns the current code for an item seed stored as base32.
func (s *Service) CurrentTOTP(id string, now time.Time) (string, int, error) {
	item, err := s.GetItem(id)
	if err != nil {
		return "", 0, err
	}
	secret := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(item.TOTP), " ", ""))
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(raw) == 0 {
		return "", 0, vaultcrypto.ErrParams
	}
	code, err := vaultcrypto.TOTP(raw, now, 30, 6, "sha1")
	if err != nil {
		return "", 0, err
	}
	remain := 30 - int(now.Unix()%30)
	return code, remain, nil
}
