## ADDED Requirements

### Requirement: Linux host runtime supports Nano UDF execution under SELinux
The system SHALL start Nano with container-scoped security settings that allow UDF subprocesses to execute when the Linux host enforces SELinux, without requiring host-wide SELinux enforcement to be disabled.

#### Scenario: Execute a UDF with enforcing SELinux
- **WHEN** a local deployment runs on a Linux host with SELinux enforcing and invokes a configured script language container
- **THEN** the UDF subprocess starts and can complete its database operation

#### Scenario: Start Nano on a host without SELinux enforcement
- **WHEN** a local deployment runs on a host where SELinux labeling is unavailable or not enforced
- **THEN** Nano starts with the same container-scoped security configuration and database behavior remains available
