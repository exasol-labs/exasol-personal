# linux-host-podman-install Specification

## Purpose

Defines the minimal persistent lifecycle for running an Exasol Nano database directly through Podman on a Linux host.

## Requirements

### Requirement: Linux host runtime starts a persistent Nano container
The system SHALL start an Exasol Nano container through Podman when a local deployment uses the Linux host runtime, and SHALL keep the database data in the deployment's local runtime directory.

#### Scenario: Start a fresh deployment
- **WHEN** the deployment data directory does not contain an initialized database
- **THEN** the system loads the configured Nano image, starts the deployment's named container, mounts the persistent data directory at `/exa`, and passes the initial database parameters

#### Scenario: Start an existing deployment
- **WHEN** the deployment data directory already contains an initialized database
- **THEN** the system starts the deployment's named container with the persistent data directory and does not pass first-start database parameters

#### Scenario: Start an already running deployment
- **WHEN** the deployment's named container is already running
- **THEN** the system returns success without loading the image or replacing the container

### Requirement: Linux host runtime publishes the configured database endpoint
The system SHALL publish the deployment's configured host database port to the Nano container's fixed database port.

#### Scenario: Use the default database port
- **WHEN** no database port override is configured
- **THEN** the system publishes host port `8563` to container port `8563`

#### Scenario: Use a database port override
- **WHEN** the deployment configures `db:<host-port>`
- **THEN** the system publishes `<host-port>` on the host to port `8563` in the container

#### Scenario: Reject an invalid database port mapping
- **WHEN** the configured database port mapping cannot be represented as a single host port and container port
- **THEN** the start operation fails before Podman is invoked

### Requirement: Linux host containers remain unlimited by VM sizing
The system SHALL NOT apply VM CPU, memory, or data-size settings as Podman container resource limits when using the Linux host runtime.

#### Scenario: VM sizing fields are present
- **WHEN** VM CPU, memory, or data-size values exist in shared configuration state and the Linux host runtime starts the container
- **THEN** the Podman invocation contains no limits derived from those values

### Requirement: Podman lifecycle failures are reported
The system SHALL stop the start sequence at the failing Podman operation and return an error that identifies the failed lifecycle step.

#### Scenario: Image loading fails
- **WHEN** Podman fails to load the Nano image
- **THEN** the start operation returns an image-loading error and does not attempt to run the image

#### Scenario: Loaded image cannot be identified
- **WHEN** Podman reports a successful load but its documented output does not identify the loaded image
- **THEN** the start operation fails before running a container

### Requirement: Linux host container cleanup is idempotent
The system SHALL tolerate cleanup when the deployment's Podman container is absent.

#### Scenario: Stop a deployment
- **WHEN** the deployment is stopped whether or not its named container exists
- **THEN** the disposable container is absent and the persistent data directory remains

#### Scenario: Destroy a deployment
- **WHEN** the deployment is destroyed whether or not its named container exists
- **THEN** the disposable container is absent and the deployment's local runtime data is removed

### Requirement: Linux host installation reports exact container status
The system SHALL report whether the deployment-specific Nano container is running without matching unrelated containers.

#### Scenario: Deployment container is running
- **WHEN** the exact deployment container exists and is running
- **THEN** installation status is running

#### Scenario: Deployment container is absent or stopped
- **WHEN** the exact deployment container is absent or is not running
- **THEN** installation status is stopped

### Requirement: Podman startup failures include best-effort diagnostics
The system SHALL emit Podman environment, container-list, deployment-container inspection, and deployment-container log diagnostics when image materialization, container recreation, or startup fails, without replacing the original failure.

#### Scenario: Startup fails and diagnostics partly fail
- **WHEN** startup fails and one or more diagnostic commands also fail
- **THEN** the original startup error is returned and all remaining diagnostics are attempted

### Requirement: Nano version checks are configured portably
The system SHALL always tell Nano whether version checks are enabled and, when enabled, SHALL pass the configured service URL, identity, Linux operating-system identity, bounded interval, and bounded retry interval.

#### Scenario: Version checks are disabled
- **WHEN** version checking is disabled
- **THEN** Nano receives the disabled setting and no enabled-only settings

#### Scenario: Version-check interval is absent or outside supported limits
- **WHEN** version checking is enabled with an absent or out-of-range interval
- **THEN** Nano receives the default or nearest supported interval and a retry interval no greater than one day

### Requirement: Configured SLCs are materialized and mounted
The system SHALL reuse available SLC images, pull missing official images, import available custom packages as managed images, mount every available configured image, and atomically publish an availability report.

#### Scenario: Missing official image is available remotely
- **WHEN** a configured official image is not present locally and can be pulled
- **THEN** it is pulled and mounted into Nano at its configured target

#### Scenario: Official image cannot be pulled
- **WHEN** a configured official image is missing and its pull fails
- **THEN** database startup fails with the pull error

#### Scenario: Custom package is missing or invalid
- **WHEN** a configured custom package is missing or cannot be imported
- **THEN** it is reported unavailable and skipped while the database still starts

#### Scenario: Availability report is replaced
- **WHEN** configured SLC materialization completes
- **THEN** the complete new report atomically replaces the previous report without exposing a partial report

### Requirement: Linux host runtime supports Nano UDF execution under SELinux
The system SHALL start Nano with container-scoped security settings that allow UDF subprocesses to execute when the Linux host enforces SELinux, without requiring host-wide SELinux enforcement to be disabled.

#### Scenario: Execute a UDF with enforcing SELinux
- **WHEN** a local deployment runs on a Linux host with SELinux enforcing and invokes a configured script language container
- **THEN** the UDF subprocess starts and can complete its database operation

#### Scenario: Start Nano on a host without SELinux enforcement
- **WHEN** a local deployment runs on a host where SELinux labeling is unavailable or not enforced
- **THEN** Nano starts with the same container-scoped security configuration and database behavior remains available

### Requirement: Unreferenced managed SLC images are pruned safely
The system SHALL prune only unreferenced tagged images from the exact official SLC repository and labeled custom imports, and SHALL treat equivalent registry prefixes and implicit `latest` tags as the same reference.

#### Scenario: Caller is unaware of SLC state
- **WHEN** the startup configuration contains a nil SLC list
- **THEN** no SLC images are pruned

#### Scenario: Caller declares an authoritative empty SLC set
- **WHEN** the startup configuration contains a non-nil empty SLC list
- **THEN** all otherwise-prunable managed SLC images are considered unreferenced

#### Scenario: Image is unrelated or untagged
- **WHEN** a local image belongs to another repository, lacks the custom-import label, or is untagged
- **THEN** it is not pruned

#### Scenario: Pruning fails
- **WHEN** an eligible image cannot be removed
- **THEN** startup continues and reports the cleanup failure as a warning

### Requirement: Interrupted initial Nano creation is recovered
The system SHALL detect an interrupted initial-create marker, remove the disposable container, quarantine the partial persistent data, recreate an empty data directory, and retry initial creation without destroying the quarantined data.

#### Scenario: Initial creation was interrupted
- **WHEN** the initial-create marker exists at startup
- **THEN** the partial data directory is moved to a timestamped sibling and startup proceeds with an empty data directory

#### Scenario: Stale TLS files exist without database configuration
- **WHEN** no database configuration exists but stale Nano TLS key or certificate files exist
- **THEN** only those stale TLS files are removed before initial creation

#### Scenario: Existing database has TLS files
- **WHEN** database configuration exists
- **THEN** existing TLS files are preserved and first-start parameters are not passed again

### Requirement: Legacy overlay-backed Nano data is migrated safely
The system SHALL migrate a legacy deployment container whose `/exa` is not persistently mounted into deployment-owned storage without removing the source container until a complete staged copy can be atomically installed.

#### Scenario: Persistent destination is populated
- **WHEN** legacy overlay data is detected and the persistent destination contains data
- **THEN** migration is refused without overwriting the destination or removing the legacy container

#### Scenario: Legacy copy fails
- **WHEN** copying `/exa` from the stopped legacy container fails
- **THEN** the legacy container and any recoverable staged state are retained and the error explains how to recover

#### Scenario: Legacy copy succeeds
- **WHEN** the legacy `/exa` copy completes into a sibling staging directory and can be atomically installed
- **THEN** the persistent destination becomes active before the old container is removed

### Requirement: Podman image loading uses runtime artifact paths
The system SHALL load a Nano image using `podman load -i` with the runtime path supplied for the materialized image and SHALL run the exact image reference reported by the load without creating a deployment-specific image tag.

#### Scenario: Linux host loads Nano image
- **WHEN** the Linux runtime materializes the embedded Nano image
- **THEN** the runtime path equals the host path and Podman reads that path through `-i`

#### Scenario: VM-backed runtime loads Nano image
- **WHEN** a VM-backed runtime materializes the embedded Nano image in its shared directory
- **THEN** Podman reads the mapped guest path through `-i` without installation code selecting a transport

#### Scenario: Loaded image is identified by name
- **WHEN** Podman reports the loaded Nano image as a named image reference
- **THEN** the system runs that exact reference without creating another image tag

#### Scenario: Loaded image is identified by ID
- **WHEN** Podman reports the loaded Nano image as an image ID
- **THEN** the system runs that exact ID without creating an image tag

### Requirement: Podman installation uses its execution environment
The system SHALL perform command execution and installation-owned filesystem operations through the selected runtime execution environment.

#### Scenario: Linux host installation executes
- **WHEN** the selected environment is the Linux host
- **THEN** commands and filesystem operations execute directly with existing Linux behavior

#### Scenario: VM-backed installation executes
- **WHEN** the selected environment is the macOS VM
- **THEN** commands and guest filesystem operations execute through the VM runner

### Requirement: custom SLC imports use runtime paths
The system SHALL pass runtime-visible paths rather than host-only paths when importing custom SLC packages.

#### Scenario: Custom package is staged for a VM
- **WHEN** a custom SLC package must be imported by guest Podman
- **THEN** the importer receives the mapped guest path while host-side staging retains the corresponding host path
