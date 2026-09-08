## Context

`internal/launcherpaths` currently resolves one flat root, `<home>/.exasol/personal`,
identically on every platform. Everything launcher-owned nests under it or, in two
cases, doesn't nest under anything launcher-owned at all:

- Managed deployments (`internal/config/deployment_dir.go`) and the runtime-artifact
  cache (`internal/runtimeartifacts/cache.go`) both sit under the flat root.
- The runtime-artifact cache's own config file, `runtime-artifacts.yaml`, is written
  directly into the flat root, i.e. into the deployments root, not the cache it
  configures.
- SQL history (`internal/connect/shell.go`) is written as `exasol_history` directly
  into `os.UserCacheDir()`, with no launcher namespace at all.

This is also the change that puts launcher configuration in the platform's
configuration location for the first time. Nothing in the codebase writes to that
location today.

## Goals / Non-Goals

**Goals:**

- Every launcher-owned path lives under one `exasol/launcher` namespace per platform,
  in the platform-conventional location for its lifecycle (durable data, config, or
  cache).
- Existing users keep their deployments, SQL history, and cache retention setting
  across the upgrade, with no data loss and no silent overwrite on a naming collision.
- The disposable resource cache is treated as disposable: it is deleted, not migrated.
- Each subsystem's settings live in their own file at the configuration location, so a
  later subsystem adds its own file without touching the resource cache's.

**Non-Goals:**

- The broader SPOT-32441 resource-resolution pipeline redesign (unifying HTTP/Git/
  local/container/embedded sources, preset-scoped resources). This change only moves
  and renames the resource cache's on-disk location and config file.
- Renaming the `internal/runtimeartifacts` Go package, its types, or its terminology.
- Moving deployment-specific configuration out of its deployment directory.
- Preserving the legacy resource cache's contents.

## Decisions

1. **Platform-specific roots, resolved per purpose, not per flat root.**

   `internal/launcherpaths` currently exposes a single `RootDirPath`/`DirPath` pair.
   This change replaces it with one function per purpose (deployments root, history
   root, config file path, cache root), because the four purposes don't share one base
   directory consistently across platforms:

   | Purpose | Linux | macOS | Windows |
   | --- | --- | --- | --- |
   | Managed deployments | `~/.exasol/launcher/deployments` | `~/Library/Application Support/exasol/launcher/deployments` | `%APPDATA%\exasol\launcher\deployments` |
   | SQL history | `~/.exasol/launcher/history/exasol_history` | `~/Library/Application Support/exasol/launcher/history/exasol_history` | `%APPDATA%\exasol\launcher\history\exasol_history` |
   | Resource cache configuration | `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` | `~/Library/Application Support/exasol/launcher/resources.yaml` | `%APPDATA%\exasol\launcher\resources.yaml` |
   | Resource cache | `${XDG_CACHE_HOME:-~/.cache}/exasol/launcher/resources` | `~/Library/Caches/exasol/launcher/resources` | `%LOCALAPPDATA%\exasol\launcher\resources` |

   On Linux, deployments and history deliberately do *not* follow `os.UserConfigDir()`
   or an XDG data-home convention: they keep a stable, discoverable dotfile location
   analogous to the current `~/.exasol/personal`, distinct from configuration. On
   macOS and Windows, `os.UserConfigDir()` already resolves to the platform's
   general-purpose per-app directory (`Application Support`, `%APPDATA%`), so
   deployments, history, and configuration collapse onto the same base there and are
   only distinguished by their leaf name. The cache purpose always uses
   `os.UserCacheDir()` (or its Linux XDG equivalent), on every platform.

2. **Migrate deployments and history; delete, don't migrate, the cache.**

   Deployments and history are user-managed or user-generated data, so losing them is
   a real regression. The resource cache is rebuilt transparently on next use, so this
   change deletes the legacy cache directory outright instead of copying it, matching
   the existing precedent of treating this cache as fully disposable (see
   `runtime-artifact-cache`).

3. **Collision handling refuses to guess.**

   If a deployment name exists at both the legacy and new managed-deployments root,
   silently preferring either one risks discarding the other's state. Migration
   refuses that name with an error naming both paths and asking the user to resolve it
   (rename or remove one side) before retrying. Every other, non-colliding legacy
   deployment still migrates normally in the same run.

4. **Migration runs once, at startup, before any operation is recorded in progress,
   without prompting, and the invocation quits rather than proceeds on failure.**

   Migrating a deployment directory while a command may be mid-operation (e.g.
   destroying cloud resources) risks a race between the migration and the command's
   own use of that directory. Startup migration runs before `cmd/exasol` dispatches to
   any command, so a command never observes a half-migrated deployment tree. A marker
   in the new root records that migration already ran, so it is not repeated on every
   invocation.

   Migration does not prompt for confirmation, matching every other automatic
   migration already in this codebase (`internal/localruntime/mac_data_migration.go`
   migrates a VM's Nano data with no user prompt). It does, however, tell the user
   what happened: a clear human-facing message on standard error naming what moved or
   was deleted and where, emitted before the requested command's own output.

   If migration fails partway (I/O error, a refused collision), the CLI exits with a
   clear error before dispatching to the requested command. It does not fall back to
   the legacy paths, and it does not proceed with a mixed old/new state. The marker is
   only written after migration fully completes, so the next invocation retries from
   the same state rather than resuming a partially applied one silently.

5. **Migration is resumable, not merely "before any command runs."**

   A rename within the same filesystem is already atomic, but the deployments and
   history locations are not guaranteed to share a filesystem with their
   legacy counterparts (e.g. `~/.exasol/launcher` vs. a separately mounted
   `XDG_CACHE_HOME`), so a cross-filesystem move needs copy-then-delete, which is not
   atomic and can be interrupted mid-copy. Rather than accept that as a risk to be
   waved off, this change reuses the stage → mark-complete → atomic-rename → cleanup-
   source pattern already implemented for macOS Nano data in
   `internal/localruntime/mac_data_migration.go`: copy into a staging directory next
   to the destination, write a completion marker once the copy is flushed, atomically
   rename staging to the final destination, and only then remove the legacy source.
   An interrupted run leaves the legacy source untouched and an incomplete staging
   directory that the next invocation resets and retries, never a half-populated
   destination mistaken for a complete one. Where source and destination share a
   filesystem, a plain rename is used directly and this staging dance is skipped.

6. **One settings file per subsystem, each owned by the subsystem itself.**

   `resources.yaml` holds only the resource cache's settings, so
   `internal/runtimeartifacts` reads and writes it directly, depending on
   `internal/launcherpaths` for path resolution alone. This keeps the ownership
   boundary the package already needs: `internal/runtimeartifacts` must not depend on
   `internal/config` (the `launcherpaths` package doc calls this out explicitly, to
   avoid an import cycle risk if `internal/presets` ever depends on
   `internal/runtimeartifacts`), and a file read only by its owner never has to. A
   later subsystem that needs persisted settings adds its own file at the same
   configuration location.

7. **The cache config file keeps its own identity.**

   `runtime-artifacts.yaml`'s only field, `retention_days`, stays as it is in
   `resources.yaml` at the configuration location, its name matching the `resources`
   cache directory leaf. Launcher settings files use YAML, as the rest of the
   repository does. On first run after upgrade, if the legacy file exists, its value
   seeds the new file; the legacy file is then removed as part of the same migration
   that relocates deployments and history. The value is read and rewritten rather than
   moved, so the file is validated on the way through and the staging primitive from
   Decision 5 is not involved.

## Risks / Trade-offs

- Running migration at every startup (gated by a marker check) adds a small amount of
  work to every invocation even after migration is complete; the check is a single
  marker-file stat, kept cheap deliberately.
- Startup migration assumes one launcher process runs it at a time, which holds for
  interactive use. Several first-run invocations started together would each see the
  marker missing and migrate the same legacy paths, so the losing invocation reports a
  collision and stops instead of running its command. Serializing them is left out
  deliberately: the marker keeps the window to the first run on a machine.
- Collision refusal means a pre-existing name clash blocks that one deployment from
  migrating until the user acts, rather than the launcher picking a side. This is
  intentional (see Decision 3) but is a hard stop, not a warning.
- Migrating without a confirmation prompt means a user's deployments, history file,
  and cache config move (and the legacy cache is deleted) without them opting in at
  that exact moment. This matches existing precedent in this codebase (no other
  migration prompts either) and is mitigated, not eliminated, by the informational
  message: a user who doesn't read CLI output can still be surprised that legacy
  paths are gone.
- Quitting the invocation on a failed migration means every command is unusable until
  the underlying failure is resolved: there is no "skip migration and proceed with
  legacy paths for now" fallback. This is intentional (mixed old/new state is worse
  than a blocked invocation).
- The stage/mark/rename/cleanup sequence adds real implementation surface (per
  migrated item: a staging directory, a completion marker, resume-or-reset logic) for
  a case (cross-filesystem legacy vs. new roots) that will be uncommon in practice;
  it is justified here because the alternative (accepting non-atomic copy-then-delete
  as a known gap) is exactly what this design decision exists to avoid.

## Migration Plan

Migration is not a separate rollout step: it runs as part of every CLI invocation's
startup sequence (see Decision 4), gated by the completion marker, until it has run
once per location. Shipping the new launcher version is the only rollout action; the
first invocation of that version against an existing installation is what triggers it.

Order of operations within one CLI startup, once the marker shows work remains:

1. Check the completion marker; if migration is already complete, skip entirely (a
   single cheap stat).
2. Migrate the cache configuration into `resources.yaml` and remove the legacy
   `runtime-artifacts.yaml`.
3. Relocate the resource cache root and delete the legacy cache directory (no cache
   content is migrated).
4. Migrate the SQL history file to its new location.
5. Migrate each deployment directory, refusing (without aborting the others) any name
   that collides with an existing new-location deployment.
6. Report what moved or was deleted in a human-facing message on standard error.
7. Only once every step above succeeds does the CLI write the completion marker and
   dispatch to the requested command.

A partial failure at any step leaves the marker unset, so the next invocation retries
from scratch; steps that already succeeded are safe to repeat because there is nothing
left at the legacy location to move.

**Rollback: not supported, by design.** Deployments, history, and the cache config are
moved rather than copied: once migration completes, the legacy locations no longer
hold that data, so downgrading to a launcher version that only reads legacy paths will
not find it there. This is accepted deliberately rather than mitigated: a durable
backup would add ongoing disk usage and cleanup complexity in service of a downgrade
path this change does not intend to support.
