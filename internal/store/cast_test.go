package store

import (
	"errors"
	"strings"
	"testing"
)

// The cast helpers are defensive — under healthy operation the type
// assertion always succeeds because schema.go pins the Insert types.
// These tests cover the error branches directly so they stay
// understandable when something inevitably goes wrong.

type wrongType struct{}

func TestCastHelpers_WrongType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   func(any) error
		want string
	}{
		{"asUser", func(v any) error { _, err := asUser(v); return err }, "*User"},
		{"asGroup", func(v any) error { _, err := asGroup(v); return err }, "*Group"},
		{"asApp", func(v any) error { _, err := asApp(v); return err }, "*App"},
		{"asGroupMembership", func(v any) error { _, err := asGroupMembership(v); return err }, "*GroupMembership"},
		{"asAuditEntry", func(v any) error { _, err := asAuditEntry(v); return err }, "*AuditEntry"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn(wrongType{})
			if err == nil {
				t.Fatalf("%s(wrongType{}) = nil error, want type mismatch", tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s error = %q, want it to mention %q", tt.name, err.Error(), tt.want)
			}
		})
	}
}

func TestCastHelpers_RightType(t *testing.T) {
	t.Parallel()

	if _, err := asUser(&User{}); err != nil {
		t.Errorf("asUser(*User) = %v, want no error", err)
	}
	if _, err := asGroup(&Group{}); err != nil {
		t.Errorf("asGroup(*Group) = %v, want no error", err)
	}
	if _, err := asApp(&App{}); err != nil {
		t.Errorf("asApp(*App) = %v, want no error", err)
	}
	if _, err := asGroupMembership(&GroupMembership{}); err != nil {
		t.Errorf("asGroupMembership(*GroupMembership) = %v, want no error", err)
	}
	if _, err := asAuditEntry(&AuditEntry{}); err != nil {
		t.Errorf("asAuditEntry(*AuditEntry) = %v, want no error", err)
	}
}

// Smoke test that sentinel errors are comparable via errors.Is. This
// is the contract handlers rely on to map ErrNotFound → 404 etc.
func TestSentinels_ErrorsIs(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.GetUser("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(ErrNotFound) = false; want true for %v", err)
	}

	if err := s.CreateUser(&User{ID: "U1", Login: "a", Email: "a"}); err != nil {
		t.Fatal(err)
	}
	dupe := &User{ID: "U2", Login: "a", Email: "a"}
	if err := s.CreateUser(dupe); !errors.Is(err, ErrConflict) {
		t.Errorf("errors.Is(ErrConflict) = false; want true for %v", err)
	}
}
