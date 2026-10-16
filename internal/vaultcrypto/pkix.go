package vaultcrypto

import "crypto/x509"

func marshalPKIX(pub any) ([]byte, error) {
	return x509.MarshalPKIXPublicKey(pub)
}
