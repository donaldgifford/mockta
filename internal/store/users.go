package store

import (
	"fmt"
)

// CreateUser inserts u. Returns ErrConflict if Login is already taken
// or if ID collides with an existing row. The caller is responsible
// for setting ID/CreatedAt/UpdatedAt before calling — keeping those
// out of the store means handlers control the clock for testability.
func (s *Store) CreateUser(u *User) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableUsers, IndexLogin, u.Login)
	if err != nil {
		return fmt.Errorf("lookup user by login: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("user with login %q: %w", u.Login, ErrConflict)
	}

	if err := txn.Insert(TableUsers, u); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	txn.Commit()
	return nil
}

// GetUser returns the user with the given ID, or ErrNotFound.
func (s *Store) GetUser(id string) (*User, error) {
	txn := s.readTxn()
	defer txn.Abort()

	raw, err := txn.First(TableUsers, IndexID, id)
	if err != nil {
		return nil, fmt.Errorf("lookup user by id: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("user %q: %w", id, ErrNotFound)
	}
	return asUser(raw)
}

// GetUserByLogin returns the user with the given Login, or ErrNotFound.
// Used for Okta's /api/v1/users/{id_or_login} convenience.
func (s *Store) GetUserByLogin(login string) (*User, error) {
	txn := s.readTxn()
	defer txn.Abort()

	raw, err := txn.First(TableUsers, IndexLogin, login)
	if err != nil {
		return nil, fmt.Errorf("lookup user by login: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("user with login %q: %w", login, ErrNotFound)
	}
	return asUser(raw)
}

// UpdateUser replaces the user with the same ID. Returns ErrNotFound
// if no such user exists, ErrConflict if the new Login collides with
// a different existing user.
func (s *Store) UpdateUser(u *User) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existingRaw, err := txn.First(TableUsers, IndexID, u.ID)
	if err != nil {
		return fmt.Errorf("lookup user by id: %w", err)
	}
	if existingRaw == nil {
		return fmt.Errorf("user %q: %w", u.ID, ErrNotFound)
	}
	existing, err := asUser(existingRaw)
	if err != nil {
		return err
	}

	if existing.Login != u.Login {
		if err := ensureLoginAvailable(txn, u.Login, u.ID); err != nil {
			return err
		}
	}

	if err := txn.Insert(TableUsers, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	txn.Commit()
	return nil
}

// ensureLoginAvailable returns ErrConflict if the login is taken by a
// user other than allowedID. Returns nil if the login is free or only
// claimed by allowedID itself.
func ensureLoginAvailable(txn txnReader, login, allowedID string) error {
	raw, err := txn.First(TableUsers, IndexLogin, login)
	if err != nil {
		return fmt.Errorf("lookup user by login: %w", err)
	}
	if raw == nil {
		return nil
	}
	other, err := asUser(raw)
	if err != nil {
		return err
	}
	if other.ID != allowedID {
		return fmt.Errorf("user with login %q: %w", login, ErrConflict)
	}
	return nil
}

// DeleteUser removes the user by ID. Cascades to group memberships:
// every (group, user) row referencing this user is also removed.
// Returns ErrNotFound if no such user exists.
func (s *Store) DeleteUser(id string) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableUsers, IndexID, id)
	if err != nil {
		return fmt.Errorf("lookup user by id: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("user %q: %w", id, ErrNotFound)
	}

	if err := txn.Delete(TableUsers, existing); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Cascade: drop every membership row for this user.
	if _, err := txn.DeleteAll(TableGroupMemberships, IndexUserID, id); err != nil {
		return fmt.Errorf("cascade memberships: %w", err)
	}

	txn.Commit()
	return nil
}

// ListUsers returns every user, ordered by ID. limit <= 0 returns all.
// v0 doesn't implement filter — handlers do filter parsing and call
// this for the unfiltered case; they fall back to per-row checks for
// filtered queries since the working set is small.
func (s *Store) ListUsers(limit int) ([]*User, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableUsers, IndexID)
	if err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	out := make([]*User, 0)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		u, err := asUser(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
