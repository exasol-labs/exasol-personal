## Purpose

Defines where the interactive shell's SQL history persists and how an existing legacy history file is carried over to the new location.

## ADDED Requirements

### Requirement: SQL history SHALL persist under a platform-specific, launcher-owned location
The interactive shell's SQL history SHALL be stored under the launcher's own namespace rather than directly in the OS-provided cache directory.

#### Scenario: Platform-specific history location
- **WHEN** the interactive shell resolves the SQL history file path
- **THEN** it uses `~/.exasol/launcher/history/exasol_history` on Linux, `~/Library/Application Support/exasol/launcher/history/exasol_history` on macOS, and `%APPDATA%\exasol\launcher\history\exasol_history` on Windows

### Requirement: An existing legacy history file SHALL be migrated with its contents preserved
On CLI startup, before any command records an operation in progress, an existing history file at the legacy, unnamespaced location SHALL be moved to the new history location.

#### Scenario: Legacy history file is migrated
- **WHEN** a history file exists at the legacy location (directly in the OS cache directory) and no file exists yet at the new history location
- **THEN** the CLI moves it to the new history location before dispatching to any command
- **AND** its contents are preserved
