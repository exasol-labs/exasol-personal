# Install the launcher

The Exasol Launcher (`exasol`) is the command-line tool that deploys and manages Exasol Personal.
The launcher runs on macOS, Linux, and Windows.

## Before you begin

Choose where the database will run:

- For a local deployment, use a supported macOS, Linux, or Windows computer. Linux requires Podman.
  Windows requires Windows Package Manager and the prerequisites for a Podman machine. See
  [System requirements](system-requirements.md) for platform-specific details.
- For a cloud deployment, prepare an account with permission to provision compute resources. See
  the account setup guide for [AWS](cloud/aws.md), [Azure](cloud/azure.md),
  [Exoscale](cloud/exoscale.md), or [STACKIT](cloud/stackit.md).

## Install on macOS, Linux or Windows (WSL)

Run:

```bash
curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
```

The installer places the `exasol` binary in `~/.local/bin`. If that directory is not in `PATH`,
follow the instructions printed by the installer.

## Verify the installation

```bash
exasol version
```

Continue with a [local deployment](local-deployment.md) or a
[cloud deployment](cloud-deployment.md).

## Install with an AI agent

Exasol publishes [agent skills](https://github.com/exasol-labs/exasol-agent-skills) that teach
compatible coding agents how to install Exasol Personal, connect to it, load data, and write Exasol
SQL. For example:

```bash
claude "Install skills from https://github.com/exasol-labs/exasol-agent-skills/ and use these skills to set up Exasol Personal"
```

```bash
codex "Install skills from https://github.com/exasol-labs/exasol-agent-skills/ and use these skills to set up Exasol Personal"
```

The agent installs the launcher and asks before making changes to your computer or cloud account.
