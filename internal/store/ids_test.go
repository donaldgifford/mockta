package store

import (
	"strings"
	"testing"
)

func TestNewID_Determinism(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kind       string
		primaryKey string
	}{
		{"user by login", KindUser, "alice@example.com"},
		{"group by name", KindGroup, "engineers"},
		{"app by label", KindApp, "AWS SSO"},
		{"empty primary key", KindUser, ""},
		{"unicode primary key", KindUser, "用户@例.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := NewID(tt.kind, tt.primaryKey)
			b := NewID(tt.kind, tt.primaryKey)
			if a != b {
				t.Fatalf("NewID(%q, %q) = %q on first call, %q on second; want stable",
					tt.kind, tt.primaryKey, a, b)
			}
			if len(a) != idLength {
				t.Fatalf("NewID(%q, %q) length = %d, want %d",
					tt.kind, tt.primaryKey, len(a), idLength)
			}
			if a != strings.ToUpper(a) {
				t.Fatalf("NewID(%q, %q) = %q, want uppercase only",
					tt.kind, tt.primaryKey, a)
			}
		})
	}
}

func TestNewID_DifferentInputsDifferentIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kindA  string
		keyA   string
		kindB  string
		keyB   string
		reason string
	}{
		{
			name:  "same kind, different key",
			kindA: KindUser, keyA: "alice@example.com",
			kindB: KindUser, keyB: "bob@example.com",
			reason: "different primary keys must hash differently",
		},
		{
			name:  "different kind, same key",
			kindA: KindUser, keyA: "alice",
			kindB: KindGroup, keyB: "alice",
			reason: "kind discriminator must prevent a user ID from colliding with a group ID for the same logical name",
		},
		{
			name:  "kind boundary delimiter",
			kindA: "use", keyA: "ralice",
			kindB: "user", keyB: "alice",
			reason: "without the null-byte separator, these would hash to the same value",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := NewID(tt.kindA, tt.keyA)
			b := NewID(tt.kindB, tt.keyB)
			if a == b {
				t.Fatalf("NewID(%q,%q) and NewID(%q,%q) collided to %q (%s)",
					tt.kindA, tt.keyA, tt.kindB, tt.keyB, a, tt.reason)
			}
		})
	}
}

func TestNewID_OnlyBase32Alphabet(t *testing.T) {
	t.Parallel()

	id := NewID(KindUser, "alice@example.com")
	const allowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	for i, r := range id {
		if !strings.ContainsRune(allowed, r) {
			t.Fatalf("id[%d] = %q in %q; only RFC 4648 base32 uppercase is allowed",
				i, r, id)
		}
	}
}
