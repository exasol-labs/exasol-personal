## Context

Custom SLCs are currently identified by the launcher alias supplied on the command line, while
the database validates aliases declared inside the package metadata. Two packages can therefore
have different launcher aliases but still declare the same database language alias. The conflict
is discovered only when the database starts.

## Goals / Non-Goals

**Goals:**

- Detect internal alias conflicts before staging or applying a custom SLC install/update.
- Keep the check generic for every custom SLC language.
- Allow an update to retain the aliases of the SLC it replaces.
- Avoid extracting the full archive and preserve existing SHA/no-op behaviour.

**Non-Goals:**

- Changing Podman image loading or database startup behaviour.
- Adding Rust-specific handling.
- Using the archive SHA as the alias-conflict key.
- Addressing unrelated SLC VM crashes.

## Decisions

- Read `build_info/language_definitions.json` directly from the gzip/tar stream. This reuses the
  package bytes already acquired, avoids full extraction, and leaves the original archive and its
  SHA unchanged for staging and update comparisons.
- Store the discovered internal aliases with each installed custom SLC. This makes conflict
  checks possible without reopening every staged package and also supports older state by treating
  missing metadata as an explicit validation error when it is needed.
- Compare candidate aliases with aliases from all other installed custom SLCs. For update and
  install replacement, exclude the existing record being replaced; for a new install, exclude no
  record. Comparison is case-insensitive, matching database alias semantics.
- Run validation before recording/staging the candidate and before any restart confirmation that
  could lead to a state-changing operation. Existing same-alias SHA no-op behaviour remains in
  place.

## Risks / Trade-offs

- [Older state has no persisted aliases] → Read metadata from the corresponding staged package
  when possible; otherwise fail with a clear message rather than permitting an unsafe operation.
- [Malformed or incomplete package metadata] → Reject the package with a validation error before
  changing deployment state.
- [Alias comparison differs from database normalisation] → Normalise aliases consistently and add
  tests for case differences and multiple aliases.
