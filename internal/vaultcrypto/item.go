package vaultcrypto

import "fmt"

// SealItem encrypts an item body under a domain-separated key from vaultKey.
// A fresh nonce is inside the envelope, so revisions do not reuse one.
func SealItem(vaultKey []byte, aad AAD, plaintext []byte) ([]byte, error) {
	if aad.Domain == "" {
		aad.Domain = DomainItem
	}
	if aad.ItemID == "" || aad.VaultID == "" || len(vaultKey) != 32 {
		return nil, fmt.Errorf("%w: item", ErrParams)
	}
	aad.AlgVersion = uint16(AlgAES256GCM)
	key, err := itemKey(vaultKey, aad.ItemID)
	if err != nil {
		return nil, err
	}
	defer Wipe(key)
	raw, err := aad.Marshal()
	if err != nil {
		return nil, err
	}
	return seal(AlgAES256GCM, key, plaintext, raw)
}

// OpenItem decrypts an item. AAD must be rebuilt by the caller from metadata
// it trusts, not from unauthenticated side channels.
func OpenItem(vaultKey []byte, aad AAD, envelope []byte) ([]byte, error) {
	if aad.Domain == "" {
		aad.Domain = DomainItem
	}
	aad.AlgVersion = uint16(AlgAES256GCM)
	if len(vaultKey) != 32 || aad.ItemID == "" {
		return nil, ErrDecrypt
	}
	key, err := itemKey(vaultKey, aad.ItemID)
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	raw, err := aad.Marshal()
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgAES256GCM, key, envelope, raw)
}

func itemKey(vaultKey []byte, itemID string) ([]byte, error) {
	return hkdfKey(vaultKey, InfoItem+"/"+itemID, []byte(itemID))
}

// SealChunk encrypts one attachment chunk. Chunk number is in the KDF and the AAD.
func SealChunk(vaultKey []byte, aad AAD, chunkNo uint32, plaintext []byte) ([]byte, error) {
	aad.Domain = DomainAttachment
	aad.ContentType = fmt.Sprintf("chunk:%d", chunkNo)
	aad.AlgVersion = uint16(AlgAES256GCM)
	if aad.ItemID == "" || len(vaultKey) != 32 {
		return nil, fmt.Errorf("%w: chunk", ErrParams)
	}
	key, err := hkdfKey(vaultKey, fmt.Sprintf("%s/%s/%d", InfoChunk, aad.ItemID, chunkNo), []byte(aad.ItemID))
	if err != nil {
		return nil, err
	}
	defer Wipe(key)
	raw, err := aad.Marshal()
	if err != nil {
		return nil, err
	}
	return seal(AlgAES256GCM, key, plaintext, raw)
}

// OpenChunk decrypts one attachment chunk.
func OpenChunk(vaultKey []byte, aad AAD, chunkNo uint32, envelope []byte) ([]byte, error) {
	aad.Domain = DomainAttachment
	aad.ContentType = fmt.Sprintf("chunk:%d", chunkNo)
	aad.AlgVersion = uint16(AlgAES256GCM)
	if len(vaultKey) != 32 {
		return nil, ErrDecrypt
	}
	key, err := hkdfKey(vaultKey, fmt.Sprintf("%s/%s/%d", InfoChunk, aad.ItemID, chunkNo), []byte(aad.ItemID))
	if err != nil {
		return nil, ErrDecrypt
	}
	defer Wipe(key)
	raw, err := aad.Marshal()
	if err != nil {
		return nil, ErrDecrypt
	}
	return open(AlgAES256GCM, key, envelope, raw)
}
