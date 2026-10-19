package vaultcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	magic            = "WSEC"
	ProtocolVersion  = 1
	maxField         = 1024
	maxCiphertext    = 8 << 20
	envelopeOverhead = 4 + 1 + 1 + 16 + 1

	AlgAES256GCM  uint8 = 1
	AlgX25519     uint8 = 2
	AlgPassphrase uint8 = 3
	AlgRecovery   uint8 = 4
	AlgFIDO2      uint8 = 5
	AlgPIV        uint8 = 6
)

const (
	DomainPassphrase    = "withsecrets/v1/aad/passphrase-slot"
	DomainRecovery      = "withsecrets/v1/aad/recovery-slot"
	DomainFIDO2         = "withsecrets/v1/aad/fido2-slot"
	DomainPIV           = "withsecrets/v1/aad/piv-slot"
	DomainItem          = "withsecrets/v1/aad/item"
	DomainAccountVault  = "withsecrets/v1/aad/account-vault-key"
	DomainDeviceWrap    = "withsecrets/v1/aad/device-vault-key"
	DomainAttachment    = "withsecrets/v1/aad/attachment-chunk"
	DomainDeviceSecrets = "withsecrets/v1/aad/device-private-keys"

	InfoPassphrase   = "withsecrets/v1/hkdf/passphrase-slot"
	InfoRecovery     = "withsecrets/v1/hkdf/recovery-slot"
	InfoFIDO2        = "withsecrets/v1/hkdf/fido2-slot"
	InfoPIV          = "withsecrets/v1/hkdf/piv-cek"
	InfoItem         = "withsecrets/v1/hkdf/item-key"
	InfoAccountVault = "withsecrets/v1/hkdf/account-vault-key"
	InfoDeviceWrap   = "withsecrets/v1/hkdf/device-vault-key"
	InfoChunk        = "withsecrets/v1/hkdf/chunk"
	InfoDeviceSecret = "withsecrets/v1/hkdf/device-private-keys"
)

var (
	ErrDecrypt  = errors.New("vaultcrypto: decryption failed")
	ErrFormat   = errors.New("vaultcrypto: invalid envelope")
	ErrParams   = errors.New("vaultcrypto: parameters rejected")
	ErrPolicy   = errors.New("vaultcrypto: slot policy rejected")
	ErrRollback = errors.New("vaultcrypto: manifest rollback")
	ErrToken    = errors.New("vaultcrypto: hardware token rejected")
)

// AAD is the canonical associated data for every AEAD operation.
// Empty strings are encoded explicitly so a missing field cannot collide
// with a shorter neighbor.
type AAD struct {
	Domain       string
	AccountID    string
	InstanceID   string
	GenerationID string
	SlotID       string
	VaultID      string
	ItemID       string
	Revision     uint64
	ContentType  string
	SlotType     string
	AlgVersion   uint16
}

// Marshal returns the versioned length-prefixed encoding.
func (a AAD) Marshal() ([]byte, error) {
	buf := []byte{ProtocolVersion}
	var err error
	for _, field := range []string{
		a.Domain, a.AccountID, a.InstanceID, a.GenerationID, a.SlotID,
		a.VaultID, a.ItemID, a.ContentType, a.SlotType,
	} {
		buf, err = appendLP(buf, field)
		if err != nil {
			return nil, err
		}
	}
	var rev [8]byte
	binary.BigEndian.PutUint64(rev[:], a.Revision)
	buf = append(buf, rev[:]...)
	var alg [2]byte
	binary.BigEndian.PutUint16(alg[:], a.AlgVersion)
	buf = append(buf, alg[:]...)
	return buf, nil
}

func appendLP(dst []byte, s string) ([]byte, error) {
	if len(s) > maxField {
		return nil, fmt.Errorf("%w: field length", ErrFormat)
	}
	dst = append(dst, byte(len(s)>>8), byte(len(s)))
	dst = append(dst, s...)
	return dst, nil
}

// Wipe overwrites b. Go may have copied the contents elsewhere; this is a
// best-effort scrub, not a promise that RAM is clear.
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func hkdfKey(ikm []byte, info string, salt []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, ikm, salt, []byte(info))
	out := make([]byte, 32)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

type parsedEnvelope struct {
	Version uint8
	Alg     uint8
	KeyID   []byte
	Nonce   []byte
	CT      []byte
}

func seal(alg uint8, key, plaintext, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: key length", ErrParams)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return nil, err
	}
	keyID, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, aad)
	return marshalEnvelope(alg, keyID, nonce, ct)
}

func open(expectedAlg uint8, key, envelope, aad []byte) ([]byte, error) {
	env, err := parseEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	if env.Version != ProtocolVersion || env.Alg != expectedAlg || len(key) != 32 {
		return nil, ErrDecrypt
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrDecrypt
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrDecrypt
	}
	if len(env.Nonce) != gcm.NonceSize() {
		return nil, ErrDecrypt
	}
	pt, err := gcm.Open(nil, env.Nonce, env.CT, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

func marshalEnvelope(alg uint8, keyID, nonce, ct []byte) ([]byte, error) {
	if len(keyID) != 16 || len(nonce) > 255 || len(ct) > maxCiphertext || len(ct) == 0 {
		return nil, ErrFormat
	}
	buf := make([]byte, 0, envelopeOverhead+len(nonce)+4+len(ct))
	buf = append(buf, magic...)
	buf = append(buf, ProtocolVersion, alg)
	buf = append(buf, keyID...)
	buf = append(buf, byte(len(nonce)))
	buf = append(buf, nonce...)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(ct)))
	buf = append(buf, n[:]...)
	buf = append(buf, ct...)
	return buf, nil
}

func parseEnvelope(b []byte) (parsedEnvelope, error) {
	if len(b) < envelopeOverhead+4 {
		return parsedEnvelope{}, ErrFormat
	}
	if string(b[:4]) != magic {
		return parsedEnvelope{}, ErrFormat
	}
	version := b[4]
	alg := b[5]
	if version != ProtocolVersion {
		return parsedEnvelope{}, ErrFormat
	}
	keyID := append([]byte(nil), b[6:22]...)
	nLen := int(b[22])
	off := 23
	if nLen == 0 || len(b) < off+nLen+4 {
		return parsedEnvelope{}, ErrFormat
	}
	nonce := append([]byte(nil), b[off:off+nLen]...)
	off += nLen
	ctLen := binary.BigEndian.Uint32(b[off : off+4])
	off += 4
	if ctLen == 0 || ctLen > maxCiphertext || len(b) != off+int(ctLen) {
		return parsedEnvelope{}, ErrFormat
	}
	ct := append([]byte(nil), b[off:]...)
	return parsedEnvelope{Version: version, Alg: alg, KeyID: keyID, Nonce: nonce, CT: ct}, nil
}

func ctEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}
