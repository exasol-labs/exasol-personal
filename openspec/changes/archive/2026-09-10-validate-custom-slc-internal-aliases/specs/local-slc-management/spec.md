## ADDED Requirements

### Requirement: Custom SLC internal aliases are unique

Before applying a custom SLC install or update, the launcher SHALL read the aliases declared by
the package and reject any alias that is already declared by another installed custom SLC. The
check SHALL be independent of the launcher-supplied external alias and package SHA. Existing
official/custom launcher-alias validation remains unchanged.

#### Scenario: Custom install with a conflicting internal alias is rejected

- **WHEN** a custom SLC is installed under a new launcher alias
- **AND** its package declares an alias already declared by an installed custom SLC
- **THEN** the command fails before staging or restarting
- **AND** the existing deployment state remains unchanged
- **AND** the error identifies the conflicting internal alias

#### Scenario: Custom install with unique internal aliases succeeds

- **WHEN** a custom SLC package declares no internal alias used by an installed SLC
- **THEN** the custom SLC is staged and installed normally

#### Scenario: Custom update may retain its own aliases

- **WHEN** an installed custom SLC is updated using the same launcher alias
- **AND** the replacement package declares an alias already owned by the custom SLC being replaced
- **THEN** the update is allowed to proceed
- **AND** aliases owned by other installed custom SLCs remain conflicts

#### Scenario: Internal alias comparison is case-insensitive

- **WHEN** a candidate package declares an internal alias differing only in letter case from an
  alias declared by another installed SLC
- **THEN** the candidate is rejected as a duplicate
