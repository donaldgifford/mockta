// Package store is mockta's persistence layer.
//
// v0 uses hashicorp/go-memdb — pure-Go, in-memory, MVCC with secondary
// indexes. No external DB, no migrations, no cgo. Per DESIGN-0001
// §State model, `terraform test` working sets are bounded and per-run,
// so a real KV/SQL engine would be overkill; go-memdb gives indexed
// lookups and snapshot reads for free.
//
// Domain structs in this file are the unit of read/write between the
// store and the handlers. `Profile` / `Settings` fields are stored as
// raw JSON ([]byte) so the store doesn't need to model every field of
// the provider's payload — the provider sends and reads the blob
// round-trip.
package store

import "time"

// User is the persisted form of /api/v1/users.
//
// Login and Email are duplicated from Profile so go-memdb can build
// secondary indexes on them — the handlers extract these from the
// inbound payload and we keep them in sync on every write.
type User struct {
	ID        string
	Login     string
	Email     string
	Status    string
	Profile   []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Group is the persisted form of /api/v1/groups.
//
// v0 only accepts Type == "OKTA_GROUP"; APP_GROUP and BUILT_IN return
// 501 with a gap-list ID from the handler layer.
type Group struct {
	ID          string
	Name        string
	Type        string
	Description string
	Profile     []byte
	CreatedAt   time.Time
}

// App is the persisted form of /api/v1/apps.
//
// v0 only accepts SignOnMode == "SAML_2_0"; other modes return 501.
// Status flips synchronously via the lifecycle endpoints.
type App struct {
	ID         string
	Name       string
	Label      string
	Status     string
	SignOnMode string
	Settings   []byte
	CreatedAt  time.Time
}

// GroupMembership is the join row between a Group and a User. The
// (GroupID, UserID) pair is the primary key — go-memdb's
// CompoundIndex handles the composite.
type GroupMembership struct {
	GroupID   string
	UserID    string
	CreatedAt time.Time
}

// AuditEntry captures one HTTP request for the audit log + gap report.
//
// ID is a per-Store monotonically increasing uint64 (not a ULID — the
// DESIGN doc said ULID, but UnixNano + a tiebreaker would have the
// same properties with two extra problems; a simple monotonic counter
// is unique, ordered, and dep-free).
//
// GapID is empty when the request hit an implemented endpoint. When
// non-empty it carries the MOCKTA_GAP_NNNN code the gap registry
// (Phase 5) emitted with the 501 response.
type AuditEntry struct {
	ID     uint64
	TS     time.Time
	Method string
	Path   string
	Status int
	GapID  string
}
