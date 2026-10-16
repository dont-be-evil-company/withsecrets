//go:build !fido2

package fido2

import (
	"errors"
	"testing"
)

func TestStubRefusesHardware(t *testing.T) {
	if _, err := Devices(); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := MakeCredential("/dev/hidraw0", "654321"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
