package store

import "github.com/hashicorp/go-memdb"

// Table names. Kept as exported constants so handlers and tests can
// reference them via the `store.Table*` symbols rather than by string
// literal — typos turn into compile errors instead of silent failures.
const (
	TableUsers            = "users"
	TableGroups           = "groups"
	TableApps             = "apps"
	TableGroupMemberships = "group_memberships"
	TableAuditLog         = "audit_log"
)

// Index names. Following the same exported-constant pattern as table
// names. The "id" index is implicit on every table.
const (
	IndexID         = "id"
	IndexLogin      = "login"
	IndexEmail      = "email"
	IndexName       = "name"
	IndexType       = "type"
	IndexLabel      = "label"
	IndexSignOnMode = "sign_on_mode"
	IndexGroupID    = "group_id"
	IndexUserID     = "user_id"
	IndexTS         = "ts"
	IndexGapID      = "gap_id"
)

// schema builds the go-memdb schema described in DESIGN-0001 §Data
// Model. Called once at Store construction; the result is immutable.
// Per-table helpers below keep this function short and the table
// schemas individually readable.
func schema() *memdb.DBSchema {
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			TableUsers:            usersSchema(),
			TableGroups:           groupsSchema(),
			TableApps:             appsSchema(),
			TableGroupMemberships: groupMembershipsSchema(),
			TableAuditLog:         auditLogSchema(),
		},
	}
}

func usersSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableUsers,
		Indexes: map[string]*memdb.IndexSchema{
			IndexID: {
				Name:    IndexID,
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "ID"},
			},
			IndexLogin: {
				Name:    IndexLogin,
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "Login"},
			},
			IndexEmail: {
				Name:    IndexEmail,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "Email"},
			},
		},
	}
}

func groupsSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableGroups,
		Indexes: map[string]*memdb.IndexSchema{
			IndexID: {
				Name:    IndexID,
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "ID"},
			},
			IndexName: {
				Name:    IndexName,
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "Name"},
			},
			IndexType: {
				Name:    IndexType,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "Type"},
			},
		},
	}
}

func appsSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableApps,
		Indexes: map[string]*memdb.IndexSchema{
			IndexID: {
				Name:    IndexID,
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "ID"},
			},
			IndexLabel: {
				Name:    IndexLabel,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "Label"},
			},
			IndexSignOnMode: {
				Name:    IndexSignOnMode,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "SignOnMode"},
			},
		},
	}
}

func groupMembershipsSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableGroupMemberships,
		Indexes: map[string]*memdb.IndexSchema{
			// Primary: composite (GroupID, UserID). go-memdb
			// requires a unique primary, so we synthesize one
			// from both fields.
			IndexID: {
				Name:   IndexID,
				Unique: true,
				Indexer: &memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{Field: "GroupID"},
						&memdb.StringFieldIndex{Field: "UserID"},
					},
				},
			},
			IndexGroupID: {
				Name:    IndexGroupID,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "GroupID"},
			},
			IndexUserID: {
				Name:    IndexUserID,
				Unique:  false,
				Indexer: &memdb.StringFieldIndex{Field: "UserID"},
			},
		},
	}
}

func auditLogSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableAuditLog,
		Indexes: map[string]*memdb.IndexSchema{
			IndexID: {
				Name:    IndexID,
				Unique:  true,
				Indexer: &memdb.UintFieldIndex{Field: "ID"},
			},
			// Secondary on GapID lets `mockta gaps list
			// --runtime` walk the audit log by gap quickly.
			// AllowMissing lets empty-string GapID rows in (most
			// audit rows are non-gap hits and we still want them
			// indexed for completeness); iteration callers filter
			// the empty entries out.
			IndexGapID: {
				Name:         IndexGapID,
				Unique:       false,
				AllowMissing: true,
				Indexer:      &memdb.StringFieldIndex{Field: "GapID"},
			},
		},
	}
}
