# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-05-15

### Highlights

First public release of mockta — a lightweight, embeddable Okta mock for
Terraform acceptance tests and Go service tests. v0 ships the four
resources the `okta/okta` provider's happy-path uses
(`okta_user`, `okta_group`, `okta_group_membership`, `okta_app_saml`),
a deterministic gap registry that surfaces unimplemented endpoints as
stable `MOCKTA_GAP_NNNN` IDs, and a ~4 MB distroless container image
that drops into the same `docker_container` block consumers already
use for LocalStack. The companion `libtftest/mockta` adapter
(DESIGN-0002) lands next.

### Features

- Scaffold Phase 1 — CLI, config, and server stub
- *(store)* Scaffold Phase 2 — go-memdb persistence layer
- Scaffold Phase 3 — HTTP server skeleton
- *(handlers)* Implement Phase 4 resource handlers
- *(gaps)* Populate gap registry with publication and drift check
- *(docker)* Verify Phase 6 container release pipeline
- *(tests)* Add contract suite and terraform-test smoke fixture

### Documentation

- Add v0 planning docs (RFC-0001, DESIGN-0001/0002, IMPL-0001)
- Check off IMPL-0001 Phase 1 tasks; record CLI layout decision

### Miscellaneous Tasks

- Initialize Go module
- Fix double-escaped variable references in justfile
