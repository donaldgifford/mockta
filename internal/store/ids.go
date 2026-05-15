package store

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// Resource-kind constants used as the first input to NewID. Keeping
// these as constants prevents callers from passing arbitrary kind
// strings — drift in the kind input changes the ID for the same
// primary key, which is exactly what determinism is meant to prevent.
const (
	KindUser  = "user"
	KindGroup = "group"
	KindApp   = "app"
)

// idLength is the truncated character length of a generated ID. 20
// matches Okta's `00u...` ID shape (20 chars total) so downstream
// systems that look at ID width find the same shape, without us
// emulating the `00u`/`00g`/`0oa` prefixes.
const idLength = 20

// idEncoding is base32 RFC 4648 uppercase without padding. base32 is
// chosen over base64 because URL paths often contain IDs and base32
// avoids the `+` / `/` characters that need encoding. Strip padding
// because we truncate anyway.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewID returns a deterministic ID for the given (kind, primaryKey)
// pair. Same inputs always yield the same output, across processes,
// across machines, across releases. This matters for `terraform test`
// assertions that pin output values.
//
// The implementation is: SHA-256 of `kind + "\x00" + primaryKey`,
// base32-encoded, uppercased, truncated to 20 chars.
func NewID(kind, primaryKey string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(primaryKey))

	encoded := idEncoding.EncodeToString(h.Sum(nil))
	// EncodeToString returns uppercase already, but be explicit
	// against future encoding swaps.
	encoded = strings.ToUpper(encoded)
	return encoded[:idLength]
}
