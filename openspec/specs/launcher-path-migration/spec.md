# launcher-path-migration Specification

## Purpose
Defines the mechanics shared by every legacy-to-new-location path migration: automatic execution, reporting what changed, quitting on failure, and safe retry after interruption.
## Requirements
### Requirement: Migration SHALL run automatically
Startup path migration SHALL apply its changes automatically as part of CLI startup, consistent with every other automatic migration already in the launcher.

#### Scenario: Migration applies changes automatically
- **WHEN** startup migration has changes to make
- **THEN** the CLI makes those changes immediately, the same way whether or not stdin is interactive

### Requirement: Migration SHALL report what it changed
Startup path migration SHALL emit a clear human-facing message identifying what it moved or deleted and where, before any output from the requested command.

#### Scenario: Migration reports its changes
- **WHEN** startup migration makes any change (moving a deployment, history file, or cache config; deleting the legacy cache)
- **THEN** the CLI emits a message on standard error naming what changed and its new location, before the requested command produces any output

#### Scenario: Migration stays quiet when there is nothing to do
- **WHEN** startup migration finds nothing to migrate
- **THEN** the CLI proceeds straight to the requested command's own output

### Requirement: The invocation SHALL quit before command dispatch on migration failure
When startup migration fails, the CLI SHALL exit with a clear error before dispatching to the requested command, rather than proceeding against a mixed legacy/new-location state.

#### Scenario: Failed migration blocks the requested command
- **WHEN** startup migration fails partway (an I/O error, or a refused naming collision)
- **THEN** the CLI exits with a non-zero status describing the failure before running the requested command

#### Scenario: Successful migration proceeds to the requested command
- **WHEN** startup migration completes successfully (including when there was nothing to migrate)
- **THEN** the CLI proceeds to dispatch the requested command normally

### Requirement: Migration SHALL run at most once per completed location
Once migration for a given location (managed deployments, SQL history, or cache configuration) has completed, later CLI invocations SHALL treat that location as already migrated.

#### Scenario: A completed location is skipped on later invocations
- **WHEN** startup migration has already completed for a given location
- **THEN** subsequent CLI invocations skip migrating that location again

### Requirement: An interrupted migration SHALL retry cleanly on the next invocation
Migration SHALL tolerate being interrupted at any point: the legacy source SHALL remain intact until the new location is confirmed fully and correctly written, and only a new-location copy confirmed fully and correctly written SHALL be treated as valid.

#### Scenario: Legacy source is preserved until the new location is confirmed complete
- **WHEN** migration is copying data to a new location
- **THEN** the legacy source remains present and unchanged until the new location is confirmed fully and correctly written

#### Scenario: Interrupted migration retries from scratch
- **WHEN** a previous migration attempt was interrupted before the new location was confirmed complete
- **THEN** the next invocation retries that migration from scratch
- **AND** the legacy source is still present and unmodified when the retry begins
