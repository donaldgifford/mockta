# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/).
## [unreleased]

### Features

- Scaffold Phase 1 — CLI, config, and server stub
- *(store)* Scaffold Phase 2 — go-memdb persistence layer
- Scaffold Phase 3 — HTTP server skeleton
- *(handlers)* Implement Phase 4 resource handlers
- *(gaps)* Populate gap registry with publication and drift check
- *(docker)* Verify Phase 6 container release pipeline
- *(tests)* Add contract suite and terraform-test smoke fixture
- *(gaps)* Add gap-list determinism golden test
- *(smoke)* TLS-terminate via caddy so okta provider can dial :443
- *(middleware)* Accept SSWS auth scheme alongside Bearer
- *(handlers)* Synthetic GET /api/v1/users/me for provider configure

### Bug Fixes

- *(smoke)* Replace run.<self> with output.* in apply_module asserts
- *(smoke)* Hardcode /tmp/mockta-smoke-certs as cert location
- Two-step DELETE for users + sign SAML response in smoke fixture

### Documentation

- Add v0 planning docs (RFC-0001, DESIGN-0001/0002, IMPL-0001)
- Check off IMPL-0001 Phase 1 tasks; record CLI layout decision

### Testing

- Raise oktaerr and cli coverage above IMPL targets

### Miscellaneous Tasks

- Initialize Go module
- Fix double-escaped variable references in justfile
- *(release)* Wire Phase 8 release prep
- *(docker)* Explicitly exclude test dirs from build context
- Swap broken make call for just in test-go job
- Bump smoke terraform to 1.10 so test-file modules resolve
- Regenerate CHANGELOG.md from cliff
- Add Apache-2.0 LICENSE
- Init each smoke submodule explicitly and bump terraform to 1.14

