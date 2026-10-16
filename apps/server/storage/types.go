package storage

import "time"

// Models are ciphertext and routing metadata. Item bodies are opaque bytes.

type User struct {
	ID          string
	DisplayName string
}

type Identity struct {
	UserID   string
	Provider string
	Issuer   string
	Subject  string
	Email    string
}

type Transaction struct {
	ID               string
	State            string
	StateHash        string
	PKCEChallenge    string
	Callback         string
	InstanceID       string
	Expires          time.Time
	Consumed         bool
	Provider         string
	Nonce            string
	ProviderVerifier string
}

type Grant struct {
	CodeHash      string
	UserID        string
	PKCEChallenge string
	StateHash     string
	Expires       time.Time
	Used          bool
}

type Session struct {
	ID            string
	UserID        string
	DeviceID      string
	FamilyID      string
	AccessHash    string
	RefreshHash   string
	AccessExp     time.Time
	RefreshExp    time.Time
	Revoked       bool
	FamilyRevoked bool
}

type Device struct {
	ID          string
	UserID      string
	SignPub     []byte
	BoxPub      []byte
	Fingerprint string
	Status      string
}

type Account struct {
	ID             string
	UserID         string
	RecoveryPolicy string
	GenerationID   string
	Sequence       uint64
}

type Slot struct {
	ID           string
	AccountID    string
	GenerationID string
	Type         string
	Label        string
	AlgVersion   int
	PublicParams []byte
	Wrap         []byte
	State        string
	Revision     int64
}

type KeyGeneration struct {
	ID        string
	AccountID string
	Sequence  uint64
	State     string
	Manifest  []byte
	Signature []byte
	Signer    string
}

type Vault struct {
	ID             string
	NameHint       string
	OwnerID        string
	OrgID          string
	Revision       int64
	AccountKeyWrap []byte
}

type Membership struct {
	VaultID     string
	PrincipalID string
	Permission  string
}

type Item struct {
	ID            string
	VaultID       string
	Ciphertext    []byte
	CryptoVersion int
	Revision      int64
	Deleted       bool
	Idempotency   string
}

type ItemVersion struct {
	ItemID     string
	Revision   int64
	Ciphertext []byte
}

type Org struct {
	ID      string
	Name    string
	OwnerID string
}

type OrgMember struct {
	OrgID  string
	UserID string
	Role   string
}

// OrgDevice is an active device that can receive a vault-key envelope.
type OrgDevice struct {
	ID     string
	BoxPub []byte
}

// OrgMemberInfo is one organization member and the devices an owner can wrap a key to.
type OrgMemberInfo struct {
	UserID      string
	DisplayName string
	Role        string
	Devices     []OrgDevice
}

type Invite struct {
	TokenHash string
	OrgID     string
	Role      string
	Expires   time.Time
	Accepted  bool
}

type Envelope struct {
	VaultID           string
	Generation        int64
	RecipientDeviceID string
	Ciphertext        []byte
	Signature         []byte
}

type Chunk struct {
	AttachmentID string
	OwnerID      string
	N            int
	Sum          []byte
	Body         []byte
	Complete     bool
}

type Change struct {
	Seq      int64
	UserID   string
	VaultID  string
	Kind     string
	ObjectID string
	Revision int64
}

type Audit struct {
	ID       string
	ActorID  string
	Action   string
	Target   string
	Metadata string
	At       time.Time
}

// ConflictError is returned when an item write loses compare-and-swap.
type ConflictError struct {
	Current int64
}

func (e *ConflictError) Error() string { return "revision conflict" }
