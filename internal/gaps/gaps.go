// Package gaps owns the source-of-truth registry of Okta API
// endpoints mockta does not implement.
//
// Every 501 response (and every 400 routed through gap-tracking, like
// bad filter expressions) carries a stable MOCKTA_GAP_NNNN ID drawn
// from this registry. Consumers file mockta issues referencing the
// ID; the registry feeds `mockta gaps list/export` and the published
// docs/gaps.md page.
//
// IDs are allocated monotonically and never reused — even if a gap
// closes in a later release, its ID stays retired in this file with
// Status="closed-in-vX.Y" so a fixture pinned to the old ID still
// resolves to something meaningful.
package gaps

import "strings"

// UncataloguedID is emitted when a request hits an endpoint not in
// the static registry. The audit log captures the (method, path)
// pair, which the gap triage process uses to allocate a real ID.
const UncataloguedID = "MOCKTA_GAP_UNCATALOGED"

// Handler-internal gap IDs. These cover gaps inside endpoints that
// *are* routed (so the path-pattern lookup wouldn't catch them) —
// bad filter expressions, unsupported group types, etc. Handlers
// reference these constants when they emit the X-Mockta-Gap header.
const (
	IDUserFilter        = "MOCKTA_GAP_0001"
	IDGroupFilter       = "MOCKTA_GAP_0002"
	IDAppFilter         = "MOCKTA_GAP_0003"
	IDGroupTypeAppGroup = "MOCKTA_GAP_0004"
	IDGroupTypeBuiltIn  = "MOCKTA_GAP_0005"
	IDAppSignOnNonSAML  = "MOCKTA_GAP_0006"
)

// Gap describes one unimplemented surface of the Okta API.
//
// The shape mirrors DESIGN-0001 §Gap list. HitBy is a list of
// fixtures known to hit the gap — populated by hand as fixtures
// are added, not auto-discovered, so the registry stays the
// declarative source of truth.
type Gap struct {
	ID       string   `json:"id"`
	Method   string   `json:"method,omitempty"`
	Endpoint string   `json:"endpoint"`
	Resource string   `json:"resource,omitempty"`
	HitBy    []string `json:"hitBy,omitempty"`
	Status   string   `json:"status"`
	Notes    string   `json:"notes,omitempty"`
}

// Status values for Gap.Status. Closed-in-vN.N entries stay in the
// registry so historical references keep resolving.
const (
	StatusOpen       = "open"
	StatusInProgress = "in-progress"
)

// Registry resolves (method, path) pairs to MOCKTA_GAP_NNNN IDs.
// The notimplemented handler consumes this interface; production
// callers should use Static() to get the real implementation, while
// tests can swap in any value that satisfies the interface.
type Registry interface {
	// Lookup returns the gap ID for the given (method, path). The
	// boolean is true when the lookup hit a known gap; false means
	// "we've never seen this endpoint, file it under
	// UncataloguedID."
	Lookup(method, path string) (gapID string, known bool)

	// All returns every static gap entry — used by the
	// `mockta gaps list/export` subcommands.
	All() []Gap
}

// Static returns the populated Registry backed by the package-level
// gap list. The result is safe for concurrent use; entries are
// copied on read.
func Static() Registry { return staticRegistry{} }

type staticRegistry struct{}

// Lookup implements Registry. The walk is linear because the
// registry is small (tens of entries) and lookups happen on the
// 501 cold path — a map would add complexity for no measurable
// gain.
func (staticRegistry) Lookup(method, path string) (string, bool) {
	for _, g := range gaps {
		if g.Method != "" && g.Method != method {
			continue
		}
		if matchPattern(g.Endpoint, method, path) {
			return g.ID, true
		}
	}
	return UncataloguedID, false
}

// All implements Registry by returning a defensive copy of the
// underlying slice — callers (notably the CLI export) can sort or
// mutate the result without poisoning future lookups.
func (staticRegistry) All() []Gap {
	out := make([]Gap, len(gaps))
	copy(out, gaps)
	return out
}

// matchPattern reports whether path matches the endpoint declared in
// a Gap entry. The Endpoint field is "METHOD /prefix" — match is by
// method exact and path prefix. Bare paths (without a leading method)
// match any verb.
func matchPattern(endpoint, method, path string) bool {
	parts := strings.SplitN(endpoint, " ", 2)
	switch len(parts) {
	case 1:
		return strings.HasPrefix(path, parts[0])
	case 2:
		return parts[0] == method && strings.HasPrefix(path, parts[1])
	default:
		return false
	}
}

// StubRegistry is a fallback used by tests and by code paths that
// want the package-level zero behavior. It always returns
// UncataloguedID. Production code should use Static() instead.
type StubRegistry struct{}

// Lookup implements Registry.
func (StubRegistry) Lookup(_, _ string) (string, bool) {
	return UncataloguedID, false
}

// All implements Registry. The stub has no static entries.
func (StubRegistry) All() []Gap { return nil }

// gaps is the source-of-truth registry. Entries are allocated
// monotonically; do not renumber or remove. When a gap closes, set
// Status to "closed-in-vX.Y" rather than deleting the row, so
// downstream references keep resolving.
//
// Phase 4 handler-internal gaps (0001–0006) cover validation paths
// inside routed endpoints. Phase 5 entries (0010+) cover whole
// resource surfaces mockta v0 does not implement.
var gaps = []Gap{
	{
		ID:       IDUserFilter,
		Endpoint: "GET /api/v1/users",
		Resource: "okta_users (data source)",
		Status:   StatusOpen,
		Notes: "Filter expressions outside `eq`/`sw` on " +
			"id|login|email|status return 400 with this ID. " +
			"Other operators (co, gt, lt, pr) are not implemented.",
	},
	{
		ID:       IDGroupFilter,
		Endpoint: "GET /api/v1/groups",
		Resource: "okta_groups (data source)",
		Status:   StatusOpen,
		Notes: "Group list rejects `filter=` entirely; only " +
			"`q=` prefix search is implemented.",
	},
	{
		ID:       IDAppFilter,
		Endpoint: "GET /api/v1/apps",
		Resource: "okta_apps (data source)",
		Status:   StatusOpen,
		Notes: "Filter expressions outside `eq`/`sw` on " +
			"id|label|name|status|signOnMode return 400 with this ID.",
	},
	{
		ID:       IDGroupTypeAppGroup,
		Endpoint: "POST /api/v1/groups",
		Resource: "okta_group (type=APP_GROUP)",
		Status:   StatusOpen,
		Notes:    "Only OKTA_GROUP is implemented; APP_GROUP 501s.",
	},
	{
		ID:       IDGroupTypeBuiltIn,
		Endpoint: "POST /api/v1/groups",
		Resource: "okta_group (type=BUILT_IN)",
		Status:   StatusOpen,
		Notes:    "Only OKTA_GROUP is implemented; BUILT_IN 501s.",
	},
	{
		ID:       IDAppSignOnNonSAML,
		Endpoint: "POST /api/v1/apps",
		Resource: "okta_app_* (signOnMode≠SAML_2_0)",
		Status:   StatusOpen,
		Notes: "OIDC, BOOKMARK, AUTO_LOGIN, and all other " +
			"sign-on modes 501. v0 implements SAML_2_0 only.",
	},

	// 0010+: whole-resource gaps. Path patterns are matched as
	// prefixes, so e.g. `/api/v1/policies/{id}/rules` falls under
	// the same gap as `/api/v1/policies`.
	{
		ID:       "MOCKTA_GAP_0010",
		Endpoint: "/api/v1/policies",
		Resource: "okta_policy_*",
		Status:   StatusOpen,
		Notes:    "Policies API surface is not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0011",
		Endpoint: "/api/v1/authorizationServers",
		Resource: "okta_auth_server, okta_auth_server_*",
		Status:   StatusOpen,
		Notes:    "Authorization servers, scopes, and claims are not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0012",
		Endpoint: "/api/v1/idps",
		Resource: "okta_idp_*",
		Status:   StatusOpen,
		Notes:    "Identity-provider configuration is not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0013",
		Endpoint: "/api/v1/trustedOrigins",
		Resource: "okta_trusted_origin",
		Status:   StatusOpen,
		Notes:    "Trusted-origin CRUD is not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0014",
		Endpoint: "/api/v1/inlineHooks",
		Resource: "okta_inline_hook",
		Status:   StatusOpen,
		Notes:    "Inline hooks are not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0015",
		Endpoint: "/api/v1/eventHooks",
		Resource: "okta_event_hook",
		Status:   StatusOpen,
		Notes:    "Event hooks are not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0016",
		Endpoint: "/api/v1/behaviors",
		Resource: "okta_behavior",
		Status:   StatusOpen,
		Notes:    "Behavior detection rules are not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0017",
		Endpoint: "/api/v1/networkZones",
		Resource: "okta_network_zone",
		Status:   StatusOpen,
		Notes:    "Network-zone configuration is not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0018",
		Endpoint: "/api/v1/meta/schemas",
		Resource: "okta_*_schema_property",
		Status:   StatusOpen,
		Notes:    "Custom user/group schemas are not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0019",
		Endpoint: "/api/v1/sessions",
		Resource: "okta session API",
		Status:   StatusOpen,
		Notes:    "Session CRUD is not implemented.",
	},
	{
		ID:       "MOCKTA_GAP_0020",
		Endpoint: "/scim/v2",
		Resource: "okta SCIM 2.0 endpoint",
		Status:   StatusOpen,
		Notes:    "SCIM provisioning endpoints are not implemented.",
	},
}
