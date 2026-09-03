## Context

See `proposal.md` for motivation. Local configuration currently reaches infrastructure backends as only the user-supplied overrides, while backend-specific defaults are applied inside `Configure`. The macOS runtime accepts `auto` or port zero and lets the VM runner choose a host port; the shared direct-host runtime on Linux and Windows already publishes a concrete port but defaults directly to `8563`. Lifecycle failures are normally recorded as interrupted, while configuration changes are currently restricted to initialized deployments.

The selected port is advisory until Podman or the VM launcher performs the actual bind. No availability probe can eliminate the race between releasing its test listeners and starting the runtime, so the runtime's bind attempt must remain authoritative.

## Goals / Non-Goals

**Goals:**

- Give backend implementations an explicit initialization hook for dynamic defaults of unset infrastructure variables.
- Persist a deterministic concrete port for each exposed local service.
- Preserve exact configured ports across starts on every supported local platform and report actual bind conflicts actionably.
- Keep a deployment configurable after a port-bind failure.
- Split implementation into self-contained commits that each pass the relevant checks.

**Non-Goals:**

- Reserving ports for stopped deployments outside runtime processes.
- Changing the user-facing `--ports` mapping syntax.
- Exposing local services on non-loopback interfaces or allocating privileged ports.
- Adding another exposed service in this change; the design supports a service catalog, but only `db` is registered initially.

## Decisions

### Backend defaults are resolved before configuration is written

Extend `deploymentBackend` with a context-aware method that accepts the supplied infrastructure overrides and returns defaults only for declared variables that are unset. The configuration orchestrator merges those defaults first and explicit overrides second, then passes the resulting values through the existing configuration writer. Tofu returns no launcher-owned defaults; the local backend returns its effective CPU, memory, data-size, and concrete port defaults.

The same method supplies effective values for reset operations. A patch operation does not default omitted options because omission means preserving the current value.

This keeps dynamic choices in the backend that owns them while making initialization and reset behavior explicit. Adding port selection directly to `init` was rejected because it would duplicate local-backend policy in generic orchestration. Keeping it hidden inside `Configure` was rejected because unset values would remain absent from the persisted configuration contract.

### Automatic ports use a service catalog and a deterministic circular scan

Represent every exposed local service with a stable name, guest port, and default host port. The first catalog entry is the database service with guest and default host port `8563`.

For each unset or explicitly automatic service, resolve `localhost`, retain and deduplicate its IPv4 and IPv6 loopback addresses, and test candidates in this order:

1. The service default through `65535`, inclusive.
2. `1024` through the port immediately before the service default.

Bind the same candidate on all resolved loopbacks at once. Keep successful listeners open while selecting the remaining services so two services cannot receive the same host port, then close them after producing the defaults. An exhausted scan returns an error naming the service. Resolver and listener functions are injectable for deterministic unit tests.

Explicit non-zero mappings bypass selection and remain user-owned. Empty configuration, `auto`, and zero-valued service mappings request automatic selection and are normalized to the canonical `service:port` form before persistence.

Random kernel-assigned ports were rejected because they do not prefer familiar endpoints or produce consecutive assignments for concurrently running deployments. Scanning only upward without wrapping was rejected because it can falsely report exhaustion while usable unprivileged ports remain below the default.

### The runtime bind attempt is the availability authority

Do not add lifecycle preflight as the enforcement mechanism. The shared Linux and Windows direct-host runtime passes the persisted host port to Podman's publication argument, and macOS passes it to the VM launcher's labeled forward. Both runtime paths reject zero or `auto`; macOS also verifies that runner state reports the requested host port.

Introduce a shared typed unavailable-port error in a package usable by deployment, local runtime, and installation code. The Podman installation and VM runtime translate an actual launch failure caused by binding the requested port into this error, retaining the original command error through `Unwrap`. Detection uses the command's bind diagnostic and, where needed, a post-failure check that the requested port is unavailable; it never substitutes another port. Errors unrelated to binding remain unchanged.

The lifecycle layer recognizes the typed error, restores the source state (`initialized` for deploy or `stopped` for start), and adds a call to action naming `exasol config set --ports db:<available-port>` and `exasol config set --ports auto`. This makes the suggested recovery command immediately usable and covers a process claiming the port at any point before the runtime's atomic bind.

Performing only an availability check before launch was rejected because the result becomes stale before the actual bind. Treating every runtime launch error as a port conflict was rejected because it would hide unrelated failures.

### Stopped local configuration is a backend-aware state exception

Make configuration permission checks aware of the deployment backend and workflow state. Initialized deployments remain configurable. A stopped local deployment permits both `config set` and `config reset` for every advertised local option, while a running local deployment remains blocked. Cloud deployments retain the existing rule that potentially deployed resources must be destroyed before reconfiguration.

Successful stopped-local changes direct the user to `exasol start`; initialized changes continue to direct the user to `exasol deploy`.

### Legacy automatic values are normalized at the next safe write

When a stopped local deployment contains an empty, `auto`, or zero-valued database mapping, resolve backend defaults and persist the concrete result before invoking the runtime. Existing positive fixed mappings remain unchanged. Rollback remains compatible because the persisted `db:<positive-port>` syntax is already accepted by current launchers.

## Risks / Trade-offs

- **A full circular scan can perform many bind attempts when the host has no free unprivileged ports.** → Stop after exactly one pass and keep the common path at the service default.
- **A stopped deployment does not hold its configured port.** → Treat runtime bind as authoritative and return actionable recovery without random fallback.
- **`localhost` can resolve inconsistently across hosts.** → Accept only resolved loopback addresses, deduplicate them, and fail clearly when none are available.
- **External command diagnostics vary across Podman and VM-runner versions.** → Combine bind-diagnostic classification with a post-failure availability check, preserve the original error, and keep non-port failures generic.
- **Restoring workflow state after a bind failure could be unsafe if the runtime partially started.** → Restore only for the typed error after the runtime/install layer confirms the requested endpoint was not successfully established; otherwise retain normal interrupted-state handling.

## Migration Plan

1. Add backend default resolution and persist concrete defaults for new deployments without changing runtime behavior.
2. Allow stopped local reconfiguration and state-aware guidance.
3. Require fixed runtime ports on Linux, Windows, and macOS.
4. Add legacy normalization, then authoritative bind-conflict errors and workflow restoration.
5. Add portable lifecycle coverage and user documentation.
6. Archive the OpenSpec change only after all tasks and checks pass; commit archived artifacts separately from implementation commits.

Rollback requires no data migration: concrete port mappings use the existing `ports` syntax and remain readable by older versions. A deployment normalized from `auto` keeps the chosen fixed port after rollback.
