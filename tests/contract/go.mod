// tests/contract is a separate Go module so heavy test-only
// dependencies (and any future Okta SDK / provider deps) don't bleed
// into the main module's go.sum. The replace directive points at the
// parent so the contract tests always run against the working copy
// of pkg/mockta, never a tagged release.
module github.com/donaldgifford/mockta/tests/contract

go 1.26.2

replace github.com/donaldgifford/mockta => ../..

require github.com/donaldgifford/mockta v0.0.0-00010101000000-000000000000

require github.com/hashicorp/go-memdb v1.3.5 // indirect

require (
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
)
