## ADDED Requirements

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
