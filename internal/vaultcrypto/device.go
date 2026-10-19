package vaultcrypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const deviceEnvelopeMagic = "WDEV"

// DeviceKeys are generated on the client. Private halves stay wrapped locally.
type DeviceKeys struct {
	Box     *ecdh.PrivateKey
	Sign    ed25519.PrivateKey
	SignPub ed25519.PublicKey
}

// NewDeviceKeys generates an X25519 encryption key and an Ed25519 signing key.
func NewDeviceKeys() (DeviceKeys, error) {
	box, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return DeviceKeys{}, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return DeviceKeys{}, err
	}
	return DeviceKeys{Box: box, Sign: priv, SignPub: pub}, nil
}

// DeviceEnvelope carries a vault key to one device public key.
type DeviceEnvelope struct {
	VaultID      string
	Generation   uint64
	RecipientPub []byte
	EphemeralPub []byte
	Envelope     []byte
	Signature    []byte
	SignerPub    []byte
}

// SealForDevice wraps vaultKey to recipient and signs the result.
func SealForDevice(vaultKey []byte, vaultID, instanceID string, generation uint64, recipient *ecdh.PublicKey, signer ed25519.PrivateKey) (DeviceEnvelope, error) {
	if len(vaultKey) != 32 || recipient == nil || len(signer) != ed25519.PrivateKeySize || vaultID == "" {
		return DeviceEnvelope{}, fmt.Errorf("%w: device seal", ErrParams)
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return DeviceEnvelope{}, err
	}
	shared, err := eph.ECDH(recipient)
	if err != nil {
		return DeviceEnvelope{}, err
	}
	defer Wipe(shared)
	salt := append(append([]byte{}, eph.PublicKey().Bytes()...), recipient.Bytes()...)
	key, err := hkdfKey(shared, InfoDeviceWrap, salt)
	if err != nil {
		return DeviceEnvelope{}, err
	}
	defer Wipe(key)
	aad, err := AAD{
		Domain:      DomainDeviceWrap,
		InstanceID:  instanceID,
		VaultID:     vaultID,
		Revision:    generation,
		ContentType: "vault-key",
		AlgVersion:  uint16(AlgX25519),
	}.Marshal()
	if err != nil {
		return DeviceEnvelope{}, err
	}
	env, err := seal(AlgX25519, key, vaultKey, aad)
	if err != nil {
		return DeviceEnvelope{}, err
	}
	envp := DeviceEnvelope{
		VaultID:      vaultID,
		Generation:   generation,
		RecipientPub: recipient.Bytes(),
		EphemeralPub: eph.PublicKey().Bytes(),
		Envelope:     env,
		SignerPub:    signer.Public().(ed25519.PublicKey),
	}
	envp.Signature = ed25519.Sign(signer, envp.signingBytes(instanceID))
	return envp, nil
}

// OpenForDevice verifies the signature and unwraps with the device private key.
func OpenForDevice(envp DeviceEnvelope, instanceID string, recipient *ecdh.PrivateKey) ([]byte, error) {
	if recipient == nil || !ed25519.Verify(envp.SignerPub, envp.signingBytes(instanceID), envp.Signature) {
		return nil, ErrDecrypt
	}
	if !ctEqual(recipient.PublicKey().Bytes(), envp.RecipientPub) {
		return nil, ErrDecrypt
	}
	ephPub, err := ecdh.X25519().NewPublicKey(envp.EphemeralPub)
	if err != nil {
		return nil, ErrDecrypt
	}
	shared, err := recipient.ECDH(ephPub)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(shared)
	salt := append(append([]byte{}, envp.EphemeralPub...), envp.RecipientPub...)
	key, err := hkdfKey(shared, InfoDeviceWrap, salt)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := AAD{
		Domain:      DomainDeviceWrap,
		InstanceID:  instanceID,
		VaultID:     envp.VaultID,
		Revision:    envp.Generation,
		ContentType: "vault-key",
		AlgVersion:  uint16(AlgX25519),
	}.Marshal()
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgX25519, key, envp.Envelope, aad)
}

// MarshalDeviceEnvelope packs the fields OpenForDevice needs into the ciphertext
// the server stores. The server does not interpret the blob.
func MarshalDeviceEnvelope(e DeviceEnvelope) ([]byte, error) {
	if e.VaultID == "" || len(e.RecipientPub) == 0 || len(e.EphemeralPub) == 0 || len(e.Envelope) == 0 || len(e.Signature) == 0 || len(e.SignerPub) == 0 {
		return nil, fmt.Errorf("%w: device envelope", ErrFormat)
	}
	buf := append([]byte{}, deviceEnvelopeMagic...)
	buf = append(buf, 1)
	var err error
	if buf, err = appendLP(buf, e.VaultID); err != nil {
		return nil, err
	}
	var g [8]byte
	binary.BigEndian.PutUint64(g[:], e.Generation)
	buf = append(buf, g[:]...)
	if buf, err = appendBytes(buf, e.RecipientPub); err != nil {
		return nil, err
	}
	if buf, err = appendBytes(buf, e.EphemeralPub); err != nil {
		return nil, err
	}
	if buf, err = appendBytes(buf, e.Envelope); err != nil {
		return nil, err
	}
	if buf, err = appendBytes(buf, e.Signature); err != nil {
		return nil, err
	}
	return appendBytes(buf, e.SignerPub)
}

// UnmarshalDeviceEnvelope reverses MarshalDeviceEnvelope.
func UnmarshalDeviceEnvelope(raw []byte) (DeviceEnvelope, error) {
	if len(raw) < 5 || string(raw[:4]) != deviceEnvelopeMagic || raw[4] != 1 {
		return DeviceEnvelope{}, ErrFormat
	}
	rest := raw[5:]
	var e DeviceEnvelope
	var err error
	if e.VaultID, rest, err = takeLP(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if len(rest) < 8 {
		return DeviceEnvelope{}, ErrFormat
	}
	e.Generation = binary.BigEndian.Uint64(rest[:8])
	rest = rest[8:]
	if e.RecipientPub, rest, err = takeBytes(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if e.EphemeralPub, rest, err = takeBytes(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if e.Envelope, rest, err = takeBytes(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if e.Signature, rest, err = takeBytes(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if e.SignerPub, rest, err = takeBytes(rest); err != nil {
		return DeviceEnvelope{}, err
	}
	if len(rest) != 0 {
		return DeviceEnvelope{}, ErrFormat
	}
	return e, nil
}

func appendBytes(dst, b []byte) ([]byte, error) {
	if len(b) > maxField && len(b) > maxCiphertext {
		return nil, fmt.Errorf("%w: field length", ErrFormat)
	}
	if len(b) > 0xffff {
		return nil, fmt.Errorf("%w: field length", ErrFormat)
	}
	dst = append(dst, byte(len(b)>>8), byte(len(b)))
	dst = append(dst, b...)
	return dst, nil
}

func takeLP(b []byte) (string, []byte, error) {
	raw, rest, err := takeBytes(b)
	return string(raw), rest, err
}

func takeBytes(b []byte) ([]byte, []byte, error) {
	if len(b) < 2 {
		return nil, nil, ErrFormat
	}
	n := int(b[0])<<8 | int(b[1])
	b = b[2:]
	if n > len(b) {
		return nil, nil, ErrFormat
	}
	return append([]byte(nil), b[:n]...), b[n:], nil
}

// WrapDeviceSecrets seals this device's private keys under the account master key.
func WrapDeviceSecrets(master, boxPriv, signPriv []byte, accountID, instanceID string) ([]byte, error) {
	if len(master) != 32 || len(boxPriv) != 32 || len(signPriv) != ed25519.PrivateKeySize || accountID == "" || instanceID == "" {
		return nil, fmt.Errorf("%w: device secrets", ErrParams)
	}
	key, err := hkdfKey(master, InfoDeviceSecret, []byte(accountID+"|"+instanceID))
	if err != nil {
		return nil, err
	}
	defer Wipe(key)
	plain := append(append([]byte{}, boxPriv...), signPriv...)
	defer Wipe(plain)
	aad, err := AAD{
		Domain: DomainDeviceSecrets, AccountID: accountID, InstanceID: instanceID,
		ContentType: "device-keys", AlgVersion: uint16(AlgAES256GCM),
	}.Marshal()
	if err != nil {
		return nil, err
	}
	return seal(AlgAES256GCM, key, plain, aad)
}

// OpenDeviceSecrets unwraps private keys stored by WrapDeviceSecrets.
func OpenDeviceSecrets(master, envelope []byte, accountID, instanceID string) (boxPriv, signPriv []byte, err error) {
	if len(master) != 32 || accountID == "" || instanceID == "" {
		return nil, nil, ErrDecrypt
	}
	key, err := hkdfKey(master, InfoDeviceSecret, []byte(accountID+"|"+instanceID))
	if err != nil {
		return nil, nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := AAD{
		Domain: DomainDeviceSecrets, AccountID: accountID, InstanceID: instanceID,
		ContentType: "device-keys", AlgVersion: uint16(AlgAES256GCM),
	}.Marshal()
	if err != nil {
		return nil, nil, ErrDecrypt
	}
	plain, err := open(AlgAES256GCM, key, envelope, aad)
	if err != nil {
		return nil, nil, err
	}
	if len(plain) != 32+ed25519.PrivateKeySize {
		Wipe(plain)
		return nil, nil, ErrDecrypt
	}
	return plain[:32], plain[32:], nil
}

func (e DeviceEnvelope) signingBytes(instanceID string) []byte {
	buf := make([]byte, 0, 256+len(e.Envelope))
	buf, _ = appendLP(buf, e.VaultID)
	buf, _ = appendLP(buf, instanceID)
	var g [8]byte
	binary.BigEndian.PutUint64(g[:], e.Generation)
	buf = append(buf, g[:]...)
	buf = append(buf, byte(len(e.RecipientPub)>>8), byte(len(e.RecipientPub)))
	buf = append(buf, e.RecipientPub...)
	buf = append(buf, byte(len(e.EphemeralPub)>>8), byte(len(e.EphemeralPub)))
	buf = append(buf, e.EphemeralPub...)
	sum := sha256.Sum256(e.Envelope)
	buf = append(buf, sum[:]...)
	return buf
}

// WrapVaultKeyForAccount lets any client that can unwrap the account master
// key recover a personal vault without the original device.
func WrapVaultKeyForAccount(master, vaultKey []byte, accountID, instanceID, vaultID string) ([]byte, error) {
	if len(master) != 32 || len(vaultKey) != 32 || vaultID == "" {
		return nil, fmt.Errorf("%w: account vault wrap", ErrParams)
	}
	key, err := hkdfKey(master, InfoAccountVault, []byte(vaultID))
	if err != nil {
		return nil, err
	}
	defer Wipe(key)
	aad, err := AAD{
		Domain:      DomainAccountVault,
		AccountID:   accountID,
		InstanceID:  instanceID,
		VaultID:     vaultID,
		ContentType: "vault-key",
		AlgVersion:  uint16(AlgAES256GCM),
	}.Marshal()
	if err != nil {
		return nil, err
	}
	return seal(AlgAES256GCM, key, vaultKey, aad)
}

// UnwrapVaultKeyForAccount opens a personal vault key wrap.
func UnwrapVaultKeyForAccount(master []byte, envelope []byte, accountID, instanceID, vaultID string) ([]byte, error) {
	if len(master) != 32 {
		return nil, ErrDecrypt
	}
	key, err := hkdfKey(master, InfoAccountVault, []byte(vaultID))
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	aad, err := AAD{
		Domain:      DomainAccountVault,
		AccountID:   accountID,
		InstanceID:  instanceID,
		VaultID:     vaultID,
		ContentType: "vault-key",
		AlgVersion:  uint16(AlgAES256GCM),
	}.Marshal()
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgAES256GCM, key, envelope, aad)
}
