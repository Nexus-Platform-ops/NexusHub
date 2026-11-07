package api

import (
	"crypto/rand"
	"errors"
)

var errNotFound = errors.New("not found")

// cryptoRead wraps crypto/rand.Read.
func cryptoRead(b []byte) (int, error) { return rand.Read(b) }
