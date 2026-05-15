package store

import (
	"errors"
	"testing"
)

// newTestStore is a test helper that returns a fresh Store and fails
// the test on construction error — the schema is hard-coded, so an
// error here is a programmer bug, not a test condition.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatalf("New() = %v, want no error", err)
	}
	return s
}

func TestStore_UserCRUD(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	alice := &User{
		ID:     NewID(KindUser, "alice@example.com"),
		Login:  "alice@example.com",
		Email:  "alice@example.com",
		Status: "ACTIVE",
	}

	// Create.
	if err := s.CreateUser(alice); err != nil {
		t.Fatalf("CreateUser(alice) = %v, want no error", err)
	}

	// Duplicate Login → ErrConflict.
	dupe := *alice
	dupe.ID = NewID(KindUser, "different-key")
	if err := s.CreateUser(&dupe); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateUser(dupe login) = %v, want ErrConflict", err)
	}

	// Get by ID.
	got, err := s.GetUser(alice.ID)
	if err != nil {
		t.Fatalf("GetUser(alice.ID) = %v, want no error", err)
	}
	if got.Login != alice.Login {
		t.Errorf("GetUser(...).Login = %q, want %q", got.Login, alice.Login)
	}

	// Get by login.
	gotByLogin, err := s.GetUserByLogin(alice.Login)
	if err != nil {
		t.Fatalf("GetUserByLogin(alice.Login) = %v, want no error", err)
	}
	if gotByLogin.ID != alice.ID {
		t.Errorf("GetUserByLogin(...).ID = %q, want %q", gotByLogin.ID, alice.ID)
	}

	// Get missing → ErrNotFound.
	if _, err := s.GetUser("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUser(missing) = %v, want ErrNotFound", err)
	}

	// Update.
	aliceUpdated := *alice
	aliceUpdated.Status = "SUSPENDED"
	if err := s.UpdateUser(&aliceUpdated); err != nil {
		t.Fatalf("UpdateUser(alice) = %v, want no error", err)
	}
	got, _ = s.GetUser(alice.ID)
	if got.Status != "SUSPENDED" {
		t.Errorf("after UpdateUser, status = %q, want SUSPENDED", got.Status)
	}

	// Delete.
	if err := s.DeleteUser(alice.ID); err != nil {
		t.Fatalf("DeleteUser(alice.ID) = %v, want no error", err)
	}
	if _, err := s.GetUser(alice.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUser after delete = %v, want ErrNotFound", err)
	}

	// Delete missing → ErrNotFound.
	if err := s.DeleteUser("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteUser(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateUser_LoginConflict(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	alice := &User{
		ID: NewID(KindUser, "alice"), Login: "alice@x", Email: "alice@x",
	}
	bob := &User{
		ID: NewID(KindUser, "bob"), Login: "bob@x", Email: "bob@x",
	}
	if err := s.CreateUser(alice); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(bob); err != nil {
		t.Fatal(err)
	}

	// Alice tries to take Bob's login.
	aliceWantsBob := *alice
	aliceWantsBob.Login = "bob@x"
	if err := s.UpdateUser(&aliceWantsBob); !errors.Is(err, ErrConflict) {
		t.Errorf("UpdateUser(alice → bob's login) = %v, want ErrConflict", err)
	}

	// Updating Alice with her own login is fine (no-op on Login).
	if err := s.UpdateUser(alice); err != nil {
		t.Errorf("UpdateUser(alice unchanged) = %v, want no error", err)
	}

	// Updating a missing user.
	missing := &User{ID: "missing", Login: "x", Email: "x"}
	if err := s.UpdateUser(missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateUser(missing) = %v, want ErrNotFound", err)
	}

	// Renaming Alice's login to a brand-new value (covers
	// ensureLoginAvailable's "no clash" path).
	aliceRenamed := *alice
	aliceRenamed.Login = "alice.new@x"
	if err := s.UpdateUser(&aliceRenamed); err != nil {
		t.Errorf("UpdateUser(alice rename) = %v, want no error", err)
	}
}

func TestStore_GetUserByLogin_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if _, err := s.GetUserByLogin("missing@x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUserByLogin(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_AddGroupMembership_MissingUser(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	eng := &Group{ID: NewID(KindGroup, "eng"), Name: "eng", Type: "OKTA_GROUP"}
	if err := s.CreateGroup(eng); err != nil {
		t.Fatal(err)
	}
	err := s.AddGroupMembership(&GroupMembership{GroupID: eng.ID, UserID: "missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("AddGroupMembership(missing user) = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteGroup_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.DeleteGroup("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteGroup(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteApp_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.DeleteApp("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteApp(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_GetApp_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if _, err := s.GetApp("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetApp(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_CreateApp_Duplicate(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	app := &App{ID: "A1", Label: "x", SignOnMode: "SAML_2_0"}
	if err := s.CreateApp(app); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApp(app); !errors.Is(err, ErrConflict) {
		t.Errorf("CreateApp(dupe id) = %v, want ErrConflict", err)
	}
}

func TestStore_ListUsers_Limit(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	for i := range 5 {
		u := &User{
			ID:    NewID(KindUser, string(rune('a'+i))),
			Login: string(rune('a' + i)),
			Email: string(rune('a' + i)),
		}
		if err := s.CreateUser(u); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListUsers(0)
	if err != nil || len(all) != 5 {
		t.Errorf("ListUsers(0) = %v / %d, want 5", err, len(all))
	}
	two, err := s.ListUsers(2)
	if err != nil || len(two) != 2 {
		t.Errorf("ListUsers(2) = %v / %d, want 2", err, len(two))
	}
}

func TestStore_UpdateGroup_NameConflict(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	eng := &Group{ID: NewID(KindGroup, "engineers"), Name: "engineers", Type: "OKTA_GROUP"}
	ops := &Group{ID: NewID(KindGroup, "ops"), Name: "ops", Type: "OKTA_GROUP"}
	if err := s.CreateGroup(eng); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGroup(ops); err != nil {
		t.Fatal(err)
	}

	// Rename eng to "ops" — should conflict.
	engWantsOps := *eng
	engWantsOps.Name = "ops"
	if err := s.UpdateGroup(&engWantsOps); !errors.Is(err, ErrConflict) {
		t.Errorf("UpdateGroup(name conflict) = %v, want ErrConflict", err)
	}

	// Rename eng to a free name — should succeed.
	engRenamed := *eng
	engRenamed.Name = "backend"
	if err := s.UpdateGroup(&engRenamed); err != nil {
		t.Errorf("UpdateGroup(free name) = %v, want no error", err)
	}

	// Updating missing group.
	missing := &Group{ID: "missing", Name: "x", Type: "OKTA_GROUP"}
	if err := s.UpdateGroup(missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateGroup(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_ListGroups(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, name := range []string{"a", "b", "c"} {
		if err := s.CreateGroup(&Group{ID: NewID(KindGroup, name), Name: name, Type: "OKTA_GROUP"}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListGroups(0)
	if err != nil || len(all) != 3 {
		t.Errorf("ListGroups = %v / %d, want 3", err, len(all))
	}
	one, err := s.ListGroups(1)
	if err != nil || len(one) != 1 {
		t.Errorf("ListGroups(1) = %v / %d, want 1", err, len(one))
	}
}

func TestStore_RemoveGroupMembership(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	alice := &User{ID: NewID(KindUser, "alice"), Login: "alice", Email: "alice"}
	eng := &Group{ID: NewID(KindGroup, "eng"), Name: "eng", Type: "OKTA_GROUP"}
	if err := s.CreateUser(alice); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGroup(eng); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGroupMembership(&GroupMembership{GroupID: eng.ID, UserID: alice.ID}); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveGroupMembership(eng.ID, alice.ID); err != nil {
		t.Errorf("RemoveGroupMembership = %v, want no error", err)
	}
	// Second remove → ErrNotFound.
	if err := s.RemoveGroupMembership(eng.ID, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("RemoveGroupMembership(twice) = %v, want ErrNotFound", err)
	}
}

func TestStore_GroupCRUD(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	eng := &Group{
		ID:   NewID(KindGroup, "engineers"),
		Name: "engineers",
		Type: "OKTA_GROUP",
	}

	if err := s.CreateGroup(eng); err != nil {
		t.Fatalf("CreateGroup(eng) = %v", err)
	}

	dupe := *eng
	dupe.ID = NewID(KindGroup, "different-key")
	if err := s.CreateGroup(&dupe); !errors.Is(err, ErrConflict) {
		t.Errorf("CreateGroup(dupe name) = %v, want ErrConflict", err)
	}

	got, err := s.GetGroup(eng.ID)
	if err != nil || got.Name != eng.Name {
		t.Errorf("GetGroup = %v / %q, want eng/%q", err, got.Name, eng.Name)
	}

	if err := s.DeleteGroup(eng.ID); err != nil {
		t.Fatalf("DeleteGroup = %v", err)
	}
	if _, err := s.GetGroup(eng.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetGroup after delete = %v, want ErrNotFound", err)
	}
}

func TestStore_GroupMembership(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	alice := &User{
		ID: NewID(KindUser, "alice"), Login: "alice", Email: "alice@x",
	}
	eng := &Group{
		ID: NewID(KindGroup, "engineers"), Name: "engineers", Type: "OKTA_GROUP",
	}
	if err := s.CreateUser(alice); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGroup(eng); err != nil {
		t.Fatal(err)
	}

	m := &GroupMembership{GroupID: eng.ID, UserID: alice.ID}
	if err := s.AddGroupMembership(m); err != nil {
		t.Fatalf("AddGroupMembership = %v", err)
	}

	members, err := s.ListGroupMembers(eng.ID)
	if err != nil || len(members) != 1 || members[0].UserID != alice.ID {
		t.Errorf("ListGroupMembers = %v / %v, want one membership for alice",
			err, members)
	}

	// Adding a membership for a non-existent group fails.
	bad := &GroupMembership{GroupID: "missing", UserID: alice.ID}
	if err := s.AddGroupMembership(bad); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddGroupMembership(missing group) = %v, want ErrNotFound", err)
	}

	// Deleting the user cascades to its memberships.
	if err := s.DeleteUser(alice.ID); err != nil {
		t.Fatal(err)
	}
	members, _ = s.ListGroupMembers(eng.ID)
	if len(members) != 0 {
		t.Errorf("after DeleteUser, members = %v, want empty", members)
	}
}

func TestStore_AppCRUD(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	app := &App{
		ID:         NewID(KindApp, "AWS SSO"),
		Name:       "aws-sso",
		Label:      "AWS SSO",
		Status:     "ACTIVE",
		SignOnMode: "SAML_2_0",
	}
	if err := s.CreateApp(app); err != nil {
		t.Fatalf("CreateApp = %v", err)
	}
	got, err := s.GetApp(app.ID)
	if err != nil || got.Label != app.Label {
		t.Errorf("GetApp = %v / %q, want %q", err, got.Label, app.Label)
	}
	if err := s.DeleteApp(app.ID); err != nil {
		t.Fatalf("DeleteApp = %v", err)
	}
}

func TestStore_UpdateApp_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	missing := &App{ID: "missing", Label: "X", SignOnMode: "SAML_2_0"}
	if err := s.UpdateApp(missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateApp(missing) = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateApp_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	app := &App{
		ID: NewID(KindApp, "aws"), Label: "aws", SignOnMode: "SAML_2_0", Status: "INACTIVE",
	}
	if err := s.CreateApp(app); err != nil {
		t.Fatal(err)
	}
	app.Status = "ACTIVE"
	if err := s.UpdateApp(app); err != nil {
		t.Errorf("UpdateApp = %v", err)
	}
	got, _ := s.GetApp(app.ID)
	if got.Status != "ACTIVE" {
		t.Errorf("after UpdateApp, status = %q, want ACTIVE", got.Status)
	}
}

func TestStore_ListApps(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, label := range []string{"a", "b"} {
		if err := s.CreateApp(&App{
			ID: NewID(KindApp, label), Label: label, SignOnMode: "SAML_2_0",
		}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListApps(0)
	if err != nil || len(all) != 2 {
		t.Errorf("ListApps = %v / %d, want 2", err, len(all))
	}
	one, err := s.ListApps(1)
	if err != nil || len(one) != 1 {
		t.Errorf("ListApps(1) = %v / %d, want 1", err, len(one))
	}
}

func TestStore_AuditLog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	entries := []*AuditEntry{
		{Method: "GET", Path: "/api/v1/org", Status: 200},
		{Method: "POST", Path: "/api/v1/policies", Status: 501, GapID: "MOCKTA_GAP_0001"},
		{Method: "POST", Path: "/api/v1/factors", Status: 501, GapID: "MOCKTA_GAP_0002"},
		{Method: "POST", Path: "/api/v1/policies", Status: 501, GapID: "MOCKTA_GAP_0001"},
	}
	for _, e := range entries {
		if err := s.AppendAudit(e); err != nil {
			t.Fatalf("AppendAudit(%v) = %v", e, err)
		}
		if e.ID == 0 {
			t.Errorf("AppendAudit did not back-fill ID for %v", e)
		}
	}

	gaps, err := s.GapsHit()
	if err != nil {
		t.Fatalf("GapsHit = %v", err)
	}
	if len(gaps) != 2 ||
		gaps[0] != "MOCKTA_GAP_0001" ||
		gaps[1] != "MOCKTA_GAP_0002" {
		t.Errorf("GapsHit = %v, want [MOCKTA_GAP_0001 MOCKTA_GAP_0002]", gaps)
	}

	byGap, err := s.AuditByGap("MOCKTA_GAP_0001")
	if err != nil {
		t.Fatalf("AuditByGap = %v", err)
	}
	if len(byGap) != 2 {
		t.Errorf("AuditByGap(0001) returned %d entries, want 2", len(byGap))
	}
}

func TestStore_Reset(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	if err := s.CreateUser(&User{ID: "U1", Login: "u@x", Email: "u@x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAudit(&AuditEntry{Method: "GET", Path: "/", Status: 200}); err != nil {
		t.Fatal(err)
	}

	if err := s.Reset(); err != nil {
		t.Fatalf("Reset = %v", err)
	}

	if _, err := s.GetUser("U1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Reset, GetUser = %v, want ErrNotFound", err)
	}
	// Audit sequence is back to zero — next append yields ID=1.
	next := &AuditEntry{Method: "GET", Path: "/", Status: 200}
	if err := s.AppendAudit(next); err != nil {
		t.Fatal(err)
	}
	if next.ID != 1 {
		t.Errorf("after Reset, first AppendAudit ID = %d, want 1", next.ID)
	}
}
