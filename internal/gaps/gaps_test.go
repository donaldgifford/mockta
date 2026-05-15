package gaps

import (
	"regexp"
	"strings"
	"testing"
)

func TestStatic_LookupHandlerGap(t *testing.T) {
	t.Parallel()
	reg := Static()
	got, known := reg.Lookup("POST", "/api/v1/groups")
	if !known {
		t.Fatalf("Lookup(POST /api/v1/groups) = (%q, false), want known", got)
	}
	// The first match wins; the registry orders handler-internal gaps
	// before path-resource gaps, so this should resolve to the
	// APP_GROUP entry. Either group entry is acceptable — both share
	// the endpoint.
	if got != IDGroupTypeAppGroup && got != IDGroupTypeBuiltIn {
		t.Errorf("Lookup = %q, want one of the group-type gaps", got)
	}
}

func TestStatic_LookupResourcePrefix(t *testing.T) {
	t.Parallel()
	reg := Static()
	cases := map[string]string{
		"/api/v1/policies":             "MOCKTA_GAP_0010",
		"/api/v1/policies/abc/rules":   "MOCKTA_GAP_0010",
		"/api/v1/authorizationServers": "MOCKTA_GAP_0011",
		"/api/v1/idps":                 "MOCKTA_GAP_0012",
		"/api/v1/trustedOrigins":       "MOCKTA_GAP_0013",
		"/api/v1/inlineHooks/x":        "MOCKTA_GAP_0014",
		"/scim/v2/Users":               "MOCKTA_GAP_0020",
	}
	for path, want := range cases {
		got, known := reg.Lookup("GET", path)
		if !known {
			t.Errorf("path=%q: known=false, want %s", path, want)
			continue
		}
		if got != want {
			t.Errorf("path=%q: got %s, want %s", path, got, want)
		}
	}
}

func TestStatic_LookupUnknown(t *testing.T) {
	t.Parallel()
	got, known := Static().Lookup("GET", "/api/v1/this-endpoint-does-not-exist")
	if known {
		t.Errorf("unknown path reported as known: %s", got)
	}
	if got != UncataloguedID {
		t.Errorf("unknown path got %q, want %q", got, UncataloguedID)
	}
}

func TestStatic_All_IsCopy(t *testing.T) {
	t.Parallel()
	reg := Static()
	all := reg.All()
	if len(all) == 0 {
		t.Fatal("All() returned empty slice")
	}
	// Mutate the returned slice and verify subsequent calls aren't
	// affected — All must return a defensive copy.
	all[0].Status = "tampered"
	again := reg.All()
	if again[0].Status == "tampered" {
		t.Error("All() returned slice aliases internal state")
	}
}

func TestIDsAreWellFormed(t *testing.T) {
	t.Parallel()
	pat := regexp.MustCompile(`^MOCKTA_GAP_\d{4}$`)
	for _, g := range Static().All() {
		if !pat.MatchString(g.ID) {
			t.Errorf("gap %q has malformed ID", g.ID)
		}
	}
}

func TestIDsAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for _, g := range Static().All() {
		if _, dup := seen[g.ID]; dup {
			t.Errorf("duplicate gap ID: %s", g.ID)
		}
		seen[g.ID] = struct{}{}
	}
}

func TestRegistryCoversHandlerIDs(t *testing.T) {
	t.Parallel()
	want := []string{
		IDUserFilter, IDGroupFilter, IDAppFilter,
		IDGroupTypeAppGroup, IDGroupTypeBuiltIn, IDAppSignOnNonSAML,
	}
	have := make(map[string]struct{})
	for _, g := range Static().All() {
		have[g.ID] = struct{}{}
	}
	for _, id := range want {
		if _, ok := have[id]; !ok {
			t.Errorf("handler-internal ID %s missing from registry", id)
		}
	}
}

func TestStub_Lookup(t *testing.T) {
	t.Parallel()
	got, known := StubRegistry{}.Lookup("GET", "/anything")
	if known {
		t.Error("stub reported known=true")
	}
	if got != UncataloguedID {
		t.Errorf("stub got %q, want %q", got, UncataloguedID)
	}
	if all := (StubRegistry{}.All()); all != nil {
		t.Errorf("stub.All() = %v, want nil", all)
	}
}

func TestMatchPattern(t *testing.T) {
	t.Parallel()
	cases := []struct {
		endpoint, method, path string
		want                   bool
	}{
		{"POST /api/v1/groups", "POST", "/api/v1/groups", true},
		{"POST /api/v1/groups", "GET", "/api/v1/groups", false},
		{"/api/v1/policies", "GET", "/api/v1/policies/abc", true},
		{"/api/v1/policies", "GET", "/api/v1/groups", false},
	}
	for _, tc := range cases {
		t.Run(tc.endpoint+":"+tc.method+tc.path, func(t *testing.T) {
			t.Parallel()
			if got := matchPattern(tc.endpoint, tc.method, tc.path); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEndpointFieldShape(t *testing.T) {
	t.Parallel()
	// Every endpoint is either "METHOD /path" or "/path" — never an
	// empty string. Catches accidental zero-value entries.
	for _, g := range Static().All() {
		if g.Endpoint == "" {
			t.Errorf("gap %s has empty Endpoint", g.ID)
		}
		if strings.Contains(g.Endpoint, "  ") {
			t.Errorf("gap %s endpoint has double space: %q", g.ID, g.Endpoint)
		}
	}
}
