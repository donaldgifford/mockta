// Package gaps owns the source-of-truth registry of Okta API
// endpoints mockta does not implement. Phase 3 ships an interface
// and a stub registry that returns MOCKTA_GAP_UNCATALOGED for any
// path; Phase 5 replaces the stub with a populated registry and
// adds the `mockta gaps` subcommands.
package gaps

// UncataloguedID is the placeholder gap ID emitted when a request
// hits an endpoint the registry doesn't know about. Phase 5
// populates the registry; until then every 501 lands here.
const UncataloguedID = "MOCKTA_GAP_UNCATALOGED"

// Registry resolves (method, path) pairs into MOCKTA_GAP_NNNN IDs.
// The notimplemented handler consumes this; tests can swap in a fake.
type Registry interface {
	// Lookup returns the gap ID for the given (method, path). The
	// boolean is true when the lookup hit a known gap; false means
	// "we've never seen this endpoint, file it under
	// UncataloguedID."
	Lookup(method, path string) (gapID string, known bool)
}

// StubRegistry is the Phase 3 placeholder. It always returns
// UncataloguedID. Phase 5 wires up the real, populated registry.
type StubRegistry struct{}

// Lookup implements Registry.
func (StubRegistry) Lookup(_, _ string) (string, bool) {
	return UncataloguedID, false
}
