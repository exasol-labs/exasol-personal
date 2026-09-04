## MODIFIED Requirements

### Requirement: Local deployments SHALL bind the persisted service ports exactly

A local deployment SHALL use each persisted concrete host port as its sole bind candidate, and the actual runtime bind SHALL determine whether startup succeeds. When the runtime launch reports a recognized bind conflict for a configured service port and any required partial-start cleanup succeeds, the command SHALL report the affected service and port and SHALL provide `exasol config set` commands for selecting an explicit or automatic replacement. Other runtime launch failures SHALL retain their ordinary lifecycle failure handling.

#### Scenario: Configured database port is published

- **WHEN** a local deployment starts on a supported platform with a concrete database port mapping
- **THEN** the database service is published on that exact host port
- **AND** deployment connection information reports that port

#### Scenario: Port is claimed before the runtime binds it

- **WHEN** a user runs `exasol deploy`, `exasol start`, or `exasol install`
- **AND** the actual local runtime launch reports a recognized bind conflict for the configured database port
- **AND** any required partial-start cleanup succeeds
- **THEN** the command fails and keeps the persisted port mapping unchanged
- **AND** the error identifies the unavailable service and port in human-readable form
- **AND** the error shows `exasol config set --ports db:<available-port>` and `exasol config set --ports auto` as recovery commands
- **AND** the deployment remains in its prior initialized or stopped state so the configuration command is permitted

#### Scenario: Runtime launch diagnostic is not a recognized bind conflict

- **WHEN** the actual local runtime launch fails without a recognized bind-conflict diagnostic
- **THEN** the command reports the runtime launch failure through the ordinary lifecycle failure path

#### Scenario: Partial-start cleanup fails after a recognized bind conflict

- **WHEN** the actual local runtime launch reports a recognized bind conflict
- **AND** cleanup of a potentially partially started runtime fails
- **THEN** the command reports both the launch and cleanup failures through the ordinary lifecycle failure path
- **AND** the deployment records the applicable failed or interrupted workflow state for further recovery

#### Scenario: Fixed port remains stable across restarts

- **WHEN** a local deployment with concrete service-port mappings is stopped and started
- **THEN** every service is exposed on the same persisted host port after restart

### Requirement: Legacy automatic local ports SHALL migrate to concrete mappings

Before every permitted local deploy or start attempt that can launch the runtime, the launcher SHALL replace each legacy empty, `auto`, zero-valued, or known-service-missing port mapping with a concrete deterministic mapping. Existing positive mappings SHALL remain unchanged.

#### Scenario: Stopped legacy deployment is started

- **WHEN** a stopped local deployment contains an empty, `auto`, or zero-valued database port mapping
- **AND** the user runs `exasol start`
- **THEN** the launcher selects and persists the first usable port using the service's default-first wrapping order before launching the runtime
- **AND** the database service is published on the resulting concrete port

#### Scenario: Legacy deployment is deployed or retried

- **WHEN** a local deployment in any state that permits `exasol deploy` contains a legacy automatic database port mapping
- **AND** the user runs `exasol deploy` or retries with `exasol install`
- **THEN** the launcher selects and persists the first usable port using the service's default-first wrapping order before launching the runtime
- **AND** the deployment uses the resulting concrete port

#### Scenario: Legacy mapping omits the database service

- **WHEN** a permitted local deploy or start attempt reads a valid port mapping that omits the database service
- **THEN** the launcher selects and persists a concrete database port before launching the runtime
- **AND** existing positive mappings for other services remain unchanged
