package app

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaultsync"
)

// ErrEpoch means the vault changed while a sync was in flight.
var ErrEpoch = errors.New("sync epoch moved")

type itemMeta struct {
	sealed        int64
	base          int64
	dirty         bool
	pendingDelete bool
	conflictAt    int64
}

// SyncStatus reports the last sync attempt. Empty means this vault has not synced.
func (s *Service) SyncStatus() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		if team := s.teams[s.active]; team != nil {
			if team.waiting {
				return "waiting"
			}
			return team.syncStatus
		}
	}
	return s.syncStatus
}

// SetSyncStatus records offline, synced, conflict, or off.
func (s *Service) SetSyncStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncStatus = status
}

// ServerVaultID is the bound server vault, or empty when this device has not synced.
func (s *Service) ServerVaultID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == "local" || s.instance == "local-dev" {
		return ""
	}
	return s.vaultID
}

// AccountWrap copies the account-wrapped vault key.
func (s *Service) AccountWrap() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.vaultWrap...)
}

// Policy returns the recovery policy stored for this vault.
func (s *Service) Policy() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policy == "" {
		return vaultcrypto.PolicyPassphrase
	}
	return s.policy
}

// Snapshot copies ciphertext and sync metadata for one reconcile.
func (s *Service) Snapshot() (int64, vaultsync.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := vaultsync.Snapshot{
		VaultID: serverVaultID(s),
		Cursor:  s.cursor,
		Wrap:    append([]byte(nil), s.vaultWrap...),
	}
	for id, meta := range s.meta {
		item := vaultsync.LocalItem{
			ID: id, SealedRevision: meta.sealed, BaseRevision: meta.base,
			Dirty: meta.dirty, PendingDelete: meta.pendingDelete, ConflictAt: meta.conflictAt,
		}
		if env, ok := s.envelopes[id]; ok {
			item.Ciphertext = append([]byte(nil), env...)
		}
		snap.Items = append(snap.Items, item)
	}
	return s.epoch, snap
}

func serverVaultID(s *Service) string {
	if s.account == "local" || s.instance == "local-dev" {
		return ""
	}
	return s.vaultID
}

// BindIdentity attaches this unlocked vault to the signed-in account.
// upload is true when the local vault-key wrap should be stored on the server.
func (s *Service) BindIdentity(account, instance, vaultID string, remoteWrap []byte) (upload bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) != 32 || account == "" || instance == "" || vaultID == "" {
		return false, vaultcrypto.ErrDecrypt
	}
	if len(s.vaultKey) != 32 {
		if len(remoteWrap) == 0 || len(s.items) > 0 || len(s.envelopes) > 0 {
			return false, vaultcrypto.ErrDecrypt
		}
		key, err := vaultcrypto.UnwrapVaultKeyForAccount(s.master, remoteWrap, account, instance, vaultID)
		if err != nil {
			return false, err
		}
		s.vaultKey = key
		s.vaultWrap = append([]byte(nil), remoteWrap...)
		s.account, s.instance, s.vaultID = account, instance, vaultID
		s.cursor = 0
		s.epoch++
		return false, nil
	}
	if s.account == account && s.instance == instance && s.vaultID == vaultID {
		if len(s.vaultWrap) == 0 {
			return false, vaultcrypto.ErrDecrypt
		}
		return len(remoteWrap) == 0 || !bytes.Equal(remoteWrap, s.vaultWrap), nil
	}
	if len(remoteWrap) > 0 && len(s.items) == 0 && len(s.envelopes) == 0 {
		key, err := vaultcrypto.UnwrapVaultKeyForAccount(s.master, remoteWrap, account, instance, vaultID)
		if err != nil {
			return false, err
		}
		vaultcrypto.Wipe(s.vaultKey)
		s.vaultKey = key
		s.vaultWrap = append([]byte(nil), remoteWrap...)
		s.account, s.instance, s.vaultID = account, instance, vaultID
		s.cursor = 0
		s.epoch++
		return false, nil
	}
	if len(remoteWrap) > 0 {
		key, err := vaultcrypto.UnwrapVaultKeyForAccount(s.master, remoteWrap, account, instance, vaultID)
		if err != nil {
			return false, err
		}
		vaultcrypto.Wipe(s.vaultKey)
		s.vaultKey = key
		s.vaultWrap = append([]byte(nil), remoteWrap...)
		s.account, s.instance, s.vaultID = account, instance, vaultID
		if err := s.resealAllLocked(); err != nil {
			return false, err
		}
		s.cursor = 0
		s.epoch++
		return false, nil
	}
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(s.master, s.vaultKey, account, instance, vaultID)
	if err != nil {
		return false, err
	}
	s.vaultWrap = wrap
	s.account, s.instance, s.vaultID = account, instance, vaultID
	if err := s.resealAllLocked(); err != nil {
		return false, err
	}
	s.cursor = 0
	s.epoch++
	return true, nil
}

func (s *Service) resealAllLocked() error {
	nextEnv := map[string][]byte{}
	nextMeta := map[string]itemMeta{}
	for id, item := range s.items {
		item.VaultID = s.vaultID
		raw, err := json.Marshal(item)
		if err != nil {
			return err
		}
		env, err := vaultcrypto.SealItem(s.vaultKey, vaultcrypto.AAD{
			AccountID: s.account, InstanceID: s.instance, VaultID: s.vaultID, ItemID: id, Revision: 1,
		}, raw)
		if err != nil {
			return err
		}
		s.items[id] = item
		nextEnv[id] = env
		nextMeta[id] = itemMeta{sealed: 1, dirty: true}
	}
	s.envelopes = nextEnv
	s.meta = nextMeta
	return nil
}

// Apply stores the result of a reconcile when the vault has not changed since epoch.
func (s *Service) Apply(epoch int64, result vaultsync.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch != s.epoch {
		return ErrEpoch
	}
	if len(s.vaultKey) != 32 {
		return vaultcrypto.ErrDecrypt
	}
	for _, id := range result.Removed {
		delete(s.items, id)
		delete(s.envelopes, id)
		delete(s.meta, id)
	}
	for _, item := range result.Pulled {
		if item.Deleted {
			delete(s.items, item.ID)
			delete(s.envelopes, item.ID)
			delete(s.meta, item.ID)
			continue
		}
		if err := s.installLocked(item.ID, item.Revision, item.Ciphertext); err != nil {
			return err
		}
	}
	for id, rev := range result.Pushed {
		meta, ok := s.meta[id]
		if !ok {
			continue
		}
		if meta.pendingDelete {
			delete(s.items, id)
			delete(s.envelopes, id)
			delete(s.meta, id)
			continue
		}
		meta.base = rev
		meta.dirty = false
		meta.conflictAt = 0
		if env := s.envelopes[id]; len(env) > 0 {
			if plain, err := s.openLocked(id, rev, env); err == nil {
				vaultcrypto.Wipe(plain)
				meta.sealed = rev
			}
		}
		s.meta[id] = meta
	}
	for _, item := range result.Conflicts {
		meta := s.meta[item.ID]
		if meta.conflictAt > 0 && meta.conflictAt >= item.Revision {
			continue
		}
		if !item.Deleted && len(item.Ciphertext) > 0 {
			if err := s.conflictCopyLocked(item.ID, item.Revision, item.Ciphertext); err != nil {
				return err
			}
		}
		if meta, ok := s.meta[item.ID]; ok {
			meta.conflictAt = item.Revision
			s.meta[item.ID] = meta
		}
	}
	s.cursor = result.Cursor
	s.syncStatus = "synced"
	for _, meta := range s.meta {
		if meta.conflictAt > 0 {
			s.syncStatus = "conflict"
			break
		}
	}
	if len(result.Conflicts) > 0 {
		s.syncStatus = "conflict"
	}
	return nil
}

func (s *Service) installLocked(id string, rev int64, ciphertext []byte) error {
	plain, err := s.openLocked(id, rev, ciphertext)
	if err != nil {
		return err
	}
	var item ItemBody
	if err := json.Unmarshal(plain, &item); err != nil {
		vaultcrypto.Wipe(plain)
		return err
	}
	vaultcrypto.Wipe(plain)
	item.ID = id
	item.VaultID = s.vaultID
	s.items[id] = item
	s.envelopes[id] = append([]byte(nil), ciphertext...)
	s.meta[id] = itemMeta{sealed: rev, base: rev}
	return nil
}

func (s *Service) openLocked(id string, rev int64, ciphertext []byte) ([]byte, error) {
	return vaultcrypto.OpenItem(s.vaultKey, vaultcrypto.AAD{
		AccountID: s.account, InstanceID: s.instance, VaultID: s.vaultID, ItemID: id, Revision: uint64(rev),
	}, ciphertext)
}

func (s *Service) conflictCopyLocked(remoteID string, rev int64, ciphertext []byte) error {
	plain, err := s.openLocked(remoteID, rev, ciphertext)
	if err != nil {
		return err
	}
	var item ItemBody
	if err := json.Unmarshal(plain, &item); err != nil {
		vaultcrypto.Wipe(plain)
		return err
	}
	vaultcrypto.Wipe(plain)
	if !stringsHasSuffix(item.Title, " (server copy)") {
		item.Title += " (server copy)"
	}
	item.ID = randomID()
	item.VaultID = s.vaultID
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	env, err := vaultcrypto.SealItem(s.vaultKey, vaultcrypto.AAD{
		AccountID: s.account, InstanceID: s.instance, VaultID: s.vaultID, ItemID: item.ID, Revision: 1,
	}, raw)
	if err != nil {
		return err
	}
	s.items[item.ID] = item
	s.envelopes[item.ID] = env
	s.meta[item.ID] = itemMeta{sealed: 1, dirty: true}
	return nil
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// SlotStale reports that a slot was wrapped for a different account than the one now bound.
func (s *Service) SlotStale(slot vaultcrypto.UnlockSlot) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == "local" || s.instance == "local-dev" {
		return false
	}
	return slot.Context.AccountID != s.account || slot.Context.InstanceID != s.instance
}

// ReenrollPassphrase wraps the current master key for the bound account.
func (s *Service) ReenrollPassphrase(passphrase string, existing vaultcrypto.UnlockSlot) (vaultcrypto.UnlockSlot, error) {
	ctx := slotContext(s, existing, "pass", "Passphrase")
	master, err := s.masterCopy()
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(master)
	return vaultcrypto.EnrollPassphraseFromParams(master, ctx, passphrase, existing.Params)
}

// ReenrollRecoveryKey wraps the current master key with the same recovery secret.
func (s *Service) ReenrollRecoveryKey(key string, existing vaultcrypto.UnlockSlot) (vaultcrypto.UnlockSlot, error) {
	raw, err := parseRecoveryKey(key)
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(raw)
	ctx := slotContext(s, existing, "recovery", "Printed recovery")
	master, err := s.masterCopy()
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(master)
	return vaultcrypto.EnrollRecovery(master, raw, ctx)
}

// ReenrollFIDO wraps the current master key again on the next authenticator touch.
func (s *Service) ReenrollFIDO(existing vaultcrypto.UnlockSlot, hmacFn vaultcrypto.HMACSecretFunc) (vaultcrypto.UnlockSlot, error) {
	var params struct {
		CredentialID []byte `json:"credentialId"`
		Salt         []byte `json:"salt"`
	}
	if err := json.Unmarshal(existing.Params, &params); err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	ctx := slotContext(s, existing, existing.Context.SlotID, existing.Label)
	master, err := s.masterCopy()
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(master)
	return vaultcrypto.EnrollFIDO2(master, ctx, params.CredentialID, params.Salt, hmacFn)
}

func slotContext(s *Service, existing vaultcrypto.UnlockSlot, fallbackID, fallbackLabel string) vaultcrypto.SlotContext {
	s.mu.Lock()
	account, instance := s.account, s.instance
	s.mu.Unlock()
	ctx := existing.Context
	ctx.AccountID = account
	ctx.InstanceID = instance
	if ctx.GenerationID == "" {
		ctx.GenerationID = "g1"
	}
	if ctx.SlotID == "" {
		ctx.SlotID = fallbackID
	}
	if ctx.Label == "" {
		ctx.Label = fallbackLabel
	}
	return ctx
}
