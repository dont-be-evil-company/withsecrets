// Package fake is a deterministic hardware stand-in for CI.
// It performs real HMAC-SHA-256 and RSA-OAEP. It is not a YubiKey.
package fake

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"sync"
)

var (
	ErrUnplugged   = errors.New("fake: token unplugged")
	ErrPIN         = errors.New("fake: wrong pin")
	ErrLocked      = errors.New("fake: pin locked")
	ErrTouch       = errors.New("fake: touch required")
	ErrUnsupported = errors.New("fake: hmac-secret unsupported")
	ErrOccupied    = errors.New("fake: piv slot occupied")
	ErrDefaultPIN  = errors.New("fake: factory pin refused")
	ErrNoCred      = errors.New("fake: unknown credential")
)

// Token models one physical authenticator.
type Token struct {
	mu           sync.Mutex
	Serial       string
	PIN          string
	Unplugged    bool
	SupportsHMAC bool
	PINTries     int
	creds        map[string][]byte
	piv          *rsa.PrivateKey
	pivOccupied  bool
	TouchPolicy  string
}

// NewToken returns a token that supports hmac-secret and has eight PIN tries.
func NewToken(serial, pin string) *Token {
	return &Token{
		Serial: serial, PIN: pin, SupportsHMAC: true, PINTries: 8,
		creds: map[string][]byte{}, TouchPolicy: "always",
	}
}

func (t *Token) gate(pin string, touch bool) error {
	if t.Unplugged {
		return ErrUnplugged
	}
	if t.PINTries <= 0 {
		return ErrLocked
	}
	if pin != t.PIN {
		t.PINTries--
		if t.PINTries <= 0 {
			return ErrLocked
		}
		return ErrPIN
	}
	if t.TouchPolicy == "always" && !touch {
		return ErrTouch
	}
	return nil
}

// MakeCredential creates a non-discoverable credential and stores its hmac key.
func (t *Token) MakeCredential(pin string, touch bool) (credentialID []byte, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.SupportsHMAC {
		return nil, ErrUnsupported
	}
	if err := t.gate(pin, touch); err != nil {
		return nil, err
	}
	id := make([]byte, 32)
	secret := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	t.creds[string(id)] = secret
	return id, nil
}

// HMACSecret returns HMAC-SHA-256(credential secret, salt), matching the
// output length the vault protocol requires from CTAP2 hmac-secret.
func (t *Token) HMACSecret(credentialID, salt []byte, pin string, touch bool) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.SupportsHMAC {
		return nil, ErrUnsupported
	}
	if err := t.gate(pin, touch); err != nil {
		return nil, err
	}
	secret, ok := t.creds[string(credentialID)]
	if !ok {
		return nil, ErrNoCred
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(salt)
	return mac.Sum(nil), nil
}

// GeneratePIV creates an RSA-2048 key in the fake PIV slot.
func (t *Token) GeneratePIV(pin, touchPolicy string) (*rsa.PublicKey, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pivOccupied {
		return nil, ErrOccupied
	}
	if pin == "" || pin == "123456" || pin == "12345678" {
		return nil, ErrDefaultPIN
	}
	if touchPolicy == "never" || touchPolicy == "" {
		return nil, errors.New("fake: touch-never refused")
	}
	if err := t.gate(pin, true); err != nil {
		return nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	t.piv = key
	t.pivOccupied = true
	t.TouchPolicy = touchPolicy
	t.PIN = pin
	return &key.PublicKey, nil
}

// DecryptOAEP performs the on-token RSA-OAEP operation.
func (t *Token) DecryptOAEP(pin string, touch bool, ciphertext, label []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.piv == nil {
		return nil, errors.New("fake: no piv key")
	}
	if err := t.gate(pin, touch); err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, t.piv, ciphertext, label)
}
