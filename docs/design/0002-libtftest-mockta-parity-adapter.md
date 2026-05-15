---
id: DESIGN-0002
title: "libtftest mockta parity adapter"
status: Draft
author: Donald Gifford
created: 2026-05-15
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0002: libtftest mockta parity adapter

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-15

<!--toc:start-->
- [Overview](#overview)
- [Goals and Non-Goals](#goals-and-non-goals)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Background](#background)
- [Detailed Design](#detailed-design)
  - [Package shape](#package-shape)
  - [Env contract](#env-contract)
  - [Lifecycle](#lifecycle)
  - [In-process vs container mode](#in-process-vs-container-mode)
  - [Composite environments (forward-looking)](#composite-environments-forward-looking)
  - [Gap propagation](#gap-propagation)
- [API / Interface Changes](#api--interface-changes)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Overview

This design covers the `libtftest/mockta` adapter that consumer module
repos use to wire mockta into their `terraform test` suites. For v0
the adapter is a deliberately thin pass-through; this doc captures both
the v0 shape and the forward design for when the adapter accumulates
real responsibilities.

## Goals and Non-Goals

### Goals

- Provide a single entry point so consumer repos don't write
  `docker_container "mockta"` boilerplate by hand.
- Expose a stable Go-level contract (`Env` struct, env-var map) that
  works identically whether mockta runs in-process or as a container.
- Match the shape of existing libtftest adapters (notably the
  LocalStack/sneakystack adapters) so consumers don't learn a second
  pattern.
- Make the indirection cheap to keep in place even when it's
  "unnecessary" at v0 — the cost of carrying it from day one is much
  lower than retrofitting it later.
- Surface mockta's gap-list IDs back into the consumer test output
  with enough context to file a useful issue.

### Non-Goals

- **No mockta business logic in the adapter.** This package is wiring,
  not behavior. The Okta API surface lives in mockta itself.
- **No composite `iamenv` work in v0.** Composite environments
  (mockta + sneakystack on a shared Docker network) are Tier 3; this
  design notes the future shape but doesn't ship it now.
- **No fork of `oktadeveloper/okta`.** The adapter configures the
  provider via standard env vars; no custom build.

## Background

libtftest is the org's shared test-harness library, built on
testcontainers-go. It already exposes:

- A LocalStack adapter (`libtftest/localstack`) — the closest analog
  to what we're building here.
- A sneakystack adapter (`libtftest/sneakystack`) for AWS services
  LocalStack covers poorly.
- A composite `iamenv` for multi-service tests on a shared Docker
  network.

DESIGN-0001 specifies mockta itself: a container that speaks the Okta
Management API. This design specifies the Go shim that lets module
repos consume mockta through libtftest the same way they consume
LocalStack today.

RFC-0001 commits to shipping the adapter in lockstep with mockta v0
even though, for pure-Okta tests, mockta-on-its-own would suffice. The
rationale is in RFC-0001 [Pure-Okta vs. composed tests](../rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md#pure-okta-vs-composed-tests):
parity with the existing pattern matters more than minimalism.

## Detailed Design

### Package shape

The adapter lives in the `libtftest` repo, **not** the mockta repo —
it's a libtftest concern. Proposed path: `libtftest/mockta/`.

```
libtftest/
├── localstack/         # existing
├── sneakystack/        # existing
├── iamenv/             # existing composite
└── mockta/             # new in v0
    ├── env.go          # New(t, opts...) → *Env (in-process, v1+)
    ├── container.go    # NewContainer(t, opts...) → *Env (v0 default)
    ├── options.go      # Option / ContainerOption functional options
    ├── env_contract.go # ProviderEnv() map[string]string, well-known keys
    ├── gaps.go         # gap-list helpers (parse 501s, surface in t.Log)
    └── doc.go
```

The public Go surface, in order of consumer importance:

```go
// libtftest/mockta/container.go
type Env struct {
    BaseURL  string // http://host:port for the Okta API
    OrgName  string
    APIToken string

    container testcontainers.Container // unexported; nil in in-process mode
}

func NewContainer(t *testing.T, opts ...ContainerOption) *Env

// libtftest/mockta/env.go (stub in v0, real in v1+)
func New(t *testing.T, opts ...Option) *Env

// libtftest/mockta/env_contract.go
func (e *Env) ProviderEnv() map[string]string

// libtftest/mockta/options.go
func WithImage(ref string) ContainerOption
func WithAdminToken(tok string) ContainerOption
func WithOrgName(name string) ContainerOption
func WithSeedFile(path string) ContainerOption    // future use; mockta v0 ignores
func WithNetwork(name string) ContainerOption     // for composite envs
func WithLogConsumer(c testcontainers.LogConsumer) ContainerOption
```

`Env` is the same shape consumers already use from the LocalStack
adapter — same field names where they overlap, same `ProviderEnv()`
method, same `*testing.T`-first constructor signature. Drop-in for
anyone who already knows libtftest.

### Env contract

`ProviderEnv()` returns the exact env keys the `oktadeveloper/okta`
provider reads, in the form a `terraform test` `variables` block or a
Go `os.Setenv` call can consume:

```go
func (e *Env) ProviderEnv() map[string]string {
    return map[string]string{
        "OKTA_ORG_NAME":  e.OrgName,
        "OKTA_BASE_URL":  e.BaseURL,
        "OKTA_API_TOKEN": e.APIToken,
    }
}
```

This is the contract that doesn't change between in-process and
container modes — the whole point of the adapter is that consumers
write against `ProviderEnv()` and the underlying mockta runtime swaps
underneath them.

For `terraform test` consumption, the same values are exposed as
outputs from a libtftest-provided setup module (see DESIGN-0001
[`terraform test` wiring](0001-terraform-test-compliant-mockta-v0.md#terraform-test-wiring)).
The Go-level `ProviderEnv()` map and the Terraform-level outputs are
two views of the same contract; the adapter is responsible for keeping
them in sync.

### Lifecycle

```
NewContainer(t)
    │
    ├─► pull image (testcontainers handles cache)
    ├─► docker run with MOCKTA_ADMIN_TOKEN, MOCKTA_ORG_NAME set
    ├─► wait for GET /health → 200
    ├─► resolve mapped ports → populate Env.BaseURL
    └─► return *Env

    t.Cleanup ────► Terminate(ctx)   (container removed; volumes
                                      pruned where applicable)
```

Wait strategy: `wait.ForHTTP("/health").WithPort("8080/tcp").WithStatusCodeMatcher(...)` —
identical to LocalStack's. Default timeout 60s.

Concurrency: each `NewContainer` call gets its own container. No
sharing across tests in v0. `testcontainers.WithReuse()` is plumbed
through `WithReuse(true)` for local-dev iteration but documented as
breaking isolation.

### In-process vs container mode

Container mode is the v0 default for two reasons:

1. **Parity with `terraform test`.** Inside `terraform test`, mockta
   is necessarily a container (Terraform can't dial an in-process Go
   server in another process). Making the default match what TF
   itself uses avoids surprise behavior differences between
   Terraform-driven and Go-driven tests.
2. **mockta's library API isn't yet stable.** DESIGN-0001 explicitly
   defers `pkg/mockta.New` polish to Tier 1+.

In-process mode (`New(t, opts...)`) is stubbed in v0 to **wrap
`NewContainer`** so consumer code can be written against `New` from
day one and seamlessly switch to true in-process when Tier 1 lands.

```go
// v0: New is sugar for NewContainer; same Env, just slower to start.
func New(t *testing.T, opts ...Option) *Env {
    return NewContainer(t, optsToContainerOpts(opts)...)
}
```

This is intentional: consumers should be able to `git blame` later
and see that `New` was always the right import; only the
implementation behind it changed.

### Composite environments (forward-looking)

When Tier 3 lands (SCIM bridge, AWS composition), `libtftest/iamenv`
will gain a constructor that wires mockta + sneakystack on a shared
Docker network:

```go
// libtftest/iamenv/env.go — sketched for forward reference; not in v0
func NewIAMEnv(t *testing.T) *IAMEnv {
    net, _ := network.New(ctx)
    t.Cleanup(func() { net.Remove(ctx) })

    ls := sneakystack.Run(t, sneakystack.WithNetwork(net.Name))
    mk := mockta.NewContainer(t,
        mockta.WithNetwork(net.Name),
        mockta.WithSCIMTarget("http://"+ls.NetworkAlias()+":4566"),
    )
    return &IAMEnv{Mockta: mk, LocalStack: ls, Network: net}
}
```

The shape of `mockta.WithNetwork` and `mockta.WithSCIMTarget` is
planned into v0's `options.go` so that day-zero callers don't need to
be rewritten when iamenv arrives. v0 accepts the options and may
ignore them (with a `t.Log` note); Tier 3 wires them up for real.

### Gap propagation

The adapter wraps test output with a small helper that watches HTTP
errors flowing back from mockta and surfaces gap IDs into `t.Log`:

```go
// libtftest/mockta/gaps.go
// AssertNoGaps fails the test if mockta returned any 501 with a
// MOCKTA_GAP_* code during the test. Call as t.Cleanup(env.AssertNoGaps).
func (e *Env) AssertNoGaps(t *testing.T)

// GapsHit returns the list of gap IDs hit during this Env's lifetime,
// queried from mockta's /admin/audit endpoint.
func (e *Env) GapsHit() []string
```

This is the consumer-side counterpart to mockta's gap registry
(DESIGN-0001 [Gap list](0001-terraform-test-compliant-mockta-v0.md#gap-list)).
A consumer module's test can opt in to "fail loudly if mockta is
missing something I depend on" with one `t.Cleanup` line — and
those failures carry the gap ID, so filing an upstream issue against
mockta is mechanical.

## API / Interface Changes

This is the first release of `libtftest/mockta`; all the symbols in
[Package shape](#package-shape) are new. No breaking changes to
existing libtftest packages.

The one libtftest-wide question is whether `iamenv` adds a stub
`NewIAMEnv` placeholder in v0 (returning "Tier 3, not yet
implemented") or waits until Tier 3 to introduce that name. Defer to
"introduce in Tier 3" to keep the v0 surface minimal.

## Data Model

No persistent data in the adapter itself; all state lives in mockta
(the container). The adapter's only state is the `Env` struct, which
holds container handle + resolved endpoints for the test's lifetime.

## Testing Strategy

- **Adapter unit tests** — verify `Option` plumbing, `ProviderEnv()`
  output, error paths (e.g., image pull failure, health wait timeout).
- **Adapter integration test** — run `NewContainer` end-to-end
  against a real mockta image in CI; assert `/api/v1/org` is
  reachable and returns a plausible payload.
- **Consumer smoke** — a fixture module repo (under
  `libtftest/mockta/internal/testfixture/`) that runs `terraform
  test` against the adapter to validate the full chain works the
  way DESIGN-0001 promises.
- **Cross-version compat matrix** — the adapter pins the default
  mockta image tag; CI verifies the adapter works against the
  pinned tag plus `latest`. Catches breaking changes in mockta
  before consumers see them.

## Migration / Rollout Plan

1. **v0 shipped alongside mockta v0.1.0.** Tag `libtftest/v?.?.?`
   with the new `mockta` subpackage and the env-contract surface.
2. **Pilot consumer.** First Okta module repo replaces hand-rolled
   `docker_container "mockta"` blocks (if any) with the libtftest
   setup module. Gap IDs flow through `AssertNoGaps`.
3. **Documentation in libtftest's README.** New "Adding mockta to
   your tests" section, mirroring the existing LocalStack section.
4. **Tier 1 — true in-process mode.** When mockta's library API
   stabilizes, `New` swaps from "wraps NewContainer" to "starts an
   in-process server." Consumer code does not change.
5. **Tier 3 — iamenv composition.** `libtftest/iamenv` grows the
   `NewIAMEnv` constructor that wires mockta + sneakystack.

## Open Questions

- **Should `New` (in-process default) really wrap `NewContainer`,
  or be unimplemented and return a clear error?** Wrapping gives
  forward-compatible consumer code; the trade-off is that it hides
  the v0 limitation. Lean toward wrap — the cost of a slower-than-
  expected `New` is low; the cost of a flag-day rename later is
  high.
- **Network mode default.** Bridge (default Docker network) or
  expect a caller-provided network? Bridge for v0; composite envs
  override.
- **Reuse / leak handling.** `testcontainers.CleanupContainer(t,
  mk)` covers cleanup; the open question is whether to add a
  panicking guard if a test forgets to call `Cleanup`. Probably
  not — testcontainers already does this with its Ryuk reaper.
- **Where do shared seed YAMLs live?** In each consumer repo
  (per-test fixtures), or in libtftest as a "common" set? Per-test
  initially; promote to common only if duplication shows up.
- **Public API stability promise.** Pre-1.0 libtftest; the adapter
  is best-effort stable but breaking changes are allowed if they
  improve the consumer experience. Document this loudly in the
  package doc.

## References

- [RFC-0001: mockta](../rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md)
- [DESIGN-0001: terraform-test compliant mockta v0](0001-terraform-test-compliant-mockta-v0.md)
- [testcontainers-go](https://golang.testcontainers.org/) — the
  underlying container-management library.
- `libtftest/localstack/` (existing) — pattern reference for the
  adapter shape.
- `libtftest/sneakystack/` (existing) — pattern reference for the
  AWS-services-LocalStack-doesn't-cover adapter shape.
- `libtftest/iamenv/` (existing) — composite-environment pattern
  this design extends in Tier 3.
