// Package vaultcrypto implements the version 1 client protocol from
// docs/architecture/adr-0002-cryptographic-protocol.md.
//
// The server must not import unwrap helpers for user secrets. Hardware
// transports live in internal/vaulthardware. This package is an unaudited
// beta: it is covered by vectors and is not a certification.
package vaultcrypto
