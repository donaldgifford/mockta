# Provider quirks

This directory captures provider-specific oddities that mockta has to
match to keep `okta/okta` happy. Each subdirectory holds a focused
fixture plus a note on the behavior it pins down.

The intent is **regression protection**: when the provider author
fixes a quirk in a future release, we delete the matching fixture
here; when they regress, the fixture catches it.

## Format

Each quirk gets its own subdirectory with:

- `quirk.md` — what the provider does, why it matters, version range
  observed, and the corresponding mockta behavior.
- `<quirk>_test.go` — Go contract test asserting mockta still matches
  the documented quirk.

## Current quirks

None yet. Populated organically as Phase 7 contract testing surfaces
oddities against the pinned provider version.
