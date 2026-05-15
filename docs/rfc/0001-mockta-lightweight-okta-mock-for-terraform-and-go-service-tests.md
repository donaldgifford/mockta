---
id: RFC-0001
title: "mockta: lightweight Okta mock for Terraform and Go service tests"
status: Draft
author: Donald Gifford
created: 2026-05-15
---
<!-- markdownlint-disable-file MD025 MD041 -->

# RFC 0001: mockta: lightweight Okta mock for Terraform and Go service tests

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-15

<!--toc:start-->
- [Summary](#summary)
- [Problem Statement](#problem-statement)
- [Proposed Solution](#proposed-solution)
  - [Non-goals](#non-goals)
- [MVP Scope](#mvp-scope)
  - [The existing pattern we are mirroring](#the-existing-pattern-we-are-mirroring)
  - [What mockta's v0 loop looks like](#what-mocktas-v0-loop-looks-like)
  - [Why terraform test first](#why-terraform-test-first)
  - [Pure-Okta vs. composed tests](#pure-okta-vs-composed-tests)
  - [What ships in v0 (headline)](#what-ships-in-v0-headline)
  - [Out of scope for v0](#out-of-scope-for-v0)
- [Alternatives Considered](#alternatives-considered)
- [Implementation Phases](#implementation-phases)
- [Risks and Mitigations](#risks-and-mitigations)
- [Success Criteria](#success-criteria)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Summary

mockta is a lightweight, embeddable Okta mock for Terraform acceptance tests
and Go service tests — real OIDC/SAML protocols, no separate IdP process,
no JVM. Think "LocalStack for Okta, but you only get the resources you
actually need." The first release (v0) is narrowly scoped to plug into the
existing `terraform test` + LocalStack workflow the org already uses for
every other module: mockta replaces the "Okta cloud" the same way
LocalStack replaces "AWS cloud."

## Problem Statement

Testing Terraform modules and Go services that integrate with Okta has
three unsatisfying options today:

1. **Real preview tenant.** High fidelity, but rate-limited, slow, and
   leaks orphaned resources on failed runs.
2. **Plan-only.** Fast and hermetic, but can't validate anything requiring
   an API round-trip — SAML metadata, claim mapping, group propagation,
   token issuance.
3. **Hand-rolled `httptest` stubs.** Works for one module, doesn't
   compose, and re-invents the same mock surface in every repo.

The result is that Okta-touching modules either ship with weaker test
coverage than their AWS counterparts, or pay an outsized human-time cost
because every test run hits a real preview tenant.

The cost shows up in two places:

- **Module repos.** AWS modules get `terraform test` + LocalStack coverage
  on every PR; Okta modules don't, because nothing fills the LocalStack
  slot.
- **Cross-cloud modules (AWS + Okta SSO).** End-to-end federation flows
  cannot be exercised in CI at all today.

## Proposed Solution

Build mockta — a single Go binary that speaks the slice of the Okta
Management API, OIDC, and SAML our Terraform modules and services actually
exercise. The same codebase ships three distribution shapes (library,
binary, container) so it can be embedded in-process for Go tests, run as a
sidecar, or pulled as a testcontainers-go module. These shapes are
complementary; library mode is the fast path for Go-only tests, the
container is the natural shape when Docker is already in the test setup
(which it is whenever LocalStack is).

The detailed architecture, component breakdown, and repo layout are
deferred to the follow-up DESIGN docs called out in
[References](#references). This RFC commits to the shape of v0 and the
roadmap past it, not the implementation.

### Non-goals

- **Not a production IdP.** Test infrastructure only.
- **Not comprehensive Okta API coverage.** Narrowly scoped to the
  resources our Terraform actually creates.
- **Not an interactive login UI.** Test harnesses pass user identity via
  header or session config; no browser flows.

## MVP Scope

The first release will support **Okta Terraform modules tested directly
via `terraform test`** — the same workflow we already use for every other
module in the org. No new test-harness shape, no new mental model; mockta
slots into the slot LocalStack already occupies for AWS.

### The existing pattern we are mirroring

For AWS-touching modules today:

1. Author the module.
2. Run `terraform test` against LocalStack to validate plan/apply coverage.
3. Wherever LocalStack falls short of real AWS at the plan/apply layer,
   file the gap into `libtftest` + `sneakystack` so the next module
   inherits the fix.

### What mockta's v0 loop looks like

mockta's MVP replicates that loop one-for-one for Okta:

1. Author an `okta_*` Terraform module.
2. Run `terraform test` against mockta to validate plan/apply coverage.
3. Wherever mockta falls short of real Okta at the plan/apply layer, file
   the gap into `libtftest` (and, where the test crosses into AWS,
   `sneakystack`) so the next module inherits the fix.

### Why `terraform test` first

`terraform test` is fast, hermetic, and lives in the module repo next to
the code it covers. It exercises the plan/apply layer end-to-end without
standing up a separate Go harness, which is exactly what we want from a
v0 — the question we need answered first is "does mockta's Management
API behave well enough for `terraform apply` to round-trip?" Everything
past that (signed assertions, SCIM, STS, claim mapping) is Tier 2+ and
needs a different test shape; we don't need to design for it yet.

### Pure-Okta vs. composed tests

For a pure-Okta module, mockta on its own is sufficient — no LocalStack,
no libtftest needed. **But the MVP still wires mockta through libtftest by
default**, even for pure-Okta cases, because:

- Remote state lives in S3, so any non-trivial module already needs
  LocalStack for the state backend.
- libtftest is where the org's test scaffolding lives; routing mockta
  through it from day one means consumers don't learn a second pattern.
- Parity matters more than minimalism here. The `libtftest/mockta` adapter
  may be a thin pass-through at first — that's fine. Whatever works in
  `terraform test` against mockta gets the same shape in `libtftest`,
  even if the indirection is, strictly speaking, unnecessary at v0. Over
  time that layer will accumulate real responsibilities; starting with
  the indirection in place avoids a later flag-day rename.

### What ships in v0 (headline)

- A mockta binary + container image sufficient for `terraform apply` of
  the initial `okta_*` resource set to round-trip (plan, apply, refresh,
  destroy).
- A `libtftest/mockta` adapter so module authors follow the existing
  pattern.
- A documented gap list driving Tier 1+ work.

Specific resource set, API surface, container wiring, adapter shape, and
acceptance criteria live in the DESIGN docs.

### Out of scope for v0

SCIM, real signed SAML assertions, OIDC token issuance, claim
expressions, AWS composition. Those land tier-by-tier per
[Implementation Phases](#implementation-phases) once the plan/apply layer
is solid.

## Alternatives Considered

| Alternative                                                    | Why rejected                                                                                                                                |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| **Keep using real Okta preview tenants**                       | Slow, rate-limited, leaks orphaned state on failed runs, cannot run in parallel across PRs.                                                 |
| **Keep using hand-rolled `httptest` per repo**                 | Doesn't compose, re-implements the same surface repeatedly, drifts between repos.                                                           |
| **Extend an existing OSS Okta mock**                           | Surveyed options — none combine OIDC, SAML, and management API in a single Go-embeddable process. The protocol-faithfulness bar is the gap. |
| **Skip mockta, only test plan-time behavior**                  | Doesn't validate apply-time behavior; misses the entire class of bugs `terraform test` is meant to catch.                                   |
| **Build mockta as a testcontainers-only image**                | Forecloses on in-process embedding, which is the fastest path for Go service tests and a real future need.                                  |
| **Identity Center portal flow instead of IAM SAML federation** | LocalStack's STS path handles IAM SAML federation end-to-end; the portal path doesn't, so credentials wouldn't be real.                     |

## Implementation Phases

| Tier  | Scope                                                                                  | Effort        | Unlocks                                                   |
| ----- | -------------------------------------------------------------------------------------- | ------------- | --------------------------------------------------------- |
| **0** | Stub REST API, in-memory state, fake IDs, container image, `terraform test` round-trip | 1 week        | TF apply/destroy works against mockta in `terraform test` |
| **1** | + real JWT signing, JWKS, OIDC discovery, polished testcontainers module               | +1 week       | Tests can validate token contents; polyglot via container |
| **2** | + SAML IdP via crewjam/saml, signed assertions                                         | +2 weeks      | End-to-end SAML federation works                          |
| **3** | + SCIM bridge to identitystore, AWS integration                                        | +2 weeks      | Canonical end-to-end test works                           |
| **4** | + Wiz and GitHub integrations                                                          | +2 weeks each | Coverage broadens                                         |
| **5** | + claim expression engine (subset)                                                     | +1 week       | Realistic group-claim mapping                             |

Roughly 8–10 weeks to the full vision, but every tier is independently
useful and shippable. Tier 0 is the only tier this RFC commits to in
detail; subsequent tiers will produce their own DESIGN docs as they come
into focus.

## Risks and Mitigations

| Risk                                                                                                               | Impact | Likelihood | Mitigation                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------------ | ------ | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| Audience / entity-ID misalignment between TF `okta_app_saml`, TF `aws_iam_saml_provider`, and the mockta assertion | High   | High       | Compute all three from a single source of truth in `iamenv` helpers; add an intentional-mismatch test. (Tier 3 concern.)                  |
| SAML XML canonicalization quirks (multi-valued attributes, namespace prefixes)                                     | Medium | Medium     | Pin tests to known attribute ordering; lean on crewjam/saml; add fixture tests for the awkward cases. (Tier 2 concern.)                   |
| LocalStack's `SAML:aud` enforcement diverges from real AWS                                                         | High   | Medium     | Intentional-mismatch test that probes; if LocalStack is too permissive, surface in docs and add a wrapper assertion. (Tier 3 concern.)    |
| Identity Center permission-set materialization on LocalStack is incomplete                                         | Medium | High       | Fixtures create IAM roles by hand and wire trust policies explicitly until LocalStack closes the gap. (Tier 3 concern.)                   |
| Sync-by-default SCIM diverges from real Okta's eventual consistency                                                | Low    | High       | Document loudly; offer opt-in async mode for tests that specifically need to exercise consistency. (Tier 3 concern.)                      |
| mockta's API coverage drifts from real Okta                                                                        | Medium | Medium     | Pin to a specific upstream OpenAPI tag; documented gap list; nightly CI against real Okta post-Tier 1.                                    |
| Module repos can't onboard because libtftest doesn't expose mockta cleanly                                         | High   | Medium     | DESIGN doc for the libtftest adapter is part of v0 delivery; adapter ships in lockstep with mockta v0.                                    |

## Success Criteria

**v0 (Tier 0)**

- A new `okta_*` Terraform module can be authored and validated via
  `terraform test` against mockta + LocalStack with no real Okta calls.
- `terraform test` plan/apply/refresh/destroy round-trip works for the
  initial resource set defined in the v0 DESIGN doc.
- `libtftest/mockta` adapter exposes the Okta provider env contract; a
  module repo consumes it without learning a new pattern.
- A gap list is published and at least one gap is closed in `libtftest`.

**Full vision (Tier 3+)**

- A cross-cloud Okta-AWS-SSO module's federation flow runs end-to-end in
  CI with real signed assertions and real `AssumeRoleWithSAML`
  credentials.
- Test runtime for an Okta-touching module is ≤ 10× the runtime of an
  equivalent AWS-only module against LocalStack (today it's 100×+
  against a real preview tenant).
- At least one external consumer (after open-sourcing) ships a PR.

## Open Questions

- **Persistence?** v0 is in-process only via `hashicorp/go-memdb` —
  state dies with the container. Resolved in DESIGN-0001. If the
  sidecar-binary path later needs durability, the opt-in is a JSON
  snapshot on shutdown, not a real DB.
- **API token / admin auth model.** Real Okta has rich scoped tokens.
  Start with a single static admin token; add scoped tokens only when a
  test actually needs to assert on auth failures.
- **OpenAPI vendor strategy.** Pin a specific upstream tag, or vendor
  the slice we use? Pin first; vendor only if we need modifications.
- **Strict vs permissive mode.** Should mockta reject requests real Okta
  would reject, or accept anything well-formed? Strict by default,
  permissive opt-in for negative tests.
- **`libtftest/mockta` default mode.** In-process by default with
  container opt-in, or container by default for parity with
  sneakystack? Leaning in-process default — startup cost matters when
  test counts grow — but worth a closer look once we have real test
  counts to reason about.
- **Open-source it?** Probably yes once Tier 2 lands. Nothing here is
  proprietary, and the ecosystem clearly wants this.

## References

- `docs/mockta.md` — original design notes that seeded this RFC.
- [zitadel/oidc](https://github.com/zitadel/oidc) — OIDC OP library.
- [crewjam/saml](https://github.com/crewjam/saml) — SAML IdP library.
- [testcontainers-go](https://golang.testcontainers.org/) — module
  pattern reference.
- [LocalStack](https://www.localstack.cloud/) — pattern we are mirroring
  for Okta.
- Follow-up: `DESIGN-0001: terraform-test compliant mockta v0` — detailed
  v0 design (resource set, container wiring, gap list shape, acceptance
  criteria).
- Follow-up: `DESIGN-0002: libtftest mockta parity adapter` — adapter
  shape, env contract, in-process vs container routing, forward design.
