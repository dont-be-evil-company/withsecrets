//go:build fido2

package fido2

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/keys-pub/go-libfido2"
)

// Devices lists plugged FIDO2 authenticators and whether they advertise hmac-secret.
func Devices() ([]Info, error) {
	locs, err := libfido2.DeviceLocations()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(locs))
	for _, loc := range locs {
		if loc == nil || loc.Path == "" {
			continue
		}
		dev, err := libfido2.NewDevice(loc.Path)
		if err != nil {
			continue
		}
		fido, err := dev.IsFIDO2()
		if err != nil {
			if openFailed(err) {
				return nil, fmt.Errorf("fido2: open %s: %w", loc.Path, err)
			}
			continue
		}
		if !fido {
			continue
		}
		info := Info{
			Path: loc.Path, Product: loc.Product, Manufacturer: loc.Manufacturer,
			Serial: usbSerial(loc.Path),
		}
		if details, err := dev.Info(); err == nil {
			for _, ext := range details.Extensions {
				if ext == string(libfido2.HMACSecretExtension) {
					info.HMAC = true
					break
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// MakeCredential creates a non-discoverable hmac-secret credential.
// The caller has already refused factory PINs. Touch and UV are required.
func MakeCredential(path, pin string) ([]byte, error) {
	dev, err := libfido2.NewDevice(path)
	if err != nil {
		return nil, err
	}
	hash, err := randomBytes(32)
	if err != nil {
		return nil, err
	}
	userID, err := randomBytes(32)
	if err != nil {
		return nil, err
	}
	att, err := dev.MakeCredential(hash, libfido2.RelyingParty{
		ID: RelyingPartyID, Name: "withsecrets",
	}, libfido2.User{
		ID: userID, Name: "withsecrets", DisplayName: "withsecrets",
	}, libfido2.ES256, pin, &libfido2.MakeCredentialOpts{
		Extensions: []libfido2.Extension{libfido2.HMACSecretExtension},
		RK:         libfido2.False,
		UV:         libfido2.True,
	})
	if err != nil {
		return nil, err
	}
	if att == nil || len(att.CredentialID) == 0 {
		return nil, fmt.Errorf("fido2: empty credential")
	}
	return att.CredentialID, nil
}

// HMACSecret asks the authenticator for the 32-byte hmac-secret output.
func HMACSecret(path, pin string, credentialID, salt []byte) ([]byte, error) {
	if len(credentialID) == 0 || len(salt) != 32 {
		return nil, fmt.Errorf("fido2: credential or salt")
	}
	dev, err := libfido2.NewDevice(path)
	if err != nil {
		return nil, err
	}
	hash, err := randomBytes(32)
	if err != nil {
		return nil, err
	}
	assertion, err := dev.Assertion(RelyingPartyID, hash, [][]byte{credentialID}, pin, &libfido2.AssertionOpts{
		Extensions: []libfido2.Extension{libfido2.HMACSecretExtension},
		UV:         libfido2.True,
		UP:         libfido2.True,
		HMACSalt:   salt,
	})
	if err != nil {
		return nil, err
	}
	if assertion == nil || len(assertion.HMACSecret) != 32 {
		return nil, fmt.Errorf("fido2: hmac-secret length")
	}
	return assertion.HMACSecret, nil
}

// Explain turns a libfido2 failure into a sentence safe to show in the window.
func Explain(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnavailable):
		return "This desktop build cannot talk to a security key. Rebuild with -tags fido2."
	case errors.Is(err, ErrNoDevice):
		return "Plug in a security key that supports hmac-secret."
	case errors.Is(err, ErrSameToken):
		return "Unplug this key and insert a different one."
	case errors.Is(err, ErrManyDevices):
		return "More than one security key is plugged in. Leave only the one you want to enroll."
	case errors.Is(err, libfido2.ErrPinInvalid), errors.Is(err, libfido2.ErrPinPolicyViolation):
		return "The PIN was rejected."
	case errors.Is(err, libfido2.ErrPinAuthBlocked):
		return "The key is PIN-locked. Unplug it and follow the manufacturer's unblock steps."
	case errors.Is(err, libfido2.ErrPinNotSet), errors.Is(err, libfido2.ErrPinRequired):
		return "Set a PIN on this key first. Factory PINs are refused."
	case errors.Is(err, libfido2.ErrActionTimeout), errors.Is(err, libfido2.ErrUserPresenceRequired), errors.Is(err, libfido2.ErrUPRequired):
		return "Touch the key when it blinks."
	case errors.Is(err, libfido2.ErrNoCredentials), errors.Is(err, libfido2.ErrInvalidCredential):
		return "This key does not hold the withsecrets credential."
	default:
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(msg, "failed to open") || strings.Contains(msg, "hidraw") {
			return "Cannot open the security key. Allow this user to read the hidraw device."
		}
		return "The security key could not complete hmac-secret."
	}
}

// PINFailure reports a wrong or blocked PIN so the desktop does not retry it.
func PINFailure(err error) bool {
	return errors.Is(err, libfido2.ErrPinInvalid) || errors.Is(err, libfido2.ErrPinPolicyViolation) || errors.Is(err, libfido2.ErrPinAuthBlocked)
}

func openFailed(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "failed to open") || strings.Contains(msg, "permission")
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func usbSerial(hidPath string) string {
	dir := filepath.Join("/sys/class/hidraw", filepath.Base(hidPath), "device")
	for range 4 {
		raw, err := os.ReadFile(filepath.Join(dir, "serial"))
		serial := strings.TrimSpace(string(raw))
		if err == nil && serial != "" && serial != "00000000" {
			return serial
		}
		dir = filepath.Join(dir, "..")
	}
	return ""
}
