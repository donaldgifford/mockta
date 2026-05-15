# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`mockta` is a lightweight, embeddable Okta mock for Terraform acceptance tests and Go service tests. v0 implementation tracks IMPL-0001 — Phases 1–4 complete (CLI + config + server + store + middleware + resource handlers wired). DESIGN-0001 specs the mockta surface; DESIGN-0002 specs the libtftest adapter; RFC-0001 is the umbrella.

The Go module is `github.com/donaldgifford/mockta`, Go 1.26.2 (mise-resolved). Layout:

- `cmd/mockta/main.go` — thin entry point; only holds `version`/`commit` ldflag vars and calls `cli.NewRootCmd`.
- `internal/cli/` — cobra subcommand tree (root, serve, healthcheck, version, logger setup). Deliberate departure from `cmd/mockta/cmd/` — keeps `cmd/` minimal and the CLI package importable.
- `internal/config/` — env-var driven `Config` loader. No viper; v0 is env-only.
- `pkg/mockta/` — `Server` with `Start`/`Stop`. Public package but the Go API is not yet stable — downstream consumers should use the container.
- `internal/store/` — hashicorp/go-memdb persistence layer. Domain types (User, Group, App, GroupMembership, AuditEntry) stored with raw-JSON `Profile`/`Settings` blobs. Sentinel errors `ErrNotFound` / `ErrConflict` are the contract handlers map to HTTP statuses. IDs are deterministic SHA-256 → base32 → 20 chars; AuditEntry.ID is a monotonic uint64 (not a ULID).
- `internal/oktaerr/` — Okta-shaped error envelope writer. `errorId` is always `mockta-<hex>` so failures are obvious in provider logs.
- `internal/middleware/` — `Auth` (constant-time bearer match, strict + permissive modes), `Audit` (writes every request into the store; gap IDs flow via the `X-Mockta-Gap` response header which the middleware strips), `Chain` composer, `EmitNextLink` helper. Empty `MOCKTA_ADMIN_TOKEN` disables auth entirely — useful for scripts.
- `internal/handlers/` — HTTP handlers. `dto.go` holds shared decode + error-mapping helpers (`decodeJSONBodyLenient`, `writeStoreError`). `org.go` / `health.go` / `admin.go` / `notimplemented.go` are the non-resource endpoints; `users.go`, `groups.go`, `apps.go` implement the v0 resource CRUD + lifecycle + list flows. Filters use a SCIM-ish `attr op "value"` grammar — `eq` + `sw` for users/apps, `q=` prefix-on-name for groups. Out-of-scope variants (APP_GROUP, BUILT_IN, non-SAML signOnMode, group filter, unsupported user attributes/operators) return 501/400 with a `MOCKTA_GAP_*` ID surfaced via the `X-Mockta-Gap` header.
- `internal/gaps/` — `Registry` interface + `StubRegistry`. Phase 5 replaces the stub with a populated registry and adds `mockta gaps` CLI subcommands.
- `pkg/mockta/` — `Server` opens `:8080` (API) and `:9090` (admin/health); graceful shutdown via `context.WithoutCancel` so canceled parents don't skip the drain window.

## Common commands

`just` is the task runner — run `just` with no args to list recipes. Recipes are grouped under `build`, `run`, `test`, `lint`, `license`, `release`, `gate`, and `docker`.

- Build the binary into `build/bin/mockta`: `just build`
- Run tests with race detector: `just test`
- Run a single package: `just test-pkg ./path/to/pkg`
- HTML coverage report: `just test-report`
- Lint (golangci-lint): `just lint` (or `just lint-fix`)
- Format: `just fmt` (gofmt + goimports with `github.com/donaldgifford` as local prefix)
- Pre-commit gate (lint + test): `just check`
- Full CI gate (lint + test + build + license-check): `just ci`
- License compliance: `just license-check` (allowed: Apache-2.0, MIT, BSD-2/3-Clause, ISC, MPL-2.0)
- Docker build via bake: `just docker-build` (recipes live in `docker.just`, imported into the main justfile)
- Tag and push a release: `just release v0.1.0` (`.goreleaser.yml` drives the release)

Tool versions are pinned via `mise.toml` — run `mise install` to bootstrap the toolchain (Go, golangci-lint, just, goreleaser, docz, forge, etc.).

## Architecture and structure

- `cmd/mockta/main.go` — single-binary entry point. `version` and `commit` are injected via `-ldflags` at build time from `git describe` / `git rev-parse`.
- Go module path will be `github.com/donaldgifford/mockta` (see `justfile` `go_package` and `.golangci.yml` `goimports.local-prefixes`).
- Container image: distroless multi-stage `Dockerfile`. Multi-arch builds are orchestrated by `docker-bake.hcl` with three targets:
  - `mockta` — local single-arch
  - `mockta-ci` — linux/amd64 only (PR validation; arm64 via QEMU is too slow for PR feedback)
  - `mockta-release` — linux/amd64 + linux/arm64, pushed to `ghcr.io/donaldgifford/mockta`, tags injected by `docker/metadata-action` (the `docker-metadata-action` HCL target intentionally has no tags so the CI override fully replaces them — HCL child `tags` lists replace, not extend).
- Exposed ports in the container: `8080` and `9090`.

## Linting conventions

`.golangci.yml` enforces a heavily Uber-flavored config. A few non-obvious points worth knowing before adding code:

- `goimports`/`gci` group imports as: stdlib → third-party → `github.com/donaldgifford/*`.
- `golines` max line length is 150.
- Cyclomatic complexity cap is 15, cognitive 30, function length 100 lines / 50 statements, nesting depth 4.
- `errcheck` checks blank assignments and type assertions; a curated allow list covers common `Close()` defers and cobra/color helpers.
- `nolintlint` requires both an explanation and specific linter names for every `//nolint` directive.
- `goconst` triggers at 3 occurrences of a 3+ character literal.
- Test files relax `errcheck`, `funlen`, `gocyclo`, `gocognit`, `goconst`, `gosec`, `dupl`, `nilnil`. Mock files (`mock_*.go`) and generated `*.pb.go` get broader exclusions.

## Documentation

Project docs live under `docs/` and are managed by the `docz` CLI (config in `.docz.yaml`). Document types: `rfc`, `adr`, `design`, `impl`, `plan`, `investigation` — each with its own ID prefix, status workflow, and directory. `docz update` regenerates the README index tables; MkDocs nav is also auto-updated (`mkdocs.yml`, `techdocs-core` plugin for Backstage TechDocs integration).

Create new docs with `docz create <type>` rather than hand-writing files; the index and wiki nav sync automatically.

## Branch and commit conventions

- Branch prefixes drive PR auto-labeling via `.github/labeler.yml`: `feat/`, `fix/`, `chore/`, `docs/`, `bug/`. The `git-workflow:branch` skill scaffolds these.
- Conventional commits feed `git-cliff` (config in `cliff.toml`) which generates `CHANGELOG.md`.
- Commits with `[skip ci]` in the subject are used for changelog regeneration and release-only mechanics — do not pile feature work into them.
