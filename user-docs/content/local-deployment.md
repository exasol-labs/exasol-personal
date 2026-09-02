# Run Exasol locally

Local deployments run on supported macOS, Linux, and Windows computers.

- On macOS, the launcher manages a virtual machine containing Podman and the database. An Apple
  Silicon Mac with at least 8 GB of memory is required.
- On Linux, the database runs directly in Podman on the host. Install Podman and make sure it is
  available in `PATH`.
- On Windows, the database runs through Podman's default machine. Windows Package Manager
  (`winget`) and the prerequisites for a Podman machine are required.

After [installing the launcher](getting-started.md), run:

```bash
exasol install local
```

The launcher starts a single-node Exasol database. Run `exasol info` at any time to display
connection information.

The launcher selects and saves a database port during initialization, preferring `8563`, and keeps
it stable across restarts. To choose one explicitly, run `exasol install local --ports db:9563`.
To change the port later, stop the deployment first, update the configuration, and restart it:

```bash
exasol stop
exasol config set --ports db:9563  # or: exasol config set --ports auto
exasol start
```

If another process claims the saved port while the deployment is stopped, startup reports the
conflict and shows the same explicit and automatic replacement commands.

On macOS, the managed virtual machine uses about half of the detected host memory by default. An
explicitly configured memory value must be at least 4096 MB. Linux and Windows use host-managed
resources, so the virtual-machine sizing options do not apply. The initial local database credentials
are `sys` / `exasol`.

## Windows host preparation

If Podman is missing, the launcher can offer to install it. It also creates or starts Podman's
default machine when necessary, without changing the configuration of an existing machine. Because
installing Podman changes shared host state, the launcher displays the exact command and asks for
approval.

Use `--auto-approve` with `install`, `deploy`, or `start` for unattended preparation. A command that
cannot prompt refuses the host change unless this option is present. Stopping or destroying an
Exasol deployment leaves the shared Podman machine running.

## Open a shell

On macOS, open a shell in the managed virtual machine:

```bash
exasol shell host
```

Open a shell inside the database container:

```bash
exasol shell container
```

These shell commands are not available for local deployments on Linux or Windows.

## Next steps

- [Connect to the database](connect.md).
- [Load sample data](load-data.md).
- [Install a script language container](udfs.md) to run UDFs.
- [Manage the deployment lifecycle](manage-deployments.md).

If the local deployment does not behave as expected, see [Troubleshooting](troubleshooting.md).
