// Package doctor reports hardware prerequisites without claiming a token is present.
package doctor

import (
	"os"
	"runtime"
)

// Check is one diagnostic line.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Report describes the binary and the local smart-card/HID environment.
type Report struct {
	OS       string  `json:"os"`
	FIDO2Tag bool    `json:"fido2Tag"`
	PIVTag   bool    `json:"pivTag"`
	Checks   []Check `json:"checks"`
}

// Inspect never prompts a token and never returns secret material.
func Inspect(fido2Tag, pivTag bool) Report {
	r := Report{OS: runtime.GOOS, FIDO2Tag: fido2Tag, PIVTag: pivTag}
	if !fido2Tag {
		r.Checks = append(r.Checks, Check{
			Name: "fido2-build", OK: false,
			Detail: "This binary was built without -tags fido2. hmac-secret enrollment is unavailable. Rebuild with libfido2 installed.",
		})
	} else {
		r.Checks = append(r.Checks, Check{Name: "fido2-build", OK: true, Detail: "FIDO2 tag enabled."})
	}
	if !pivTag {
		r.Checks = append(r.Checks, Check{
			Name: "piv-build", OK: false,
			Detail: "This binary was built without -tags piv. YubiKey PIV is unavailable. FIDO2 and passphrase slots still work on a tagged FIDO2 build.",
		})
	} else {
		r.Checks = append(r.Checks, Check{Name: "piv-build", OK: true, Detail: "PIV tag enabled."})
	}
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/dev/hidraw0"); err != nil {
			r.Checks = append(r.Checks, Check{
				Name: "hidraw", OK: false,
				Detail: "No /dev/hidraw0. Plug in a USB token and install a udev rule that grants your user access to the FIDO HID interface.",
			})
		} else {
			r.Checks = append(r.Checks, Check{Name: "hidraw", OK: true, Detail: "HID device node exists. Permissions may still block access."})
		}
		if _, err := os.Stat("/run/pcscd/pcscd.comm"); err != nil {
			r.Checks = append(r.Checks, Check{
				Name: "pcscd", OK: false,
				Detail: "pcscd socket is missing. Install pcscd (Arch: pcsclite, ccid) and start the service before using PIV.",
			})
		} else {
			r.Checks = append(r.Checks, Check{Name: "pcscd", OK: true, Detail: "pcscd socket is present."})
		}
	}
	return r
}
