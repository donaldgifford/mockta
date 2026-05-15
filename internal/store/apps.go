package store

import "fmt"

// CreateApp inserts a. No uniqueness constraint on Label (Okta allows
// duplicate labels), so the only check is on the ID.
func (s *Store) CreateApp(a *App) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableApps, IndexID, a.ID)
	if err != nil {
		return fmt.Errorf("lookup app by id: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("app %q: %w", a.ID, ErrConflict)
	}

	if err := txn.Insert(TableApps, a); err != nil {
		return fmt.Errorf("insert app: %w", err)
	}
	txn.Commit()
	return nil
}

// GetApp returns the app with the given ID, or ErrNotFound.
func (s *Store) GetApp(id string) (*App, error) {
	txn := s.readTxn()
	defer txn.Abort()

	raw, err := txn.First(TableApps, IndexID, id)
	if err != nil {
		return nil, fmt.Errorf("lookup app by id: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("app %q: %w", id, ErrNotFound)
	}
	return asApp(raw)
}

// UpdateApp replaces the app with the same ID. Returns ErrNotFound if
// no such app exists.
func (s *Store) UpdateApp(a *App) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableApps, IndexID, a.ID)
	if err != nil {
		return fmt.Errorf("lookup app by id: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("app %q: %w", a.ID, ErrNotFound)
	}

	if err := txn.Insert(TableApps, a); err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	txn.Commit()
	return nil
}

// DeleteApp removes the app by ID. Returns ErrNotFound if no such app
// exists.
func (s *Store) DeleteApp(id string) error {
	txn := s.writeTxn()
	defer txn.Abort()

	existing, err := txn.First(TableApps, IndexID, id)
	if err != nil {
		return fmt.Errorf("lookup app by id: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("app %q: %w", id, ErrNotFound)
	}

	if err := txn.Delete(TableApps, existing); err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	txn.Commit()
	return nil
}

// ListApps returns every app, ordered by ID. limit <= 0 returns all.
func (s *Store) ListApps(limit int) ([]*App, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableApps, IndexID)
	if err != nil {
		return nil, fmt.Errorf("iterate apps: %w", err)
	}

	out := make([]*App, 0)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		a, err := asApp(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
