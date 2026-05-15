# mockta

A lightweight, embeddable Okta mock for Terraform acceptance tests and Go
service tests. Real OIDC/SAML protocols, no separate IdP process, no JVM.

> _"LocalStack for Okta — but you only get the resources you actually need."_

---

## Why

Testing Terraform modules and Go services that integrate with Okta has three
unsatisfying options today:

1. **Real preview tenant** — high fidelity, but rate-limited, slow, and leaks
   orphaned resources on failed runs.
2. **Plan-only** — fast and hermetic, but can't validate anything that requires
   an API round-trip (SAML metadata, claim mapping, group propagation, token
   issuance).
3. **Hand-rolled httptest stubs** — works for one module, doesn't compose.

mockta targets the gap: real protocol behavior for the resources we care about,
single Go binary, in-process or sidecar, no operational tax.

## Non-goals

- Not a production IdP. Test infrastructure only.
- Not comprehensive Okta API coverage. Narrowly scoped to the resources our
  Terraform actually creates.
- Not an interactive login UI. Test harnesses pass user identity via header or
  session config; no browser flows.

## MVP scope

The first release will support **Okta Terraform modules tested directly via
`terraform test`** — the same workflow we already use for every other module
in the org. No new test-harness shape, no new mental model; mockta slots into
the slot LocalStack already occupies for AWS.

Our existing pattern for AWS-touching modules:

1. Author the module.
2. Run `terraform test` against LocalStack to validate plan/apply coverage.
3. Wherever LocalStack falls short of real AWS at the plan/apply layer, file
   the gap into `libtftest` + `sneakystack` so the next module inherits the
   fix.

mockta's MVP replicates that loop one-for-one for Okta:

1. Author an `okta_*` Terraform module.
2. Run `terraform test` against mockta to validate plan/apply coverage.
3. Wherever mockta falls short of real Okta at the plan/apply layer, file the
   gap into `libtftest` (and, where the test crosses into AWS, `sneakystack`)
   so the next module inherits the fix.

### Why `terraform test` and not Go acceptance tests yet

`terraform test` is fast, hermetic, and lives in the module repo next to the
code it covers. It exercises the plan/apply layer end-to-end without standing
up a separate Go harness, which is exactly what we want from a v0 — the
question we need answered first is "does mockta's Management API behave well
enough for `terraform apply` to round-trip?" Everything past that (signed
assertions, SCIM, STS, claim mapping) is Tier 2+ in the roadmap below and
needs a different test shape.

### Pure-Okta vs. composed tests

For a pure-Okta module, mockta on its own is sufficient — no LocalStack, no
libtftest needed. **But the MVP still wires mockta through libtftest by
default**, even for pure-Okta cases, because:

- Remote state lives in S3, so any non-trivial module already needs LocalStack
  for the state backend.
- libtftest is where the org's test scaffolding lives; routing mockta through
  it from day one means consumers don't learn a second pattern.
- Parity matters more than minimalism here. The `libtftest/mockta` adapter
  may be a thin pass-through at first — that's fine. Whatever works in
  `terraform test` against mockta gets the same shape in `libtftest` even if
  the indirection is, strictly speaking, unnecessary at v0. Over time that
  layer will accumulate real responsibilities (waiters, audience computation,
  composite environments); starting with the indirection in place avoids a
  later flag-day rename.

### What ships in v0

- mockta core sufficient for `terraform apply` of `okta_user`, `okta_group`,
  `okta_app_saml`, `okta_group_membership` against the `oktadeveloper/okta`
  provider (plan, apply, refresh, destroy all round-trip).
- Distroless container image at `ghcr.io/donaldgifford/mockta` so
  `terraform test` `run` blocks can spin it up like LocalStack.
- A `libtftest/mockta` adapter — even if it does little more than expose
  `OKTA_BASE_URL` / `OKTA_API_TOKEN` / `OKTA_ORG_NAME` — so module authors
  follow the existing pattern.
- Documented gap list: every Okta API path mockta does not yet implement,
  with the smallest reproducer that hits it. Gaps drive the Tier 1+ work.

Out of scope for v0: SCIM, real signed assertions, OIDC token issuance, claim
expressions, AWS composition. Those land tier-by-tier per the roadmap below
once the plan/apply layer is solid.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ Terratest acceptance test (Go)                                      │
└──────────────────────┬──────────────────────────────────────────────┘
                       │ libtftest.NewIAMEnv(t)
       ┌───────────────┴────────────────┐
       ▼                                ▼
┌────────────────┐              ┌──────────────────┐
│ mockta         │              │ LocalStack       │
│                │              │                  │
│ ┌────────────┐ │  SCIM push   │ ┌──────────────┐ │
│ │ Mgmt API   │─┼─────────────►│ │ identitystore│ │
│ │ /api/v1/*  │ │              │ └──────────────┘ │
│ └────────────┘ │              │ ┌──────────────┐ │
│ ┌────────────┐ │              │ │ sso-admin    │ │
│ │ OIDC (op)  │ │              │ └──────────────┘ │
│ │ /oauth2/*  │ │              │ ┌──────────────┐ │
│ └────────────┘ │ SAML POST    │ │ STS + IAM    │ │
│ ┌────────────┐ │ (assertion)  │ │ SAML provider│ │
│ │ SAML IdP   │─┼─────────────►│ └──────────────┘ │
│ │ /app/*/sso │ │              │                  │
│ └────────────┘ │              │                  │
│  shared state  │              │                  │
└────────────────┘              └──────────────────┘
       ▲                                ▲
       │ okta provider                  │ aws provider
       │                                │
┌──────┴────────────────────────────────┴────────────────────────────┐
│ Terraform fixtures                                                 │
│   okta_user, okta_group, okta_app_saml, okta_group_membership      │
│   aws_iam_saml_provider, aws_iam_role,                             │
│   aws_ssoadmin_permission_set, aws_ssoadmin_account_assignment     │
└────────────────────────────────────────────────────────────────────┘
```

### Two-plane model

| Plane          | Layer                                 | Responsibility                                                                        |
| -------------- | ------------------------------------- | ------------------------------------------------------------------------------------- |
| **Management** | mockta REST + LocalStack admin APIs   | What TF creates: apps, users, groups, permission sets, assignments                    |
| **Runtime**    | mockta OIDC/SAML IdP + LocalStack STS | What "real login" exercises: signed assertions, `AssumeRoleWithSAML`, temporary creds |

Keeping these distinct is the design's load-bearing decision. The runtime plane
uses old-school IAM SAML federation (not Identity Center's portal), because that
path is fully implemented by LocalStack STS and lets us assert on real
credentials. Identity Center stays purely as the management abstraction TF
reconciles into the identity store.

## Components

### mockta core

| Surface              | Backed by                                   | Purpose                                                           |
| -------------------- | ------------------------------------------- | ----------------------------------------------------------------- |
| `/api/v1/*`          | hand-rolled handlers + `oapi-codegen` stubs | Okta Management API (apps, users, groups, auth servers, policies) |
| `/oauth2/v1/*`       | `github.com/zitadel/oidc/v3/pkg/op`         | OIDC OP — discovery, authorize, token, userinfo, JWKS, introspect |
| `/app/{id}/sso/saml` | `github.com/crewjam/saml/samlidp`           | SAML IdP — metadata, signed assertions                            |
| state                | `hashicorp/go-memdb` (in-process, MVCC)     | apps, users, groups, sessions, keys                               |

Single Go binary. Embeddable as a library: `mockta.New(opts).Start()` returns an
`httptest.Server`-shaped handle, perfect for in-process use.

### SCIM bridge

mockta acts as a SCIM client, pushing user/group changes to downstream identity
stores when an app has provisioning enabled. For AWS, the "identity store"
target is LocalStack's `identitystore` service.

- **Synchronous by default.** Push happens during the Okta API call that
  triggered it (e.g. `POST /api/v1/groups/{g}/users/{u}` → immediately call
  `identitystore:CreateGroupMembership`). Tests don't need waiters.
- **Async mode available.** Optional toggle for tests that specifically want to
  exercise eventual-consistency behavior.
- **Out-of-scope:** real SCIM v2 protocol marshaling. We call the destination's
  native API (`identitystore`, GitHub REST, etc.) directly. Testing
  SCIM-the-protocol is a separate concern.

### Claim mapper

Group membership claims are the most-requested feature. Implementation:

```go
type ClaimMapper struct {
    Attributes []AttributeStatement
}

type AttributeStatement struct {
    Name       string  // e.g. "https://aws.amazon.com/SAML/Attributes/Role"
    Format     string  // basic | uri | unspecified
    NameFormat string
    ValueFn    func(user *User, groups []*Group) []string
}
```

The expression subset we support initially (matches `okta_app_saml` field
semantics):

- `STARTS_WITH(name, prefix)`
- `EQUALS(name, value)`
- `MATCHES_REGEX(name, pattern)`
- `CONTAINS(name, substring)`

Documented as "the subset of Okta's expression language mockta implements."
Tests only use what we implement; document gaps, don't fake them.

### Testcontainers module

mockta ships a [testcontainers-go](https://golang.testcontainers.org/) module so
consumers can run it as an ephemeral Docker container with one line of Go. Same
pattern as `modules/localstack`, `modules/postgres`:

```go
import "github.com/donaldgifford/mockta/modules/mockta"

func TestSomething(t *testing.T) {
    ctx := context.Background()
    mk, err := mockta.Run(ctx, "ghcr.io/donaldgifford/mockta:v0.1.0",
        mockta.WithSeedFile("./testdata/seed.yaml"),
    )
    testcontainers.CleanupContainer(t, mk)
    require.NoError(t, err)

    // mk.BaseURL is the URL to point an Okta provider at
}
```

Module options:

- `WithAdminToken(token string)` — fixed token instead of generated.
- `WithSeedFile(path string)` — mount a YAML seed file, loaded on startup.
- `WithSCIMTarget(url string)` — outbound SCIM destination (e.g. LocalStack).
- `WithNetwork(name string)` — join an existing Docker network for
  inter-container communication.

Implementation notes:

- **Base image:** `gcr.io/distroless/static` or `scratch`, single static Go
  binary. Target sub-15MB image size.
- **Multi-arch:** `linux/amd64` + `linux/arm64` via `docker buildx`.
- **Health endpoint:** `GET /health` returns 200 once OIDC keys generate and
  SAML metadata is ready; module wait strategy keys off this.
- **Logs:** streamed to `t.Log` via testcontainers' `LogConsumer`, so failures
  surface mockta-side context automatically.
- **Reuse:** `testcontainers.WithReuse()` is supported for local-dev iteration
  but breaks isolation; documented, not defaulted.
- **Seed format:** matches `okta_*` Terraform resource shapes so users can
  copy-paste fixtures into seed YAML without a translation step.

Polyglot for free — the same image works from `testcontainers-python`,
`testcontainers-java`, etc.

### libtftest integration layer

Lives in `libtftest`, not mockta. Exposes both in-process and container modes;
consumers pick based on what they're testing.

```go
// libtftest/mockta/env.go — in-process (fast, no Docker)
func New(t *testing.T, opts ...Option) *Env {
    srv := mockta.New(mockta.Options{
        Storage: mockta.InMemory(),
        Keys:    mockta.GenerateKeys(t),
    })
    srv.Start()
    t.Cleanup(srv.Close)

    return &Env{
        Server:   srv,
        OrgName:  srv.OrgName(),
        APIToken: srv.AdminToken(),
        BaseURL:  srv.URL(),
    }
}

// libtftest/mockta/container.go — testcontainers-backed (isolated, Docker required)
func NewContainer(t *testing.T, opts ...ContainerOption) *Env {
    ctx := context.Background()
    mk, err := tcmockta.Run(ctx, defaultMocktaImage)
    require.NoError(t, err)
    testcontainers.CleanupContainer(t, mk)

    return &Env{ /* same shape, container-backed */ }
}

func (e *Env) OktaProviderEnv() map[string]string {
    return map[string]string{
        "OKTA_ORG_NAME":  e.OrgName,
        "OKTA_BASE_URL":  e.BaseURL,
        "OKTA_API_TOKEN": e.APIToken,
    }
}

// libtftest/iamenv/env.go — the composite (always containers, shared network)
func NewIAMEnv(t *testing.T) *IAMEnv {
    ctx := context.Background()
    net, _ := network.New(ctx)
    t.Cleanup(func() { net.Remove(ctx) })

    ls := sneakystack.Run(t, sneakystack.WithNetwork(net.Name))
    mk, _ := tcmockta.Run(ctx, defaultMocktaImage,
        tcmockta.WithNetwork(net.Name),
        tcmockta.WithSCIMTarget("http://"+ls.NetworkAlias()+":4566"),
    )
    testcontainers.CleanupContainer(t, mk)

    return &IAMEnv{Mockta: mk, LocalStack: ls, Network: net}
}
```

Rule of thumb: `New()` for unit-style Okta-only tests where startup speed
matters; `NewContainer()` when isolation matters more than speed; `NewIAMEnv()`
for any test that crosses into AWS-land (always containers because sneakystack
is).

## Repo layout

### mockta (own repo)

```
mockta/
├── cmd/mockta/
│   └── main.go                 # binary entrypoint (sidecar mode)
├── pkg/mockta/
│   ├── server.go               # New(opts) returns *Server, embeddable
│   ├── options.go
│   └── handle.go               # public Handle for in-process use
├── pkg/api/                    # Okta REST handlers
│   ├── apps.go
│   ├── users.go
│   ├── groups.go
│   ├── authservers.go
│   ├── policies.go
│   └── grouprules.go
├── pkg/oidc/
│   └── storage.go              # op.Storage impl over shared state
├── pkg/saml/
│   └── store.go                # samlidp.Store impl over shared state
├── pkg/scim/
│   ├── client.go               # generic outbound SCIM-ish push
│   └── aws.go                  # AWS identitystore bridge
├── pkg/claims/
│   ├── mapper.go
│   └── expr.go                 # subset expression engine
├── modules/mockta/             # testcontainers-go module
│   ├── mockta.go               # Run(ctx, img, opts...) → *MocktaContainer
│   ├── options.go              # WithSeedFile, WithSCIMTarget, etc.
│   └── examples_test.go
├── internal/store/             # go-memdb schema + access helpers
├── build/
│   └── Dockerfile              # distroless/static, multi-arch
├── spec/
│   └── okta-management.yaml    # vendored Okta OpenAPI (resources we support)
├── docs/
└── testdata/
```

Three distribution shapes, same codebase:

- **Library** (`import "github.com/donaldgifford/mockta/pkg/mockta"`) —
  in-process embedding in Go tests. Fastest, no IPC, no Docker.
- **Binary** (`go install github.com/donaldgifford/mockta/cmd/mockta@latest`) —
  sidecar use, manual exploration, docker-compose setups.
- **Container + testcontainers module**
  (`import "github.com/donaldgifford/mockta/modules/mockta"` + image at
  `ghcr.io/donaldgifford/mockta`) — polyglot, isolated, CI parity with
  prod-shape deployments.

These are complementary, not alternatives. Library mode is the fast path for
Go-only tests; the container is the natural shape when Docker is already in the
test setup (which it is whenever sneakystack is too).

### libtftest (existing)

```
libtftest/
├── mockta/                     # thin adapter, ~50 LOC
│   ├── env.go
│   └── waiters.go
├── iamenv/                     # composite mockta + localstack
│   └── env.go
├── sneakystack/                # existing
└── ...
```

## Canonical end-to-end test

```go
func TestAliceCanAssumeEngineerRole(t *testing.T) {
    env := libtftest.NewIAMEnv(t)
    defer env.Cleanup()

    tf := terraform.Options{
        TerraformDir: "./fixtures/okta-aws-sso",
        EnvVars:      env.ProviderEnv(),
    }
    terraform.InitAndApply(t, &tf)

    // SCIM-pushed sync; assert mockta → identitystore landed
    env.WaitUntilUserInIdentityStore(t, "alice@example.com")
    env.WaitUntilGroupHasMember(t, "engineers", "alice@example.com")

    // Issue a real signed SAML assertion from mockta
    assertion := env.Mockta.IssueSAMLAssertion(t, mockta.IssueOpts{
        User:     "alice@example.com",
        AppID:    terraform.Output(t, &tf, "aws_app_id"),
        Audience: env.LocalStack.SAMLAudience(),
    })

    // Exchange via LocalStack STS
    sts := env.LocalStack.STSClient()
    out, err := sts.AssumeRoleWithSAML(ctx, &sts.AssumeRoleWithSAMLInput{
        RoleArn:       aws.String(terraform.Output(t, &tf, "engineer_role_arn")),
        PrincipalArn:  aws.String(terraform.Output(t, &tf, "saml_provider_arn")),
        SAMLAssertion: aws.String(base64.StdEncoding.EncodeToString(assertion)),
    })
    require.NoError(t, err)

    caller, _ := env.LocalStack.STSClientWith(out.Credentials).GetCallerIdentity(ctx)
    require.Contains(t, *caller.Arn, "EngineerRole")
}
```

Every step is real protocol behavior. Signed assertion, validated audience, role
trust policy match, STS issuance, identity verification. No mocked return values
inside the test itself.

## Integration scope

Start with three, chosen for protocol coverage rather than popularity:

| Integration                 | Protocols exercised       | Why                                                                                            |
| --------------------------- | ------------------------- | ---------------------------------------------------------------------------------------------- |
| **AWS IAM Identity Center** | SAML + SCIM + STS         | The headliner; real credential issuance closes the loop                                        |
| **Wiz**                     | SAML + custom SAMLMapping | Forces the `okta_auth_server` abstraction to earn its keep; aligned with existing Wiz SDK work |
| **GitHub Enterprise**       | SAML + SCIM               | Sanity check that SAML path generalizes; widely deployed                                       |

Deferred: Google Workspace (SCIM-only, derives from AWS path), Slack (SCIM-only,
trivial), everything else.

## Tier roadmap

| Tier  | Scope                                                                                 | Effort        | Unlocks                                                   |
| ----- | ------------------------------------------------------------------------------------- | ------------- | --------------------------------------------------------- |
| **0** | Stub REST API, in-memory state, fake IDs                                              | 1 week        | TF apply/destroy round-trip works                         |
| **1** | + real JWT signing, JWKS endpoint, OIDC discovery, Dockerfile + testcontainers module | +1 week       | Tests can validate token contents; polyglot via container |
| **2** | + SAML IdP via crewjam/saml, signed assertions                                        | +2 weeks      | End-to-end SAML federation works                          |
| **3** | + SCIM bridge to identitystore, AWS integration                                       | +2 weeks      | The canonical test above works                            |
| **4** | + Wiz and GitHub integrations                                                         | +2 weeks each | Coverage broadens                                         |
| **5** | + claim expression engine (subset)                                                    | +1 week       | Realistic group-claim mapping                             |

Roughly 8–10 weeks to the full vision, but every tier is independently useful
and shippable.

## Hard parts

Just so it's written down:

- **Audience / entity ID alignment.** Three places (TF okta_app_saml, TF
  aws_iam_saml_provider, mockta-issued assertion) must agree. Helpers in
  `iamenv` should compute these from a single source of truth.
- **SAML XML canonicalization edge cases.** crewjam/saml handles the protocol
  well; the surprises are in attribute statements with multi- valued claims and
  namespace prefix quirks. Pin test assertions to known attribute ordering.
- **Role trust policy SAML conditions.** Real AWS validates `SAML:aud` strictly.
  Verify LocalStack does the same — if it doesn't, tests can pass while prod
  breaks. Write an intentional-mismatch test to probe.
- **Permission set materialization.** Real Identity Center provisions an IAM
  role per `account × permission_set`. LocalStack's behavior here is the biggest
  unknown; if it doesn't materialize roles, fixtures need to create them by hand
  and wire the trust policy explicitly.
- **Eventual consistency.** Sync-by-default is the right call for tests, but
  document it loudly. Anyone porting test patterns from real Okta needs to know
  the model is different.
- **Auth server semantics.** Okta's "authorization server" abstraction (custom
  claims, scopes, policies per server) is uniquely Okta. Each one in mockta maps
  to its own issuer URL, key pair, claim transforms. The
  `okta_auth_server`-to-issuer mapping is design work, not library work.

## Open questions

- **Persistence?** `hashicorp/go-memdb`, in-process, no durability. If
  the sidecar-binary path later needs reproducibility across runs, the
  opt-in is a JSON snapshot on shutdown — not a real DB.
- **API token / admin auth model.** Real Okta has rich scoped tokens. Start with
  a single static admin token; add scoped tokens only when a test actually needs
  to assert on auth failures.
- **OpenAPI vendor strategy.** Pin a specific upstream tag, or vendor the slice
  we use? Pin first; vendor only if we need to make modifications.
- **Test mode vs strict mode.** Should mockta have a "strict" mode that rejects
  requests Okta would reject, vs "permissive" mode that accepts anything
  well-formed? Strict by default, permissive opt-in for negative tests.
- **`libtftest/mockta` default mode.** In-process by default with container
  opt-in via `NewContainer()`, or container by default for parity with
  sneakystack? Leaning in-process default — startup cost matters when test
  counts grow — but worth a closer look once we have real test counts to reason
  about.
- **Open-source it?** Probably yes once Tier 2 lands. Nothing here is
  proprietary, and the ecosystem clearly wants this.

## Status

Pre-design. This doc is the starting point — promote to a docz RFC once the open
questions resolve.
