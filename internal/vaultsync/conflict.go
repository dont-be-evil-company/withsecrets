package vaultsync

import "bytes"

// Conflict keeps both ciphertext revisions. The server never compares plaintext.
type Conflict struct {
	ItemID           string
	BaseRevision     int64
	LocalCiphertext  []byte
	RemoteCiphertext []byte
	RemoteRevision   int64
}

// Decide returns a conflict when the local edit and the remote edit diverged.
// It does not pick a winner.
func Decide(itemID string, base, localRev, remoteRev int64, local, remote []byte) (push bool, conflict *Conflict) {
	if remoteRev == base {
		return true, nil
	}
	if localRev == base && bytes.Equal(local, remote) {
		return false, nil
	}
	return false, &Conflict{
		ItemID: itemID, BaseRevision: base,
		LocalCiphertext: append([]byte(nil), local...), RemoteCiphertext: append([]byte(nil), remote...),
		RemoteRevision: remoteRev,
	}
}
