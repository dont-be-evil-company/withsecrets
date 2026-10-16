package main

import (
	"net/http"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaulthardware/fido2"
)

func (h *host) fidoDevices(w http.ResponseWriter, _ *http.Request) {
	devs, err := fido2.Devices()
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusServiceUnavailable)
		return
	}
	if devs == nil {
		devs = []fido2.Info{}
	}
	h.noteRemoved(devs)
	write(w, map[string]any{"devices": devs})
}

func (h *host) fidoEnroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PIN    string `json:"pin"`
		Path   string `json:"path"`
		Policy string `json:"policy"`
	}
	if err := read(r, &body); err != nil {
		http.Error(w, "pin", http.StatusBadRequest)
		return
	}
	if body.Policy != "" {
		h.policyChoice = body.Policy
	}
	if err := vaultcrypto.RejectPIN(body.PIN); err != nil {
		http.Error(w, "Use a PIN that is not empty and not a factory default.", http.StatusBadRequest)
		return
	}
	devs, err := fido2.Devices()
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusServiceUnavailable)
		return
	}
	path, info, err := chooseDevice(devs, body.Path)
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusConflict)
		return
	}
	if err := h.prepareFIDO(path, info.Serial, devs); err != nil {
		http.Error(w, fido2.Explain(err), http.StatusConflict)
		return
	}
	cred, err := fido2.MakeCredential(path, body.PIN)
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusBadRequest)
		return
	}
	pin := body.PIN
	slot, err := h.svc.EnrollFIDOSlot(cred, func(id, salt []byte) ([]byte, error) {
		return fido2.HMACSecret(path, pin, id, salt)
	})
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusBadRequest)
		return
	}
	proof := h.proofFor(path, info.Serial, cred, devs)
	for _, prev := range h.fido {
		if err := vaultcrypto.DistinctTokens(prev.proof, proof); err != nil {
			http.Error(w, "Unplug this key and insert a different one.", http.StatusConflict)
			return
		}
	}
	h.fido = append(h.fido, fidoRecord{path: path, serial: info.Serial, cred: cred, slot: slot, proof: proof})
	if h.policyChoice == "hardware" && len(h.fido) >= 2 {
		h.svc.SetRecoveryPolicy(vaultcrypto.PolicyHardwareOnly)
	}
	if !h.saved(w) {
		return
	}
	write(w, map[string]any{
		"enrolled": true, "product": info.Product, "count": len(h.fido),
		"unlocked": h.svc.Unlocked(),
	})
}

func (h *host) fidoUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PIN  string `json:"pin"`
		Path string `json:"path"`
	}
	if err := read(r, &body); err != nil || len(h.fido) == 0 {
		http.Error(w, "unlock", http.StatusBadRequest)
		return
	}
	if err := vaultcrypto.RejectPIN(body.PIN); err != nil {
		http.Error(w, "Use a PIN that is not empty and not a factory default.", http.StatusBadRequest)
		return
	}
	devs, err := fido2.Devices()
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusServiceUnavailable)
		return
	}
	h.noteRemoved(devs)
	path, _, err := chooseDevice(devs, body.Path)
	if err != nil {
		http.Error(w, fido2.Explain(err), http.StatusConflict)
		return
	}
	var last error
	for _, rec := range slotsForPath(h.fido, path) {
		err := h.svc.UnlockWithFIDO(rec.slot, func(id, salt []byte) ([]byte, error) {
			return fido2.HMACSecret(path, body.PIN, id, salt)
		})
		if err == nil {
			h.afterUnlock("", "")
			for i := range h.fido {
				if h.fido[i].path == rec.path && h.fido[i].slot.Context.SlotID == rec.slot.Context.SlotID {
					h.rebindFIDO(i, func(id, salt []byte) ([]byte, error) {
						return fido2.HMACSecret(path, body.PIN, id, salt)
					})
					break
				}
			}
			_ = h.persist()
			write(w, map[string]any{"unlocked": true})
			return
		}
		last = err
		if fido2.PINFailure(err) {
			break
		}
	}
	if last == nil {
		last = vaultcrypto.ErrDecrypt
	}
	http.Error(w, fido2.Explain(last), http.StatusUnauthorized)
}

func chooseDevice(devs []fido2.Info, path string) (string, fido2.Info, error) {
	var hmacDevs []fido2.Info
	for _, dev := range devs {
		if dev.HMAC {
			hmacDevs = append(hmacDevs, dev)
		}
	}
	if path != "" {
		for _, dev := range hmacDevs {
			if dev.Path == path {
				return dev.Path, dev, nil
			}
		}
		return "", fido2.Info{}, fido2.ErrNoDevice
	}
	switch len(hmacDevs) {
	case 0:
		return "", fido2.Info{}, fido2.ErrNoDevice
	case 1:
		return hmacDevs[0].Path, hmacDevs[0], nil
	default:
		return "", fido2.Info{}, fido2.ErrManyDevices
	}
}

func slotsForPath(slots []fidoRecord, path string) []fidoRecord {
	var matched []fidoRecord
	var removed []fidoRecord
	for _, rec := range slots {
		if rec.path == path {
			matched = append(matched, rec)
		} else if rec.removed {
			removed = append(removed, rec)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return removed
}

func (h *host) noteRemoved(devs []fido2.Info) {
	present := map[string]bool{}
	for _, dev := range devs {
		present[dev.Path] = true
	}
	for i := range h.fido {
		if !present[h.fido[i].path] {
			h.fido[i].removed = true
		}
	}
}

func (h *host) prepareFIDO(path, serial string, devs []fido2.Info) error {
	h.noteRemoved(devs)
	for _, prev := range h.fido {
		if prev.path == path && !prev.removed {
			return fido2.ErrSameToken
		}
		if serial != "" && prev.serial == serial && !prev.removed {
			return fido2.ErrSameToken
		}
	}
	return nil
}

func (h *host) proofFor(path, serial string, cred []byte, devs []fido2.Info) vaultcrypto.TokenProof {
	present := map[string]bool{}
	for _, dev := range devs {
		present[dev.Path] = true
	}
	reinsert := false
	for _, prev := range h.fido {
		if prev.removed || (present[prev.path] && prev.path != path) {
			reinsert = true
		}
	}
	return vaultcrypto.TokenProof{Serial: serial, CredentialID: append([]byte(nil), cred...), ReinsertProven: reinsert}
}
