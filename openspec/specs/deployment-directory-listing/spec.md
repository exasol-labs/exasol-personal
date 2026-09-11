# deployment-directory-listing Specification

## Purpose
Define how `exasol deployments list` enumerates known deployment directories and reports their deployment status and preset identity, so users can discover and manage multiple named deployment directories.
## Requirements
### Requirement: CLI SHALL list known deployment directories
`exasol deployments list` SHALL enumerate the deployment directories under the managed-deployments root, including the default deployment directory and every named deployment directory. The root's location is specified by `deployment-directory-resolution`.

#### Scenario: Listing shows the default deployment directory
- **WHEN** a user runs `exasol deployments list`
- **AND** a `default` deployment directory exists under the managed-deployments root
- **THEN** the output includes an entry for `default` with its resolved path

#### Scenario: Listing shows named deployment directories
- **WHEN** a user runs `exasol deployments list`
- **AND** one or more named deployment directories exist under the managed-deployments root
- **THEN** the output includes one entry per named deployment directory, with its name and resolved path

#### Scenario: Listing is empty when no deployment directories exist
- **WHEN** a user runs `exasol deployments list`
- **AND** the managed-deployments root does not exist or contains no deployment directories
- **THEN** the command succeeds
- **AND** the output indicates that no deployment directories exist

#### Scenario: Non-directory entries are ignored
- **WHEN** a user runs `exasol deployments list`
- **AND** the managed-deployments root contains a non-directory entry (a file or symlink)
- **THEN** the output does not include an entry for it
- **AND** the command does not fail because of it

#### Scenario: Listing is sorted alphabetically by name
- **WHEN** a user runs `exasol deployments list`
- **AND** more than one deployment directory exists under the managed-deployments root
- **THEN** the entries appear in alphabetical order by name, regardless of filesystem iteration order

### Requirement: Listing SHALL report deployment status and preset identity
For each listed deployment directory, `exasol deployments list` SHALL report the same current status as `exasol status`, and when that status is not `not_initialized`, its infrastructure and installation preset identity. The command SHALL resolve deployment statuses concurrently under one five-second bound.

#### Scenario: Uninitialized deployment is reported without failing the listing
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory exists but has never been initialized
- **THEN** that entry reports status `not_initialized`
- **AND** the command still succeeds and lists the remaining entries

#### Scenario: An unreadable or corrupt deployment state is reported as not initialized
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory's state file exists but cannot be read or parsed
- **THEN** that entry reports status `not_initialized`
- **AND** the command still succeeds and lists the remaining entries

#### Scenario: Ready deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment's database is running and ready
- **THEN** that entry reports status `database_ready`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Unreachable deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment's persisted state is running but its database is unreachable
- **THEN** that entry reports status `database_connection_failed`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Stopped deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment has been deployed and subsequently stopped
- **THEN** that entry reports status `stopped`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Deployment with an in-progress, interrupted, or failed operation reports that status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory has a pending, interrupted, or failed deploy/destroy operation
- **THEN** that entry reports status `operation_in_progress`, `interrupted`, or `deployment_failed` respectively
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Multiple slow status checks share one bound
- **WHEN** multiple listed deployments do not complete their status checks within five seconds
- **THEN** the listing stops waiting for all of them after five seconds
- **AND** the command still succeeds and lists every deployment directory

#### Scenario: Listing a deployment directory does not modify it
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory's status is not `not_initialized` but its preset identity is not yet persisted in its state file
- **THEN** that entry reports a derived preset identity
- **AND** the deployment directory's state file is not modified as a result of running `exasol deployments list`

### Requirement: Listing SHALL support JSON output
`exasol deployments list` SHALL support a `--json` flag that emits the same information as structured JSON instead of human-readable text.

#### Scenario: JSON output includes all listed fields
- **WHEN** a user runs `exasol deployments list --json`
- **THEN** stdout is valid JSON
- **AND** each entry includes name, path, status, and preset identity when the status is not `not_initialized`
