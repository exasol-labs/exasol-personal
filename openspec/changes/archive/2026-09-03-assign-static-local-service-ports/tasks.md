Process notes (apply throughout):

- Each numbered checkbox below is exactly one commit with the stated Conventional Commit subject.
- Every commit is a complete, wired, independently verifiable increment and passes `task all` at its point in history, together with any focused checks named by the task.
- Cleanup, formatting, generated updates, and tests that belong to an increment stay in that increment's commit.
- OpenSpec artifacts remain uncommitted while the change is active. After all tasks pass, archive the change and commit the archived artifacts separately from implementation commits.

## 1. Backend Defaults and Deterministic Allocation

- [x] 1.1 Add backend-provided effective defaults for unset infrastructure variables and use them during initialization; for local deployments, allocate each automatic service port with the default-first circular scan, preserve explicit mappings, and persist canonical concrete mappings. Cover default, collision, wrap, exhaustion, multi-address, multi-service, explicit-value, and backend-contract behavior with focused unit and integration tests. Commit as `feat: Select deterministic static ports for local deployments`.

## 2. Stopped Local Reconfiguration

- [x] 2.1 Permit `config set` and `config reset` for every advertised option of a stopped local deployment, use current effective local defaults for resets, preserve the guards for running local and potentially deployed cloud deployments, and emit `exasol start` guidance after successful stopped changes. Cover configuration persistence, state guards, defaults, and guidance with focused unit and integration tests. Commit as `feat: Allow stopped local deployments to be reconfigured`.

## 3. Exact Runtime Port Binding

- [x] 3.1 Require positive concrete service ports in the shared Linux and Windows direct-host runtime and the macOS VM runtime, pass each configured port unchanged to the runtime, and require reported connection endpoints to match. Cover runtime arguments, endpoint reporting, mismatched VM state, and restart stability with focused unit tests. Commit as `feat: Bind configured local service ports exactly`.

## 4. Legacy Port Migration

- [x] 4.1 Before starting a stopped local deployment, replace legacy empty, `auto`, and zero-valued service mappings with concrete deterministic mappings while preserving positive explicit mappings. Cover migration persistence, runtime input, allocation failure, and repeated starts with focused unit and integration tests. Commit as `feat: Migrate automatic local service ports`.

## 5. Authoritative Bind-Conflict Errors

- [x] 5.1 Translate confirmed direct-host and VM bind failures into a shared unavailable-port error that preserves the command error, keep unrelated launch failures unchanged, restore the prior initialized or stopped workflow state when no endpoint was established, and report commands for selecting an explicit or automatic replacement port. Cover post-selection races, diagnostic and availability-based classification, unrelated failures, error chains, partial-start safety, state restoration, and call-to-action output with focused unit and integration tests. Commit as `fix: Report local port allocation races`.

## 6. Portable Lifecycle Coverage

- [x] 6.1 Extend the portable local deployment suite for deterministic automatic selection, stable stop/start ports, occupied-port failure, stopped reconfiguration, and recovery on a replacement port; update the test inventory and run the focused suite on each available supported runtime. Commit as `test: Cover static local port lifecycle`.

## 7. User Documentation

- [x] 7.1 Document automatic and explicit static ports, stopped reconfiguration, and conflict recovery in the README, and add the user-visible change under `CHANGELOG.md` `Unreleased`. Commit as `docs: Document static local service ports`.
