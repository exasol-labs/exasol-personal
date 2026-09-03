# deployment-reconfiguration Specification

## Purpose
TBD - created by archiving change support-deployment-reconfiguration. Update Purpose after archive.

## Requirements

### Requirement: Launcher SHALL persist deployment preset identity
The launcher SHALL persist the selected infrastructure preset identity and installation preset identity when a deployment directory is initialized.

#### Scenario: Built-in preset identity is persisted
- **WHEN** a user initializes a deployment directory with built-in presets
- **THEN** the launcher state records the selected infrastructure preset name
- **AND** the launcher state records the selected installation preset name

#### Scenario: Preset identity is available for later command decisions
- **WHEN** a user later runs `init`, `config`, or `install` in the initialized deployment directory
- **THEN** the command can compare the requested preset identity with the persisted preset identity
- **AND** the command does not infer same-preset behavior only from current command arguments

### Requirement: Init SHALL initialize preset extraction and patch same-preset supplied options

The `init` command SHALL initialize an empty deployment directory by extracting selected presets and writing launcher state. For an existing deployment with the same preset identity, `init` SHALL preserve local state and apply only supplied configuration options as a patch.

#### Scenario: Init initializes an empty deployment directory

- **WHEN** a user runs `exasol init <infra-preset> [install-preset]` for an empty deployment directory
- **THEN** the launcher extracts the selected presets into the deployment directory
- **AND** the launcher writes initialized deployment state with the selected preset identity
- **AND** the EULA notice is printed as a final terminal notice
- **AND** the EULA notice is not written to the deployment log file

#### Scenario: Init without options does not configure an existing same-preset deployment

- **WHEN** a user runs `exasol init <infra-preset> [install-preset]` for a deployment directory already initialized with the same preset identity
- **AND** the user did not supply configuration options
- **THEN** the command succeeds without removing local deployment files
- **AND** the command tells the user that configuration was not changed
- **AND** the command offers call-to-action guidance to use `exasol config set` for parameter changes

#### Scenario: Init patches supplied options for an existing same-preset deployment

- **WHEN** a user runs `exasol init <infra-preset> [install-preset] <configuration-options>` for a deployment directory already initialized with the same preset identity
- **THEN** the command succeeds without removing local deployment files
- **AND** the launcher updates only the supplied options in the corresponding local parameter files
- **AND** omitted options keep their current effective values
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Init refuses a different preset

- **WHEN** a user runs `exasol init <different-infra-preset>` for an initialized deployment directory
- **THEN** the command fails without modifying local deployment files
- **AND** the error tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if cloud resources are already gone

### Requirement: Config commands SHALL inspect, patch, and reset same-preset deployment parameters

The `config get`, `config set`, and `config reset` commands SHALL manage configuration parameter files for the initialized deployment's existing presets while preserving local deployment state, extracted presets, backend setup artifacts, OpenTofu state, credentials, and connection metadata. Configuration changes SHALL be permitted for initialized deployments and stopped local deployments.

#### Scenario: Config get prints active configuration

- **WHEN** a user runs `exasol config get` in an initialized deployment directory
- **THEN** the command prints the active effective configuration values on standard output
- **AND** the command supports `--json`

#### Scenario: Config get prints selected options

- **WHEN** a user runs `exasol config get <option-name> [<option-name>...]`
- **THEN** the command prints only the requested active configuration values on standard output
- **AND** the command fails when any requested option does not exist

#### Scenario: Config set patches parameters for initialized deployment

- **WHEN** a user runs `exasol config set <configuration-options>` in an initialized deployment directory
- **THEN** the launcher validates the options against the persisted preset manifests
- **AND** the command accepts preset-specific configuration options with the same `--option` flag style used by `init` and `install`
- **AND** the launcher updates only the supplied options in the corresponding local parameter files
- **AND** omitted options keep their current effective values
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration
- **AND** the user sees the apply guidance in command output while the deployment log remains unchanged by that guidance
- **AND** the launcher preserves infrastructure state files

#### Scenario: Config set patches parameters for stopped local deployment

- **WHEN** a user runs `exasol config set <configuration-options>` for a stopped local deployment
- **THEN** the launcher validates and updates the supplied local configuration options
- **AND** the launcher preserves deployment data and runtime state
- **AND** omitted options keep their current effective values
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol start` to apply the changed configuration

#### Scenario: Config reset restores selected defaults

- **WHEN** a user runs `exasol config reset <option-name> [<option-name>...]` for an initialized deployment
- **THEN** the launcher resets only the requested options to their preset defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Config reset restores stopped local defaults

- **WHEN** a user runs `exasol config reset <option-name> [<option-name>...]` for a stopped local deployment
- **THEN** the launcher resets only the requested options to their current effective local defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol start` to apply the changed configuration

#### Scenario: Config reset all restores all defaults explicitly

- **WHEN** a user runs `exasol config reset --all` for an initialized deployment
- **THEN** the launcher resets all configurable options to their preset defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Config reset all restores stopped local defaults

- **WHEN** a user runs `exasol config reset --all` for a stopped local deployment
- **THEN** the launcher resets all local configuration options to their current effective local defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol start` to apply the changed configuration

#### Scenario: Config set and reset refuse any state with possibly-deployed cloud resources

- **WHEN** a cloud deployment is in a state other than initialized
- **AND** a user runs `exasol config set <configuration-options>` or `exasol config reset <options>`
- **THEN** the command fails and leaves configuration unchanged
- **AND** the error tells the user that the deployment may already have cloud resources
- **AND** the error tells the user to run `exasol destroy` (or `exasol remove` if the cloud resources are confirmed gone) before changing configuration and redeploying

#### Scenario: Config set and reset refuse running local deployment

- **WHEN** a local deployment is running or has an operation in progress
- **AND** a user runs `exasol config set <configuration-options>` or `exasol config reset <options>`
- **THEN** the command fails and leaves configuration unchanged
- **AND** the error tells the user to stop the deployment before changing its configuration

#### Scenario: Config commands refuse uninitialized directories

- **WHEN** a user runs `exasol config get`, `exasol config set`, or `exasol config reset` for a deployment directory that is not initialized
- **THEN** the command fails with a message that the deployment directory must be initialized first

### Requirement: Install SHALL orchestrate initialization, configuration, and deployment
The `install` command SHALL combine initialization, configuration, and deployment in a state-aware way that preserves retry safety.

#### Scenario: Install initializes and deploys a new deployment
- **WHEN** a user runs `exasol install <infra-preset> [install-preset] <configuration-options>` for an empty deployment directory
- **THEN** the launcher initializes the deployment directory with the selected presets
- **AND** the launcher applies the supplied configuration options
- **AND** the launcher deploys the initialized deployment
- **AND** final connection instructions are printed after deployment log cleanup

#### Scenario: Install configures and retries same-preset deployment
- **WHEN** a user reruns `exasol install <infra-preset> [install-preset] <configuration-options>` for a deployment directory initialized with the same preset identity
- **THEN** the launcher applies the supplied configuration options as a patch without removing local deployment files
- **AND** omitted options keep their current effective values
- **AND** the launcher runs deployment using the preserved local infrastructure state
- **AND** final connection instructions are printed after deployment log cleanup

#### Scenario: Install refuses different preset
- **WHEN** a user runs `exasol install <different-infra-preset>` for an initialized deployment directory
- **THEN** the command fails before deployment
- **AND** the command does not remove local deployment files
- **AND** the message tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if cloud resources are already gone

### Requirement: Destroy remove SHALL remove the local deployment directory after successful destroy
The `destroy --remove` command SHALL destroy cloud resources and then remove the local deployment directory.

#### Scenario: Destroy remove removes local directory after successful destroy
- **WHEN** a user runs `exasol destroy --remove`
- **AND** cloud resource destruction succeeds
- **THEN** the local deployment directory is removed
- **AND** the same path can be initialized again with any preset

#### Scenario: Destroy remove confirmation names the local deployment directory
- **WHEN** a user runs `exasol destroy --remove` without `--auto-approve`
- **THEN** the confirmation prompt explicitly names the local deployment directory that will be removed after cloud destruction
- **AND** plain `exasol destroy` confirmation does not mention a local removal path

#### Scenario: Plain destroy preserves local files
- **WHEN** a user runs `exasol destroy` without `--remove`
- **THEN** cloud resources are destroyed
- **AND** local deployment files are preserved for same-preset redeployment or inspection

#### Scenario: Destroy remove keeps cleanup state when destroy fails
- **WHEN** a user runs `exasol destroy --remove`
- **AND** cloud resource destruction fails
- **THEN** local deployment files are preserved
- **AND** the workflow state records that destroy was interrupted instead of remaining in-progress
- **AND** the user can retry `exasol destroy`

#### Scenario: Status reports recovery guidance after stale destroy progress
- **WHEN** a previous destroy operation failed after marking the workflow in-progress
- **AND** no launcher process currently holds the deployment lock
- **THEN** `exasol status` explains that destroy did not finish cleanly
- **AND** the message tells the user to retry `exasol destroy` or run `exasol remove` when resources are already gone

### Requirement: Local remove SHALL support abandoned local deployment state
The CLI SHALL provide a command that removes the local deployment directory without attempting to destroy cloud resources.

#### Scenario: User removes abandoned local deployment state
- **WHEN** a user runs `exasol remove`
- **AND** the user confirms the operation or passes `--auto-approve`
- **THEN** the local deployment directory is removed
- **AND** the command does not attempt cloud resource destruction
- **AND** the command logs that local removal started and finished with the deployment directory path

#### Scenario: Local remove warns about cloud resources
- **WHEN** a user runs `exasol remove` without `--auto-approve`
- **THEN** the command warns that cloud resources are not destroyed
- **AND** the command asks for confirmation before removing local files
- **AND** the confirmation prompt explicitly names the local deployment directory that will be removed

#### Scenario: Local remove refuses non-deployment directories
- **WHEN** a user runs `exasol remove --deployment-dir <path>` for a directory that does not look like an Exasol Personal deployment directory
- **THEN** the command fails without removing files
- **AND** the error explains that the path is not an Exasol Personal deployment directory

#### Scenario: Local remove refuses when the current working directory is inside the deployment directory
- **WHEN** a user runs `exasol remove`
- **AND** the current working directory is the deployment directory or a path inside it
- **THEN** the command fails before removing any files
- **AND** the error explicitly names the current working directory and the deployment directory
- **AND** the error tells the user to change to another directory and rerun

#### Scenario: Local remove refuses when the launcher binary is inside the deployment directory
- **WHEN** a user runs `exasol remove`
- **AND** the running launcher binary resides inside the deployment directory
- **THEN** the command fails before removing any files
- **AND** the error explicitly names the launcher binary path and the deployment directory
- **AND** the error tells the user to move the launcher binary and rerun

#### Scenario: Local remove reports the exact failing path on deletion errors
- **WHEN** `exasol remove` cannot delete an entry inside the deployment directory because of a permission error, OS lock, or other I/O failure
- **THEN** the error message contains the absolute path of the entry that could not be removed
- **AND** the underlying error is preserved in the message

### Requirement: Config set SHALL report a clear error when configurable options cannot be loaded
The `config set` command SHALL resolve the target deployment's configurable infrastructure
options before parsing supplied option flags. When it cannot load those options because no
initialized deployment directory can be resolved, it SHALL fail with a clear, actionable error
that identifies the resolved deployment directory and directs the user to initialize it or to
supply `--deployment-dir`, rather than reporting supplied infrastructure options as unknown
flags.

#### Scenario: Config set names the unresolved deployment directory
- **WHEN** a user runs `exasol config set <infrastructure-options>` and no initialized
  deployment directory can be resolved (for example `--deployment-dir` is omitted and the
  current directory is not a deployment, or the target directory is not initialized)
- **THEN** the command fails with an error identifying the resolved deployment directory
- **AND** the error tells the user to initialize it with `exasol init` or `exasol install`, or
  to pass `--deployment-dir` pointing to an existing deployment
- **AND** the error does not report the supplied infrastructure options as unknown flags

#### Scenario: Config set help renders without a resolvable deployment
- **WHEN** a user runs `exasol config set --help` and the deployment's configurable options
  cannot be loaded
- **THEN** the command prints help without failing

#### Scenario: Config set still rejects genuinely unknown options
- **WHEN** a user runs `exasol config set` against an initialized deployment and supplies an
  option that the deployment's presets do not define
- **THEN** the command fails reporting that option as unknown

### Requirement: State-guarded commands SHALL surface recovery guidance when blocked by deployment state

State-guarded lifecycle commands (`install`/`deploy`, `connect`, `start`, `stop`) SHALL surface state-appropriate recovery guidance when the current deployment state does not permit the command, instead of a generic error. The message SHALL name the resolved deployment directory, report the current deployment state, and give the recovery command(s) for that state. The command SHALL NOT present a known, recoverable state as an "unexpected" error, and SHALL NOT require the user to run a separate command to learn how to recover. The recovery guidance SHALL be the same guidance reported by `exasol status` for that state.

#### Scenario: Deploy blocked by an interrupted non-deploy operation

- **WHEN** a user runs `exasol install` or `exasol deploy` while the deployment is
  interrupted during an operation other than deploy (for example destroy)
- **THEN** the command fails without deploying
- **AND** the message reports the current deployment state
- **AND** the message names the resolved deployment directory
- **AND** the message tells the user which operation was interrupted and the recovery
  command to run (for example, run `destroy`)
- **AND** the message does not label the state as "unexpected"

#### Scenario: Deploy blocked by a stopped deployment

- **WHEN** a user runs `exasol install` or `exasol deploy` for a stopped deployment
- **THEN** the command fails without deploying
- **AND** the message reports the stopped state and points the user toward `start` or
  `destroy`

#### Scenario: Connect blocked before the database is running

- **WHEN** a user runs `exasol connect` while the deployment is not running (for example
  initialized, stopped, or interrupted)
- **THEN** the command fails without connecting
- **AND** the message reports the current state, names the deployment directory, and gives
  the recovery command to reach a running database
- **AND** the message does not label the state as "unexpected"

#### Scenario: Start or stop blocked by an incompatible state

- **WHEN** a user runs `exasol start` or `exasol stop` while the deployment state does not
  permit that operation (for example starting a not-yet-deployed deployment, or stopping a
  stopped one)
- **THEN** the command fails without changing the deployment
- **AND** the message reports the current state, names the deployment directory, and gives
  the state-appropriate recovery command
- **AND** the message does not label the state as "unexpected"

#### Scenario: Permitted states still run

- **WHEN** a user runs a lifecycle command in a state that permits it (for example
  `deploy` when initialized, deployment failed, running, deploy in progress, or interrupted
  during deploy; `connect` when running)
- **THEN** the command proceeds as before
