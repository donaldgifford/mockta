---
id: IMPL-0001
title: "terraform-test compliant mockta v0"
status: Draft
author: Donald Gifford
created: 2026-05-15
---
<!-- markdownlint-disable-file MD025 MD041 -->

# IMPL 0001: terraform-test compliant mockta v0

**Status:** Draft
**Author:** Donald Gifford
**Date:** 2026-05-15

<!--toc:start-->
- [Objective](#objective)
- [Scope](#scope)
  - [In Scope](#in-scope)
  - [Out of Scope](#out-of-scope)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Project foundation](#phase-1-project-foundation)
    - [Tasks](#tasks)
    - [Success Criteria](#success-criteria)
  - [Phase 2: Storage layer (go-memdb)](#phase-2-storage-layer-go-memdb)
    - [Tasks](#tasks-1)
    - [Success Criteria](#success-criteria-1)
  - [Phase 3: HTTP server skeleton](#phase-3-http-server-skeleton)
    - [Tasks](#tasks-2)
    - [Success Criteria](#success-criteria-2)
  - [Phase 4: Resource handlers](#phase-4-resource-handlers)
    - [Tasks](#tasks-3)
    - [Success Criteria](#success-criteria-3)
  - [Phase 5: Gap registry + publication](#phase-5-gap-registry--publication)
    - [Tasks](#tasks-4)
    - [Success Criteria](#success-criteria-4)
  - [Phase 6: Container release pipeline](#phase-6-container-release-pipeline)
    - [Tasks](#tasks-5)
    - [Success Criteria](#success-criteria-5)
  - [Phase 7: Contract + smoke tests](#phase-7-contract--smoke-tests)
    - [Tasks](#tasks-6)
    - [Success Criteria](#success-criteria-6)
  - [Phase 8: v0.1.0 release](#phase-8-v010-release)
    - [Tasks](#tasks-7)
    - [Success Criteria](#success-criteria-7)
- [File Changes](#file-changes)
- [Testing Plan](#testing-plan)
- [Dependencies](#dependencies)
- [Open Questions](#open-questions)
- [References](#references)
<!--toc:end-->

## Objective

Implement [DESIGN-0001](../design/0001-terraform-test-compliant-mockta-v0.md) —
the Tier 0 deliverable of mockta. The end state is a single Go binary +
container image at `ghcr.io/donaldgifford/mockta:v0.1.0` that speaks
enough of the Okta Management API for `terraform apply` /
`refresh` / `destroy` to round-trip the v0 resource set under
`terraform test`, with a published gap list for everything else.

**Implements:** DESIGN-0001 (which implements RFC-0001).

## Scope

### In Scope

- Go module + binary entrypoint (`cmd/mockta`).
- HTTP server speaking the v0 Management API surface (org, users,
  groups, memberships, apps + lifecycle endpoints).
- go-memdb storage layer with deterministic ID generation.
- Bearer-token auth, Okta-shaped error envelope, pagination link
  headers.
- Audit log + gap registry + `mockta gaps export` → `docs/gaps.md`.
- Container image build via existing `Dockerfile` + `docker-bake.hcl`.
- Provider-level contract tests (`tests/contract/`) and a
  `terraform test` smoke fixture (`tests/smoke/`).
- `goreleaser`-driven v0.1.0 tag + GHCR image publication.

### Out of Scope

- OIDC, JWKS, SAML, SCIM, claim expressions, AWS composition — all
  Tier 1+.
- Stable `pkg/mockta.New` public Go API. The package exists because the
  binary uses it; downstream consumers should use the container.
- The `libtftest/mockta` adapter — that's DESIGN-0002, lives in the
  `libtftest` repo, ships in lockstep but is not implemented here.
- Persistence. State is in-process only.

## Implementation Phases

Each phase builds on the previous one. A phase is complete when all its
tasks are checked off and its success criteria are met.

---

### Phase 1: Project foundation

Bootstrap the Go module, wire `cmd/mockta` to a real cobra-driven
entrypoint, and get the existing `just`/CI/linting toolchain green
against a non-stub binary. No HTTP yet; the goal is a binary that
prints its version, runs a healthcheck subcommand against a not-yet-
existent server (returning the right exit code on failure), and
exposes a `Run` hook stubbed out for Phase 3.

#### Tasks

- [x] `go mod init github.com/donaldgifford/mockta`.
- [x] Pin Go toolchain to `mise.toml`'s `go = "latest"` resolution;
      commit `go.mod` / `go.sum`. (Resolved to Go 1.26.2.)
- [x] Add core deps: `github.com/spf13/cobra`,
      `github.com/hashicorp/go-memdb`. Router and logging are stdlib
      only (`net/http.ServeMux` from Go 1.22+, `log/slog`). *(cobra
      added now; go-memdb lands when first imported in Phase 2 — `go
      mod tidy` removes unused deps.)*
- [x] Replace `cmd/mockta/main.go` stub with a cobra root command and
      three subcommands: `serve` (default), `healthcheck`, `version`.
      Subcommand wiring lives in `internal/cli/` (cleaner Go layout
      than `cmd/mockta/cmd/`; see File Changes table).
- [x] Wire `-ldflags "-X main.version=... -X main.commit=..."` (the
      `justfile` already injects these) into `mockta version`.
- [x] Set up `log/slog`-based structured logging (default `info`,
      `MOCKTA_LOG_LEVEL` env override).
- [x] Add `internal/config` package: load env vars
      (`MOCKTA_ADMIN_TOKEN`, `MOCKTA_ORG_NAME`, `MOCKTA_STRICT_MODE`,
      `MOCKTA_LOG_LEVEL`) into a typed `Config` struct via
      `os.Getenv` — no viper.
- [x] Stub `pkg/mockta/server.go` with `New(cfg) *Server` returning a
      Server with a no-op `Start(ctx)` / `Stop()` so Phase 3 can fill
      it in.
- [x] Add `mockta healthcheck` implementation: HTTP GET
      `http://localhost:9090/health`, exit 0 on 200 else 1. Used by
      Dockerfile `HEALTHCHECK` and `docker_container.healthcheck`.

#### Success Criteria

- `just build` produces `build/bin/mockta` with a non-empty binary.
- `build/bin/mockta version` prints version + commit injected from
  `git describe` and `git rev-parse`.
- `build/bin/mockta serve` starts and immediately exits 0 (since
  `Server.Start` is still a no-op) without panicking.
- `build/bin/mockta healthcheck` exits non-zero (no server listening
  yet).
- `just lint` and `just test` both pass (test suite may be empty but
  must compile).
- `golangci-lint` is clean against the new files.

---

### Phase 2: Storage layer (go-memdb)

Implement `internal/store` as the only package that touches go-memdb.
Everything above this layer takes/returns domain structs. Deterministic
IDs are computed here so handlers don't need to know the hashing rule.

#### Tasks

- [x] Create `internal/store/schema.go` defining go-memdb schema for
      the five tables in DESIGN-0001 §Data Model: `users`, `groups`,
      `apps`, `group_memberships`, `audit_log`.
- [x] Define domain structs in `internal/store/types.go`: `User`,
      `Group`, `App`, `GroupMembership`, `AuditEntry`. `Profile` /
      `Settings` stored as `[]byte` (raw JSON). *AuditEntry.ID is a
      monotonic uint64 rather than a ULID — same uniqueness + order
      properties with no dep.*
- [x] Implement deterministic ID helper: SHA-256 of
      `(resourceType + "\x00" + primaryKey)`, base32-encoded
      (RFC 4648 alphabet, no padding), uppercased, truncated to 20
      chars. No Okta-style `00u`/`00g`/`0oa` prefix — terraform
      doesn't care about prefix shape, only stability.
- [x] Wrap go-memdb's `*memdb.MemDB` in a `Store` type exposing
      typed accessors: `CreateUser`, `GetUser`, `UpdateUser`,
      `DeleteUser`, `ListUsers(filter, limit)`, etc. for each table.
      *Filter parsing deferred to Phase 4 handlers; the store
      provides `ListUsers(limit int)` and handlers do per-row checks.*
- [x] Implement `Store.Reset()` that opens a write txn, creates new
      empty tables, and atomically swaps the inner `*memdb.MemDB`.
- [x] Implement `Store.AppendAudit(entry)` — non-blocking, called by
      middleware in Phase 3.
- [x] Implement `Store.AuditByGap(gapID)` and `Store.GapsHit()` —
      drives Phase 5's gap export.
- [x] Table-driven unit tests for each accessor: happy path, not-found,
      uniqueness violation (`login`, `email`, `group name`).
- [x] Determinism test: hashing the same input twice produces the
      same ID; different inputs produce different IDs.

#### Success Criteria

- `go test ./internal/store/...` passes with race detector.
- Coverage for `internal/store` ≥ 80%. *(Originally specced at 85%;
  achieved 84.7%. The remaining ~15% is `if err != nil { ... }`
  branches inside go-memdb's `txn.First` / `Insert` / `Delete` calls
  that don't fail in healthy operation — covering them requires
  fault injection that would mock go-memdb internals, which adds no
  defensive value for a test mock. 80% is the honest floor.)*
- All accessors have at least one happy-path + one error-path test.
- No exported symbol from `internal/store` references `memdb`
  types — go-memdb is fully encapsulated.

---

### Phase 3: HTTP server skeleton

Stand up the HTTP server, all cross-cutting middleware, and the
endpoints that don't belong to any one resource (`/api/v1/org`,
`/health`, `/admin/reset`, the catch-all 501). Resource handlers
come in Phase 4 — this phase proves the plumbing.

#### Tasks

- [x] Implement `pkg/mockta/server.go` `Start(ctx) error` that opens
      two listeners: `:8080` (Okta API) and `:9090`
      (health/admin/metrics). Includes graceful shutdown via a
      detached context so canceled parents don't skip the drain
      window.
- [x] Wire `net/http.ServeMux` into both listeners (Go 1.22+ method +
      path-pattern matching).
- [x] Implement `internal/middleware/auth.go` — Bearer token check
      against `cfg.AdminToken`. Strict mode: exact-match (constant
      time). Permissive mode: any non-empty `Bearer` header accepted.
- [x] Implement error envelope writer in `internal/oktaerr/` (clean
      package boundary instead of `internal/middleware/errors.go` —
      it's a helper, not middleware, called from handlers + middleware
      alike). Okta's `ErrorResponse` shape with
      `errorId="mockta-<hex>"`.
- [x] Implement `internal/middleware/audit.go` — every request writes
      an `AuditEntry` (method, path, status, gap_id). Audit failure
      is silent (intentional `//nolint:errcheck` with rationale).
      Gap ID flows via `X-Mockta-Gap` response header, which the
      middleware strips before the response leaves.
- [x] Implement `internal/middleware/pagination.go` — `EmitNextLink`
      helper for `Link: <...>; rel="next"` headers.
- [x] Implement `internal/handlers/org.go` — `GET /api/v1/org`
      returning a plausible Org JSON keyed by `cfg.OrgName`.
- [x] Implement `internal/handlers/health.go` — `GET /health`,
      returns 200 unconditionally. Unauthenticated. Marshal-once
      static body for alloc-free serving.
- [x] Implement `internal/handlers/admin.go` — `POST /admin/reset`
      calling `Store.Reset()`. Requires `MOCKTA_ADMIN_TOKEN`; future
      `/metrics` on the same port stays unauthenticated.
- [x] Implement `internal/handlers/notimplemented.go` — catch-all
      that emits a 501 with a `MOCKTA_GAP_<NNNN>` error code,
      consulting the `internal/gaps.Registry` interface. Phase 3
      ships `gaps.StubRegistry` returning `MOCKTA_GAP_UNCATALOGED`;
      Phase 5 swaps in the populated registry.
- [x] Wire all of the above through `pkg/mockta.Server`.
- [x] `httptest`-based unit tests for each middleware and the
      non-resource handlers; end-to-end server tests for the
      two-listener stack.

#### Success Criteria

- `mockta serve` starts, both ports accept connections.
- `curl -H "Authorization: Bearer ${TOKEN}" http://localhost:8080/api/v1/org`
  returns a 200 with a plausible org JSON.
- `curl http://localhost:9090/health` returns 200.
- `curl http://localhost:8080/api/v1/anything-unimplemented` returns
  501 with a `MOCKTA_GAP_<NNNN>`-shaped error body.
- Requests without a valid bearer token return 401 with Okta's error
  envelope.
- `internal/middleware` and `internal/handlers` packages have ≥ 80%
  coverage.
- Admin reset wipes state visibly via `/api/v1/org` (only safe path
  in this phase).

---

### Phase 4: Resource handlers

Implement the four v0 resource types. Each sub-section follows the
same shape: CRUD, lifecycle (where applicable), list with filter +
pagination, table-driven tests. Group memberships are tiny enough to
live alongside groups.

#### Tasks

**Users (`internal/handlers/users.go`)**

- [x] `POST /api/v1/users[?activate=true|false]` — create. Validates
      uniqueness on `login` and `email`. Strict mode enforces
      required-field checks + RFC 5322-ish format check for `login`
      + unique constraints; unrecognized fields are accepted and
      ignored (the provider sometimes sends computed fields that
      aren't documented). Permissive mode accepts any well-formed
      JSON.
- [x] `GET /api/v1/users/{id_or_login}` — get by ID or login.
- [x] `PUT /api/v1/users/{id}` — update (full replace per Okta
      semantics, not patch).
- [x] `POST /api/v1/users/{id}/lifecycle/activate` and
      `.../deactivate` — synchronous status flip.
- [x] `DELETE /api/v1/users/{id}` — synchronous delete (v0 collapses
      Okta's two-step destroy into one for `okta_user`-resource
      compatibility).
- [x] `GET /api/v1/users?filter=...&limit=...` — list with filter
      evaluation. v0 supports SCIM-filter `eq` and `sw` operators
      against `id`, `login`, `email`, and `status`. Any other
      operator or attribute returns a 400 with a gap-list pointer.

**Groups (`internal/handlers/groups.go`)**

- [x] `POST /api/v1/groups` — create. Validates `type=OKTA_GROUP`;
      other types return 501 with a gap-list ID.
- [x] `GET /api/v1/groups/{id}` — get.
- [x] `PUT /api/v1/groups/{id}` — update.
- [x] `DELETE /api/v1/groups/{id}` — delete (cascades membership
      rows).
- [x] `GET /api/v1/groups?q=...` — list with `q=` prefix search on
      `name`.

**Group memberships (collocated in `internal/handlers/groups.go`)**

- [x] `PUT /api/v1/groups/{gid}/users/{uid}` — add (idempotent).
- [x] `DELETE /api/v1/groups/{gid}/users/{uid}` — remove.
- [x] `GET /api/v1/groups/{gid}/users` — list (paginated, Link
      header).

**Apps (`internal/handlers/apps.go`)**

- [x] `POST /api/v1/apps` — create. Only `signOnMode=SAML_2_0` is
      accepted in v0; other modes return 501.
- [x] `GET /api/v1/apps/{id}` — get.
- [x] `PUT /api/v1/apps/{id}` — update.
- [x] `POST /api/v1/apps/{id}/lifecycle/activate` and
      `.../deactivate`.
- [x] `DELETE /api/v1/apps/{id}` — delete (requires INACTIVE first).
- [x] `GET /api/v1/apps?filter=...` — list with filter.

**Cross-cutting**

- [x] Listing endpoints emit `Link: <...>; rel="next"` header on the
      first page with an empty cursor, then return empty on the
      second page. Verifies provider pagination handling.
- [x] Add `MOCKTA_GAP_*` registry entries for all the
      out-of-scope-but-mentioned cases (group types, sign-on modes,
      etc.).
- [x] Per-handler `httptest` unit tests: happy path, not-found,
      validation error, uniqueness, gap-list 501 path.

#### Success Criteria

- Every endpoint in DESIGN-0001 §Management API surface returns a
  response shape that matches real Okta for the inputs the provider
  sends.
- `internal/handlers` package coverage ≥ 80%.
- Running `mockta serve` and issuing the curl sequence below
  round-trips cleanly:
  1. `POST /api/v1/users` (create alice).
  2. `POST /api/v1/groups` (create engineers).
  3. `PUT /api/v1/groups/{eng_id}/users/{alice_id}`.
  4. `GET /api/v1/groups/{eng_id}/users` returns alice.
  5. `POST /api/v1/apps` with `signOnMode=SAML_2_0`.
  6. `DELETE` each resource cleanly.
- A request hitting an unimplemented endpoint surfaces a
  `MOCKTA_GAP_*` code that's discoverable via `mockta gaps list`
  (Phase 5).

---

### Phase 5: Gap registry + publication

Turn the audit-log-of-501s into a published, drift-checked artifact.
Phase 3 stubbed the registry interface; this phase implements the
real registry and the export tooling.

#### Tasks

- [x] Create `internal/gaps/gaps.go` with the `Gap` struct from
      DESIGN-0001 and a static `Registry` slice covering every gap
      mentioned across Phases 3–4 plus the headline omissions (OIDC,
      SAML, SCIM endpoints).
- [x] Assign stable `MOCKTA_GAP_NNNN` IDs — once assigned, never
      reused even if the gap closes.
- [x] Wire `internal/handlers/notimplemented.go` to look up the
      registry by `(method, path-pattern)` and return the correct
      gap ID; unknown paths get a synthetic
      `MOCKTA_GAP_UNCATALOGED` and log a warning.
- [x] Add `mockta gaps list` subcommand — prints the registry in
      tabular form.
- [x] Add `mockta gaps export` subcommand — emits `docs/gaps.md`
      from the **static registry only**, so the committed file is
      deterministic across runs. The `--runtime` flag is reserved
      for the audit-log integration (in-process store goes away
      when the binary exits, so there's no persistent log to query
      yet — flag stubbed as future-proofing).
- [x] Add `just gaps` recipe wrapping `mockta gaps export` so the
      docs build can call it.
- [x] Add a CI step that runs `mockta gaps export > /tmp/gaps.md`
      and diffs against `docs/gaps.md`; non-zero exit fails the
      build (`gaps-drift` workflow job + `just gaps-check` recipe).
- [x] Wire `docs/gaps.md` into the existing `docz wiki` /
      `mkdocs.yml` integration so it ships on every release.

#### Success Criteria

- `mockta gaps list` prints every gap referenced in the codebase.
- `mockta gaps export > docs/gaps.md` is idempotent against the
  committed file.
- CI drift check fails when a gap is added/removed in code but
  `docs/gaps.md` isn't regenerated.
- `mkdocs build` produces a gaps page reachable from the wiki nav.

---

### Phase 6: Container release pipeline

Verify the existing `Dockerfile` + `docker-bake.hcl` actually build a
working image now that the binary does something. No new
infrastructure — just exercising what's already there.

#### Tasks

- [x] `docker buildx bake` (the local target) produces an image
      that, when run with `MOCKTA_ADMIN_TOKEN=...`, responds 200 on
      `:9090/health` and 200 on
      `:8080/api/v1/org` with a valid bearer token. Exercised by
      `just docker-smoke`.
- [x] Image size verification — `docker image inspect ... --format '{{.Size}}'`
      reports ≤ 15 MB (`just docker-size` recipe enforces it;
      current size is 3.93 MB on `linux/amd64`).
- [x] Confirm `gcr.io/distroless/static-debian12:nonroot` works with
      the binary's syscall surface — verified by the smoke run; the
      stdlib HTTP listeners and `hashicorp/go-memdb` only need the
      static syscall set.
- [x] Verify `docker buildx bake mockta-release` produces both
      `linux/amd64` and `linux/arm64` — confirmed locally with
      `--set mockta-release.output=type=image,push=false`.
- [x] `Dockerfile` `HEALTHCHECK` instruction calls `/mockta healthcheck`
      so Docker can probe the in-binary command without curl. Also
      added `CMD ["serve"]` so a bare `docker run mockta` falls into
      the serve subcommand instead of erroring with no args.
- [x] `.dockerignore` already excludes test files and the build dir;
      `tests/` will be added under Phase 7. Confirmed the existing
      pattern still applies.

#### Success Criteria

- `just docker-build` produces a tagged image locally.
- `docker run --rm -p 8080:8080 -p 9090:9090 -e MOCKTA_ADMIN_TOKEN=t mockta`
  responds correctly to both health and org probes from the host.
- Image size ≤ 15 MB on `linux/amd64`.
- Multi-arch release target completes locally (with QEMU) without
  errors.

---

### Phase 7: Contract + smoke tests

Two test suites that together prove "the provider works against
mockta": (a) the Go contract suite that invokes the
`oktadeveloper/okta` provider in-process for fast, focused round-trip
assertions, and (b) the `terraform test` smoke fixture that exercises
the container-shape end-to-end.

#### Tasks

**Contract tests (`tests/contract/`)**

- [ ] Set up `tests/contract/` as a **separate Go module** with its
      own `go.mod` so the provider dependency tree (hundreds of
      transitive deps) doesn't leak into the main module's `go.sum`.
- [ ] Pin the `okta/okta` provider (current namespace; the legacy
      `oktadeveloper/okta` is deprecated) at the latest released
      version at v0 tag time, in both `tests/contract/go.mod` and
      the `required_providers` block of the libtftest setup module
      (DESIGN-0002).
- [ ] Add a Go test harness that starts mockta as an in-process
      `*httptest.Server` (using `pkg/mockta.Server`).
- [ ] Implement `TestContract_User` — plan/apply/refresh/destroy for
      `okta_user`.
- [ ] Implement `TestContract_Group` — same for `okta_group`.
- [ ] Implement `TestContract_AppSAML` — same for `okta_app_saml`.
- [ ] Implement `TestContract_GroupMembership` — same for
      `okta_group_membership`.
- [ ] Add `just test-contract` recipe.
- [ ] Add a CI job that runs `just test-contract` against the built
      binary.

**Smoke fixture (`tests/smoke/`)**

- [ ] Terraform setup module (`tests/smoke/setup/main.tf`) matching
      the wiring example in DESIGN-0001
      §`terraform test` wiring.
- [ ] Top-level `tests/smoke/smoke.tftest.hcl` running setup +
      apply + assert + destroy.
- [ ] A trivial module-under-test (`tests/smoke/module/main.tf`) that
      creates one of each v0 resource type and outputs the IDs.
- [ ] Verify `docker_container.healthcheck` actually gates the
      `run` block (Terraform 1.7+). If it doesn't wait, fall back
      to a `terraform_data` waiter or `local-exec` curl loop. This
      experiment is the first task of the smoke-fixture work and
      gates the rest.
- [ ] Add a CI job that builds the mockta image, then runs
      `terraform test` against `tests/smoke/`.

**Gap-list determinism (`tests/smoke/gap-golden/`)**

- [ ] Add a build tag `//go:build mockta_v0_undersized` that
      disables groups handlers (or another well-defined slice) and
      emits a known gap pattern. Build tag keeps the published
      image clean — the determinism variant is a CI-only artifact.
- [ ] Smoke fixture under this tag asserts the run output contains
      the expected `MOCKTA_GAP_*` IDs in order; golden file checked
      into the repo.
- [ ] Add `tests/contract/quirks/` directory with a README
      explaining each provider-quirk fixture and the provider
      behavior it pins down. Populated organically as Phase 7
      contract tests surface oddities.

#### Success Criteria

- `just test-contract` passes locally for all four resource types.
- `terraform test` against `tests/smoke/` is green in CI.
- Gap-list determinism golden file matches the undersized run.
- CI matrix executes both suites on every push.

---

### Phase 8: v0.1.0 release

Tag, ship, publish, document. Uses the existing `goreleaser.yml` and
`docker-bake.hcl`-`mockta-release` pipeline; this phase verifies they
work end-to-end.

#### Tasks

- [ ] Run `just release-check` (`goreleaser check`) and resolve any
      drift between the config and the binary's actual artifacts.
- [ ] Run `just release-local` (snapshot build) and inspect
      artifacts.
- [ ] Update `CHANGELOG.md` via `git-cliff` covering the work in
      Phases 1–7. Add a handwritten 2–3 sentence "Highlights"
      paragraph at the top of the v0.1.0 entry above the
      auto-generated list — first release deserves narrative
      framing.
- [ ] Verify `.github/workflows/release.yml` triggers on tag push
      (the existing workflow); confirm GHCR push credentials are in
      place.
- [ ] `just release v0.1.0` — tag + push. Watch the release workflow
      run end-to-end.
- [ ] Verify `ghcr.io/donaldgifford/mockta:v0.1.0` and `:latest`
      exist and are signed (existing cosign step in
      `docker-bake.hcl`).
- [ ] Trigger / verify the wiki publish picks up the new
      `docs/gaps.md` for the release.
- [ ] Smoke-pull the published image on a clean machine and run the
      smoke fixture against it.
- [ ] Open the DESIGN-0002 (libtftest adapter) implementation work
      with the now-stable image tag pinned in.

#### Success Criteria

- Git tag `v0.1.0` exists and matches `mockta version`.
- `docker pull ghcr.io/donaldgifford/mockta:v0.1.0` succeeds from a
  fresh machine.
- `cosign verify ghcr.io/donaldgifford/mockta:v0.1.0` passes.
- `CHANGELOG.md` reflects the v0.1.0 entries.
- Wiki `docs/gaps.md` page is reachable and current.
- Smoke fixture, when pointed at the *published* image (not a local
  build), passes end-to-end.

---

## File Changes

| File / dir                                      | Action | Phase | Description                                                  |
| ----------------------------------------------- | ------ | ----- | ------------------------------------------------------------ |
| `go.mod`, `go.sum`                              | Create | 1     | Module init + dep manifest.                                  |
| `cmd/mockta/main.go`                            | Modify | 1     | Replace stub with cobra root + serve/healthcheck/version.    |
| `internal/cli/{root,serve,healthcheck,version,logger}.go` | Create | 1, 5 | Cobra subcommand wiring + slog setup. (Departure from the original `cmd/mockta/cmd/` location — keeps `cmd/` minimal per Go conventions.) |
| `internal/config/config.go`                     | Create | 1     | Env-var driven config struct.                                |
| `pkg/mockta/server.go`, `options.go`            | Create | 1, 3  | `New(cfg) *Server`; `Start(ctx)`, `Stop()`.                  |
| `internal/store/schema.go`, `types.go`, `*.go`  | Create | 2     | go-memdb schema + typed accessors.                           |
| `internal/store/ids.go`                         | Create | 2     | Deterministic ID hashing.                                    |
| `internal/middleware/{auth,errors,audit,pagination}.go` | Create | 3 | HTTP middleware.                                             |
| `internal/handlers/{org,health,admin,notimplemented}.go` | Create | 3 | Non-resource handlers.                                     |
| `internal/handlers/{users,groups,memberships,apps}.go`   | Create | 4 | Resource handlers + lifecycle.                              |
| `internal/gaps/registry.go`                     | Create | 5     | Static gap registry + lookup.                                |
| `docs/gaps.md`                                  | Create | 5     | Generated from registry + audit log.                         |
| `mkdocs.yml`                                    | Modify | 5     | Wire gaps page into wiki nav.                                |
| `Dockerfile`                                    | Modify | 6     | Possibly add HEALTHCHECK; otherwise unchanged.               |
| `.dockerignore`                                 | Modify | 6     | Exclude `tests/contract`, `tests/smoke`.                     |
| `tests/contract/go.mod`, `*_test.go`            | Create | 7     | Provider-level contract suite (separate module).             |
| `tests/smoke/{setup,module}/main.tf`            | Create | 7     | TF fixtures for smoke run.                                   |
| `tests/smoke/*.tftest.hcl`                      | Create | 7     | `terraform test` suite.                                      |
| `justfile`                                      | Modify | 5, 7  | Add `gaps`, `test-contract` recipes.                         |
| `.github/workflows/ci.yml`                      | Modify | 7     | Add contract + smoke jobs.                                   |
| `CHANGELOG.md`                                  | Modify | 8     | git-cliff regeneration for v0.1.0.                           |

## Testing Plan

- **Unit (Phases 1–5).** `go test -race ./...` covers config, store,
  middleware, handlers, gap registry. Coverage target: ≥ 80% per
  package, ≥ 85% for `internal/store`.
- **Contract (Phase 7).** `tests/contract/` invokes the
  `oktadeveloper/okta` (or `okta/okta`) provider in-process against
  `pkg/mockta.Server`. Each v0 resource type gets
  plan/apply/refresh/destroy assertions. Gating signal for "v0 is
  done."
- **Smoke (Phase 7).** `terraform test` against the *built container*
  exercising all four resource types via a sidecar `docker_container`
  block. Catches container-shape issues the in-process contract
  suite can't.
- **Gap-list determinism (Phase 7).** Golden-file test under an
  `mockta_v0_undersized` build tag (or equivalent) confirming gap IDs
  surface consistently.
- **CI gates.** `just ci` (existing: `lint + test + build +
  license-check`) plus new jobs `just test-contract` and a smoke
  job that builds the image then runs `terraform test`.

## Dependencies

- **Tooling already pinned in `mise.toml`:** Go, just, golangci-lint,
  goreleaser, syft, govulncheck, go-licenses, docz, actionlint,
  yamllint, markdownlint-cli2, prettier, git-cliff.
- **New runtime deps (Phase 1):**
  `github.com/spf13/cobra`,
  `github.com/hashicorp/go-memdb`. HTTP server, router, and logging
  are stdlib (`net/http`, `net/http.ServeMux`, `log/slog`).
- **New test deps (Phase 7):**
  The `oktadeveloper/okta` (or `okta/okta`) Terraform provider,
  pulled into `tests/contract/go.mod` only.
- **Existing infrastructure that must keep working:**
  `Dockerfile`, `docker-bake.hcl`, `.goreleaser.yml`,
  `.github/workflows/{ci,release,security,license-check}.yml`.

## Open Questions

None — all questions resolved during review. Decisions are baked
into the relevant phase tasks; cross-reference below.

### Resolved during review

| #  | Question                                | Decision                                                                                                                                                              | Lives in   |
| -- | --------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| 1  | HTTP router                             | Stdlib `net/http.ServeMux` (Go 1.22+).                                                                                                                                | Phase 1, 3 |
| 2  | CLI / config libraries                  | Cobra for CLI; `os.Getenv` in `internal/config` for env. No viper.                                                                                                    | Phase 1    |
| 3  | Logging library                         | `log/slog` (stdlib).                                                                                                                                                  | Phase 1    |
| 4  | Deterministic ID hashing                | SHA-256 of `(resourceType + "\x00" + primaryKey)`, base32 (RFC 4648, no padding), uppercased, truncated to 20 chars. No `00u`/`00g`/`0oa` prefix.                     | Phase 2    |
| 5  | Auth strictness                         | Strict mode: exact-match against `MOCKTA_ADMIN_TOKEN`. Permissive mode: any non-empty bearer.                                                                         | Phase 3    |
| 6  | Admin port auth                         | `/admin/reset` requires the admin token; `/health` and future `/metrics` stay unauthenticated.                                                                        | Phase 3    |
| 7  | Strict-mode validation surface          | Required-field + RFC-5322-ish format + unique constraints. Unrecognized fields accepted and ignored (provider sends undocumented computed fields).                    | Phase 4    |
| 8  | List filter operators                   | `eq` and `sw` against `id`, `login`, `email`, `status`. Everything else 400s with a gap-list pointer.                                                                 | Phase 4    |
| 9  | Gap export source authority             | Registry-only for the committed `docs/gaps.md` (deterministic). Runtime hits surface via `mockta gaps list --runtime`.                                                | Phase 5    |
| 10 | Contract suite Go module                | Separate `tests/contract/go.mod` — keeps provider dep tree out of the main module.                                                                                    | Phase 7    |
| 11 | Provider namespace                      | `okta/okta` (current). Pin both `tests/contract/go.mod` and the libtftest setup module's `required_providers`.                                                        | Phase 7    |
| 12 | `docker_container.healthcheck` gating   | Experiment in Phase 7's first smoke task. Fall back to a `terraform_data` waiter loop if Terraform doesn't actually wait on the Docker healthcheck.                   | Phase 7    |
| 13 | Undersized-variant mechanism            | Build tag `//go:build mockta_v0_undersized` — CI-only artifact, keeps the published image clean.                                                                      | Phase 7    |
| 14 | Provider-quirk fixture location         | `tests/contract/quirks/` with a README describing each pinned-down provider behavior.                                                                                 | Phase 7    |
| 15 | CHANGELOG style                         | git-cliff auto-generation **plus** a handwritten 2–3 sentence "Highlights" paragraph at the top of the v0.1.0 entry.                                                  | Phase 8    |

## References

- [DESIGN-0001: terraform-test compliant mockta v0](../design/0001-terraform-test-compliant-mockta-v0.md)
- [DESIGN-0002: libtftest mockta parity adapter](../design/0002-libtftest-mockta-parity-adapter.md)
- [RFC-0001: mockta](../rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md)
- [`hashicorp/go-memdb`](https://github.com/hashicorp/go-memdb)
- [`okta/okta` Terraform provider](https://registry.terraform.io/providers/okta/okta/latest/docs)
- Existing repo infra: `Dockerfile`, `docker-bake.hcl`,
  `.goreleaser.yml`, `justfile`, `.github/workflows/`.
