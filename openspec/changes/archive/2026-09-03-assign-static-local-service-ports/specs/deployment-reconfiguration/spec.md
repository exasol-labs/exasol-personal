## MODIFIED Requirements

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
