package store

import (
	"fmt"
	"slices"
	"time"
)

// AppendAudit writes an audit entry for one HTTP request. The ID is
// assigned here (not by the caller) so the monotonic sequence stays
// authoritative in one place. The TS is also stamped here using
// time.Now if the caller didn't set one — handlers can pre-set TS for
// determinism in tests.
//
// AppendAudit takes *AuditEntry so the caller's struct is mutated in
// place (ID gets back-filled, TS may get back-filled). This avoids
// the 88-byte copy gocritic flags on the value form.
func (s *Store) AppendAudit(e *AuditEntry) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	e.ID = s.auditSeq.Add(1)

	txn := s.writeTxn()
	defer txn.Abort()

	if err := txn.Insert(TableAuditLog, e); err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	txn.Commit()
	return nil
}

// AuditByGap returns every audit entry whose GapID matches. Drives
// `mockta gaps list --runtime`.
func (s *Store) AuditByGap(gapID string) ([]*AuditEntry, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableAuditLog, IndexGapID, gapID)
	if err != nil {
		return nil, fmt.Errorf("iterate audit by gap: %w", err)
	}

	out := make([]*AuditEntry, 0)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		e, err := asAuditEntry(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// GapsHit returns the set of distinct GapIDs that appear in the audit
// log. The empty-string GapID (i.e., requests that hit an implemented
// endpoint) is excluded. Order is deterministic — sorted ascending —
// so callers can golden-compare without extra sorting.
func (s *Store) GapsHit() ([]string, error) {
	txn := s.readTxn()
	defer txn.Abort()

	it, err := txn.Get(TableAuditLog, IndexGapID)
	if err != nil {
		return nil, fmt.Errorf("iterate audit by gap: %w", err)
	}

	seen := make(map[string]struct{})
	for raw := it.Next(); raw != nil; raw = it.Next() {
		e, err := asAuditEntry(raw)
		if err != nil {
			return nil, err
		}
		if e.GapID == "" {
			continue
		}
		seen[e.GapID] = struct{}{}
	}

	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	// Deterministic ordering. go-memdb's iterator order is index
	// order, but our seen map randomizes it, so we re-sort.
	slices.Sort(out)
	return out, nil
}
