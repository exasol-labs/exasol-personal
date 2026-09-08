## MODIFIED Requirements

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
