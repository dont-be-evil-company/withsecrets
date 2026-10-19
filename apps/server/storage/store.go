package storage

import (
	"context"
	"time"
)

// Store is the server persistence boundary. Implementations must not decrypt payloads.
type Store interface {
	Ready(ctx context.Context) error
	InstanceID(ctx context.Context) (string, error)

	CreateTransaction(ctx context.Context, tx Transaction) error
	Transaction(ctx context.Context, id string) (Transaction, error)
	ConsumeTransaction(ctx context.Context, id string) error

	UpsertIdentity(ctx context.Context, in Identity, display string) (userID string, err error)

	PutGrant(ctx context.Context, g Grant) error
	ConsumeGrant(ctx context.Context, codeHash, pkce, stateHash string, now time.Time) (string, error)

	PutSession(ctx context.Context, s Session) error
	SessionByAccess(ctx context.Context, accessHash string) (Session, error)
	SessionByRefresh(ctx context.Context, refreshHash string) (Session, error)
	RotateRefresh(ctx context.Context, oldRefreshHash string, next Session) error
	RevokeFamily(ctx context.Context, familyID string) error

	PutDevice(ctx context.Context, d Device) error
	Device(ctx context.Context, id string) (Device, error)
	ListDevices(ctx context.Context, userID string) ([]Device, error)
	SetDeviceStatus(ctx context.Context, id, status string) error

	EnsureAccount(ctx context.Context, a Account) error
	Account(ctx context.Context, userID string) (Account, error)
	SetAccountPolicy(ctx context.Context, userID, policy, generationID string, sequence uint64) error

	PutSlot(ctx context.Context, s Slot) error
	ListSlots(ctx context.Context, accountID string) ([]Slot, error)
	Slot(ctx context.Context, id string) (Slot, error)
	SetSlotState(ctx context.Context, id, state string) error

	PutGeneration(ctx context.Context, g KeyGeneration) error
	Generations(ctx context.Context, accountID string) ([]KeyGeneration, error)

	CreateVault(ctx context.Context, v Vault) error
	ListVaults(ctx context.Context, userID string) ([]Vault, error)
	Vault(ctx context.Context, id string) (Vault, error)
	SetAccountKeyWrap(ctx context.Context, id string, wrap []byte) error
	DeleteVault(ctx context.Context, id string) error

	PutMembership(ctx context.Context, m Membership) error
	DeleteMembership(ctx context.Context, vaultID, principalID string) error
	Membership(ctx context.Context, vaultID, principalID string) (Membership, bool)

	PutItem(ctx context.Context, item Item, expected int64) (Item, error)
	Tombstone(ctx context.Context, id string, expected int64) (Item, error)
	Item(ctx context.Context, id string) (Item, error)
	ListItems(ctx context.Context, vaultID string, cursor int64, limit int) ([]Item, error)
	ItemHistory(ctx context.Context, id string) ([]ItemVersion, error)

	CreateOrg(ctx context.Context, o Org, ownerRole string) error
	ListOrgs(ctx context.Context, userID string) ([]Org, error)
	OrgRole(ctx context.Context, orgID, userID string) (string, bool)
	ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberInfo, error)
	PutInvite(ctx context.Context, inv Invite) error
	Invite(ctx context.Context, tokenHash string) (Invite, error)
	AcceptInvite(ctx context.Context, tokenHash, userID string) error

	PutEnvelope(ctx context.Context, e Envelope) error
	ListEnvelopes(ctx context.Context, vaultID string) ([]Envelope, error)
	DeleteEnvelopesForDevice(ctx context.Context, vaultID, deviceID string) error

	PutChunk(ctx context.Context, c Chunk) error
	Chunk(ctx context.Context, attachmentID string, n int) (Chunk, error)
	CompleteAttachment(ctx context.Context, attachmentID, ownerID string) error

	Changes(ctx context.Context, userID string, cursor int64, limit int) ([]Change, error)
	AddChange(ctx context.Context, c Change) error
	AddAudit(ctx context.Context, a Audit) error
	ListAudit(ctx context.Context, userID string) ([]Audit, error)
}
