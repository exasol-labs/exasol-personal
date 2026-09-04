## Why

Static local service ports can currently override custom preset defaults, miss legacy mappings on deploy retries, and present unsafe or incomplete recovery guidance after bind failures. The bind-failure classifier also re-probes the port after the runtime fails, adding complexity without proving what caused the original failure.

## What Changes

- Preserve local values declared by custom infrastructure presets unless the user explicitly overrides or resets them; compute launcher defaults only for values the preset leaves unset.
- Normalize legacy empty, automatic, zero-valued, or database-missing port mappings before every permitted local deploy or start attempt, including retries from failed or interrupted lifecycle states.
- Diagnose port conflicts from the actual launcher failure: match narrowly scoped bind-conflict diagnostics from Podman stderr or the macOS VM runner, preserve unrecognized failures, and remove post-failure availability probes.
- Restore a deployment's prior workflow state only when any partially started macOS VM was cleaned up successfully; otherwise retain normal interrupted-state recovery.
- Show the same local-port recovery commands for `exasol install` failures that are already shown by `exasol deploy` and `exasol start`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Broaden legacy-port normalization to all retryable launch paths and condition actionable bind-conflict recovery on a recognized diagnostic from the actual runtime failure.

## Impact

- Changes local backend defaulting and pre-launch normalization, direct-host Podman and macOS VM failure handling, lifecycle state recovery, and install command guidance.
- Simplifies the local-port error package by removing post-failure socket probing and its test injection points.
- Adds focused unit and integration coverage; no user-facing configuration syntax or dependency changes are required.
