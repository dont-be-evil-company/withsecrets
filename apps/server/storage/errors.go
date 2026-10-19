package storage

import "errors"

var (
	ErrNotFound = errors.New("storage: not found")
	ErrConsumed = errors.New("storage: already used")
	ErrExpired  = errors.New("storage: expired")
	ErrMismatch = errors.New("storage: challenge mismatch")
	ErrRevoked  = errors.New("storage: revoked")
)
