## 1. Preserve Preset Defaults

- [x] 1.1 Remove the backend `ConfigurationDefaults` contract and its initialization/reset merge plumbing; let backend configuration overlay only explicit user values on the extracted preset and resolve automatic local ports, preserving custom local ports and macOS sizing while retaining reset and Tofu behavior. Add focused initialization and reset tests for built-in defaults, custom preset values, and explicit override precedence; run `task all`. Commit as `fix: Preserve custom local preset defaults`.

## 2. Normalize Legacy Ports on Every Launch Path

- [x] 2.1 Move legacy local-port normalization into local runtime preparation and remove the start-only migrator interface; recognize empty, `auto`, zero-valued, and known-service-missing mappings while preserving positive mappings. Add focused tests for deploy and start from their permitted initial, failed, in-progress, and interrupted retry states, missing database mappings, allocation failure, and persistence before launch; run `task all`. Commit as `fix: Normalize legacy ports before local launch`.

## 3. Diagnose Actual Bind Failures Safely

- [x] 3.1 Make captured Podman and macOS VM-runner launch diagnostics the sole source of bind-conflict classification; remove post-failure availability probes and injection fields, remove the broad `failed to bind` marker, preserve direct-host and partial-start guards, and return a typed recoverable error only after required macOS cleanup succeeds. Add focused tests for each accepted marker, unrecognized and non-port failures, Podman stderr capture, VM-runner stderr capture, original error identity, successful cleanup, cleanup failure without recoverability, and lifecycle state outcomes; update the changelog and run `task all`. Commit as `fix: Diagnose local port conflicts at runtime bind`.

## 4. Present Recovery Guidance from Install

- [x] 4.1 Invoke the existing local-port recovery presentation when `exasol install` receives a wrapped recoverable deployment error. Add command tests proving install shows both replacement commands for port conflicts and shows no port guidance for unrelated deployment failures; run `task all`. Commit as `fix: Show local port recovery after install failure`.
