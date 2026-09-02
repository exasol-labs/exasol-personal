## Why

Local deployments can currently delegate host-port selection to the runtime, which makes endpoints change across starts and gives users poor recovery guidance when a requested port cannot be bound. Persisting deterministic, usable ports makes local endpoints stable while still allowing multiple deployments and explicit reconfiguration.

## What Changes

- Let infrastructure backends provide concrete defaults for unset preset variables during deployment initialization; the local backend uses this hook to assign every exposed service a fixed host port.
- Select automatic local ports deterministically from each service's default, scanning upward through `65535`, wrapping to `1024`, and stopping before revisiting the default; a candidate must be usable on every localhost loopback address.
- Persist concrete local service-port mappings so the direct-host runtime on Linux and Windows and the macOS VM runtime always use the selected fixed port.
- Have the runtime or installation layer surface a human-readable port-unavailable error when the actual bind fails, including races after initial selection, and add a call to action directing the user to `exasol config set` with another port.
- Allow all local deployment configuration to be changed or reset while the deployment is stopped, with guidance to run `exasol start` afterward; retain the existing guards for running local and deployed cloud configurations.
- Replace legacy empty, `auto`, and zero-valued local port mappings with concrete fixed assignments before a stopped deployment starts.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Local services receive persisted, deterministic static ports; runtime bind conflicts are actionable; legacy automatic mappings are migrated.
- `deployment-reconfiguration`: Stopped local deployments allow configuration changes and direct the user to restart the deployment to apply them.

## Impact

- Extends the internal infrastructure-backend interface with a method that supplies defaults for unset preset variables.
- Changes local backend configuration, direct-host publication, macOS VM forwarding, lifecycle error handling, and configuration guidance.
- Adds unit, integration, and portable local deployment coverage for supported local platforms and updates the README, test inventory, and changelog.
