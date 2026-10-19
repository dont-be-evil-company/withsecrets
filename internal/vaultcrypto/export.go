package vaultcrypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const exportFormat = "withsecrets-vault-export"

// ExportBundle is an encrypted export. It has no plaintext item fields.
type ExportBundle struct {
	Format  string   `json:"format"`
	Version int      `json:"version"`
	Items   []string `json:"items"`
}

// EncodeExport serializes ciphertext envelopes.
func EncodeExport(envelopes [][]byte) ([]byte, error) {
	items := make([]string, len(envelopes))
	for i, env := range envelopes {
		if _, err := parseEnvelope(env); err != nil {
			return nil, err
		}
		items[i] = base64.StdEncoding.EncodeToString(env)
	}
	return json.Marshal(ExportBundle{Format: exportFormat, Version: ProtocolVersion, Items: items})
}

// DecodeExport rejects plaintext fields, unknown formats, and oversized files.
func DecodeExport(raw []byte) ([][]byte, error) {
	if len(raw) > 32<<20 {
		return nil, fmt.Errorf("%w: export too large", ErrFormat)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, ErrFormat
	}
	for _, banned := range []string{"password", "totp", "seed", "title", "notes", "privateKey"} {
		if _, ok := probe[banned]; ok {
			return nil, errors.New("vaultcrypto: export contains a plaintext field")
		}
	}
	var bundle ExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, ErrFormat
	}
	if bundle.Format != exportFormat || bundle.Version != ProtocolVersion {
		return nil, ErrFormat
	}
	out := make([][]byte, len(bundle.Items))
	for i, item := range bundle.Items {
		env, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil, ErrFormat
		}
		if _, err := parseEnvelope(env); err != nil {
			return nil, err
		}
		out[i] = env
	}
	return out, nil
}
