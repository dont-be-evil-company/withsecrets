// Package fido2 talks to a USB CTAP2 authenticator when the binary is built
// with -tags fido2. The default build returns ErrUnavailable.
package fido2

import "errors"

const (
	// RelyingPartyID is stable for this desktop vault. It is not the server
	// hostname: a later hostname change must not invalidate an enrolled key.
	// hmac-secret here is not a website passkey.
	RelyingPartyID = "localhost"
)

// ErrUnavailable means this binary was not built with -tags fido2.
var ErrUnavailable = errors.New("fido2: build without -tags fido2 cannot talk to a security key")

// ErrNoDevice means no plugged token advertised hmac-secret.
var ErrNoDevice = errors.New("fido2: no hmac-secret key is plugged in")

// ErrSameToken means this enrollment would reuse the key already enrolled.
var ErrSameToken = errors.New("fido2: unplug this key and insert a different one")

// ErrManyDevices means more than one hmac-secret key is plugged in.
var ErrManyDevices = errors.New("fido2: more than one security key is plugged in")

// Info describes one HID authenticator. Serial is kept off the wire.
type Info struct {
	Path         string `json:"path"`
	Product      string `json:"product"`
	Manufacturer string `json:"manufacturer"`
	HMAC         bool   `json:"hmac"`
	Serial       string `json:"-"`
}
