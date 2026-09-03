# exasol-local-deployment Specification

## Purpose
TBD - created by archiving change add-exasol-local-backend. Update Purpose after archive.

## Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a VM-backed Podman installation on macOS Apple Silicon and host Podman on Linux AMD64 and ARM64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes and starts the VM with application forwards, installs Nano through guest Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Launcher-owned local runtime

The system SHALL own host-side Nano data, the macOS VM runtime disk, and the
managed deployment share inside the deployment directory, and SHALL resolve the
Exasol Local VM runner through the resource manager on every use rather than
maintaining a per-deployment copy of it.

#### Scenario: Runner is resolved without a per-deployment copy

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it resolves the Exasol Local runner through the resource manager and invokes it directly from the resolved location, without copying it into the deployment directory

#### Scenario: Missing version marker is initialized

- **WHEN** the launcher prepares a local deployment that has no persisted runner-version marker, or an invalid one, and the resolved runner reports a valid semantic version
- **THEN** it records the resolved runner's version as the deployment's persisted marker before invoking the runner

#### Scenario: Compatible runner update is recorded

- **WHEN** the resolved runner is a newer patch or minor version within the persisted marker's major version
- **THEN** the launcher updates the persisted marker to the resolved runner's version before starting the local deployment

#### Scenario: Release-candidate runner update is recorded

- **WHEN** a `v`-prefixed resolved runner release candidate has greater semantic precedence than the persisted marker's release candidate within the same major version
- **THEN** the launcher updates the persisted marker to the resolved runner's version before starting the local deployment

#### Scenario: Unsafe version relationship proceeds with a warning

- **WHEN** the resolved runner's version differs in major version from the persisted marker, or is older than the persisted marker within the same major version
- **THEN** the launcher proceeds using the resolved runner, logs a warning describing the version relationship, and updates the persisted marker to the resolved runner's version

#### Scenario: Version reconciliation is skipped for non-starting lifecycle behavior

- **WHEN** the launcher performs status, stop, or destroy behavior for a local deployment
- **THEN** it resolves and invokes the runner without comparing or updating the persisted version marker

#### Scenario: Resolved runner version is invalid

- **WHEN** the resolved runner does not report a valid semantic version during preparation
- **THEN** the launcher fails before invoking it, unless forced reconciliation is enabled

#### Scenario: Internal forced-reconciliation bypass is enabled

- **WHEN** development explicitly enables forced reconciliation and the resolved runner does not report a valid semantic version
- **THEN** the launcher proceeds with the resolved runner without version compatibility checks and warns that reconciliation was forced

#### Scenario: Runner VM sizing is prepared

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it exposes VM CPU, VM memory, and VM runtime-disk sizing through the
  runner start command

#### Scenario: Managed share is prepared

- **WHEN** the launcher initializes a local deployment
- **THEN** it creates a launcher-managed share for guest coordination, SSH key
  import, and host-visible Nano data

#### Scenario: User shares are not exposed

- **WHEN** the user initializes or starts a local deployment
- **THEN** the launcher does not require or expose user-configurable shared folder settings

### Requirement: Local Nano data is host-visible

The system SHALL expose the complete persistent Nano `/exa` tree at
`local/runtime/exa` in the deployment directory on every supported local
platform.

#### Scenario: Fresh local deployment starts

- **WHEN** a user installs a fresh local deployment on Linux, macOS, or Windows
- **THEN** Nano stores its persistent `/exa` content in
  `local/runtime/exa`

#### Scenario: Host creates a runtime file

- **WHEN** a file is created below `local/runtime/exa` while the local runtime
  is available
- **THEN** the file is visible at the corresponding path below Nano's `/exa`

#### Scenario: Nano creates a runtime file

- **WHEN** Nano creates a file below `/exa`
- **THEN** the file is visible at the corresponding path below
  `local/runtime/exa`

#### Scenario: Nano runs an SLC

- **WHEN** Nano initializes or runs an SLC in a local deployment
- **THEN** persistent SLC files remain visible below
  `local/runtime/exa/slc`

#### Scenario: Existing direct-host deployment starts

- **WHEN** an existing Linux or Windows deployment has managed host-side Nano
  data
- **THEN** the launcher continues using that data at `local/runtime/exa`

### Requirement: Local deployment artifacts

The system SHALL write standard launcher artifacts for local deployments so existing information, connection, status, and shell commands can operate on the deployment directory.

#### Scenario: Connection artifacts are written after startup

- **WHEN** the local Podman installation starts successfully
- **THEN** the launcher writes `deployment.json`, `secrets.json`, and connection instructions with loopback application connection details

#### Scenario: Local credentials are available

- **WHEN** the launcher writes secrets for a local deployment
- **THEN** `secrets.json` contains database credentials for user `sys` with password `exasol`

#### Scenario: Forwarded ports are refreshed

- **WHEN** a macOS local deployment is started after being stopped
- **THEN** the launcher refreshes `deployment.json` with the current labeled database and UI forwards without recording an SSH endpoint

### Requirement: Local SQL connection

The system SHALL allow `exasol connect` to connect to the Exasol Local database using the local deployment artifacts.

#### Scenario: Connect to local database

- **WHEN** a local deployment is running and the user runs `exasol connect`
- **THEN** the launcher connects to the Exasol Local database through the loopback database endpoint using the stored local credentials, within a bounded timeout

#### Scenario: Local certificate validation mode

- **WHEN** the launcher creates connection metadata for a local deployment without a stable database certificate fingerprint
- **THEN** the metadata marks certificate validation as insecure for that local deployment

### Requirement: Local lifecycle commands
The system SHALL support standard lifecycle commands for local deployments and
preserve host-backed Nano data until the deployment is destroyed.

#### Scenario: Stop local deployment
- **WHEN** a macOS local deployment is running and the user runs `exasol stop`
- **THEN** the launcher removes the disposable Nano container, stops the VM,
  preserves host-backed Nano data, and records the deployment as stopped

#### Scenario: Start local deployment
- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher starts its execution environment with the existing
  host-backed Nano data, starts the shared Podman installation, waits up to a
  bounded timeout until the database accepts connections, refreshes connection
  artifacts, and records the deployment as running

#### Scenario: Start local deployment times out
- **WHEN** a local deployment is stopped, the user runs `exasol start`, and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Local execution environment stops unexpectedly
- **WHEN** a local execution environment stops without a graceful Nano shutdown
  and the deployment is started again
- **THEN** the launcher recovers the runtime with the same host-backed Nano data

#### Scenario: Destroy local deployment
- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher removes the disposable container, stops its execution
  environment if needed, deletes deployment-owned host-backed Nano data, any
  macOS migration backup, and connection artifacts, and records the deployment
  as initialized

### Requirement: Local shell access

The CLI SHALL provide `exasol shell host` and `exasol shell container` for supported local deployments and SHALL report the shell-specific cause as the authoritative error when a requested shell cannot be opened.

#### Scenario: Host shell

- **WHEN** a macOS local deployment is running and the user runs `exasol shell host`
- **THEN** the command opens an interactive shell in the environment hosting the local deployment

#### Scenario: Container shell

- **WHEN** a macOS local deployment is running and the user runs `exasol shell container`
- **THEN** the command opens an interactive shell in the local deployment's database container environment

#### Scenario: Shell launch fails

- **WHEN** a supported local shell command cannot open the requested environment
- **THEN** the command fails and reports the shell-specific cause as the authoritative error

### Requirement: macOS local VM memory default
The system SHALL default local deployment VM memory on macOS to approximately 50% of total host memory when the user has not configured local VM memory explicitly.

#### Scenario: Default local VM memory on macOS
- **WHEN** a user initializes or starts a local deployment on macOS without setting `memory_mb`
- **THEN** the launcher uses a default local VM memory value of approximately 50% of total host memory

#### Scenario: Explicit local VM memory overrides macOS default
- **WHEN** a user configures `memory_mb` for a local deployment on macOS
- **THEN** the launcher uses the configured value instead of the computed default

### Requirement: minimum configured local VM memory
The system SHALL reject user-configured local deployment memory below 4096 MB.

#### Scenario: Configured local VM memory below minimum
- **WHEN** a user configures `memory_mb` below 4096 for a local deployment
- **THEN** the launcher fails before using the configuration and explains that `memory_mb` must be at least 4096 MB

### Requirement: minimum host memory for macOS local deployment
The system SHALL fail local deployment on macOS when detected host memory is below 8192 MB.

#### Scenario: Host memory below minimum
- **WHEN** the launcher detects host memory below 8192 MB for a macOS local deployment
- **THEN** the launcher fails before starting the local deployment and explains the required and detected host memory

### Requirement: Local configuration is platform-specific
The system SHALL expose and validate VM CPU, memory, and runtime-disk sizing
only for the macOS VM runtime, while direct-host platforms SHALL ignore
persisted VM sizing and use host-managed resources.

#### Scenario: Linux local configuration contains VM sizing
- **WHEN** a Linux deployment contains shared VM CPU, memory, or data-size values
- **THEN** local validation and startup ignore those values and do not advertise them as effective Linux settings

#### Scenario: Windows local configuration contains VM sizing
- **WHEN** a Windows deployment contains shared VM CPU, memory, or data-size
  values
- **THEN** local validation and startup ignore those values and do not advertise
  them as effective Windows settings

#### Scenario: macOS local configuration contains invalid VM sizing
- **WHEN** a macOS Apple Silicon deployment contains invalid VM sizing
- **THEN** validation fails as before

#### Scenario: macOS data-size configuration is inspected
- **WHEN** a user inspects local configuration on macOS
- **THEN** the data-size setting describes VM runtime-disk capacity for Podman
  images and runtime state


### Requirement: Linux local status and health reflect the published database endpoint
The system SHALL derive Linux runtime status from the exact Podman deployment container, recover missing in-memory endpoints from deployment information, and probe the published database port for health.

#### Scenario: Runtime memory has no endpoints
- **WHEN** Linux local status or health is requested after process restart and deployment information contains the database endpoint
- **THEN** the runtime recovers that endpoint and uses it for status and health reporting

#### Scenario: Published database endpoint accepts connections
- **WHEN** the exact deployment container is running and the published database port accepts a connection
- **THEN** local health is healthy

#### Scenario: Linux database is unreachable
- **WHEN** the published Linux database endpoint cannot be reached
- **THEN** diagnostics describe the Linux host-container failure without macOS host-to-VM guidance

### Requirement: Linux local shell commands fail explicitly

The CLI SHALL report host and container shell commands as unsupported for Linux local deployments.

#### Scenario: Host shell requested on Linux

- **WHEN** a user runs `exasol shell host` for a Linux local deployment
- **THEN** the command fails with an explicit error identifying host shell access as unsupported on Linux

#### Scenario: Container shell requested on Linux

- **WHEN** a user runs `exasol shell container` for a Linux local deployment
- **THEN** the command fails with an explicit error identifying container shell access as unsupported on Linux

### Requirement: Historical local data remains persistent during adoption

The system SHALL preserve Nano data when adopting a historical local container, migrating `/exa` only when it is overlay-backed or mounted from a location other than the deployment's managed persistent data directory.

#### Scenario: Adopt a legacy-named container with managed persistent data

- **WHEN** a historical local container uses a legacy name and its `/exa` mount already originates from the deployment's managed persistent data directory
- **THEN** the launcher adopts the container without copying `/exa` or overwriting the managed persistent data directory

#### Scenario: Adopt a legacy container with data outside the managed directory

- **WHEN** a historical local container's `/exa` data is overlay-backed or mounted from a location other than the deployment's managed persistent data directory
- **THEN** the launcher stages that data into the managed persistent data directory before replacing the historical container

### Requirement: Local startup crosses explicit durability boundaries

The system SHALL synchronize local runtime storage on Linux and macOS after preparing container images and SHALL flush Nano startup state after the database becomes ready, without automatically repairing or resetting the Podman store.

#### Scenario: Images are synchronized before container startup

- **WHEN** the launcher has loaded, tagged, materialized, and pruned the images required by a local deployment
- **THEN** it synchronizes the execution environment before creating the Nano container

#### Scenario: Nano startup state is synchronized before startup succeeds

- **WHEN** a local database accepts connections during install or start
- **THEN** the launcher synchronizes the execution environment before reporting startup success

#### Scenario: Storage synchronization fails

- **WHEN** either startup synchronization fails
- **THEN** the launcher fails startup without automatically repairing or resetting the Podman store

### Requirement: Local readiness wait is short and diagnostic
The system SHALL check local database readiness every one to two seconds and SHALL fail a local install or start operation within 30 seconds when the database does not accept connections. When the local runtime identifies a network-wide reachability problem, the failure SHALL retain the most recent database connection error and include the runtime's actionable reachability guidance.

#### Scenario: Local database becomes ready
- **WHEN** a local database accepts connections during install or start
- **THEN** the launcher completes startup without an unnecessary readiness delay

#### Scenario: Local database does not become ready
- **WHEN** a local database does not accept connections during install or start
- **THEN** the launcher fails the operation within 30 seconds and reports the most recent connection failure

#### Scenario: Local network path is blocked
- **WHEN** a local readiness wait ends and the runtime identifies a network-wide reachability problem
- **THEN** the launcher reports actionable reachability guidance together with the most recent connection failure

### Requirement: Local deployments SHALL persist deterministic service ports

When a local deployment is initialized or reconfigured with an unset or automatic port mapping, the launcher SHALL select and persist a concrete host port for every exposed service. For each service, automatic allocation SHALL try its default port first, scan upward through `65535`, wrap to the lowest unprivileged port `1024`, and continue through the port immediately before the default. A selected port SHALL be available on every IPv4 and IPv6 loopback address resolved for `localhost` at selection time.

#### Scenario: Default database port is available

- **WHEN** a user initializes a local deployment with the port mapping omitted
- **AND** database port `8563` is usable on every resolved localhost loopback address
- **THEN** the launcher persists `db:8563` as the deployment's concrete port mapping

#### Scenario: Allocation advances from the service default

- **WHEN** a user initializes a local deployment with the port mapping omitted
- **AND** the database default port and one or more immediately following ports are unavailable on at least one resolved localhost loopback address
- **THEN** the launcher persists the first following port that is usable on every resolved localhost loopback address

#### Scenario: Allocation wraps through the unprivileged range

- **WHEN** every port from a service's default through `65535` is unavailable
- **AND** a port is available between `1024` and the port immediately before the default
- **THEN** automatic allocation wraps to `1024` and persists the first usable port in that range

#### Scenario: No automatic port is available

- **WHEN** no port in the inclusive range `1024` through `65535` is usable for an exposed service
- **THEN** initialization fails and leaves the deployment uninitialized
- **AND** the error identifies the service for which allocation failed

#### Scenario: Explicit port mapping is preserved

- **WHEN** a user initializes or configures a local deployment with an explicit valid service-port mapping
- **THEN** the launcher persists that mapping unchanged

#### Scenario: Automatic port mapping is requested explicitly

- **WHEN** a user initializes a local deployment or reconfigures a stopped local deployment with `--ports auto`
- **THEN** the launcher selects and persists a concrete service-port mapping using the default-first wrapping order
- **AND** `exasol config get ports` reports the concrete mapping

#### Scenario: Stopped deployment port mapping is reset

- **WHEN** a user runs `exasol config reset ports` for a stopped local deployment
- **THEN** the launcher selects and persists a concrete service-port mapping using the default-first wrapping order
- **AND** the command reports the concrete mapping and guidance to run `exasol start`

#### Scenario: Automatic reconfiguration exhausts the port range

- **WHEN** a user requests automatic ports for a stopped local deployment
- **AND** no port in the inclusive range `1024` through `65535` is usable for an exposed service
- **THEN** the command fails and preserves the deployment's current port mapping
- **AND** the deployment remains stopped
- **AND** the error identifies the service for which allocation failed

#### Scenario: Multiple services receive distinct ports

- **WHEN** a local deployment exposes more than one service
- **THEN** automatic allocation starts from each service's own default port
- **AND** the launcher assigns distinct host ports to the services

### Requirement: Local deployments SHALL bind the persisted service ports exactly

A local deployment SHALL use each persisted concrete host port as its sole bind candidate. The runtime's bind result SHALL determine whether startup succeeds. When a configured port is unavailable, including when another process claims it after configuration, the command SHALL report the affected service and port and SHALL provide `exasol config set` commands for selecting an explicit or automatic replacement.

#### Scenario: Configured database port is published

- **WHEN** a local deployment starts on a supported platform with a concrete database port mapping
- **THEN** the database service is published on that exact host port
- **AND** deployment connection information reports that port

#### Scenario: Port is claimed before the runtime binds it

- **WHEN** a configured service port becomes unavailable before the local runtime binds it
- **THEN** the runtime start fails and keeps the persisted port mapping unchanged
- **AND** the error identifies the unavailable service and port in human-readable form
- **AND** the error shows `exasol config set --ports db:<available-port>` and `exasol config set --ports auto` as recovery commands
- **AND** the deployment remains in its prior initialized or stopped state so the configuration command is permitted

#### Scenario: Fixed port remains stable across restarts

- **WHEN** a local deployment with concrete service-port mappings is stopped and started
- **THEN** every service is exposed on the same persisted host port after restart

### Requirement: Legacy automatic local ports SHALL migrate to concrete mappings

The launcher SHALL replace a legacy local empty, `auto`, or zero-valued service-port mapping with a concrete deterministic mapping before starting a stopped deployment.

#### Scenario: Stopped legacy deployment is started

- **WHEN** a stopped local deployment contains an empty, `auto`, or zero-valued database port mapping
- **AND** the user runs `exasol start`
- **THEN** the launcher selects and persists the first usable port using the service's default-first wrapping order
- **AND** the database service is published on the resulting concrete port
