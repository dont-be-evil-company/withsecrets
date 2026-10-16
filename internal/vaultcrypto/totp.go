package vaultcrypto

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash"
	"time"
)

// TOTP implements RFC 6238. digests are sha1, sha256, or sha512.
// This is for item one-time codes, not for signing in to withsecrets.
func TOTP(secret []byte, at time.Time, period, digits int, alg string) (string, error) {
	if period <= 0 || digits < 6 || digits > 8 || len(secret) == 0 {
		return "", fmt.Errorf("%w: totp", ErrParams)
	}
	counter := uint64(at.Unix()) / uint64(period)
	var mac hash.Hash
	switch alg {
	case "", "sha1":
		mac = hmac.New(sha1.New, secret)
	case "sha256":
		mac = hmac.New(sha256.New, secret)
	case "sha512":
		mac = hmac.New(sha512.New, secret)
	default:
		return "", fmt.Errorf("%w: totp alg", ErrParams)
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	code := bin % mod
	return fmt.Sprintf("%0*d", digits, code), nil
}
