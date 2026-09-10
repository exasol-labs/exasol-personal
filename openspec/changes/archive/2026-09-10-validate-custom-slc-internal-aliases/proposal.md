## Why

Installing the same custom SLC package under two launcher aliases can expose the same internal
language alias twice. The database rejects that configuration only during startup, leaving the
deployment stopped; the launcher should reject the operation before changing the deployment.

## What Changes

- Read the internal language aliases from a custom SLC package before staging it.
- Reject custom install/update when those aliases conflict with another installed custom SLC.
- Allow an update to retain aliases already owned by the SLC being updated.
- Persist the discovered aliases in deployment state for later comparisons.
- Keep package SHA handling for existing no-op/update behaviour; do not use SHA as the conflict key.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `local-slc-management`: custom SLC install and update validate package alias uniqueness before applying changes.

## Impact

The custom-SLC archive reader, local deployment state, and custom SLC install/update paths are
affected. Existing official/custom launcher-alias handling, the database, Podman image loading,
and Rust-specific behaviour are unchanged.
