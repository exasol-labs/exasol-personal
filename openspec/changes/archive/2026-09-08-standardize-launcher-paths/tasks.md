Each numbered task below maps to exactly one commit. Every commit must pass the full
check suite (`task all`: unit tests, integration tests, lint, build) on its own, so
tasks are ordered and scoped so no intermediate commit leaves a location
half-migrated or a behavior claim untested.

## 1. Path Resolution Foundations

- [x] 1.1 Add one function per purpose (deployments root, history root, config file
      path, cache root) to `internal/launcherpaths`, implemented per platform per the
      design's path table, alongside the existing flat-root functions; cover each with
      unit tests. Purely additive: no existing caller changes yet.
- [x] 1.2 Add a startup migration marker type so callers can check and record that
      migration for a given location has completed, with unit tests.

## 2. Resumable Migration Primitive

- [x] 2.1 Add a staging/resume helper, generalized from
      `internal/localruntime/mac_data_migration.go`'s stage/mark-complete/rename/
      cleanup pattern, for migrating a directory or file whose source and destination
      may not share a filesystem, with unit tests covering an interrupted-then-
      retried migration. Flush the staged copy and the directory entries recording
      it before marking it complete, keep the published destination durable before
      removing the source, and preserve the source's permissions by teaching
      `internal/util`'s directory copy to restore them. Standalone: no caller yet.

## 3. Cache Configuration Relocation

- [x] 3.1 Move the runtime-artifact cache's `RetentionDays` config to `resources.yaml`
      at the new configuration location, read and written by
      `internal/runtimeartifacts` itself; on first encounter of an existing
      standalone `runtime-artifacts.yaml`, migrate its value over and remove the legacy
      file (a content transform, not a raw move, so this does not use the section-2
      primitive). Cover with unit tests, including the case where no legacy file
      exists.

## 4. Cache Root Relocation

- [x] 4.1 Point the runtime-artifact cache root at the new platform-specific cache
      location under the `resources` leaf name, and delete the legacy cache directory
      once the new location is in use (no prompt; contents are not migrated). Cover
      with unit tests.

## 5. SQL History Relocation

- [x] 5.1 Point `internal/connect/shell.go`'s history path at the new history
      location and migrate an existing legacy history file to it without data loss,
      using the primitive from section 2. Create the history directory when the
      interactive shell starts, so a fresh install persists history too. Cover with
      unit tests.

## 6. Deployments Relocation

- [x] 6.1 Point `internal/config/deployment_dir.go`'s deployments root at the new
      managed-deployments location; migrate each legacy deployment directory to it
      using the primitive from section 2; refuse (with a clear error naming both
      paths) any name that already exists at the new location while still migrating
      other, non-colliding deployments in the same run. Cover with unit tests,
      including the collision case. Update the integration tests that assert
      deployment locations, and point the launcher's platform configuration and
      cache directories at their temporary home so those tests stay hermetic.

## 7. Startup Wiring

- [x] 7.1 Run the migrations from sections 3-6 once at CLI startup, before
      deployment-dependent flag pre-registration and before any command records an
      operation in progress, without prompting for confirmation, exiting before
      command dispatch on failure, and writing the completion marker only after full
      success. Report what changed in a human-facing message on
      standard error before the requested command's own output. Cover the end-to-end
      sequence with tests: marker-gated no-repeat behavior across all locations, the
      reported message content, and quitting on failure.

## 8. Documentation

- [x] 8.1 Update command help text that names the legacy path. Already satisfied:
      help text builds its paths through `defaultDeploymentDirDisplayPath()` /
      `deploymentsRootDisplayPath()`, which already resolve through the updated
      `config` functions from section 6. Verified by running `exasol --help` with a
      fresh HOME. No code change; no commit for this item.
- [x] 8.2 Update `user-docs/content/manage-deployments.md` and any other user-facing
      docs naming `~/.exasol/personal`. Verified with `mkdocs build --strict`.
- [x] 8.3 Add a CHANGELOG entry under `Unreleased`.
