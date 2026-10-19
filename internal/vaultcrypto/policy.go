package vaultcrypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
)

const (
	PolicyPassphrase           = "passphrase"
	PolicyHardwarePlusRecovery = "hardware_plus_recovery"
	PolicyHardwareOnly         = "hardware_only"
)

// TokenProof is the evidence that two enrollments are different physical tokens.
// Credential IDs alone are not enough when serial numbers are missing.
type TokenProof struct {
	Serial         string
	CredentialID   []byte
	ReinsertProven bool
}

// DistinctTokens reports whether two enrollments may count as spare keys.
func DistinctTokens(a, b TokenProof) error {
	if len(a.CredentialID) > 0 && ctEqual(a.CredentialID, b.CredentialID) {
		return fmt.Errorf("%w: same credential", ErrToken)
	}
	if a.Serial != "" && b.Serial != "" {
		if a.Serial == b.Serial {
			return fmt.Errorf("%w: same serial", ErrToken)
		}
		return nil
	}
	if !a.ReinsertProven || !b.ReinsertProven {
		return fmt.Errorf("%w: remove and reinsert the second token", ErrToken)
	}
	return nil
}

func isHardware(t string) bool {
	return t == SlotFIDO2 || t == SlotPIV
}

func activeSlots(slots []UnlockSlot, except string) []UnlockSlot {
	out := make([]UnlockSlot, 0, len(slots))
	for _, s := range slots {
		if s.State != StateActive || s.Context.SlotID == except {
			continue
		}
		out = append(out, s)
	}
	return out
}

// CanRemoveSlot enforces last-path protection.
// Hardware-only accounts must keep two active hardware slots.
func CanRemoveSlot(policy string, slots []UnlockSlot, removeID string) error {
	left := activeSlots(slots, removeID)
	switch policy {
	case PolicyHardwareOnly:
		n := 0
		for _, s := range left {
			if isHardware(s.Type) {
				n++
			}
		}
		if n < 2 {
			return fmt.Errorf("%w: hardware-only needs two remaining hardware slots", ErrPolicy)
		}
	case PolicyPassphrase, PolicyHardwarePlusRecovery:
		if len(left) < 1 {
			return fmt.Errorf("%w: last recovery path", ErrPolicy)
		}
	default:
		return fmt.Errorf("%w: unknown policy", ErrPolicy)
	}
	return nil
}

// HardwareOnlyReady is true only after two active hardware slots and a
// distinct-token proof. Software slots may exist but are not required.
func HardwareOnlyReady(slots []UnlockSlot, distinct error) error {
	if distinct != nil {
		return distinct
	}
	n := 0
	for _, s := range slots {
		if s.State == StateActive && isHardware(s.Type) {
			n++
		}
	}
	if n < 2 {
		return fmt.Errorf("%w: enroll two hardware keys before hardware-only", ErrPolicy)
	}
	return nil
}

// Generation is one account-key generation and its slots.
type Generation struct {
	ID       string
	Sequence uint64
	State    string
	Slots    []UnlockSlot
}

// CanCommitGeneration requires every slot on the new generation to be active
// and the policy minimum to be met. The caller then marks it active.
func CanCommitGeneration(next Generation, policy string) error {
	if next.State != StatePending {
		return fmt.Errorf("%w: generation is not pending", ErrPolicy)
	}
	for _, s := range next.Slots {
		if s.State != StateActive {
			return fmt.Errorf("%w: slot %s is not active", ErrPolicy, s.Context.SlotID)
		}
	}
	if err := CanRemoveSlot(policy, next.Slots, ""); err != nil && policy == PolicyHardwareOnly {
		return err
	}
	if policy == PolicyHardwareOnly {
		return CanRemoveSlot(policy, append(next.Slots, UnlockSlot{State: StateActive, Context: SlotContext{SlotID: "sentinel"}}), "sentinel")
	}
	if len(activeSlots(next.Slots, "")) < 1 {
		return fmt.Errorf("%w: no active slot", ErrPolicy)
	}
	return nil
}

// ManifestEntry is one slot in a signed generation manifest.
type ManifestEntry struct {
	SlotID string
	Type   string
	State  string
	Hash   [32]byte
}

// Manifest is signed by a trusted device. The server stores it opaquely.
type Manifest struct {
	AccountID      string
	InstanceID     string
	GenerationID   string
	RecoveryPolicy string
	Sequence       uint64
	Entries        []ManifestEntry
}

// HashEnvelope is the manifest commitment to one wrap.
func HashEnvelope(envelope []byte) [32]byte {
	return sha256.Sum256(envelope)
}

// CanonicalManifest is the signed byte string.
func CanonicalManifest(m Manifest) ([]byte, error) {
	entries := append([]ManifestEntry(nil), m.Entries...)
	slices.SortFunc(entries, func(a, b ManifestEntry) int {
		if a.SlotID < b.SlotID {
			return -1
		}
		if a.SlotID > b.SlotID {
			return 1
		}
		return 0
	})
	buf := []byte{ProtocolVersion}
	var err error
	for _, s := range []string{m.AccountID, m.InstanceID, m.GenerationID, m.RecoveryPolicy} {
		buf, err = appendLP(buf, s)
		if err != nil {
			return nil, err
		}
	}
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], m.Sequence)
	buf = append(buf, seq[:]...)
	if len(entries) > 65535 {
		return nil, ErrFormat
	}
	buf = append(buf, byte(len(entries)>>8), byte(len(entries)))
	for _, e := range entries {
		buf, err = appendLP(buf, e.SlotID)
		if err != nil {
			return nil, err
		}
		buf, err = appendLP(buf, e.Type)
		if err != nil {
			return nil, err
		}
		buf, err = appendLP(buf, e.State)
		if err != nil {
			return nil, err
		}
		buf = append(buf, e.Hash[:]...)
	}
	return buf, nil
}

// SignManifest signs a generation manifest.
func SignManifest(priv ed25519.PrivateKey, m Manifest) ([]byte, error) {
	raw, err := CanonicalManifest(m)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, raw), nil
}

// VerifyManifest checks a device signature.
func VerifyManifest(pub ed25519.PublicKey, sig []byte, m Manifest) error {
	raw, err := CanonicalManifest(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, raw, sig) {
		return ErrDecrypt
	}
	return nil
}

// CheckSequence rejects a manifest older than the one the client already accepted.
func CheckSequence(accepted, next uint64) error {
	if next <= accepted {
		return fmt.Errorf("%w: sequence %d is not after %d", ErrRollback, next, accepted)
	}
	return nil
}
