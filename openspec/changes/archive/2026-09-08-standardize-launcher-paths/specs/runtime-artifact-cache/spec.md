## MODIFIED Requirements

### Requirement: Cache cleanup SHALL use configured retention
The launcher SHALL use per-user cache configuration to determine when cached runtime artifacts are old enough to be cleaned. This configuration SHALL be read from a `resources.yaml` file at the platform's conventional configuration location.

#### Scenario: Configured retention identifies stale artifacts
- **WHEN** cache cleanup evaluates cached runtime artifacts
- **THEN** artifacts whose last-use timestamp is older than the configured retention are treated as stale
- **AND** artifacts whose last-use timestamp is within the configured retention are preserved

#### Scenario: Missing retention configuration uses a default
- **WHEN** cache cleanup runs without an existing user retention configuration
- **THEN** the launcher uses a default retention value

## ADDED Requirements

### Requirement: The resource cache SHALL live under a platform-specific cache root
The cache root SHALL be a platform-conventional, launcher-owned cache directory, using the `resources` leaf name.

#### Scenario: Platform-specific cache root
- **WHEN** the launcher resolves the resource cache root
- **THEN** it uses `${XDG_CACHE_HOME:-~/.cache}/exasol/launcher/resources` on Linux, `~/Library/Caches/exasol/launcher/resources` on macOS, and `%LOCALAPPDATA%\exasol\launcher\resources` on Windows

### Requirement: The cache configuration file SHALL live at a platform-specific configuration location
The resource cache's configuration SHALL be one `resources.yaml` file per user, located at the platform's conventional configuration directory.

#### Scenario: Platform-specific cache config file location
- **WHEN** the launcher resolves the cache configuration file path
- **THEN** it uses `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` on Linux, `~/Library/Application Support/exasol/launcher/resources.yaml` on macOS, and `%APPDATA%\exasol\launcher\resources.yaml` on Windows

### Requirement: An existing legacy cache config file SHALL be migrated to resources.yaml and then removed
On CLI startup, before any command records an operation in progress, an existing `runtime-artifacts.yaml` cache config file SHALL have its retention setting written to `resources.yaml`, and SHALL then be removed.

#### Scenario: Legacy cache config is migrated once
- **WHEN** a `runtime-artifacts.yaml` file exists at its legacy location
- **THEN** its `retention_days` value is written to the `retention_days` field of `resources.yaml`
- **AND** the legacy `runtime-artifacts.yaml` file is removed

### Requirement: The legacy resource cache directory SHALL be deleted once the new cache location is in use
The legacy resource cache directory SHALL be deleted outright; the new cache starts empty and repopulates on demand.

#### Scenario: Legacy cache is deleted
- **WHEN** CLI startup migration runs and a legacy resource cache directory exists
- **THEN** the legacy directory is deleted
- **AND** the new cache location contains only entries created after migration
