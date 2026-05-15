package store

import "fmt"

// CreateGroup inserts g. Returns ErrConflict if Name is already taken.
func (s *Store) CreateGroup(g *Group) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableGroups, IndexName, g.Name)
	if err != nil {
		return fmt.Errorf("lookup group by name: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("group with name %q: %w", g.Name, ErrConflict)
	}

	if err := txn.Insert(TableGroups, g); err != nil {
		return fmt.Errorf("insert group: %w", err)
	}
	txn.Commit()
	return nil
}

// GetGroup returns the group with the given ID, or ErrNotFound.
func (s *Store) GetGroup(id string) (*Group, error) {
	txn := s.readTxn()
	defer txn.Abort()

	raw, err := txn.First(TableGroups, IndexID, id)
	if err != nil {
		return nil, fmt.Errorf("lookup group by id: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("group %q: %w", id, ErrNotFound)
	}
	return asGroup(raw)
}

// UpdateGroup replaces the group with the same ID. Returns ErrNotFound
// if no such group exists, ErrConflict on Name collision with another
// group.
func (s *Store) UpdateGroup(g *Group) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existingRaw, err := txn.First(TableGroups, IndexID, g.ID)
	if err != nil {
		return fmt.Errorf("lookup group by id: %w", err)
	}
	if existingRaw == nil {
		return fmt.Errorf("group %q: %w", g.ID, ErrNotFound)
	}
	existing, err := asGroup(existingRaw)
	if err != nil {
		return err
	}

	if existing.Name != g.Name {
		if err := ensureGroupNameAvailable(txn, g.Name, g.ID); err != nil {
			return err
		}
	}

	if err := txn.Insert(TableGroups, g); err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	txn.Commit()
	return nil
}

// ensureGroupNameAvailable returns ErrConflict if the name is taken
// by a group other than allowedID.
func ensureGroupNameAvailable(txn txnReader, name, allowedID string) error {
	raw, err := txn.First(TableGroups, IndexName, name)
	if err != nil {
		return fmt.Errorf("lookup group by name: %w", err)
	}
	if raw == nil {
		return nil
	}
	other, err := asGroup(raw)
	if err != nil {
		return err
	}
	if other.ID != allowedID {
		return fmt.Errorf("group with name %q: %w", name, ErrConflict)
	}
	return nil
}

// DeleteGroup removes the group by ID. Cascades to memberships: every
// (group, user) row for this group is also removed.
func (s *Store) DeleteGroup(id string) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableGroups, IndexID, id)
	if err != nil {
		return fmt.Errorf("lookup group by id: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("group %q: %w", id, ErrNotFound)
	}

	if err := txn.Delete(TableGroups, existing); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}

	if _, err := txn.DeleteAll(TableGroupMemberships, IndexGroupID, id); err != nil {
		return fmt.Errorf("cascade memberships: %w", err)
	}

	txn.Commit()
	return nil
}

// ListGroups returns every group, ordered by ID. limit <= 0 returns all.
func (s *Store) ListGroups(limit int) ([]*Group, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableGroups, IndexID)
	if err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}

	out := make([]*Group, 0)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		g, err := asGroup(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// AddGroupMembership inserts (groupID, userID). Idempotent: re-adding
// an existing row is a no-op (Okta's PUT semantics). Returns
// ErrNotFound if either the group or the user doesn't exist.
func (s *Store) AddGroupMembership(m *GroupMembership) error {
	txn := s.writeTxn()
	defer txn.Abort()

	if err := ensureExists(txn, TableGroups, IndexID, m.GroupID, "group"); err != nil {
		return err
	}
	if err := ensureExists(txn, TableUsers, IndexID, m.UserID, "user"); err != nil {
		return err
	}

	if err := txn.Insert(TableGroupMemberships, m); err != nil {
		return fmt.Errorf("insert membership: %w", err)
	}
	txn.Commit()
	return nil
}

// RemoveGroupMembership deletes (groupID, userID). Returns ErrNotFound
// if the membership doesn't exist.
func (s *Store) RemoveGroupMembership(groupID, userID string) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableGroupMemberships, IndexID, groupID, userID)
	if err != nil {
		return fmt.Errorf("lookup membership: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("membership (%q,%q): %w", groupID, userID, ErrNotFound)
	}

	if err := txn.Delete(TableGroupMemberships, existing); err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	txn.Commit()
	return nil
}

// ListGroupMembers returns every membership for the given group.
func (s *Store) ListGroupMembers(groupID string) ([]*GroupMembership, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableGroupMemberships, IndexGroupID, groupID)
	if err != nil {
		return nil, fmt.Errorf("iterate memberships by group: %w", err)
	}

	out := make([]*GroupMembership, 0)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		m, err := asGroupMembership(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// ensureExists returns ErrNotFound if the keyed row doesn't exist.
// Used by AddGroupMembership to enforce referential integrity before
// the join row is inserted.
func ensureExists(txn txnReader, table, index, key, label string) error {
	raw, err := txn.First(table, index, key)
	if err != nil {
		return fmt.Errorf("lookup %s by %s: %w", label, index, err)
	}
	if raw == nil {
		return fmt.Errorf("%s %q: %w", label, key, ErrNotFound)
	}
	return nil
}

// txnReader narrows *memdb.Txn down to the surface ensureExists needs.
// Defining it locally documents what the helper actually uses.
type txnReader interface {
	First(table, index string, args ...any) (any, error)
}
