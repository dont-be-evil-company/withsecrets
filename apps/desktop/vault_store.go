package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
)

type savedFIDO struct {
	Path   string                 `json:"path"`
	Serial string                 `json:"serial"`
	Cred   []byte                 `json:"credentialId"`
	Slot   vaultcrypto.UnlockSlot `json:"slot"`
	Proof  vaultcrypto.TokenProof `json:"proof"`
}

type savedVault struct {
	Version       int                     `json:"version"`
	Wrap          []byte                  `json:"wrap"`
	Envelopes     map[string][]byte       `json:"envelopes"`
	Items         map[string]app.ItemSync `json:"items"`
	Policy        string                  `json:"policy"`
	RecoveryReady bool                    `json:"recoveryReady"`
	FidoCount     int                     `json:"fidoCount"`
	HavePass      bool                    `json:"havePass"`
	PassSlot      vaultcrypto.UnlockSlot  `json:"passSlot"`
	HaveRecovery  bool                    `json:"haveRecovery"`
	RecoverySlot  vaultcrypto.UnlockSlot  `json:"recoverySlot"`
	FIDO          []savedFIDO             `json:"fido"`
	Account       string                  `json:"account"`
	Instance      string                  `json:"instance"`
	VaultID       string                  `json:"vaultId"`
	Cursor        int64                   `json:"cursor"`
	SyncStatus    string                  `json:"syncStatus"`
	DeviceID      string                  `json:"deviceId"`
	DeviceWrap    []byte                  `json:"deviceWrap"`
	Teams         []app.TeamState         `json:"teams"`
}

func vaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "withsecrets", "desktop-vault.json"), nil
}

func (h *host) persist() error {
	state := h.svc.ExportVault()
	saved := savedVault{
		Version:       1,
		Wrap:          state.Wrap,
		Envelopes:     state.Envelopes,
		Items:         state.Items,
		Policy:        state.Policy,
		RecoveryReady: state.RecoveryReady,
		FidoCount:     state.FidoCount,
		HavePass:      h.have,
		PassSlot:      h.slot,
		HaveRecovery:  h.haveRecovery,
		RecoverySlot:  h.recoverySlot,
		Account:       state.Account,
		Instance:      state.Instance,
		VaultID:       state.VaultID,
		Cursor:        state.Cursor,
		SyncStatus:    state.SyncStatus,
		DeviceID:      state.DeviceID,
		DeviceWrap:    state.DeviceWrap,
		Teams:         state.Teams,
	}
	for _, rec := range h.fido {
		saved.FIDO = append(saved.FIDO, savedFIDO{
			Path: rec.path, Serial: rec.serial, Cred: append([]byte(nil), rec.cred...),
			Slot: rec.slot, Proof: rec.proof,
		})
	}
	if saved.Envelopes == nil {
		saved.Envelopes = map[string][]byte{}
	}
	path, err := vaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func (h *host) loadVault() {
	path, err := vaultPath()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved savedVault
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Version != 1 {
		return
	}
	if len(saved.Wrap) == 0 && !saved.HavePass && !saved.HaveRecovery && len(saved.FIDO) == 0 {
		return
	}
	h.svc.ImportVault(app.VaultState{
		Wrap: saved.Wrap, Envelopes: saved.Envelopes, Items: saved.Items, Policy: saved.Policy,
		RecoveryReady: saved.RecoveryReady, FidoCount: saved.FidoCount,
		Account: saved.Account, Instance: saved.Instance, VaultID: saved.VaultID,
		Cursor: saved.Cursor, SyncStatus: saved.SyncStatus,
		DeviceID: saved.DeviceID, DeviceWrap: saved.DeviceWrap, Teams: saved.Teams,
	})
	h.have = saved.HavePass
	h.slot = saved.PassSlot
	h.haveRecovery = saved.HaveRecovery
	h.recoverySlot = saved.RecoverySlot
	for _, rec := range saved.FIDO {
		h.fido = append(h.fido, fidoRecord{
			path: rec.Path, serial: rec.Serial, cred: rec.Cred, slot: rec.Slot, proof: rec.Proof,
		})
	}
}

func (h *host) saved(w http.ResponseWriter) bool {
	if err := h.persist(); err != nil {
		http.Error(w, "The vault was updated in this session, but it could not be saved on this computer.", http.StatusInternalServerError)
		return false
	}
	return true
}
