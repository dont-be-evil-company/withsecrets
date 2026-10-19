//go:build !piv

package piv

import "errors"

// ErrUnavailable means this binary was not built with -tags piv.
var ErrUnavailable = errors.New("piv: build without -tags piv cannot talk to a YubiKey PIV applet")

// Open reports that PC/SC support is not linked into this binary.
func Open() error {
	return ErrUnavailable
}
