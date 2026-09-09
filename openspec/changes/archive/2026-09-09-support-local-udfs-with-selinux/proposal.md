## Why

On Linux hosts with enforcing SELinux, the local Nano container's UDF subprocess is killed before a Java adapter can start, so Virtual Schema creation fails with an opaque `VM crashed` error. Ubuntu CI does not expose this host-dependent failure because its host does not enforce SELinux.

## What Changes

- Make local Podman deployments support UDF and Virtual Schema execution on SELinux-enforcing hosts without requiring users to weaken host-wide SELinux enforcement.
- Preserve the existing Nano sandbox configuration while disabling SELinux process labeling for that container only.
- Document the repaired Linux behavior in the changelog.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-host-podman-install`: A local Nano deployment supports UDF execution when the Linux host enforces SELinux.

## Impact

The shared local Podman Nano launch command and its unit tests will change. The existing PostgreSQL Virtual Schema deployment test will verify the repaired behavior. There are no public CLI, configuration, storage, or network contract changes and no new dependencies.
