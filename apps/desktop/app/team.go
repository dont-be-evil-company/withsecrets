package app

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaultsync"
)

// legacyTeamInstance is the instance id used before this desktop bound to a server.
const legacyTeamInstance = "local-dev"

// TeamState is one shared vault stored beside the personal vault. It has no plaintext.
type TeamState struct {
	ID         string
	Name       string
	OrgID      string
	OwnerID    string
	Owner      bool
	Waiting    bool
	Generation int64
	Wrap       []byte
	Envelopes  map[string][]byte
	Items      map[string]ItemSync
	Cursor     int64
	SyncStatus string
}

// TeamSummary is safe to show in the vault list.
type TeamSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OrgID   string `json:"orgId"`
	Owner   bool   `json:"owner"`
	Waiting bool   `json:"waiting"`
	Sync    string `json:"sync"`
	Kind    string `json:"kind"`
}

// TeamRecipient is a device that should receive the current vault key.
type TeamRecipient struct {
	DeviceID string
	BoxPub   []byte
}

// SealedEnvelope is one vault-key wrap ready to upload.
type SealedEnvelope struct {
	DeviceID   string
	Generation int64
	Ciphertext []byte
	Signature  []byte
}

// DevicePublic is this desktop's public keys when the server does not know them yet.
type DevicePublic struct {
	Sign []byte
	Box  []byte
}

type teamState struct {
	id         string
	name       string
	orgID      string
	ownerID    string
	owner      bool
	waiting    bool
	generation int64
	wrap       []byte
	vaultKey   []byte
	items      map[string]ItemBody
	envelopes  map[string][]byte
	meta       map[string]itemMeta
	cursor     int64
	epoch      int64
	syncStatus string
}

func exportTeamsLocked(s *Service) []TeamState {
	out := make([]TeamState, 0, len(s.teams))
	for _, team := range s.teams {
		exported := TeamState{
			ID: team.id, Name: team.name, OrgID: team.orgID, OwnerID: team.ownerID, Owner: team.owner, Waiting: team.waiting,
			Generation: team.generation, Wrap: append([]byte(nil), team.wrap...),
			Envelopes: map[string][]byte{}, Items: map[string]ItemSync{},
			Cursor: team.cursor, SyncStatus: team.syncStatus,
		}
		for id, env := range team.envelopes {
			exported.Envelopes[id] = append([]byte(nil), env...)
		}
		for id, meta := range team.meta {
			exported.Items[id] = ItemSync{
				Sealed: meta.sealed, Base: meta.base, Dirty: meta.dirty,
				PendingDelete: meta.pendingDelete, ConflictAt: meta.conflictAt,
			}
		}
		out = append(out, exported)
	}
	return out
}

func importTeam(in TeamState) *teamState {
	team := &teamState{
		id: in.ID, name: in.Name, orgID: in.OrgID, ownerID: in.OwnerID, owner: in.Owner, waiting: in.Waiting,
		generation: in.Generation, wrap: append([]byte(nil), in.Wrap...),
		items: map[string]ItemBody{}, envelopes: map[string][]byte{}, meta: map[string]itemMeta{},
		cursor: in.Cursor, syncStatus: in.SyncStatus,
	}
	for id, env := range in.Envelopes {
		team.envelopes[id] = append([]byte(nil), env...)
	}
	if len(in.Items) == 0 {
		for id := range team.envelopes {
			team.meta[id] = itemMeta{sealed: 1, dirty: true}
		}
	} else {
		for id, meta := range in.Items {
			sealed := meta.Sealed
			if sealed == 0 {
				sealed = 1
			}
			team.meta[id] = itemMeta{
				sealed: sealed, base: meta.Base, dirty: meta.Dirty,
				pendingDelete: meta.PendingDelete, conflictAt: meta.ConflictAt,
			}
		}
	}
	return team
}

func (s *Service) lockTeamsLocked() {
	if s.deviceBox != nil {
		vaultcrypto.Wipe(s.deviceBox.Bytes())
		s.deviceBox = nil
	}
	vaultcrypto.Wipe(s.deviceSign)
	s.deviceSign = nil
	for _, team := range s.teams {
		vaultcrypto.Wipe(team.vaultKey)
		team.vaultKey = nil
		team.items = map[string]ItemBody{}
	}
}

func (s *Service) openDeviceLocked() {
	if len(s.master) != 32 || len(s.deviceWrap) == 0 || s.account == "" || s.instance == "" {
		return
	}
	boxPriv, signPriv, err := vaultcrypto.OpenDeviceSecrets(s.master, s.deviceWrap, s.account, s.instance)
	if err != nil {
		return
	}
	box, err := ecdh.X25519().NewPrivateKey(boxPriv)
	vaultcrypto.Wipe(boxPriv)
	if err != nil {
		vaultcrypto.Wipe(signPriv)
		return
	}
	s.deviceBox = box
	s.deviceSign = ed25519.PrivateKey(signPriv)
}

func (s *Service) openTeamsLocked() {
	for _, team := range s.teams {
		if len(team.wrap) == 0 || len(s.master) != 32 {
			continue
		}
		key, err := vaultcrypto.UnwrapVaultKeyForAccount(s.master, team.wrap, s.account, s.instance, team.id)
		if err != nil {
			continue
		}
		vaultcrypto.Wipe(team.vaultKey)
		team.vaultKey = key
		team.waiting = false
		_ = reloadTeamLocked(s, team)
	}
}

func reloadTeamLocked(s *Service, team *teamState) error {
	team.items = map[string]ItemBody{}
	for id, env := range team.envelopes {
		rev := team.meta[id].sealed
		if rev == 0 {
			rev = 1
		}
		pt, err := vaultcrypto.OpenItem(team.vaultKey, vaultcrypto.AAD{
			AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: id, Revision: uint64(rev),
		}, env)
		if err != nil {
			continue
		}
		var item ItemBody
		if err := json.Unmarshal(pt, &item); err != nil {
			vaultcrypto.Wipe(pt)
			continue
		}
		vaultcrypto.Wipe(pt)
		team.items[id] = item
	}
	return nil
}

// EnsureDevice returns public keys when this desktop still needs to enroll.
func (s *Service) EnsureDevice(account, instance string) (DevicePublic, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) != 32 || account == "" || instance == "" || account == "local" {
		return DevicePublic{}, false, vaultcrypto.ErrDecrypt
	}
	if s.deviceBox != nil && len(s.deviceSign) == ed25519.PrivateKeySize && s.deviceID != "" {
		return DevicePublic{}, true, nil
	}
	if s.deviceBox == nil || len(s.deviceSign) != ed25519.PrivateKeySize {
		keys, err := vaultcrypto.NewDeviceKeys()
		if err != nil {
			return DevicePublic{}, false, err
		}
		wrap, err := vaultcrypto.WrapDeviceSecrets(s.master, keys.Box.Bytes(), keys.Sign, account, instance)
		if err != nil {
			return DevicePublic{}, false, err
		}
		s.deviceBox = keys.Box
		s.deviceSign = keys.Sign
		s.deviceWrap = wrap
		s.deviceID = ""
	}
	return DevicePublic{Sign: s.deviceSign.Public().(ed25519.PublicKey), Box: s.deviceBox.PublicKey().Bytes()}, false, nil
}

// DeviceBox copies this desktop's X25519 public key.
func (s *Service) DeviceBox() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceBox == nil {
		return nil
	}
	return append([]byte(nil), s.deviceBox.PublicKey().Bytes()...)
}

// SetDeviceID records the server id for this desktop's device.
func (s *Service) SetDeviceID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceID = id
}

// DeviceID is the enrolled device id, or empty.
func (s *Service) DeviceID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deviceID
}

// VaultChoices lists the personal vault and every known team vault.
func (s *Service) VaultChoices() []TeamSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []TeamSummary{{
		ID: "", Name: "Personal", Kind: "personal", Sync: s.syncStatus,
	}}
	for _, team := range s.teams {
		sync := team.syncStatus
		if team.waiting {
			sync = "waiting"
		}
		out = append(out, TeamSummary{
			ID: team.id, Name: team.name, OrgID: team.orgID, Owner: team.owner,
			Waiting: team.waiting, Sync: sync, Kind: "team",
		})
	}
	return out
}

// ActiveVault is the selected team vault, or empty for the personal vault.
func (s *Service) ActiveVault() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// SetActiveVault selects the personal vault or a known team vault.
func (s *Service) SetActiveVault(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		s.active = ""
		return nil
	}
	if _, ok := s.teams[id]; !ok {
		return vaultcrypto.ErrDecrypt
	}
	s.active = id
	return nil
}

func (t *teamState) accountID(fallback string) string {
	if t.ownerID != "" {
		return t.ownerID
	}
	return fallback
}

// RememberTeam records a vault the server says this account can access.
func (s *Service) RememberTeam(id, name, orgID, ownerID string, owner bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil {
		team = &teamState{
			id: id, waiting: true, items: map[string]ItemBody{},
			envelopes: map[string][]byte{}, meta: map[string]itemMeta{},
		}
		s.teams[id] = team
	}
	team.name = name
	team.orgID = orgID
	team.ownerID = ownerID
	team.owner = owner
}

// PruneTeams drops local copies of team vaults the server no longer lists.
func (s *Service) PruneTeams(keep map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, team := range s.teams {
		if keep[id] {
			continue
		}
		vaultcrypto.Wipe(team.vaultKey)
		delete(s.teams, id)
		if s.active == id {
			s.active = ""
		}
	}
}

// SetTeamStatus records the last sync result for one team vault.
func (s *Service) SetTeamStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if team := s.teams[id]; team != nil {
		team.syncStatus = status
	}
}

// MarkTeamWaiting records that this device does not have the vault key yet.
func (s *Service) MarkTeamWaiting(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || len(team.vaultKey) == 32 {
		return
	}
	team.waiting = true
	team.syncStatus = "waiting"
}

// TeamHasKey reports whether the vault key is unlocked locally.
func (s *Service) TeamHasKey(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	return team != nil && len(team.vaultKey) == 32
}

// TeamGeneration is the vault-key generation installed locally.
func (s *Service) TeamGeneration(id string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if team := s.teams[id]; team != nil {
		return team.generation
	}
	return 0
}

// InstallTeamKey unlocks a team vault with a key opened from a device envelope.
func (s *Service) InstallTeamKey(id string, key []byte, generation int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || len(s.master) != 32 || len(key) != 32 {
		return vaultcrypto.ErrDecrypt
	}
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(s.master, key, s.account, s.instance, id)
	if err != nil {
		return err
	}
	replaced := !bytes.Equal(team.vaultKey, key)
	vaultcrypto.Wipe(team.vaultKey)
	team.vaultKey = append([]byte(nil), key...)
	team.wrap = wrap
	team.generation = generation
	team.waiting = false
	team.epoch++
	if replaced {
		team.cursor = 0
	}
	return reloadTeamLocked(s, team)
}

// SealTeamKey wraps the current team vault key to one recipient device.
func (s *Service) SealTeamKey(id, deviceID string, boxPub []byte) (SealedEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sealTeamLocked(id, deviceID, boxPub)
}

func (s *Service) sealTeamLocked(id, deviceID string, boxPub []byte) (SealedEnvelope, error) {
	team := s.teams[id]
	if team == nil || len(team.vaultKey) != 32 || s.deviceBox == nil || len(s.deviceSign) != ed25519.PrivateKeySize {
		return SealedEnvelope{}, vaultcrypto.ErrDecrypt
	}
	pub, err := ecdh.X25519().NewPublicKey(boxPub)
	if err != nil {
		return SealedEnvelope{}, vaultcrypto.ErrParams
	}
	env, err := vaultcrypto.SealForDevice(team.vaultKey, team.id, s.instance, uint64(team.generation), pub, s.deviceSign)
	if err != nil {
		return SealedEnvelope{}, err
	}
	raw, err := vaultcrypto.MarshalDeviceEnvelope(env)
	if err != nil {
		return SealedEnvelope{}, err
	}
	return SealedEnvelope{DeviceID: deviceID, Generation: team.generation, Ciphertext: raw, Signature: env.Signature}, nil
}

// OpenTeamEnvelope unwraps a server envelope addressed to this device.
func (s *Service) OpenTeamEnvelope(id string, packed []byte) ([]byte, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceBox == nil {
		return nil, 0, vaultcrypto.ErrDecrypt
	}
	env, err := vaultcrypto.UnmarshalDeviceEnvelope(packed)
	if err != nil {
		return nil, 0, err
	}
	if env.VaultID != id {
		return nil, 0, vaultcrypto.ErrDecrypt
	}
	key, err := vaultcrypto.OpenForDevice(env, s.instance, s.deviceBox)
	if err != nil {
		return nil, 0, err
	}
	return key, int64(env.Generation), nil
}

// CreateTeamKey generates the first key for a vault this account owns.
func (s *Service) CreateTeamKey(id string) (SealedEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || !team.owner || len(s.master) != 32 || s.deviceID == "" {
		return SealedEnvelope{}, vaultcrypto.ErrDecrypt
	}
	if len(team.vaultKey) == 32 {
		return s.sealTeamLocked(id, s.deviceID, s.deviceBox.PublicKey().Bytes())
	}
	key, err := vaultcrypto.NewMasterKey()
	if err != nil {
		return SealedEnvelope{}, err
	}
	team.generation = 1
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(s.master, key, s.account, s.instance, id)
	if err != nil {
		vaultcrypto.Wipe(key)
		return SealedEnvelope{}, err
	}
	team.vaultKey = key
	team.wrap = wrap
	team.waiting = false
	team.epoch++
	return s.sealTeamLocked(id, s.deviceID, s.deviceBox.PublicKey().Bytes())
}

// RotateTeam replaces the vault key, reseals items, and wraps the new key for the devices that remain.
func (s *Service) RotateTeam(id string, recipients []TeamRecipient) ([]SealedEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || !team.owner || len(team.vaultKey) != 32 || len(s.master) != 32 {
		return nil, vaultcrypto.ErrDecrypt
	}
	key, err := vaultcrypto.NewMasterKey()
	if err != nil {
		return nil, err
	}
	for itemID, item := range team.items {
		meta := team.meta[itemID]
		rev := meta.base + 1
		if rev <= meta.sealed {
			rev = meta.sealed + 1
		}
		raw, err := json.Marshal(item)
		if err != nil {
			vaultcrypto.Wipe(key)
			return nil, err
		}
		env, err := vaultcrypto.SealItem(key, vaultcrypto.AAD{
			AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: itemID, Revision: uint64(rev),
		}, raw)
		if err != nil {
			vaultcrypto.Wipe(key)
			return nil, err
		}
		team.envelopes[itemID] = env
		team.meta[itemID] = itemMeta{sealed: rev, base: meta.base, dirty: true, conflictAt: meta.conflictAt}
	}
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(s.master, key, s.account, s.instance, id)
	if err != nil {
		vaultcrypto.Wipe(key)
		return nil, err
	}
	vaultcrypto.Wipe(team.vaultKey)
	team.vaultKey = key
	team.wrap = wrap
	if team.generation < 1 {
		team.generation = 1
	}
	team.generation++
	team.epoch++
	var out []SealedEnvelope
	for _, rcpt := range recipients {
		sealed, err := s.sealTeamLocked(id, rcpt.DeviceID, rcpt.BoxPub)
		if err != nil {
			return nil, err
		}
		out = append(out, sealed)
	}
	return out, nil
}

// TeamSnapshot copies one team vault for reconcile. The account wrap is not included.
func (s *Service) TeamSnapshot(id string) (int64, vaultsync.Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || len(team.vaultKey) != 32 {
		return 0, vaultsync.Snapshot{}, false
	}
	snap := vaultsync.Snapshot{VaultID: team.id, Cursor: team.cursor}
	for itemID, meta := range team.meta {
		item := vaultsync.LocalItem{
			ID: itemID, SealedRevision: meta.sealed, BaseRevision: meta.base,
			Dirty: meta.dirty, PendingDelete: meta.pendingDelete, ConflictAt: meta.conflictAt,
		}
		if env, ok := team.envelopes[itemID]; ok {
			item.Ciphertext = append([]byte(nil), env...)
		}
		snap.Items = append(snap.Items, item)
	}
	return team.epoch, snap, true
}

// ApplyTeam stores a reconcile result for one team vault.
func (s *Service) ApplyTeam(id string, epoch int64, result vaultsync.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team := s.teams[id]
	if team == nil || epoch != team.epoch || len(team.vaultKey) != 32 {
		if team != nil && epoch != team.epoch {
			return ErrEpoch
		}
		return vaultcrypto.ErrDecrypt
	}
	for _, itemID := range result.Removed {
		delete(team.items, itemID)
		delete(team.envelopes, itemID)
		delete(team.meta, itemID)
	}
	for _, item := range result.Pulled {
		if item.Deleted {
			delete(team.items, item.ID)
			delete(team.envelopes, item.ID)
			delete(team.meta, item.ID)
			continue
		}
		if err := installTeamLocked(s, team, item.ID, item.Revision, item.Ciphertext); err != nil {
			if teamCipherUnreadable(err) {
				continue
			}
			return err
		}
	}
	for itemID, rev := range result.Pushed {
		meta, ok := team.meta[itemID]
		if !ok {
			continue
		}
		if meta.pendingDelete {
			delete(team.items, itemID)
			delete(team.envelopes, itemID)
			delete(team.meta, itemID)
			continue
		}
		meta.base = rev
		meta.dirty = false
		meta.conflictAt = 0
		if env := team.envelopes[itemID]; len(env) > 0 {
			if plain, err := openTeamLocked(s, team, itemID, rev, env); err == nil {
				vaultcrypto.Wipe(plain)
				meta.sealed = rev
			}
		}
		team.meta[itemID] = meta
	}
	for _, item := range result.Conflicts {
		meta := team.meta[item.ID]
		if meta.conflictAt > 0 && meta.conflictAt >= item.Revision {
			continue
		}
		if !item.Deleted && len(item.Ciphertext) > 0 {
			if err := conflictTeamLocked(s, team, item.ID, item.Revision, item.Ciphertext); err != nil {
				if teamCipherUnreadable(err) {
					continue
				}
				return err
			}
		}
		if meta, ok := team.meta[item.ID]; ok {
			meta.conflictAt = item.Revision
			team.meta[item.ID] = meta
		}
	}
	team.cursor = result.Cursor
	team.syncStatus = "synced"
	for _, meta := range team.meta {
		if meta.conflictAt > 0 {
			team.syncStatus = "conflict"
			break
		}
	}
	if len(result.Conflicts) > 0 {
		team.syncStatus = "conflict"
	}
	return nil
}

func installTeamLocked(s *Service, team *teamState, id string, rev int64, ciphertext []byte) error {
	plain, migrated, err := openTeamForInstall(s, team, id, rev, ciphertext)
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
	item.VaultID = team.id
	if !migrated {
		team.items[id] = item
		team.envelopes[id] = append([]byte(nil), ciphertext...)
		team.meta[id] = itemMeta{sealed: rev, base: rev}
		return nil
	}
	next := rev + 1
	if next < 1 {
		next = 1
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	env, err := vaultcrypto.SealItem(team.vaultKey, vaultcrypto.AAD{
		AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: id, Revision: uint64(next),
	}, raw)
	if err != nil {
		return err
	}
	team.items[id] = item
	team.envelopes[id] = env
	team.meta[id] = itemMeta{sealed: next, base: rev, dirty: true}
	return nil
}

func openTeamForInstall(s *Service, team *teamState, id string, rev int64, ciphertext []byte) ([]byte, bool, error) {
	plain, err := openTeamLocked(s, team, id, rev, ciphertext)
	if err == nil || !teamCipherUnreadable(err) || s.instance == legacyTeamInstance {
		return plain, false, err
	}
	plain, err = vaultcrypto.OpenItem(team.vaultKey, vaultcrypto.AAD{
		AccountID: team.accountID(s.account), InstanceID: legacyTeamInstance, VaultID: team.id, ItemID: id, Revision: uint64(rev),
	}, ciphertext)
	if err != nil {
		return nil, false, err
	}
	return plain, true, nil
}

func teamCipherUnreadable(err error) bool {
	return errors.Is(err, vaultcrypto.ErrDecrypt) || errors.Is(err, vaultcrypto.ErrFormat)
}

func openTeamLocked(s *Service, team *teamState, id string, rev int64, ciphertext []byte) ([]byte, error) {
	return vaultcrypto.OpenItem(team.vaultKey, vaultcrypto.AAD{
		AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: id, Revision: uint64(rev),
	}, ciphertext)
}

func conflictTeamLocked(s *Service, team *teamState, remoteID string, rev int64, ciphertext []byte) error {
	plain, err := openTeamLocked(s, team, remoteID, rev, ciphertext)
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
	item.VaultID = team.id
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	env, err := vaultcrypto.SealItem(team.vaultKey, vaultcrypto.AAD{
		AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: item.ID, Revision: 1,
	}, raw)
	if err != nil {
		return err
	}
	team.items[item.ID] = item
	team.envelopes[item.ID] = env
	team.meta[item.ID] = itemMeta{sealed: 1, dirty: true}
	return nil
}

func (s *Service) putTeamItemLocked(item ItemBody) (int64, error) {
	team := s.teams[s.active]
	if team == nil || team.waiting || len(team.vaultKey) != 32 {
		return 0, vaultcrypto.ErrDecrypt
	}
	if item.ID == "" {
		item.ID = randomID()
	}
	if item.Kind == "" {
		item.Kind = "login"
	}
	if !knownKind(item.Kind) {
		return 0, vaultcrypto.ErrParams
	}
	item.VaultID = team.id
	meta := team.meta[item.ID]
	rev := meta.base + 1
	if rev < 1 {
		rev = 1
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return 0, err
	}
	env, err := vaultcrypto.SealItem(team.vaultKey, vaultcrypto.AAD{
		AccountID: team.accountID(s.account), InstanceID: s.instance, VaultID: team.id, ItemID: item.ID, Revision: uint64(rev),
	}, raw)
	if err != nil {
		return 0, err
	}
	team.envelopes[item.ID] = env
	team.items[item.ID] = item
	team.meta[item.ID] = itemMeta{sealed: rev, base: meta.base, dirty: true}
	team.epoch++
	s.lastActivity = time.Now()
	return rev, nil
}

func (s *Service) getTeamItemLocked(id string) (ItemBody, error) {
	team := s.teams[s.active]
	if team == nil || len(team.vaultKey) != 32 {
		return ItemBody{}, vaultcrypto.ErrDecrypt
	}
	item, ok := team.items[id]
	if !ok {
		return ItemBody{}, vaultcrypto.ErrDecrypt
	}
	s.lastActivity = time.Now()
	return item, nil
}

func (s *Service) deleteTeamItemLocked(id string) error {
	team := s.teams[s.active]
	if team == nil || len(team.vaultKey) != 32 {
		return vaultcrypto.ErrDecrypt
	}
	meta := team.meta[id]
	if _, ok := team.items[id]; !ok && !meta.pendingDelete {
		return vaultcrypto.ErrDecrypt
	}
	if meta.base == 0 {
		delete(team.items, id)
		delete(team.envelopes, id)
		delete(team.meta, id)
	} else {
		delete(team.items, id)
		delete(team.envelopes, id)
		meta.pendingDelete = true
		meta.dirty = true
		team.meta[id] = meta
	}
	team.epoch++
	s.lastActivity = time.Now()
	return nil
}
