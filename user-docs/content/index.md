# Exasol Personal

Exasol Personal gives you a full Exasol database for personal use. Run it locally on macOS, Linux,
or Windows, or deploy it into your own AWS, Azure, Exoscale, or STACKIT account.

The Exasol Launcher (`exasol`) installs and manages the database from the command line. It can
provision infrastructure, start and stop deployments, display connection details, and open SQL or
shell sessions.

These pages describe the product version selected in the version menu. Choose the version that
matches your installed launcher when commands or supported capabilities differ between releases.

## Quick start

To install and run Exasol Personal locally on macOS, Linux, or Windows, follow these steps.

Install the launcher:

```bash
curl https://www.exasol.com/install/ | sh
```

The installer places the `exasol` binary in `~/.local/bin`. If that directory is not in `PATH`,
follow the instructions printed by the installer.

Install and start the database:

```bash
exasol install local
```

Connect to your database:

```bash
exasol connect
```

Type any SQL statement terminated by `;`. For example, test your connection with `SELECT 1;`.
See the [Exasol SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm) for the
full syntax.

## Detailed user guides

- [Install the launcher](getting-started.md), then [run Exasol locally](local-deployment.md) or
  [deploy it to the cloud](cloud-deployment.md).
- [Connect to the database](connect.md) and [load the sample data](load-data.md).
- Learn how to [manage one or more deployments](manage-deployments.md).
- Check the [system requirements](system-requirements.md) and [product limitations](limitations.md).

## Key capabilities

- Start a local database on macOS, Linux, or Windows in seconds.
- Provision single-node or multi-node deployments in supported clouds.
- Connect AI agents and automation through the scriptable CLI.
- Run analytics with Exasol's in-memory, massively parallel database engine.
- Use the full Exasol Database feature set without an artificial data-size limit.

Exasol Personal is free for personal use under the applicable [license terms](privacy-and-licensing.md).
