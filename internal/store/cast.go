package store

import "fmt"

// The cast helpers convert go-memdb's `any`-typed query results back
// to the concrete domain types. The type assertion is schema-
// guaranteed to succeed under normal operation; a failure here means
// the in-memory schema and the Go type wired into Insert() disagree,
// which is a programmer error. We return an error rather than panic
// so the surrounding handler can convert to a 500 response.
//
// Each helper returns a shallow copy rather than the pointer go-memdb
// holds. Handlers mutate the result (lifecycle flips, UpdatedAt) before
// calling Update*, and mutating the stored object in place would race
// with concurrent readers and break memdb's snapshot isolation. The
// raw-JSON blobs are shared, which is fine because nothing writes into
// them in place.

func asUser(raw any) (*User, error) {
	v, ok := raw.(*User)
	if !ok {
		return nil, fmt.Errorf("internal: stored value is not *User: %T", raw)
	}
	c := *v
	return &c, nil
}

func asGroup(raw any) (*Group, error) {
	v, ok := raw.(*Group)
	if !ok {
		return nil, fmt.Errorf("internal: stored value is not *Group: %T", raw)
	}
	c := *v
	return &c, nil
}

func asApp(raw any) (*App, error) {
	v, ok := raw.(*App)
	if !ok {
		return nil, fmt.Errorf("internal: stored value is not *App: %T", raw)
	}
	c := *v
	return &c, nil
}

func asGroupMembership(raw any) (*GroupMembership, error) {
	v, ok := raw.(*GroupMembership)
	if !ok {
		return nil, fmt.Errorf("internal: stored value is not *GroupMembership: %T", raw)
	}
	c := *v
	return &c, nil
}

func asAuditEntry(raw any) (*AuditEntry, error) {
	v, ok := raw.(*AuditEntry)
	if !ok {
		return nil, fmt.Errorf("internal: stored value is not *AuditEntry: %T", raw)
	}
	c := *v
	return &c, nil
}
