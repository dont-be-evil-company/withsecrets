// Package app is the desktop service layer. It keeps unlocked key material
// in process memory and does not expose it to the webview.
package app

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mistweaverco/withsecrets/internal/vaultcrypto"
	"github.com/mistweaverco/withsecrets/internal/vaulthardware/doctor"
)

// ItemBody is the decrypted item kept only while the vault is unlocked.
type ItemBody struct {
	ID       string            `json:"id"`
	VaultID  string            `json:"vaultId"`
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Username string            `json:"username"`
	Password string            `json:"password"`
	URL      string            `json:"url"`
	Notes    string            `json:"notes"`
	TOTP     string            `json:"totp"`
	Favorite bool              `json:"favorite"`
	Tags     []string          `json:"tags"`
	Fields   map[string]string `json:"fields,omitempty"`
}

// Summary is safe to list. It omits secrets.
type Summary struct {
	ID       string   `json:"id"`
	VaultID  string   `json:"vaultId"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Username string   `json:"username"`
	Favorite bool     `json:"favorite"`
	Tags     []string `json:"tags"`
}

// Profile is one self-hosted server the desktop knows about.
type Profile struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Service is the native bridge. Methods that return secrets are explicit.
type Service struct {
	mu            sync.Mutex
	profiles      []Profile
	master        []byte
	vaultKey      []byte
	vaultWrap     []byte
	items         map[string]ItemBody
	envelopes     map[string][]byte
	vaultID       string
	account       string
	instance      string
	meta          map[string]itemMeta
	epoch         int64
	cursor        int64
	syncStatus    string
	active        string
	teams         map[string]*teamState
	deviceID      string
	deviceWrap    []byte
	deviceBox     *ecdh.PrivateKey
	deviceSign    ed25519.PrivateKey
	policy        string
	fido2         bool
	piv           bool
	recoveryReady bool
	fidoCount     int
	settings      DesktopSettings
	lastActivity  time.Time
}

// SetupResult is the one-time outcome of starting recovery in this process.
// RecoveryKey is shown once. The slots stay in the desktop process.
type SetupResult struct {
	Policy      string
	RecoveryKey string
	PassSlot    vaultcrypto.UnlockSlot
	RecSlot     vaultcrypto.UnlockSlot
	HasPass     bool
}

// New returns a locked service with the default server profile.
func New() *Service {
	return &Service{
		profiles:     []Profile{{Name: "withsecrets", URL: "https://vault.withsecrets.com"}},
		items:        map[string]ItemBody{},
		envelopes:    map[string][]byte{},
		meta:         map[string]itemMeta{},
		teams:        map[string]*teamState{},
		vaultID:      "personal",
		account:      "local",
		instance:     "local-dev",
		settings:     DefaultSettings(),
		lastActivity: time.Now(),
	}
}

// GetServerProfiles lists configured servers. Sessions are not shared across them.
func (s *Service) GetServerProfiles() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	return out
}

// AddProfile records another server. It does not copy keys from the current one.
func (s *Service) AddProfile(name, rawURL string) error {
	if name == "" || rawURL == "" {
		return vaultcrypto.ErrParams
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = append(s.profiles, Profile{Name: name, URL: rawURL})
	return nil
}

// UnlockWithPassphrase unwraps an account created by BootstrapPassphrase.
func (s *Service) UnlockWithPassphrase(passphrase string, slot vaultcrypto.UnlockSlot) error {
	master, err := vaultcrypto.UnwrapPassphrase(slot, passphrase)
	if err != nil {
		return err
	}
	return s.unlockMaster(master)
}

// UnlockWithRecovery unwraps an account created by BeginSetup.
func (s *Service) UnlockWithRecovery(key string, slot vaultcrypto.UnlockSlot) error {
	raw, err := parseRecoveryKey(key)
	if err != nil {
		return err
	}
	defer vaultcrypto.Wipe(raw)
	master, err := vaultcrypto.UnwrapRecovery(slot, raw)
	if err != nil {
		return err
	}
	return s.unlockMaster(master)
}

func (s *Service) unlockMaster(master []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.vaultWrap) == 0 {
		vaultcrypto.Wipe(s.master)
		vaultcrypto.Wipe(s.vaultKey)
		s.master = master
		s.vaultKey = nil
		s.items = map[string]ItemBody{}
		s.lastActivity = time.Now()
		s.openDeviceLocked()
		s.openTeamsLocked()
		return nil
	}
	vaultKey, err := vaultcrypto.UnwrapVaultKeyForAccount(master, s.vaultWrap, s.account, s.instance, s.vaultID)
	if err != nil {
		vaultcrypto.Wipe(master)
		return err
	}
	vaultcrypto.Wipe(s.master)
	vaultcrypto.Wipe(s.vaultKey)
	s.master = master
	s.vaultKey = vaultKey
	s.lastActivity = time.Now()
	s.openDeviceLocked()
	s.openTeamsLocked()
	return s.reloadLocked()
}

// ItemSync is the per-item sync record stored with ciphertext. It has no plaintext.
type ItemSync struct {
	Sealed        int64
	Base          int64
	Dirty         bool
	PendingDelete bool
	ConflictAt    int64
}

// VaultState is the locked vault as stored on disk. It has no master key and no item plaintext.
type VaultState struct {
	Wrap          []byte
	Envelopes     map[string][]byte
	Items         map[string]ItemSync
	Policy        string
	RecoveryReady bool
	FidoCount     int
	Account       string
	Instance      string
	VaultID       string
	Cursor        int64
	SyncStatus    string
	DeviceID      string
	DeviceWrap    []byte
	Teams         []TeamState
}

// ExportVault copies the locked vault. The master key and decrypted items are not included.
func (s *Service) ExportVault() VaultState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := VaultState{
		Wrap:          append([]byte(nil), s.vaultWrap...),
		Envelopes:     map[string][]byte{},
		Items:         map[string]ItemSync{},
		Policy:        s.policy,
		RecoveryReady: s.recoveryReady,
		FidoCount:     s.fidoCount,
		Account:       s.account,
		Instance:      s.instance,
		VaultID:       s.vaultID,
		Cursor:        s.cursor,
		SyncStatus:    s.syncStatus,
		DeviceID:      s.deviceID,
		DeviceWrap:    append([]byte(nil), s.deviceWrap...),
		Teams:         exportTeamsLocked(s),
	}
	for id, env := range s.envelopes {
		out.Envelopes[id] = append([]byte(nil), env...)
	}
	for id, meta := range s.meta {
		out.Items[id] = ItemSync{
			Sealed: meta.sealed, Base: meta.base, Dirty: meta.dirty,
			PendingDelete: meta.pendingDelete, ConflictAt: meta.conflictAt,
		}
	}
	return out
}

// ImportVault loads a locked vault. It does not unlock it.
func (s *Service) ImportVault(in VaultState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultcrypto.Wipe(s.master)
	vaultcrypto.Wipe(s.vaultKey)
	s.master = nil
	s.vaultKey = nil
	s.items = map[string]ItemBody{}
	s.vaultWrap = append([]byte(nil), in.Wrap...)
	s.envelopes = map[string][]byte{}
	for id, env := range in.Envelopes {
		s.envelopes[id] = append([]byte(nil), env...)
	}
	s.meta = map[string]itemMeta{}
	if len(in.Items) == 0 {
		for id := range s.envelopes {
			s.meta[id] = itemMeta{sealed: 1, dirty: true}
		}
	} else {
		for id, meta := range in.Items {
			sealed := meta.Sealed
			if sealed == 0 {
				sealed = 1
			}
			s.meta[id] = itemMeta{
				sealed: sealed, base: meta.Base, dirty: meta.Dirty,
				pendingDelete: meta.PendingDelete, conflictAt: meta.ConflictAt,
			}
		}
	}
	if in.Account != "" {
		s.account = in.Account
	}
	if in.Instance != "" {
		s.instance = in.Instance
	}
	if in.VaultID != "" {
		s.vaultID = in.VaultID
	}
	s.cursor = in.Cursor
	s.syncStatus = in.SyncStatus
	s.policy = in.Policy
	s.recoveryReady = in.RecoveryReady
	s.fidoCount = in.FidoCount
	s.deviceID = in.DeviceID
	s.deviceWrap = append([]byte(nil), in.DeviceWrap...)
	s.teams = map[string]*teamState{}
	for _, team := range in.Teams {
		s.teams[team.ID] = importTeam(team)
	}
}

// Lock wipes unlocked key material and decrypted items.
func (s *Service) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultcrypto.Wipe(s.master)
	vaultcrypto.Wipe(s.vaultKey)
	s.master = nil
	s.vaultKey = nil
	s.items = map[string]ItemBody{}
	s.lockTeamsLocked()
}

// Unlocked reports whether item secrets can be revealed.
func (s *Service) Unlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.master) == 32
}

// PutDecryptedItem encrypts an item under the vault key. The service must be unlocked.
func (s *Service) PutDecryptedItem(item ItemBody) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return s.putTeamItemLocked(item)
	}
	if len(s.vaultKey) != 32 {
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
	item.VaultID = s.vaultID
	meta := s.meta[item.ID]
	rev := meta.base + 1
	if rev < 1 {
		rev = 1
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return 0, err
	}
	env, err := vaultcrypto.SealItem(s.vaultKey, vaultcrypto.AAD{
		AccountID: s.account, InstanceID: s.instance, VaultID: item.VaultID, ItemID: item.ID, Revision: uint64(rev),
	}, raw)
	if err != nil {
		return 0, err
	}
	if s.meta == nil {
		s.meta = map[string]itemMeta{}
	}
	s.envelopes[item.ID] = env
	s.items[item.ID] = item
	s.meta[item.ID] = itemMeta{sealed: rev, base: meta.base, dirty: true}
	s.epoch++
	s.lastActivity = time.Now()
	return rev, nil
}

// GetItem returns one decrypted item while the vault is unlocked.
func (s *Service) GetItem(id string) (ItemBody, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return s.getTeamItemLocked(id)
	}
	if len(s.vaultKey) != 32 {
		return ItemBody{}, vaultcrypto.ErrDecrypt
	}
	item, ok := s.items[id]
	if !ok {
		return ItemBody{}, vaultcrypto.ErrDecrypt
	}
	s.lastActivity = time.Now()
	return item, nil
}

// DeleteItem removes one item from the unlocked vault.
func (s *Service) DeleteItem(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return s.deleteTeamItemLocked(id)
	}
	if len(s.vaultKey) != 32 {
		return vaultcrypto.ErrDecrypt
	}
	meta := s.meta[id]
	if _, ok := s.items[id]; !ok && !meta.pendingDelete {
		return vaultcrypto.ErrDecrypt
	}
	if meta.base == 0 {
		delete(s.items, id)
		delete(s.envelopes, id)
		delete(s.meta, id)
	} else {
		delete(s.items, id)
		delete(s.envelopes, id)
		meta.pendingDelete = true
		meta.dirty = true
		s.meta[id] = meta
	}
	s.epoch++
	s.lastActivity = time.Now()
	return nil
}

func knownKind(kind string) bool {
	switch kind {
	case "login", "note", "card", "identity", "token", "wifi":
		return true
	default:
		return false
	}
}

func (s *Service) reloadLocked() error {
	s.items = map[string]ItemBody{}
	for id, env := range s.envelopes {
		rev := s.meta[id].sealed
		if rev == 0 {
			rev = 1
		}
		pt, err := vaultcrypto.OpenItem(s.vaultKey, vaultcrypto.AAD{
			AccountID: s.account, InstanceID: s.instance, VaultID: s.vaultID, ItemID: id, Revision: uint64(rev),
		}, env)
		if err != nil {
			return err
		}
		var item ItemBody
		if err := json.Unmarshal(pt, &item); err != nil {
			vaultcrypto.Wipe(pt)
			return err
		}
		vaultcrypto.Wipe(pt)
		s.items[id] = item
	}
	return nil
}

// Search returns summaries. The query is matched locally.
func (s *Service) Search(query string) []Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items
	if s.active != "" {
		items = map[string]ItemBody{}
		if team := s.teams[s.active]; team != nil {
			items = team.items
		}
	}
	out := []Summary{}
	for _, item := range items {
		if query != "" && !contains(item.Title, query) && !contains(item.Username, query) && !hasTag(item.Tags, query) {
			continue
		}
		out = append(out, Summary{
			ID: item.ID, VaultID: item.VaultID, Kind: item.Kind, Title: item.Title,
			Username: item.Username, Favorite: item.Favorite, Tags: item.Tags,
		})
	}
	return out
}

// RevealField returns one secret field while unlocked.
func (s *Service) RevealField(id, field string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) != 32 {
		return "", vaultcrypto.ErrDecrypt
	}
	items := s.items
	if s.active != "" {
		team := s.teams[s.active]
		if team == nil {
			return "", vaultcrypto.ErrDecrypt
		}
		items = team.items
	}
	item, ok := items[id]
	if !ok {
		return "", vaultcrypto.ErrDecrypt
	}
	switch field {
	case "password":
		return item.Password, nil
	case "notes":
		return item.Notes, nil
	case "totp":
		return item.TOTP, nil
	default:
		return "", vaultcrypto.ErrParams
	}
}

// GeneratePassword delegates to the crypto package.
func (s *Service) GeneratePassword(length int) (string, error) {
	return vaultcrypto.GeneratePassword(length, true, true, true, true)
}

// Doctor reports hardware build capabilities.
func (s *Service) Doctor() doctor.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	return doctor.Inspect(s.fido2, s.piv)
}

// SetRecoveryPolicy records the recovery policy after the slots exist.
func (s *Service) SetRecoveryPolicy(policy string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = policy
}

// EnrollFIDOSlot wraps the account key with one authenticator's hmac-secret.
// hmacFn runs while the key is touched. The slot is verified before it is returned.
func (s *Service) EnrollFIDOSlot(credentialID []byte, hmacFn vaultcrypto.HMACSecretFunc) (vaultcrypto.UnlockSlot, error) {
	if len(credentialID) == 0 || hmacFn == nil {
		return vaultcrypto.UnlockSlot{}, vaultcrypto.ErrParams
	}
	if err := s.createAccount(); err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	master, err := s.masterCopy()
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(master)
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(salt)
	s.mu.Lock()
	n := s.fidoCount + 1
	s.mu.Unlock()
	slot, err := vaultcrypto.EnrollFIDO2(master, vaultcrypto.SlotContext{
		AccountID: "local", InstanceID: "local-dev", GenerationID: "g1",
		SlotID: fmt.Sprintf("fido-%d", n), Label: "Security key",
	}, credentialID, salt, hmacFn)
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	s.mu.Lock()
	s.fidoCount = n
	s.mu.Unlock()
	return slot, nil
}

// UnlockWithFIDO unwraps a slot enrolled by EnrollFIDOSlot.
func (s *Service) UnlockWithFIDO(slot vaultcrypto.UnlockSlot, hmacFn vaultcrypto.HMACSecretFunc) error {
	master, err := vaultcrypto.UnwrapFIDO2(slot, hmacFn)
	if err != nil {
		return err
	}
	return s.unlockMaster(master)
}

// SetHardwareTags records how this binary was built. It does not probe USB.
func (s *Service) SetHardwareTags(fido2, piv bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fido2 = fido2
	s.piv = piv
}

// BootstrapPassphrase creates a local account and vault for the desktop process.
func (s *Service) BootstrapPassphrase(passphrase string) (vaultcrypto.UnlockSlot, error) {
	if err := s.createAccount(); err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	master, err := s.masterCopy()
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	defer vaultcrypto.Wipe(master)
	slot, err := vaultcrypto.EnrollPassphrase(master, vaultcrypto.SlotContext{
		AccountID: "local", InstanceID: "local-dev", GenerationID: "g1", SlotID: "pass", Label: "Passphrase",
	}, passphrase, vaultcrypto.TestArgon2(), true)
	if err != nil {
		return vaultcrypto.UnlockSlot{}, err
	}
	s.mu.Lock()
	s.policy = vaultcrypto.PolicyPassphrase
	s.mu.Unlock()
	return slot, nil
}

// BeginSetup starts the chosen recovery policy in this window.
// Hardware-only is refused here: two tokens have to verify before that policy exists.
// The printed recovery key is returned once.
func (s *Service) BeginSetup(policy, passphrase string) (SetupResult, error) {
	switch policy {
	case "recommended", "passphrase":
	default:
		return SetupResult{}, vaultcrypto.ErrParams
	}
	s.mu.Lock()
	haveMaster := len(s.master) == 32
	haveRec := s.recoveryReady
	s.mu.Unlock()
	if haveRec {
		return SetupResult{}, vaultcrypto.ErrParams
	}
	result := SetupResult{}
	if policy == "passphrase" {
		if len(passphrase) < 8 {
			return SetupResult{}, vaultcrypto.ErrParams
		}
		if !haveMaster {
			slot, err := s.BootstrapPassphrase(passphrase)
			if err != nil {
				return SetupResult{}, err
			}
			result.PassSlot = slot
			result.HasPass = true
			haveMaster = true
		}
	}
	if !haveMaster {
		if err := s.createAccount(); err != nil {
			return SetupResult{}, err
		}
	}
	secret, err := vaultcrypto.NewRecoverySecret()
	if err != nil {
		return SetupResult{}, err
	}
	defer vaultcrypto.Wipe(secret)
	master, err := s.masterCopy()
	if err != nil {
		return SetupResult{}, err
	}
	defer vaultcrypto.Wipe(master)
	rec, err := vaultcrypto.EnrollRecovery(master, secret, vaultcrypto.SlotContext{
		AccountID: "local", InstanceID: "local-dev", GenerationID: "g1", SlotID: "recovery", Label: "Printed recovery",
	})
	if err != nil {
		return SetupResult{}, err
	}
	stored := vaultcrypto.PolicyHardwarePlusRecovery
	if policy == "passphrase" {
		stored = vaultcrypto.PolicyPassphrase
	}
	s.mu.Lock()
	s.recoveryReady = true
	s.policy = stored
	s.mu.Unlock()
	result.Policy = stored
	result.RecoveryKey = formatRecoveryKey(secret)
	result.RecSlot = rec
	return result, nil
}

func (s *Service) createAccount() error {
	master, err := vaultcrypto.NewMasterKey()
	if err != nil {
		return err
	}
	vaultKey, err := vaultcrypto.NewMasterKey()
	if err != nil {
		vaultcrypto.Wipe(master)
		return err
	}
	wrap, err := vaultcrypto.WrapVaultKeyForAccount(master, vaultKey, "local", "local-dev", "personal")
	if err != nil {
		vaultcrypto.Wipe(master)
		vaultcrypto.Wipe(vaultKey)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) == 32 {
		vaultcrypto.Wipe(master)
		vaultcrypto.Wipe(vaultKey)
		vaultcrypto.Wipe(wrap)
		return nil
	}
	s.master = master
	s.vaultKey = vaultKey
	s.vaultWrap = wrap
	return nil
}

func (s *Service) masterCopy() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) != 32 {
		return nil, vaultcrypto.ErrDecrypt
	}
	return append([]byte(nil), s.master...), nil
}

func formatRecoveryKey(raw []byte) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	var b strings.Builder
	for i := 0; i < len(enc); i += 4 {
		if i > 0 {
			b.WriteByte('-')
		}
		end := i + 4
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
	}
	return b.String()
}

func parseRecoveryKey(in string) ([]byte, error) {
	compact := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in), "-", ""))
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(compact)
	if err != nil || len(raw) != 32 {
		return nil, vaultcrypto.ErrDecrypt
	}
	return raw, nil
}

func randomID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 16)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

func contains(s, q string) bool {
	return len(q) == 0 || (len(s) >= len(q) && (s == q || indexFold(s, q)))
}

func indexFold(s, q string) bool {
	for i := 0; i+len(q) <= len(s); i++ {
		if equalFold(s[i:i+len(q)], q) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func hasTag(tags []string, q string) bool {
	for _, tag := range tags {
		if equalFold(tag, q) {
			return true
		}
	}
	return false
}
