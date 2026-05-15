---
id: DESIGN-0001
title: "terraform-test compliant mockta v0"
status: Draft
author: Donald Gifford
created: 2026-05-15
---
<!-- markdownlint-disable-file MD025 MD041 -->

# DESIGN 0001: terraform-test compliant mockta v0

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
  - [Resource scope](#resource-scope)
  - [Management API surface](#management-api-surface)
  - [State model](#state-model)
  - [Container shape](#container-shape)
  - [terraform test wiring](#terraform-test-wiring)
  - [Provider compatibility](#provider-compatibility)
  - [Gap list](#gap-list)
- [API / Interface Changes](#api--interface-changes)
- [Data Model](#data-model)
- [Testing Strategy](#testing-strategy)
- [Migration / Rollout Plan](#migration--rollout-plan)
- [Open Questions](#open-questions)
  - [Resolved during review](#resolved-during-review)
- [References](#references)
<!--toc:end-->

## Overview

This design covers the v0 (Tier 0) deliverable of mockta as committed in
RFC-0001: enough of the Okta Management API to round-trip the initial
`okta_*` resource set under `terraform test`, packaged as a container
that drops into the same `run` block shape we use for LocalStack today.

## Goals and Non-Goals

### Goals

- `terraform apply` / `refresh` / `destroy` round-trip for the v0
  resource set against mockta with no real Okta calls.
- A container image that `terraform test` can spin up as a sidecar in a
  `run` block, with a documented `wait` strategy.
- A documented gap list — every observed-and-not-implemented Okta API
  path, keyed by the smallest TF fixture that reaches it.
- Deterministic behavior across test runs (stable IDs, stable error
  shapes) so module authors can write `expect_failures` blocks that
  don't flake.

### Non-Goals

- OIDC token issuance, JWKS, discovery — Tier 1.
- SAML metadata, signed assertions — Tier 2.
- SCIM outbound push, AWS composition — Tier 3.
- Claim expression engine — Tier 5.
- Library / in-process embedding **API polish**. The library entrypoint
  exists in v0 (the binary uses it internally) but `pkg/mockta.New` is
  not yet considered stable; downstream consumers should use the
  container.
- Coverage breadth. Resource scope is intentionally narrow; out-of-scope
  resources return a structured 501 that lands in the gap list.

## Background

RFC-0001 frames the v0 commitment: replicate the org's AWS
module-testing loop (`terraform test` + LocalStack) for Okta-touching
modules. This DESIGN doc fills in the shape of the v0 deliverable —
what resources, what API paths, what the container looks like, how
`terraform test` consumes it.

The companion design DESIGN-0002 covers the `libtftest/mockta` adapter
that consumer module repos use. The two are designed in lockstep but
ship independently — mockta is a container, the adapter is a Go
package.

## Detailed Design

### Resource scope

v0 covers the four resource types that account for the majority of
plan/apply traffic in our existing Okta modules:

| Terraform resource       | Backed by                                                          | Behavior in v0                                                                       |
| ------------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------ |
| `okta_user`              | `POST/GET/PUT/DELETE /api/v1/users/{id}`                           | Full CRUD. Status transitions are simulated synchronously (no async activation).     |
| `okta_group`             | `POST/GET/PUT/DELETE /api/v1/groups/{id}`                          | Full CRUD. `type=OKTA_GROUP` only; APP_GROUP / BUILT_IN return 501 with gap-list ID. |
| `okta_app_saml`          | `POST/GET/PUT/DELETE /api/v1/apps/{id}` with `signOnMode=SAML_2_0` | CRUD only. No metadata generation, no signing (Tier 2).                              |
| `okta_group_membership`  | `PUT /api/v1/groups/{gid}/users/{uid}` + `DELETE` + `GET`          | Synchronous; membership is visible immediately on read.                              |

Any other resource the provider asks for returns `501 Not Implemented`
with `Errors[0].ErrorCode = "MOCKTA_GAP_<NNNN>"` (see [Gap list](#gap-list)).

### Management API surface

The v0 endpoints, in the order the provider typically hits them:

```
# Org bootstrap (read-only)
GET    /api/v1/org

# Users
POST   /api/v1/users[?activate=true|false]
GET    /api/v1/users/{id_or_login}
PUT    /api/v1/users/{id}
POST   /api/v1/users/{id}/lifecycle/activate
POST   /api/v1/users/{id}/lifecycle/deactivate
DELETE /api/v1/users/{id}
GET    /api/v1/users?filter=...&limit=...

# Groups
POST   /api/v1/groups
GET    /api/v1/groups/{id}
PUT    /api/v1/groups/{id}
DELETE /api/v1/groups/{id}
GET    /api/v1/groups?q=...

# Group membership
PUT    /api/v1/groups/{gid}/users/{uid}
DELETE /api/v1/groups/{gid}/users/{uid}
GET    /api/v1/groups/{gid}/users

# Apps
POST   /api/v1/apps
GET    /api/v1/apps/{id}
PUT    /api/v1/apps/{id}
POST   /api/v1/apps/{id}/lifecycle/activate
POST   /api/v1/apps/{id}/lifecycle/deactivate
DELETE /api/v1/apps/{id}
GET    /api/v1/apps?filter=...

# Health (mockta-specific; for testcontainers wait strategy)
GET    /health
```

Pagination semantics: `Link: <...>; rel="next"` headers exactly match
real Okta, even though v0 always returns full results in one page. The
provider's pagination code path is exercised by always emitting the
header on listing endpoints with an empty cursor, then returning empty
on the follow-up — this catches the class of bugs where the provider
mis-parses pagination links.

Error shape mirrors Okta's `ErrorResponse`:

```json
{
  "errorCode": "E0000001",
  "errorSummary": "Api validation failed",
  "errorLink": "E0000001",
  "errorId": "mockta-<uuid>",
  "errorCauses": [{ "errorSummary": "login: is required" }]
}
```

`errorId` is always prefixed `mockta-` so failures are obvious in
provider logs.

### State model

- **Storage:** [`hashicorp/go-memdb`](https://github.com/hashicorp/go-memdb)
  — pure-Go, in-memory, MVCC with secondary indexes. No external DB,
  no cgo, no migrations. `terraform test` working sets are bounded
  (dozens of resources, not millions), so a real KV/SQL engine would
  be overkill; go-memdb gives us indexed lookups and snapshot reads
  for free.
- **Persistence:** None in v0. Mockta is a per-test-run sidecar; state
  dies with the container. If the sidecar-binary use case ever needs
  durability, the path is an opt-in JSON-snapshot-on-shutdown, not a
  real DB.
- **IDs:** 20-character base32 strings (matches Okta's format). Seeded
  from a hash of `(resource type, primary key)` so the same input
  produces the same ID across runs — important for `expect_output`
  assertions.
- **Concurrency:** go-memdb's MVCC model — reads never block writes;
  writes serialize through a single transaction at a time. Plenty for
  `terraform test` workloads.
- **Reset:** `POST /admin/reset` wipes all tables (a single
  transaction creating empty replacements). Used by the testcontainers
  cleanup helper, not by Terraform.

### Container shape

- **Base image:** `gcr.io/distroless/static-debian12:nonroot`. Existing
  `Dockerfile` already targets this.
- **Multi-arch:** `linux/amd64` + `linux/arm64` (existing `docker-bake.hcl`
  `mockta-release` target).
- **Entrypoint:** `/mockta` with no required flags. Defaults to
  `:8080` (HTTP) and `:9090` (admin/health/metrics).
- **Image size target:** ≤ 15 MB.
- **Env vars:**
  - `MOCKTA_ADMIN_TOKEN` — required; the static bearer token the
    `okta_*` provider authenticates with. If unset, mockta generates
    one and logs it once on startup (testcontainers picks it up via
    `LogConsumer`).
  - `MOCKTA_ORG_NAME` — defaults to `mockta-dev`. Surfaced in
    `/api/v1/org`.
  - (No persistence env var in v0. State is in-process only.)
  - `MOCKTA_STRICT_MODE` — `true|false`, defaults `true`. Strict mode
    enforces Okta's documented validation rules; permissive mode
    accepts anything well-formed (for negative tests).
  - `MOCKTA_LOG_LEVEL` — `debug|info|warn|error`, defaults `info`.
- **Health probe:** `GET /health` returns 200 once the DB is open and
  the HTTP listener is bound. testcontainers waits on this.
- **Ports:** 8080 (Okta API), 9090 (health/admin/metrics). Both
  exposed by the existing Dockerfile.

### `terraform test` wiring

A consumer module repo's `tests/setup/main.tf` will look like:

```hcl
resource "docker_container" "mockta" {
  image = "ghcr.io/donaldgifford/mockta:v0.1.0"
  name  = "mockta-${terraform.workspace}"
  env = [
    "MOCKTA_ADMIN_TOKEN=test-admin-token",
    "MOCKTA_ORG_NAME=acme-test",
  ]
  ports {
    internal = 8080
    external = 0
  }
  ports {
    internal = 9090
    external = 0
  }
  healthcheck {
    test         = ["CMD", "/mockta", "healthcheck"]
    interval     = "1s"
    retries      = 30
    start_period = "2s"
  }
}

output "mockta_base_url" {
  value = "http://localhost:${docker_container.mockta.ports[0].external}"
}
output "okta_org_name"  { value = "acme-test" }
output "okta_api_token" { value = "test-admin-token" }
```

…and a top-level `tests/*.tftest.hcl`:

```hcl
variables {
  okta_base_url  = run.setup.mockta_base_url
  okta_org_name  = run.setup.okta_org_name
  okta_api_token = run.setup.okta_api_token
}

run "setup" { module { source = "./setup" } }

run "apply_module" {
  command = apply
  module  { source = "../" }
}
```

The `docker_container.mockta` block is exactly the same shape as the
`docker_container.localstack` block consumer repos already have. That's
deliberate: the cost of adopting mockta is a copy-paste, not a new
mental model. The `libtftest/mockta` adapter (DESIGN-0002) will
encapsulate this into a single function so consumers don't write the
boilerplate by hand.

### Provider compatibility

v0 pins to the **latest released `oktadeveloper/okta` provider version
at v0 tag time**. The exact version is captured in two places, in this
order of authority:

1. `tests/contract/go.mod` in the mockta repo — the contract suite
   imports the provider directly and is the gating signal for "v0 is
   done." Whatever it pins is the supported version.
2. The `required_providers` block in the libtftest setup module
   (DESIGN-0002) — mirrors the contract pin.

Bumping the provider is a deliberate change to both files plus a
contract-test run; we don't float to `latest` in CI. Once a real
release matrix becomes valuable (likely Tier 1+), this expands to a
tested range; for v0 it's one version.

The provider's "skip API version check" toggle (`base_url` pointing at
a non-`.okta.com` host) is required and documented. Any provider
quirks observed during v0 land as test fixtures in the mockta repo so
we catch regressions before consumers do.

The provider hits `/api/v1/org` early to validate connectivity; that
endpoint must return a plausible org response unconditionally for the
provider to proceed.

### Gap list

Every endpoint mockta does not implement returns 501 with a structured
gap-list ID. The mockta repo carries `docs/gaps.md` (or equivalent),
auto-generated from a source-of-truth registry:

```go
// internal/gaps/registry.go
type Gap struct {
    ID        string   // MOCKTA_GAP_0001
    Endpoint  string   // POST /api/v1/policies
    Resource  string   // okta_policy_password
    HitBy     []string // tests/fixtures that exercise it
    Status    string   // open | in-progress | closed-in-vN.N
    Notes     string
}
```

When a `terraform test` run hits a gap, the test output shows the gap
ID; consumers file an issue against mockta referencing that ID. This
gives us a tight feedback loop between consumer pain and mockta
roadmap — gaps with multiple `HitBy` entries jump the queue.

**Publication.** `docs/gaps.md` is auto-published to the MkDocs /
TechDocs wiki on every release (the existing `docz wiki` integration
picks it up via `mkdocs.yml`). Consumers can read the current gap
list without cloning the mockta repo. CI fails the release if the
in-repo `docs/gaps.md` is out-of-sync with what mockta would emit
from a clean run — that prevents drift between the registry and the
published doc.

## API / Interface Changes

This is the first release; everything in [Management API surface](#management-api-surface)
is new. No breaking changes possible at v0.

CLI surface for the `mockta` binary in v0:

```
mockta                  # start server on :8080 and :9090
mockta healthcheck      # exit 0 if /health is 200, else 1 (for HEALTHCHECK)
mockta version          # print version + commit
```

No subcommands for seeding, reset, etc. in v0 — those go through the
admin API on `:9090`.

## Data Model

go-memdb schema lives in `internal/store/schema.go`. v0 tables and
indexes:

| Table               | Primary index | Secondary indexes                | Fields                                                            |
| ------------------- | ------------- | -------------------------------- | ----------------------------------------------------------------- |
| `users`             | `id`          | `login` (unique), `email`        | id, login, email, status, profile (JSON blob), created_at, updated_at |
| `groups`            | `id`          | `name` (unique), `type`          | id, name, type, description, profile (JSON blob), created_at      |
| `apps`              | `id`          | `label`, `sign_on_mode`          | id, name, label, status, sign_on_mode, settings (JSON blob), created_at |
| `group_memberships` | `group_id+user_id` | `group_id`, `user_id`       | group_id, user_id, created_at                                     |
| `audit_log`         | `id` (ULID)   | `ts`, `gap_id`                   | id, ts, method, path, status, gap_id (nullable)                   |

Records are plain Go structs registered with go-memdb's schema. JSON
blobs (profile, settings) are stored as `[]byte` and parsed on read
when individual fields are needed for filter evaluation — for v0 most
filter paths read whole records and let the handler do the work.

The `audit_log` table doubles as the source for `docs/gaps.md`
generation: any 501 response writes a row, and a `mockta gaps export`
admin command iterates the `gap_id` index and emits the markdown.

## Testing Strategy

- **Unit tests (Go).** Per-handler tests using `httptest`, covering
  happy path + Okta-documented error responses. Table-driven.
- **Provider-level contract tests.** A `tests/contract/` Go test that
  invokes the `oktadeveloper/okta` provider in-process and asserts
  plan/apply/refresh/destroy round-trips for each v0 resource type.
  This is the gating signal for "v0 is done."
- **`terraform test` smoke fixture.** A minimal Terraform module
  exercising all four resource types, run via `terraform test` against
  a built mockta container. Lives in `tests/smoke/`.
- **Gap-list determinism test.** Run the smoke fixture against a
  deliberately-undersized mockta build (some endpoints intentionally
  off) and assert the gap-list output matches a golden file. Catches
  regressions in error-shape consistency.
- **No real Okta integration tests in v0.** The "nightly diff against
  real Okta" test is Tier 1.

CI gates: existing `just ci` recipe (`lint + test + build +
license-check`) plus a new `just test-contract` recipe for the
provider-level tests.

## Migration / Rollout Plan

There's no migration — this is the first release. Rollout shape:

1. **Internal milestone (end of Tier 0).** Tag `v0.1.0`, publish
   container to `ghcr.io/donaldgifford/mockta`, ship the
   `libtftest/mockta` adapter (DESIGN-0002).
2. **Pilot consumer.** One Okta-touching module repo migrates from
   real preview tenant to mockta. Pain points feed the gap list and
   the next libtftest minor.
3. **Broader rollout.** Other Okta modules adopt as the gap list
   shrinks. No deprecation of real preview testing — that path stays
   for regression / "did we drift" tests.
4. **Tier 1 design.** Begins once the v0 pilot consumer has a green
   CI run.

## Open Questions

None — all v0 questions resolved during review.

### Resolved during review

- **Provider version pinning** → Pin to the latest released
  `oktadeveloper/okta` at v0 tag time; expand to a range only when a
  real matrix becomes valuable. See [Provider compatibility](#provider-compatibility).
- **`docker_container` vs `terraform_data`** → Use the Terraform Docker
  provider, matching the LocalStack pattern consumer repos already use.
  See [`terraform test` wiring](#terraform-test-wiring).
- **Image registry** → `ghcr.io/donaldgifford/mockta`. No ECR mirror
  for v0; revisit if a consumer asks.
- **Gap list visibility** → Auto-publish `docs/gaps.md` to the
  MkDocs/TechDocs wiki on every release; CI fails on registry/markdown
  drift. See [Gap list](#gap-list).
- **`MOCKTA_ADMIN_TOKEN` rotation** → Static for v0. Real Okta's scoped
  tokens come back on the table only when a test needs to assert on
  scope-based failures. See [Container shape](#container-shape).

## References

- [RFC-0001: mockta](../rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md)
- DESIGN-0002: libtftest mockta parity adapter (companion design).
- [`oktadeveloper/okta` provider](https://registry.terraform.io/providers/okta/okta/latest/docs)
- [Okta Management API reference](https://developer.okta.com/docs/reference/api/users/)
- `Dockerfile`, `docker-bake.hcl` — existing container infrastructure
  this design builds on.
- `docs/mockta.md` — original design notes (broader scope than v0).
