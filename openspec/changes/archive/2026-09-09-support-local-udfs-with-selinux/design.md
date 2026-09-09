## Context

The local installer starts Nano through one shared Podman command for Linux hosts and VM-backed platforms. That command currently uses `unmask=ALL`, but Nano remains assigned an SELinux container process label on enforcing Linux hosts. Nano can start and accept SQL connections, while its namespaced UDF launcher exits immediately and the database reports only `VM crashed`.

## Goals / Non-Goals

**Goals:**

- Apply the narrowest container-scoped SELinux compatibility setting proven to allow Nano UDF subprocesses to run.
- Keep the shared Podman launch behavior portable across Linux and VM-backed runtimes.

**Non-Goals:**

- Changing host-wide SELinux policy or documenting permissive mode as a prerequisite.
- Granting the Nano container unrestricted privileged access.
- Changing the PostgreSQL Virtual Schema deployment test or its sidecar networking model.

## Decisions

### Add `label=disable` to the shared Nano Podman command

The installer will pass `--security-opt label=disable` alongside the existing `--security-opt unmask=ALL`. Disabling labeling only for Nano is sufficient in the reproduced enforcing-SELinux environment and leaves host-wide enforcement active. Keeping the option in the shared Podman command avoids host capability detection and gives all supported Podman environments one deterministic command; runtimes without active SELinux labeling accept the option without changing their effective isolation.

The alternatives are broader or less reliable. Running Nano with `--privileged` grants capabilities unrelated to the failure. Detecting SELinux before constructing the command introduces platform branching and can misclassify remote or VM-backed Podman engines. Requiring permissive host SELinux weakens the host globally and conflicts with the local deployment experience.

## Risks / Trade-offs

- [Disabling Nano's SELinux process label reduces one layer of container isolation] → Scope the setting to the Nano container, preserve the existing namespace and Podman security configuration, and avoid broader privileged mode.
- [An unconditional security option could behave differently in a future Podman implementation] → Cover exact command construction in unit tests and exercise the existing Linux and macOS deployment jobs.

## Migration Plan

The change affects newly started Nano containers only; there is no persistent-data migration. Existing deployments receive the setting on their next container recreation. Rollback consists of removing the additional Podman security option; deployment data remains unchanged.
