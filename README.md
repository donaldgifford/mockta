# mockta

A lightweight, embeddable Okta mock for Terraform acceptance tests and
Go service tests. One static Go binary in a ~4 MB distroless image that
speaks enough of the Okta Management API for `terraform apply` /
`refresh` / `destroy` to round-trip users, groups, group memberships,
and SAML apps under `terraform test`.

Everything else returns `501` with a stable `MOCKTA_GAP_NNNN` error
code so a failing test tells you exactly which Okta surface is missing.
The full list lives in [docs/gaps.md](docs/gaps.md).

## Run it

```bash
docker run --rm -p 8080:8080 -p 9090:9090 \
  -e MOCKTA_ADMIN_TOKEN=test-admin-token \
  -e MOCKTA_ORG_NAME=acme-test \
  ghcr.io/donaldgifford/mockta:0.1.0
```

| Port | Serves                                         |
| ---- | ---------------------------------------------- |
| 8080 | Okta Management API (`/api/v1/...`)            |
| 9090 | `GET /health` (unauthenticated), `POST /admin/reset` |

Point the `okta/okta` provider at it with `OKTA_ORG_NAME`,
`OKTA_BASE_URL`, and `OKTA_API_TOKEN=test-admin-token`. The provider
only speaks HTTPS on 443, so put a TLS terminator in front for
Terraform use; `tests/smoke/setup/` shows the Caddy sidecar pattern.

Image tags follow the git tag without the `v`: `0.1.0`, `0.1`, `latest`.
Images are multi-arch (amd64 + arm64) and cosign-signed.

### Configuration

| Env var              | Default      | Notes                                                         |
| -------------------- | ------------ | ------------------------------------------------------------- |
| `MOCKTA_ADMIN_TOKEN` | *(empty)*    | Bearer token to require. Empty disables auth entirely.        |
| `MOCKTA_ORG_NAME`    | `mockta-dev` | Surfaced via `GET /api/v1/org`.                               |
| `MOCKTA_STRICT_MODE` | `true`       | Enforce Okta's documented validation; `false` accepts anything well-formed. |
| `MOCKTA_LOG_LEVEL`   | `info`       | `debug` / `info` / `warn` / `error`.                          |

### CLI

```text
mockta serve         # start both listeners (what the image runs)
mockta healthcheck   # exit 0 if :9090/health is 200 (used by HEALTHCHECK)
mockta gaps list     # print the gap registry
mockta gaps export   # regenerate docs/gaps.md
mockta version
```

## Develop

Tooling is pinned in `mise.toml`; `mise install` bootstraps it. `just`
lists the recipes.

```bash
just check          # lint + unit tests
just test-all       # unit + contract + gap-golden suites
just ci             # the full gate CI runs
just docker-build   # local image
just test-smoke     # terraform test against the local image (needs Docker)
```

Docs are managed with `docz` under `docs/`:
[RFC-0001](docs/rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md)
is the umbrella,
[DESIGN-0001](docs/design/0001-terraform-test-compliant-mockta-v0.md)
specs the v0 surface, and
[DESIGN-0002](docs/design/0002-libtftest-mockta-parity-adapter.md)
specs the libtftest adapter that wraps the container.

## Status

v0.1.0 is released. The Go package under `pkg/mockta` is public but not
yet a stable API; consume the container. OIDC, SAML assertions, SCIM,
policies, and auth servers are Tier 1+ and tracked as gaps.

## License

Apache-2.0. See [LICENSE](LICENSE).
