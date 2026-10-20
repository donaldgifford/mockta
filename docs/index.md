# mockta

Welcome to the documentation for mockta, a lightweight Okta mock for
Terraform acceptance tests and Go service tests.

## Start here

- [Gap list](gaps.md) — every slice of the Okta Management API mockta
  does not implement, with the `MOCKTA_GAP_*` ID each one returns.
- [RFC-0001](rfc/0001-mockta-lightweight-okta-mock-for-terraform-and-go-service-tests.md)
  — the umbrella proposal and tiering.
- [DESIGN-0001](design/0001-terraform-test-compliant-mockta-v0.md) —
  the v0 surface that shipped as `ghcr.io/donaldgifford/mockta:0.1.0`.
- [DESIGN-0002](design/0002-libtftest-mockta-parity-adapter.md) — the
  libtftest adapter (lives in the libtftest repo).
- [Original vision notes](mockta.md) — the pre-RFC design sketch,
  kept for the Tier 1+ ideas it still carries.

## Document Types

- [RFCs](rfc/README.md)
- [ADRs](adr/README.md)
- [Design](design/README.md)
- [Implementation Plans](impl/README.md)
- [Plans](plan/README.md)
- [Investigations](investigation/README.md)
