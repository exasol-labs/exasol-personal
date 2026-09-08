## MODIFIED Requirements

### Requirement: CLI SHALL resolve an active deployment directory
Commands that operate on a deployment directory SHALL resolve exactly one active deployment directory before command-specific execution.

#### Scenario: Explicit deployment directory wins
- **WHEN** a user invokes a deployment command with `--deployment-dir <path>`
- **THEN** the CLI uses `<path>` as the active deployment directory
- **AND** the CLI does not use `--deployment`, the current working directory, or the default deployment directory for that command

#### Scenario: Explicit named deployment directory wins
- **WHEN** a user invokes a deployment command with `--deployment <name>` and without `--deployment-dir`
- **THEN** the CLI uses `<name>` under the managed-deployments root as the active deployment directory
- **AND** the CLI does not use the current working directory or the default deployment directory for that command, even if the current working directory is a recognized deployment directory

#### Scenario: Recognized current deployment directory wins when no flag is provided
- **WHEN** a user invokes a deployment command without `--deployment-dir` and without `--deployment`
- **AND** the current working directory contains launcher-owned deployment state or marker files
- **THEN** the CLI uses the current working directory as the active deployment directory

#### Scenario: Default deployment directory is used when no stronger source exists
- **WHEN** a user invokes a deployment command without `--deployment-dir` and without `--deployment`
- **AND** the current working directory is not recognized as a deployment directory
- **THEN** the CLI uses the `default` directory under the managed-deployments root as the active deployment directory

### Requirement: CLI SHALL reject ambiguous or invalid deployment directory selection
The CLI SHALL reject invocations that cannot resolve to exactly one unambiguous deployment directory selection.

#### Scenario: --deployment-dir and --deployment are mutually exclusive
- **WHEN** a user invokes a deployment command with both `--deployment-dir <path>` and `--deployment <name>`
- **THEN** the CLI fails with a usage error
- **AND** the error identifies both flags as mutually exclusive
- **AND** the failure occurs before any deployment-directory side effect, including resolution, compatibility enforcement, deployment file logging, and version-update checks

#### Scenario: Named deployment directory rejects unsafe characters
- **WHEN** a user invokes a deployment command with `--deployment <name>` where `<name>` contains any character other than letters, digits, `-`, or `_`
- **THEN** the CLI fails with a usage error before command-specific execution
- **AND** the error explains which characters are allowed

#### Scenario: The literal name "default" is accepted
- **WHEN** a user invokes a deployment command with `--deployment default`
- **THEN** the CLI uses the `default` directory under the managed-deployments root as the active deployment directory
- **AND** the CLI does not treat this as an error or as a distinct directory from the implicit default

### Requirement: CLI SHALL keep the resolved deployment directory visible
Commands that resolve to the default deployment directory or to a named deployment directory SHALL emit a clear human-facing message containing the resolved path, on standard error.

#### Scenario: Default directory message is shown
- **WHEN** a deployment command resolves to the default deployment directory
- **THEN** the user sees a message on standard error containing the resolved default deployment directory path

#### Scenario: Named directory message is shown
- **WHEN** a deployment command resolves to a named deployment directory via `--deployment <name>`
- **THEN** the user sees a message on standard error containing `<name>` and the resolved path

#### Scenario: JSON payload output remains parseable
- **WHEN** a deployment command produces JSON output and resolves to the default deployment directory or a named deployment directory
- **THEN** the command's stdout remains valid JSON
- **AND** the human-facing resolved-directory message is emitted on standard error, not stdout

### Requirement: Init and install SHALL create the default deployment directory when needed
Commands that initialize a deployment SHALL automatically create the default deployment directory when resolution selects it and it does not already exist. Commands that initialize a deployment SHALL likewise automatically create a named deployment directory when resolution selects it via `--deployment` and it does not already exist.

#### Scenario: Install creates the default deployment directory
- **WHEN** a user runs `exasol install <preset>` outside a recognized deployment directory without `--deployment-dir` and without `--deployment`
- **THEN** the CLI creates the `default` directory under the managed-deployments root
- **AND** the install proceeds using that directory

#### Scenario: Init creates the default deployment directory
- **WHEN** a user runs `exasol init <preset>` outside a recognized deployment directory without `--deployment-dir` and without `--deployment`
- **THEN** the CLI creates the `default` directory under the managed-deployments root
- **AND** initialization proceeds using that directory

#### Scenario: Install creates a named deployment directory
- **WHEN** a user runs `exasol install <preset> --deployment <name>` and `<name>` does not already exist under the managed-deployments root
- **THEN** the CLI creates `<name>` under the managed-deployments root
- **AND** the install proceeds using that directory

### Requirement: Commands that require initialized state SHALL fail clearly for uninitialized directories
Deployment commands that require an initialized deployment SHALL fail when the resolved active deployment directory is not initialized, and the error SHALL include the resolved path.

#### Scenario: Required initialized command uses default directory but it is uninitialized
- **WHEN** a user runs a command that requires initialized deployment state outside a recognized deployment directory without `--deployment-dir` and without `--deployment`
- **AND** the default deployment directory is not initialized
- **THEN** the command fails with a message that says the deployment directory is not initialized
- **AND** the message includes the resolved default deployment directory path

#### Scenario: Required initialized command uses explicit directory but it is uninitialized
- **WHEN** a user runs a command that requires initialized deployment state with `--deployment-dir <path>`
- **AND** `<path>` is not initialized
- **THEN** the command fails with a message that says the deployment directory is not initialized
- **AND** the message includes `<path>`

#### Scenario: Required initialized command uses named directory but it is uninitialized
- **WHEN** a user runs a command that requires initialized deployment state with `--deployment <name>`
- **AND** `<name>` under the managed-deployments root is not initialized
- **THEN** the command fails with a message that says the deployment directory is not initialized
- **AND** the message includes the resolved named deployment directory path

### Requirement: Status SHALL report the active deployment directory
The `status` command SHALL include the active deployment directory path in both text and JSON output.

#### Scenario: Status reports explicit deployment directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **THEN** the status output includes `<path>` as the active deployment directory

#### Scenario: Status reports named deployment directory
- **WHEN** a user runs `exasol status --deployment <name>`
- **THEN** the status output includes the resolved `<name>` path under the managed-deployments root as the active deployment directory

#### Scenario: Status reports current deployment directory
- **WHEN** a user runs `exasol status` from a recognized deployment directory
- **THEN** the status output includes the current working directory as the active deployment directory

#### Scenario: Status reports default deployment directory
- **WHEN** a user runs `exasol status` outside a recognized deployment directory without `--deployment-dir` and without `--deployment`
- **THEN** the status output includes the `default` path under the managed-deployments root as the active deployment directory

### Requirement: Status SHALL report uninitialized resolved directories
The `status` command SHALL report `not_initialized` for an uninitialized resolved deployment directory instead of failing only because initialization has not happened.

#### Scenario: Status reports uninitialized default directory
- **WHEN** a user runs `exasol status` outside a recognized deployment directory without `--deployment-dir` and without `--deployment`
- **AND** the default deployment directory is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes the resolved default deployment directory path

#### Scenario: Status reports uninitialized explicit directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **AND** `<path>` is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes `<path>` as the active deployment directory

#### Scenario: Status reports uninitialized named directory
- **WHEN** a user runs `exasol status --deployment <name>`
- **AND** `<name>` under the managed-deployments root is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes the resolved named deployment directory path

### Requirement: Documentation SHALL explain deployment directory behavior
User-facing help and documentation SHALL explain the default deployment directory, named deployment directories, precedence rules, override behavior, and cloud cleanup implications.

#### Scenario: Help describes how to override the active deployment directory
- **WHEN** a user reads command help for a deployment command
- **THEN** the help explains that `--deployment-dir` overrides automatic deployment directory resolution
- **AND** the help explains that `--deployment <name>` (or its shorthand `-d <name>`) selects `<name>` under the managed-deployments root as an alternative to `--deployment-dir`
- **AND** the help explains that `--deployment-dir` and `--deployment` cannot be used together

#### Scenario: Documentation warns about preserving deployment directories for cloud cleanup
- **WHEN** a user reads deployment documentation
- **THEN** the documentation explains that cloud deployment directories should be kept until cloud resources are destroyed

### Requirement: Launcher-managed deployment directory reuse SHALL avoid stale preset deployment
When commands resolve to the default deployment directory or to a named deployment directory, they SHALL validate requested preset identity against initialized local state before deploying resources.

#### Scenario: Same default directory install retry uses same preset state
- **WHEN** a user reruns `exasol install <infra-preset>` outside a recognized deployment directory
- **AND** the default deployment directory is initialized with the same preset identity
- **THEN** the command reuses the default deployment directory
- **AND** the command does not remove local deployment files before deployment retry

#### Scenario: Different default directory preset is refused
- **WHEN** a user runs `exasol install <different-infra-preset>` outside a recognized deployment directory
- **AND** the default deployment directory is initialized with another preset identity
- **THEN** the command fails before deployment
- **AND** the command does not deploy the stale preset from the default deployment directory
- **AND** the command tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if the cloud resources are already gone

#### Scenario: Different named directory preset is refused
- **WHEN** a user runs `exasol install <different-infra-preset> --deployment <name>`
- **AND** `<name>` under the managed-deployments root is initialized with another preset identity
- **THEN** the command fails before deployment
- **AND** the command does not deploy the stale preset from the named deployment directory
- **AND** the command tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if the cloud resources are already gone

## ADDED Requirements

### Requirement: Managed deployments SHALL resolve under a platform-specific root
The managed-deployments root SHALL be a platform-conventional, launcher-owned directory distinct from the launcher's configuration and cache locations, rather than one flat path shared across platforms.

#### Scenario: Platform-specific managed-deployments root
- **WHEN** the CLI resolves the managed-deployments root
- **THEN** it uses `~/.exasol/launcher/deployments` on Linux, `~/Library/Application Support/exasol/launcher/deployments` on macOS, and `%APPDATA%\exasol\launcher\deployments` on Windows

### Requirement: Existing deployments at the legacy location SHALL be migrated
On CLI startup, before any command records an operation in progress, a deployment directory found only at the legacy managed-deployments root SHALL be moved to the new managed-deployments root.

#### Scenario: Legacy deployment is migrated
- **WHEN** a deployment directory named `<name>` exists at the legacy managed-deployments root and no directory named `<name>` exists at the new managed-deployments root
- **THEN** the CLI moves it to `<name>` under the new managed-deployments root before dispatching to any command
- **AND** the deployment remains fully usable at its new location afterward

### Requirement: A naming collision between legacy and new-location deployments SHALL surface as an explicit error
When a deployment directory named `<name>` exists at both the legacy and the new managed-deployments root, the CLI SHALL report the collision as an error identifying both paths and leave both directories exactly as found.

#### Scenario: Colliding name is reported with both paths named
- **WHEN** a deployment directory named `<name>` exists at both the legacy and the new managed-deployments root
- **THEN** the CLI fails startup migration for `<name>` with an error naming both paths
- **AND** both directories remain exactly as they were
- **AND** other, non-colliding legacy deployment directories still migrate in the same run
