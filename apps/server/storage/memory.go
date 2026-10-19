package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Memory is an in-process store used by tests and by local runs without PostgreSQL.
type Memory struct {
	mu        sync.Mutex
	instance  string
	users     map[string]User
	idents    map[string]Identity // issuer|subject
	txs       map[string]Transaction
	grants    map[string]Grant
	sessions  map[string]Session // by id
	byAccess  map[string]string
	byRefresh map[string]string
	devices   map[string]Device
	accounts  map[string]Account // user id
	slots     map[string]Slot
	gens      []KeyGeneration
	vaults    map[string]Vault
	members   map[string]Membership // vault|principal
	items     map[string]Item
	history   map[string][]ItemVersion
	idem      map[string]string // vault|key -> item
	orgs      map[string]Org
	orgRoles  map[string]string // org|user
	invites   map[string]Invite
	envelopes []Envelope
	chunks    map[string]Chunk // attachment|n
	changes   []Change
	audits    []Audit
	seq       int64
}

// NewMemory returns an empty store with a stable instance id.
func NewMemory() *Memory {
	return &Memory{
		instance:  newID(),
		users:     map[string]User{},
		idents:    map[string]Identity{},
		txs:       map[string]Transaction{},
		grants:    map[string]Grant{},
		sessions:  map[string]Session{},
		byAccess:  map[string]string{},
		byRefresh: map[string]string{},
		devices:   map[string]Device{},
		accounts:  map[string]Account{},
		slots:     map[string]Slot{},
		vaults:    map[string]Vault{},
		members:   map[string]Membership{},
		items:     map[string]Item{},
		history:   map[string][]ItemVersion{},
		idem:      map[string]string{},
		orgs:      map[string]Org{},
		orgRoles:  map[string]string{},
		invites:   map[string]Invite{},
		chunks:    map[string]Chunk{},
	}
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (m *Memory) Ready(context.Context) error { return nil }

func (m *Memory) InstanceID(context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instance, nil
}

func (m *Memory) CreateTransaction(_ context.Context, tx Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.txs[tx.ID] = tx
	return nil
}

func (m *Memory) Transaction(_ context.Context, id string) (Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, ok := m.txs[id]
	if !ok {
		return Transaction{}, ErrNotFound
	}
	return tx, nil
}

func (m *Memory) ConsumeTransaction(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, ok := m.txs[id]
	if !ok {
		return ErrNotFound
	}
	if tx.Consumed {
		return ErrConsumed
	}
	tx.Consumed = true
	m.txs[id] = tx
	return nil
}

func (m *Memory) UpsertIdentity(_ context.Context, in Identity, display string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := in.Issuer + "|" + in.Subject
	if existing, ok := m.idents[key]; ok {
		return existing.UserID, nil
	}
	id := newID()
	m.users[id] = User{ID: id, DisplayName: display}
	in.UserID = id
	m.idents[key] = in
	return id, nil
}

func (m *Memory) PutGrant(_ context.Context, g Grant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grants[g.CodeHash] = g
	return nil
}

func (m *Memory) ConsumeGrant(_ context.Context, codeHash, pkce, stateHash string, now time.Time) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.grants[codeHash]
	if !ok {
		return "", ErrNotFound
	}
	if g.Used {
		return "", ErrConsumed
	}
	if now.After(g.Expires) {
		return "", ErrExpired
	}
	if g.PKCEChallenge != pkce || g.StateHash != stateHash {
		return "", ErrMismatch
	}
	g.Used = true
	m.grants[codeHash] = g
	return g.UserID, nil
}

func (m *Memory) PutSession(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	m.byAccess[s.AccessHash] = s.ID
	m.byRefresh[s.RefreshHash] = s.ID
	return nil
}

func (m *Memory) SessionByAccess(_ context.Context, accessHash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byAccess[accessHash]
	if !ok {
		return Session{}, ErrNotFound
	}
	s := m.sessions[id]
	if s.Revoked || s.FamilyRevoked {
		return Session{}, ErrRevoked
	}
	return s, nil
}

func (m *Memory) SessionByRefresh(_ context.Context, refreshHash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byRefresh[refreshHash]
	if !ok {
		return Session{}, ErrNotFound
	}
	return m.sessions[id], nil
}

func (m *Memory) RotateRefresh(_ context.Context, oldRefreshHash string, next Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byRefresh[oldRefreshHash]
	if !ok {
		return ErrNotFound
	}
	old := m.sessions[id]
	if old.Revoked || old.FamilyRevoked {
		m.revokeFamilyLocked(old.FamilyID)
		return ErrRevoked
	}
	if time.Now().After(old.RefreshExp) {
		return ErrExpired
	}
	old.Revoked = true
	m.sessions[id] = old
	delete(m.byAccess, old.AccessHash)
	next.FamilyID = old.FamilyID
	next.UserID = old.UserID
	m.sessions[next.ID] = next
	m.byAccess[next.AccessHash] = next.ID
	m.byRefresh[next.RefreshHash] = next.ID
	return nil
}

func (m *Memory) revokeFamilyLocked(familyID string) {
	for id, s := range m.sessions {
		if s.FamilyID == familyID {
			s.FamilyRevoked = true
			s.Revoked = true
			m.sessions[id] = s
		}
	}
}

func (m *Memory) RevokeFamily(_ context.Context, familyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokeFamilyLocked(familyID)
	return nil
}

func (m *Memory) PutDevice(_ context.Context, d Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[d.ID] = d
	return nil
}

func (m *Memory) Device(_ context.Context, id string) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	if !ok {
		return Device{}, ErrNotFound
	}
	return d, nil
}

func (m *Memory) ListDevices(_ context.Context, userID string) ([]Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Device
	for _, d := range m.devices {
		if d.UserID == userID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *Memory) SetDeviceStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	if !ok {
		return ErrNotFound
	}
	d.Status = status
	m.devices[id] = d
	return nil
}

func (m *Memory) EnsureAccount(_ context.Context, a Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[a.UserID]; ok {
		return nil
	}
	m.accounts[a.UserID] = a
	return nil
}

func (m *Memory) Account(_ context.Context, userID string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	return a, nil
}

func (m *Memory) SetAccountPolicy(_ context.Context, userID, policy, generationID string, sequence uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[userID]
	if !ok {
		return ErrNotFound
	}
	if sequence < a.Sequence || (sequence == a.Sequence && a.GenerationID != "" && generationID != a.GenerationID) {
		return ErrMismatch
	}
	a.RecoveryPolicy = policy
	a.GenerationID = generationID
	a.Sequence = sequence
	m.accounts[userID] = a
	return nil
}

func (m *Memory) PutSlot(_ context.Context, s Slot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Revision == 0 {
		s.Revision = 1
	}
	if prev, ok := m.slots[s.ID]; ok && prev.AccountID != s.AccountID {
		return ErrNotFound
	}
	m.slots[s.ID] = s
	return nil
}

func (m *Memory) ListSlots(_ context.Context, accountID string) ([]Slot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Slot
	for _, s := range m.slots {
		if s.AccountID == accountID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *Memory) Slot(_ context.Context, id string) (Slot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.slots[id]
	if !ok {
		return Slot{}, ErrNotFound
	}
	return s, nil
}

func (m *Memory) SetSlotState(_ context.Context, id, state string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.slots[id]
	if !ok {
		return ErrNotFound
	}
	s.State = state
	s.Revision++
	m.slots[id] = s
	return nil
}

func (m *Memory) PutGeneration(_ context.Context, g KeyGeneration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gens = append(m.gens, g)
	return nil
}

func (m *Memory) Generations(_ context.Context, accountID string) ([]KeyGeneration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []KeyGeneration
	for _, g := range m.gens {
		if g.AccountID == accountID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (m *Memory) CreateVault(_ context.Context, v Vault) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v.Revision == 0 {
		v.Revision = 1
	}
	m.vaults[v.ID] = v
	m.addChangeLocked(Change{UserID: v.OwnerID, VaultID: v.ID, Kind: "vault", ObjectID: v.ID, Revision: v.Revision})
	return nil
}

func (m *Memory) ListVaults(_ context.Context, userID string) ([]Vault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Vault
	for _, v := range m.vaults {
		if m.canSeeLocked(v, userID) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (m *Memory) canSeeLocked(v Vault, userID string) bool {
	if v.OwnerID == userID {
		return true
	}
	_, ok := m.members[v.ID+"|"+userID]
	return ok
}

func (m *Memory) Vault(_ context.Context, id string) (Vault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vaults[id]
	if !ok {
		return Vault{}, ErrNotFound
	}
	v.AccountKeyWrap = append([]byte(nil), v.AccountKeyWrap...)
	return v, nil
}

func (m *Memory) SetAccountKeyWrap(_ context.Context, id string, wrap []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vaults[id]
	if !ok || v.OrgID != "" {
		return ErrNotFound
	}
	v.AccountKeyWrap = append([]byte(nil), wrap...)
	m.vaults[id] = v
	return nil
}

func (m *Memory) DeleteVault(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vaults[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.vaults, id)
	m.addChangeLocked(Change{UserID: v.OwnerID, VaultID: id, Kind: "vault", ObjectID: id, Revision: v.Revision + 1})
	return nil
}

func (m *Memory) PutMembership(_ context.Context, mem Membership) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.members[mem.VaultID+"|"+mem.PrincipalID] = mem
	return nil
}

func (m *Memory) DeleteMembership(_ context.Context, vaultID, principalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.members, vaultID+"|"+principalID)
	return nil
}

func (m *Memory) Membership(_ context.Context, vaultID, principalID string) (Membership, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mem, ok := m.members[vaultID+"|"+principalID]
	return mem, ok
}

func (m *Memory) PutItem(_ context.Context, item Item, expected int64) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.Idempotency != "" {
		if id, ok := m.idem[item.VaultID+"|"+item.Idempotency]; ok {
			return m.items[id], nil
		}
	}
	if existing, ok := m.items[item.ID]; ok {
		if existing.Revision != expected {
			return Item{}, &ConflictError{Current: existing.Revision}
		}
		existing.Ciphertext = append([]byte(nil), item.Ciphertext...)
		existing.CryptoVersion = item.CryptoVersion
		existing.Revision++
		existing.Deleted = false
		m.items[item.ID] = existing
		m.history[item.ID] = append(m.history[item.ID], ItemVersion{
			ItemID: item.ID, Revision: existing.Revision, Ciphertext: append([]byte(nil), existing.Ciphertext...),
		})
		m.addChangeLocked(Change{VaultID: existing.VaultID, Kind: "item", ObjectID: existing.ID, Revision: existing.Revision})
		return existing, nil
	}
	if expected != 0 {
		return Item{}, &ConflictError{Current: 0}
	}
	if item.ID == "" {
		item.ID = newID()
	}
	item.Revision = 1
	item.Ciphertext = append([]byte(nil), item.Ciphertext...)
	m.items[item.ID] = item
	m.history[item.ID] = []ItemVersion{{ItemID: item.ID, Revision: 1, Ciphertext: append([]byte(nil), item.Ciphertext...)}}
	if item.Idempotency != "" {
		m.idem[item.VaultID+"|"+item.Idempotency] = item.ID
	}
	m.addChangeLocked(Change{VaultID: item.VaultID, Kind: "item", ObjectID: item.ID, Revision: 1})
	return item, nil
}

func (m *Memory) Item(_ context.Context, id string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	return item, nil
}

func (m *Memory) ListItems(_ context.Context, vaultID string, cursor int64, limit int) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []Item
	for _, item := range m.items {
		if item.VaultID != vaultID || item.Revision <= cursor {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) ItemHistory(_ context.Context, id string) ([]ItemVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return nil, ErrNotFound
	}
	return append([]ItemVersion(nil), m.history[id]...), nil
}

func (m *Memory) Tombstone(_ context.Context, id string, expected int64) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	if item.Revision != expected {
		return Item{}, &ConflictError{Current: item.Revision}
	}
	item.Deleted = true
	item.Revision++
	m.items[id] = item
	m.addChangeLocked(Change{VaultID: item.VaultID, Kind: "item", ObjectID: item.ID, Revision: item.Revision})
	return item, nil
}

func (m *Memory) CreateOrg(_ context.Context, o Org, ownerRole string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[o.ID] = o
	m.orgRoles[o.ID+"|"+o.OwnerID] = ownerRole
	return nil
}

func (m *Memory) ListOrgs(_ context.Context, userID string) ([]Org, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Org
	for _, o := range m.orgs {
		if _, ok := m.orgRoles[o.ID+"|"+userID]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m *Memory) OrgRole(_ context.Context, orgID, userID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	role, ok := m.orgRoles[orgID+"|"+userID]
	return role, ok
}

func (m *Memory) ListOrgMembers(_ context.Context, orgID string) ([]OrgMemberInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := orgID + "|"
	var out []OrgMemberInfo
	for key, role := range m.orgRoles {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		userID := key[len(prefix):]
		info := OrgMemberInfo{UserID: userID, DisplayName: m.users[userID].DisplayName, Role: role, Devices: []OrgDevice{}}
		for _, d := range m.devices {
			if d.UserID == userID && d.Status == "active" {
				info.Devices = append(info.Devices, OrgDevice{ID: d.ID, BoxPub: append([]byte(nil), d.BoxPub...)})
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func (m *Memory) PutInvite(_ context.Context, inv Invite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invites[inv.TokenHash] = inv
	return nil
}

func (m *Memory) Invite(_ context.Context, tokenHash string) (Invite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[tokenHash]
	if !ok {
		return Invite{}, ErrNotFound
	}
	return inv, nil
}

func (m *Memory) AcceptInvite(_ context.Context, tokenHash, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[tokenHash]
	if !ok {
		return ErrNotFound
	}
	if inv.Accepted {
		return ErrConsumed
	}
	if time.Now().After(inv.Expires) {
		return ErrExpired
	}
	inv.Accepted = true
	m.invites[tokenHash] = inv
	m.orgRoles[inv.OrgID+"|"+userID] = inv.Role
	return nil
}

func (m *Memory) PutEnvelope(_ context.Context, e Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envelopes = append(m.envelopes, e)
	return nil
}

func (m *Memory) ListEnvelopes(_ context.Context, vaultID string) ([]Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Envelope
	for _, e := range m.envelopes {
		if e.VaultID == vaultID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *Memory) DeleteEnvelopesForDevice(_ context.Context, vaultID, deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.envelopes[:0]
	for _, e := range m.envelopes {
		if e.VaultID == vaultID && e.RecipientDeviceID == deviceID {
			continue
		}
		kept = append(kept, e)
	}
	m.envelopes = kept
	return nil
}

func (m *Memory) PutChunk(_ context.Context, c Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chunks[c.AttachmentID+"|"+itoa(c.N)] = c
	return nil
}

func (m *Memory) Chunk(_ context.Context, attachmentID string, n int) (Chunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.chunks[attachmentID+"|"+itoa(n)]
	if !ok {
		return Chunk{}, ErrNotFound
	}
	return c, nil
}

func (m *Memory) CompleteAttachment(_ context.Context, attachmentID, ownerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for k, c := range m.chunks {
		if c.AttachmentID != attachmentID {
			continue
		}
		if c.OwnerID != ownerID {
			return ErrNotFound
		}
		c.Complete = true
		m.chunks[k] = c
		found = true
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (m *Memory) Changes(_ context.Context, userID string, cursor int64, limit int) ([]Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []Change
	for _, c := range m.changes {
		if c.Seq <= cursor {
			continue
		}
		v, ok := m.vaults[c.VaultID]
		if !ok || !m.canSeeLocked(v, userID) {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) AddChange(_ context.Context, c Change) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addChangeLocked(c)
	return nil
}

func (m *Memory) addChangeLocked(c Change) {
	m.seq++
	c.Seq = m.seq
	m.changes = append(m.changes, c)
}

func (m *Memory) AddAudit(_ context.Context, a Audit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == "" {
		a.ID = newID()
	}
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	m.audits = append(m.audits, a)
	return nil
}

func (m *Memory) ListAudit(_ context.Context, userID string) ([]Audit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Audit
	for _, a := range m.audits {
		if a.ActorID == userID || userID == "" {
			out = append(out, a)
		}
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
