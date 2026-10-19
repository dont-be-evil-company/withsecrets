package main

import (
	"errors"
	"net/url"

	"github.com/mistweaverco/withsecrets/apps/desktop/app"
	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaultsync"
)

func (h *host) syncNow() {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	if !h.svc.Unlocked() {
		return
	}
	server, access, ok := h.sessionToken()
	if !ok {
		h.svc.SetSyncStatus("off")
		return
	}
	client := vaultsync.NewClient(server, access)
	me, err := client.Me()
	if err != nil {
		h.failSync(err)
		return
	}
	vault, err := client.EnsurePersonal(h.svc.ServerVaultID())
	if err != nil {
		h.failSync(err)
		h.syncTeams(client, me)
		_ = h.persist()
		return
	}
	upload, err := h.svc.BindIdentity(me.ID, me.Instance, vault.ID, vault.AccountKeyWrap)
	if err != nil {
		h.failSync(err)
		h.syncTeams(client, me)
		_ = h.persist()
		return
	}
	if upload {
		if err := client.PutAccountWrap(vault.ID, h.svc.AccountWrap()); err != nil {
			h.failSync(err)
			h.syncTeams(client, me)
			_ = h.persist()
			return
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		epoch, snap := h.svc.Snapshot()
		result, err := vaultsync.Reconcile(client, snap)
		if err != nil {
			h.failSync(err)
			return
		}
		err = h.svc.Apply(epoch, result)
		if err == nil {
			h.syncTeams(client, me)
			_ = h.persist()
			return
		}
		if !errors.Is(err, app.ErrEpoch) {
			h.failSync(err)
			h.syncTeams(client, me)
			_ = h.persist()
			return
		}
	}
	h.svc.SetSyncStatus("offline")
	h.syncTeams(client, me)
	_ = h.persist()
}

func (h *host) failSync(err error) {
	status := "error"
	var net *url.Error
	if errors.As(err, &net) {
		status = "offline"
	}
	h.svc.SetSyncStatus(status)
	_ = h.persist()
}

func (h *host) sessionToken() (server, access string, ok bool) {
	if signed, _ := h.accountView(); !signed {
		return "", "", false
	}
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	if h.session.Access == "" {
		return "", "", false
	}
	return h.session.Server, h.session.Access, true
}

func (h *host) afterUnlock(passphrase, recovery string) {
	h.syncNow()
	if passphrase != "" {
		h.rebindPassphrase(passphrase)
	}
	if recovery != "" {
		h.rebindRecovery(recovery)
	}
	_ = h.persist()
}

func (h *host) rebindPassphrase(passphrase string) {
	if !h.have {
		return
	}
	slot := h.slot
	if h.svc.SlotStale(slot) {
		next, err := h.svc.ReenrollPassphrase(passphrase, slot)
		if err != nil {
			return
		}
		h.slot = next
		h.passUploaded = false
		slot = next
	}
	if h.passUploaded {
		return
	}
	if h.uploadSlot(slot) == nil {
		h.passUploaded = true
	}
}

func (h *host) rebindRecovery(key string) {
	if !h.haveRecovery {
		return
	}
	slot := h.recoverySlot
	if h.svc.SlotStale(slot) {
		next, err := h.svc.ReenrollRecoveryKey(key, slot)
		if err != nil {
			return
		}
		h.recoverySlot = next
		h.recoveryUploaded = false
		slot = next
	}
	if h.recoveryUploaded {
		return
	}
	if h.uploadSlot(slot) == nil {
		h.recoveryUploaded = true
	}
}

func (h *host) rebindFIDO(index int, hmacFn vaultcrypto.HMACSecretFunc) {
	if index < 0 || index >= len(h.fido) {
		return
	}
	slot := h.fido[index].slot
	if h.svc.SlotStale(slot) {
		next, err := h.svc.ReenrollFIDO(slot, hmacFn)
		if err != nil {
			return
		}
		h.fido[index].slot = next
		slot = next
	}
	_ = h.uploadSlot(slot)
}

func (h *host) uploadSlot(slot vaultcrypto.UnlockSlot) error {
	server, access, ok := h.sessionToken()
	if !ok {
		return nil
	}
	client := vaultsync.NewClient(server, access)
	if err := client.Enroll(h.svc.Policy(), slot.Context.GenerationID); err != nil {
		return err
	}
	return client.PutSlot(vaultsync.RemoteSlot{
		ID: slot.Context.SlotID, GenerationID: slot.Context.GenerationID, Type: slot.Type,
		Label: slot.Label, AlgVersion: int(slot.Algorithm), PublicParams: slot.Params, Wrap: slot.Envelope,
	})
}

func (h *host) pullSlots() {
	if h.have && h.haveRecovery {
		return
	}
	if h.slotsPulled {
		return
	}
	server, access, ok := h.sessionToken()
	if !ok {
		return
	}
	h.slotsPulled = true
	client := vaultsync.NewClient(server, access)
	me, err := client.Me()
	if err != nil {
		h.slotsPulled = false
		return
	}
	slots, err := client.ListSlots()
	if err != nil {
		h.slotsPulled = false
		return
	}
	changed := false
	for _, slot := range slots {
		if slot.State == vaultcrypto.StateRevoked || len(slot.Wrap) == 0 {
			continue
		}
		local := vaultcrypto.UnlockSlot{
			Context: vaultcrypto.SlotContext{
				AccountID: me.ID, InstanceID: me.Instance, GenerationID: slot.GenerationID,
				SlotID: slot.ID, Label: slot.Label,
			},
			Type: slot.Type, Algorithm: uint8(slot.AlgVersion), Label: slot.Label,
			Params: slot.PublicParams, Envelope: slot.Wrap, State: slot.State,
		}
		if local.State == "" {
			local.State = vaultcrypto.StatePending
		}
		switch slot.Type {
		case vaultcrypto.SlotPassphrase:
			if !h.have {
				h.slot = local
				h.have = true
				changed = true
			}
		case vaultcrypto.SlotRecovery:
			if !h.haveRecovery {
				h.recoverySlot = local
				h.haveRecovery = true
				changed = true
			}
		}
	}
	if changed {
		_ = h.persist()
	}
}
