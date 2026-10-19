//go:build fido2

package fido2

import "testing"

func TestDevicesDoesNotEnroll(t *testing.T) {
	devs, err := Devices()
	if err != nil {
		t.Fatal(err)
	}
	for _, dev := range devs {
		t.Logf("path=%s product=%q hmac=%v", dev.Path, dev.Product, dev.HMAC)
	}
}
