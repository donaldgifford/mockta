package store

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-memdb"
)

// Sentinel errors. Handlers map these to HTTP status codes — ErrNotFound
// to 404, ErrConflict to 409 (Okta-style uniqueness violation).
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Store is mockta's in-memory persistence layer. It wraps a
// *memdb.MemDB and provides typed accessors over the schema in
// schema.go. The zero value is not useful — use New.
//
// Reset() atomically swaps the inner *memdb.MemDB, so concurrent
// readers see either the old or the new state but never a partial
// reset. The mu mutex guards the swap; the auditSeq is independent
// because audit IDs only need monotonicity within a single Store
// lifetime.
type Store struct {
	mu       sync.RWMutex
	db       *memdb.MemDB
	auditSeq atomic.Uint64
}

// New constructs an empty Store. Returns an error if the schema is
// malformed — practically unreachable since schema.go is hard-coded,
// but go-memdb's API surfaces this so we propagate it.
func New() (*Store, error) {
	db, err := memdb.NewMemDB(schema())
	if err != nil {
		return nil, fmt.Errorf("build memdb: %w", err)
	}
	return &Store{db: db}, nil
}

// Reset wipes all tables in a single atomic swap. Used by the
// /admin/reset endpoint (and by tests). After Reset returns, every
// table is empty and the audit sequence is reset to zero.
func (s *Store) Reset() error {
	fresh, err := memdb.NewMemDB(schema())
	if err != nil {
		return fmt.Errorf("build replacement memdb: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = fresh
	s.auditSeq.Store(0)
	return nil
}

// readTxn returns a read-only transaction against the current DB.
// Callers must Abort the txn when done — go-memdb's read txns are
// snapshots, so a long-lived one would pin memory.
func (s *Store) readTxn() *memdb.Txn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db.Txn(false)
}

// writeTxn returns a writable transaction. The caller must Commit or
// Abort. Concurrent writeTxn calls are allowed; go-memdb serializes
// them internally.
func (s *Store) writeTxn() *memdb.Txn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db.Txn(true)
}
