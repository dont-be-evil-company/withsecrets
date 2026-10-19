package vaultcrypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	SlotPassphrase = "passphrase-argon2id"
	SlotRecovery   = "recovery-secret"
	SlotFIDO2      = "fido2-hmac-secret"
	SlotPIV        = "yubikey-piv"

	StatePending = "pending"
	StateActive  = "active"
	StateRevoked = "revoked"
)

// SlotContext binds a wrap to one account on one server instance.
type SlotContext struct {
	AccountID    string
	InstanceID   string
	GenerationID string
	SlotID       string
	Label        string
}

// UnlockSlot is the client record persisted as ciphertext plus public metadata.
type UnlockSlot struct {
	Context   SlotContext
	Type      string
	Algorithm uint8
	Label     string
	Params    []byte
	Envelope  []byte
	State     string
}

type argonParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    []byte `json:"salt"`
	LowCost bool   `json:"lowCost,omitempty"`
}

type fidoParams struct {
	CredentialID []byte `json:"credentialId"`
	Salt         []byte `json:"salt"`
}

type pivParams struct {
	WrappedCEK  []byte `json:"wrappedCek"`
	PublicDER   []byte `json:"publicKey"`
	TouchPolicy string `json:"touchPolicy"`
	PINPolicy   string `json:"pinPolicy"`
}

// ProductionArgon2 is the RFC 9106 second recommended parameter set.
func ProductionArgon2() argonParams {
	return argonParams{Time: 3, Memory: 64 * 1024, Threads: 4}
}

// TestArgon2 is intentionally below the production floor.
func TestArgon2() argonParams {
	return argonParams{Time: 1, Memory: 8, Threads: 1, LowCost: true}
}

func (p argonParams) productionOK() bool {
	return !p.LowCost && p.Time >= 3 && p.Memory >= 64*1024 && p.Threads >= 1 && len(p.Salt) >= 16
}

func (c SlotContext) validate() error {
	if c.AccountID == "" || c.InstanceID == "" || c.GenerationID == "" || c.SlotID == "" {
		return fmt.Errorf("%w: slot context", ErrParams)
	}
	return nil
}

func (c SlotContext) aad(domain, slotType string, alg uint8) ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return AAD{
		Domain:       domain,
		AccountID:    c.AccountID,
		InstanceID:   c.InstanceID,
		GenerationID: c.GenerationID,
		SlotID:       c.SlotID,
		ContentType:  "account-master-key",
		SlotType:     slotType,
		AlgVersion:   uint16(alg),
	}.Marshal()
}

// NewMasterKey returns a fresh 32-byte account master key.
func NewMasterKey() ([]byte, error) {
	return randomBytes(32)
}

// EnrollPassphrase wraps master under a passphrase.
// Low-cost parameters are stored only when allowLowCost is set, and those
// slots must not be marked production by the caller.
func EnrollPassphrase(master []byte, ctx SlotContext, passphrase string, cost argonParams, allowLowCost bool) (UnlockSlot, error) {
	if len(master) != 32 || passphrase == "" {
		return UnlockSlot{}, fmt.Errorf("%w: passphrase enroll", ErrParams)
	}
	if len(cost.Salt) == 0 {
		salt, err := randomBytes(16)
		if err != nil {
			return UnlockSlot{}, err
		}
		cost.Salt = salt
	}
	if !cost.productionOK() && !allowLowCost {
		return UnlockSlot{}, fmt.Errorf("%w: argon2 cost", ErrParams)
	}
	if cost.LowCost && !allowLowCost {
		return UnlockSlot{}, fmt.Errorf("%w: argon2 cost", ErrParams)
	}
	key, err := passphraseKey(passphrase, cost)
	if err != nil {
		return UnlockSlot{}, err
	}
	defer Wipe(key)
	aad, err := ctx.aad(DomainPassphrase, SlotPassphrase, AlgPassphrase)
	if err != nil {
		return UnlockSlot{}, err
	}
	env, err := seal(AlgPassphrase, key, master, aad)
	if err != nil {
		return UnlockSlot{}, err
	}
	params, err := json.Marshal(cost)
	if err != nil {
		return UnlockSlot{}, err
	}
	slot := UnlockSlot{
		Context: ctx, Type: SlotPassphrase, Algorithm: AlgPassphrase,
		Label: ctx.Label, Params: params, Envelope: env, State: StatePending,
	}
	if err := verifyRoundTrip(master, slot, key); err != nil {
		return UnlockSlot{}, err
	}
	slot.State = StateActive
	return slot, nil
}

// EnrollPassphraseFromParams rewraps master using the cost JSON stored on a slot.
func EnrollPassphraseFromParams(master []byte, ctx SlotContext, passphrase string, params []byte) (UnlockSlot, error) {
	var cost argonParams
	if err := json.Unmarshal(params, &cost); err != nil {
		return UnlockSlot{}, fmt.Errorf("%w: passphrase params", ErrParams)
	}
	allowLow := cost.LowCost || !cost.productionOK()
	return EnrollPassphrase(master, ctx, passphrase, cost, allowLow)
}

// UnwrapPassphrase opens a passphrase slot.
func UnwrapPassphrase(slot UnlockSlot, passphrase string) ([]byte, error) {
	if slot.Type != SlotPassphrase || slot.State == StateRevoked {
		return nil, ErrDecrypt
	}
	var cost argonParams
	if err := json.Unmarshal(slot.Params, &cost); err != nil {
		return nil, ErrDecrypt
	}
	key, err := passphraseKey(passphrase, cost)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := slot.Context.aad(DomainPassphrase, SlotPassphrase, AlgPassphrase)
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgPassphrase, key, slot.Envelope, aad)
}

func passphraseKey(passphrase string, cost argonParams) ([]byte, error) {
	if cost.Time == 0 || cost.Memory < 8 || cost.Threads == 0 || len(cost.Salt) < 16 {
		return nil, ErrParams
	}
	raw := argon2.IDKey([]byte(passphrase), cost.Salt, cost.Time, cost.Memory, cost.Threads, 32)
	defer Wipe(raw)
	return hkdfKey(raw, InfoPassphrase, cost.Salt)
}

// NewRecoverySecret returns the 32-byte secret the user must store.
func NewRecoverySecret() ([]byte, error) {
	return randomBytes(32)
}

// EnrollRecovery wraps master under a random recovery secret.
func EnrollRecovery(master, secret []byte, ctx SlotContext) (UnlockSlot, error) {
	if len(master) != 32 || len(secret) != 32 {
		return UnlockSlot{}, fmt.Errorf("%w: recovery enroll", ErrParams)
	}
	key, err := hkdfKey(secret, InfoRecovery, []byte(ctx.SlotID))
	if err != nil {
		return UnlockSlot{}, err
	}
	defer Wipe(key)
	aad, err := ctx.aad(DomainRecovery, SlotRecovery, AlgRecovery)
	if err != nil {
		return UnlockSlot{}, err
	}
	env, err := seal(AlgRecovery, key, master, aad)
	if err != nil {
		return UnlockSlot{}, err
	}
	slot := UnlockSlot{
		Context: ctx, Type: SlotRecovery, Algorithm: AlgRecovery,
		Label: ctx.Label, Envelope: env, State: StatePending,
	}
	if err := verifyRoundTrip(master, slot, key); err != nil {
		return UnlockSlot{}, err
	}
	slot.State = StateActive
	return slot, nil
}

// UnwrapRecovery opens a recovery slot. secret is wiped by the caller.
func UnwrapRecovery(slot UnlockSlot, secret []byte) ([]byte, error) {
	if slot.Type != SlotRecovery || len(secret) != 32 || slot.State == StateRevoked {
		return nil, ErrDecrypt
	}
	key, err := hkdfKey(secret, InfoRecovery, []byte(slot.Context.SlotID))
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := slot.Context.aad(DomainRecovery, SlotRecovery, AlgRecovery)
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgRecovery, key, slot.Envelope, aad)
}

// HMACSecretFunc asks a FIDO2 authenticator for hmac-secret output.
// The implementation must return 32 bytes and must not log salt or output.
type HMACSecretFunc func(credentialID, salt []byte) ([]byte, error)

// EnrollFIDO2 wraps master using one authenticator's hmac-secret output.
func EnrollFIDO2(master []byte, ctx SlotContext, credentialID, salt []byte, hmacFn HMACSecretFunc) (UnlockSlot, error) {
	if len(master) != 32 || len(credentialID) == 0 || len(salt) != 32 || hmacFn == nil {
		return UnlockSlot{}, fmt.Errorf("%w: fido2 enroll", ErrParams)
	}
	out, err := hmacFn(credentialID, salt)
	if err != nil {
		return UnlockSlot{}, err
	}
	if len(out) != 32 {
		Wipe(out)
		return UnlockSlot{}, fmt.Errorf("%w: hmac-secret length", ErrToken)
	}
	defer Wipe(out)
	key, err := hkdfKey(out, InfoFIDO2, salt)
	if err != nil {
		return UnlockSlot{}, err
	}
	defer Wipe(key)
	aad, err := ctx.aad(DomainFIDO2, SlotFIDO2, AlgFIDO2)
	if err != nil {
		return UnlockSlot{}, err
	}
	env, err := seal(AlgFIDO2, key, master, aad)
	if err != nil {
		return UnlockSlot{}, err
	}
	params, err := json.Marshal(fidoParams{CredentialID: credentialID, Salt: salt})
	if err != nil {
		return UnlockSlot{}, err
	}
	slot := UnlockSlot{
		Context: ctx, Type: SlotFIDO2, Algorithm: AlgFIDO2,
		Label: ctx.Label, Params: params, Envelope: env, State: StatePending,
	}
	if err := verifyRoundTrip(master, slot, key); err != nil {
		return UnlockSlot{}, err
	}
	slot.State = StateActive
	return slot, nil
}

// UnwrapFIDO2 opens a FIDO2 slot with a fresh hmac-secret call.
func UnwrapFIDO2(slot UnlockSlot, hmacFn HMACSecretFunc) ([]byte, error) {
	if slot.Type != SlotFIDO2 || hmacFn == nil || slot.State == StateRevoked {
		return nil, ErrDecrypt
	}
	var p fidoParams
	if err := json.Unmarshal(slot.Params, &p); err != nil {
		return nil, ErrDecrypt
	}
	out, err := hmacFn(p.CredentialID, p.Salt)
	if err != nil {
		return nil, err
	}
	defer Wipe(out)
	if len(out) != 32 {
		return nil, ErrDecrypt
	}
	key, err := hkdfKey(out, InfoFIDO2, p.Salt)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := slot.Context.aad(DomainFIDO2, SlotFIDO2, AlgFIDO2)
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgFIDO2, key, slot.Envelope, aad)
}

// RejectPIN refuses empty and YubiKey factory PINs.
func RejectPIN(pin string) error {
	switch pin {
	case "", "123456", "12345678":
		return fmt.Errorf("%w: pin", ErrParams)
	default:
		return nil
	}
}

// RejectTouchPolicy refuses touch-never and an unset policy.
func RejectTouchPolicy(policy string) error {
	switch policy {
	case "always", "cached":
		return nil
	default:
		return fmt.Errorf("%w: touch policy", ErrParams)
	}
}

// EnrollPIV wraps master to an on-token RSA key using RSA-OAEP-SHA-256
// for a random CEK, then AES-256-GCM for the master key.
func EnrollPIV(master []byte, ctx SlotContext, pub *rsa.PublicKey, touchPolicy, pinPolicy string) (UnlockSlot, error) {
	if len(master) != 32 || pub == nil {
		return UnlockSlot{}, fmt.Errorf("%w: piv enroll", ErrParams)
	}
	if err := RejectTouchPolicy(touchPolicy); err != nil {
		return UnlockSlot{}, err
	}
	if pub.N.BitLen() < 2048 {
		return UnlockSlot{}, fmt.Errorf("%w: rsa size", ErrParams)
	}
	cek, err := randomBytes(32)
	if err != nil {
		return UnlockSlot{}, err
	}
	defer Wipe(cek)
	aad, err := ctx.aad(DomainPIV, SlotPIV, AlgPIV)
	if err != nil {
		return UnlockSlot{}, err
	}
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, cek, aad)
	if err != nil {
		return UnlockSlot{}, err
	}
	key, err := hkdfKey(cek, InfoPIV, wrapped)
	if err != nil {
		return UnlockSlot{}, err
	}
	defer Wipe(key)
	env, err := seal(AlgPIV, key, master, aad)
	if err != nil {
		return UnlockSlot{}, err
	}
	der, err := encodeRSAPublic(pub)
	if err != nil {
		return UnlockSlot{}, err
	}
	params, err := json.Marshal(pivParams{
		WrappedCEK: wrapped, PublicDER: der, TouchPolicy: touchPolicy, PINPolicy: pinPolicy,
	})
	if err != nil {
		return UnlockSlot{}, err
	}
	slot := UnlockSlot{
		Context: ctx, Type: SlotPIV, Algorithm: AlgPIV,
		Label: ctx.Label, Params: params, Envelope: env, State: StatePending,
	}
	if err := verifyRoundTrip(master, slot, key); err != nil {
		return UnlockSlot{}, err
	}
	slot.State = StateActive
	return slot, nil
}

// OAEPDecrypter decrypts with the on-token private key. label is the slot AAD.
type OAEPDecrypter func(ciphertext, label []byte) ([]byte, error)

// UnwrapPIV opens a PIV slot. The decrypter must enforce PIN and touch.
func UnwrapPIV(slot UnlockSlot, dec OAEPDecrypter) ([]byte, error) {
	if slot.Type != SlotPIV || dec == nil || slot.State == StateRevoked {
		return nil, ErrDecrypt
	}
	var p pivParams
	if err := json.Unmarshal(slot.Params, &p); err != nil {
		return nil, ErrDecrypt
	}
	aad, err := slot.Context.aad(DomainPIV, SlotPIV, AlgPIV)
	if err != nil {
		return nil, ErrDecrypt
	}
	cek, err := dec(p.WrappedCEK, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(cek)
	if len(cek) != 32 {
		return nil, ErrDecrypt
	}
	key, err := hkdfKey(cek, InfoPIV, p.WrappedCEK)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	return open(AlgPIV, key, slot.Envelope, aad)
}

func verifyRoundTrip(master []byte, slot UnlockSlot, key []byte) error {
	aad, err := aadForSlot(slot)
	if err != nil {
		return err
	}
	pt, err := open(slot.Algorithm, key, slot.Envelope, aad)
	if err != nil {
		return err
	}
	defer Wipe(pt)
	if !ctEqual(pt, master) {
		return ErrDecrypt
	}
	return nil
}

func aadForSlot(slot UnlockSlot) ([]byte, error) {
	switch slot.Type {
	case SlotPassphrase:
		return slot.Context.aad(DomainPassphrase, SlotPassphrase, AlgPassphrase)
	case SlotRecovery:
		return slot.Context.aad(DomainRecovery, SlotRecovery, AlgRecovery)
	case SlotFIDO2:
		return slot.Context.aad(DomainFIDO2, SlotFIDO2, AlgFIDO2)
	case SlotPIV:
		return slot.Context.aad(DomainPIV, SlotPIV, AlgPIV)
	default:
		return nil, ErrFormat
	}
}

func encodeRSAPublic(pub *rsa.PublicKey) ([]byte, error) {
	// PKIX encoding kept local to avoid an extra helper package.
	return marshalPKIX(pub)
}
