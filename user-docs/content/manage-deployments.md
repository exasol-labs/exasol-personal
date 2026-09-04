# Manage deployments

Each local or cloud deployment keeps its configuration, state, and credentials in a deployment
directory. Keep this directory until you have destroyed the deployment resources.

## Select a deployment

The default deployment is stored in `~/.exasol/personal/deployments/default`. When you run a command
from an existing deployment directory, the launcher selects that deployment automatically.

To maintain multiple deployments, give each one a case-sensitive name with `--deployment` or `-d`:

```bash
exasol install local -d demo
exasol install aws -d staging
exasol status -d demo
exasol connect -d demo -c "SELECT 1"
```

Named deployments live below `~/.exasol/personal/deployments/<name>`. Names that differ only by case
may collide on the case-insensitive filesystems commonly used by macOS and Windows.

Alternatively, select an arbitrary path with `--deployment-dir <path>`. You cannot combine
`--deployment-dir` and `--deployment`.

List the launcher-managed default and named deployments with:

```bash
exasol deployments list
```

Example output:

```text
default status=database_ready preset=local/ubuntu path=/Users/me/.exasol/personal/deployments/default
staging status=stopped preset=aws/ubuntu path=/Users/me/.exasol/personal/deployments/staging
```

The list does not include deployments selected through an arbitrary `--deployment-dir` path.

## Inspect or change configuration

An initialized deployment directory is tied to its infrastructure and installation presets. Retry a
failed installation with the same presets, or use these commands to inspect and change their
parameters:

```bash
exasol config get
exasol config set
exasol config reset
```

To select different presets in the same directory, first run `exasol destroy --remove`. If the
deployment resources have already been removed outside the launcher, run `exasol remove` instead.

## Inspect or clean the runtime cache

The launcher downloads runtime tools such as OpenTofu on demand and reuses them from a per-user
cache. Use:

```bash
exasol cache list
exasol cache clean
exasol cache clean --invalid
exasol cache clean --partial-downloads
exasol cache clean --all
exasol diag cache
```

Add `--dry-run` to a cleanup command to preview the files it would remove.

## Stop and start

Stop a deployment when you do not need its compute resources:

```bash
exasol stop
```

For cloud deployments, networking and data volumes can continue to incur costs while compute
instances are stopped.

Restart the deployment with:

```bash
exasol start
```

Node IP addresses can change after a restart. Check the command output or run `exasol info` before
connecting again.

## Destroy a deployment

Destroying removes the deployment resources and their data:

```bash
exasol destroy
```

The launcher asks for confirmation first. Pass `--auto-approve` to skip the prompt; a command with
no terminal attached proceeds without asking, so a script that reaches `exasol destroy` or
`exasol remove` destroys the deployment.

By default, the launcher retains the deployment directory for inspection or recreation. Remove it
after successful destruction with:

```bash
exasol destroy --remove
```

If the resources were already deleted manually or are no longer accessible, remove only the local
deployment directory with:

```bash
exasol remove
```

`exasol remove` does not destroy resources. Deleting a deployment directory or the launcher itself
also does not remove provisioned resources. If the directory is lost before destruction, you must
remove the remaining resources directly through the target environment.

For local deployments, `exasol destroy` removes the launcher-managed runtime and database data for
the selected deployment. On Windows, it leaves Podman's shared default machine running.
