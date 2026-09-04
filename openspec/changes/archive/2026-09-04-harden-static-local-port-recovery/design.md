## Context

See `proposal.md` for motivation. Initialization currently asks every deployment backend for values to merge ahead of user overrides, even though the extracted preset is already the source of defaults and backend configuration already resolves local automatic ports. Legacy normalization is exposed through a start-only optional interface. Bind-failure classification combines launcher diagnostics with a later socket probe, which can neither prove the original cause nor eliminate a race.

The runtime bind remains authoritative. Direct-host startup publishes one database port through `podman run`; macOS publishes one host forward through the VM runner. A recoverable conflict may restore the prior workflow state only after the launch path establishes that no usable runtime endpoint remains.

## Goals / Non-Goals

**Goals:**

- Preserve custom preset values while keeping user overrides and reset behavior unchanged.
- Normalize all legacy automatic database mappings before any permitted local launch path.
- Base recoverable conflict classification only on the actual launch failure.
- Keep workflow restoration safe when macOS cleanup fails.
- Keep each fix independently testable and committable.

**Non-Goals:**

- Changing deterministic port allocation order, loopback coverage, or configuration syntax.
- Guaranteeing classification across every future Podman or VM-runner diagnostic wording.
- Reserving a configured port before runtime launch or eliminating the bind race.
- Broadening stopped-deployment configuration beyond the behavior already specified.

## Decisions

### Extracted preset values remain the initialization baseline

Remove the backend-wide `ConfigurationDefaults` hook and its merge plumbing. Initialization passes only user-supplied values to the existing configuration writer. The local backend overlays those values on the extracted manifest, resolves only missing or automatic ports, and persists the result. Consequently, an explicit custom preset port or macOS sizing value remains intact, while a command-line value still takes precedence.

Configuration reset continues through each configuration value's existing raw default. Resetting ports supplies the automatic value to local configuration, which performs allocation before writing the manifest; numeric local options retain their computed reset defaults. Tofu continues to rely on preset and variable defaults when the user omits an override.

Keeping the hook and teaching it to distinguish initialization from reset was rejected because the distinction would require a wider interface or mode flag and would duplicate default ownership between orchestration and the backend.

### Local preparation normalizes legacy mappings for every launch path

Move legacy-port normalization into local backend preparation, before the lifecycle operation is marked in progress. Both deploy and start already call preparation after their state guards, so initialization, deploy retries, stopped starts, and permitted interrupted retries share one path without a lifecycle-specific migrator interface.

A mapping needs normalization when it is empty, `auto`, assigns zero to a known service, or omits a known service. Normalization uses the existing allocator and configuration writer, preserves positive mappings, and persists successfully before runtime prerequisites or launch proceed. Allocation or persistence failure leaves the pre-attempt workflow state unchanged.

Adding more state cases to the start-only migrator was rejected because deploy and install retries would remain a separate path. Migrating in command handlers was rejected because it would duplicate local backend policy.

### Bind conflicts are classified from actual launch diagnostics only

Simplify the shared classifier to accept the service, configured port, original launch error, and captured diagnostic. Remove the availability callback, socket re-probe, and related injection fields. A failure becomes the typed unavailable-port error only when its lowercase diagnostic contains one of these narrow markers:

- `address already in use`
- `bind: address in use`
- `port is already allocated`
- `port already in use`

Remove the generic `failed to bind` marker because it can describe permission, address-family, or configuration failures. Direct-host Podman startup continues capturing `podman run` stderr and classifies only host-published database starts whose container is not running. macOS captures the VM runner's start stderr, combined with its returned error text, and applies the same classifier to the requested host forward.

A preflight check was rejected because it cannot diagnose a collision occurring after the check. A post-failure probe was rejected because a busy port after failure does not establish why the launch failed. Diagnostic wording is intentionally a pragmatic integration boundary: an unknown wording falls back to the ordinary launch error without corrupting state.

### Cleanup success gates recoverable macOS errors

When macOS startup has a recognized bind diagnostic, first stop any potentially partial VM. Return the typed unavailable-port error only after that stop succeeds. If it fails, join the original untyped launch error with the cleanup error so lifecycle code records the ordinary failure state instead of restoring `initialized` or `stopped` and suggesting immediate reconfiguration.

Returning a joined error containing the typed conflict was rejected because error-chain inspection would still classify it as safely recoverable.

### Install uses the existing recovery presentation

On a deployment error, the install post-run path invokes the same terminal recovery helper used by deploy and start before returning its contextual wrapper. The helper already traverses wrapped errors, so no new public error or command-specific formatting is needed.

## Risks / Trade-offs

- **A future runtime version may change its bind-conflict wording.** → Keep the marker set small and covered by representative diagnostics; unrecognized text safely follows normal lifecycle failure handling.
- **A localized or substantially different diagnostic will not receive the specialized recovery action.** → Preserve the original error so users still receive the underlying runtime failure and can retry after correction.
- **Moving migration into preparation writes configuration earlier in more retry paths.** → Perform it before the in-progress state transition and write only after allocation succeeds.
- **Removing backend-provided initialization defaults changes internal sequencing.** → Cover built-in and custom local presets, explicit overrides, resets, and Tofu omission behavior with focused tests.

## Migration Plan

Implement and verify preset preservation first, then broaden legacy normalization, simplify runtime failure classification and cleanup gating, and finally add install guidance. Existing concrete mappings require no migration, and rollback remains compatible because normalized mappings use the existing `service:<positive-port>` syntax.

Archive the OpenSpec change only after all tasks and checks pass, then commit the archived artifacts separately from implementation commits.
