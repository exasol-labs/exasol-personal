# Exasol Cleanup (standalone)

A small, standalone utility to discover and clean up Exasol Personal deployments across supported cloud providers.

This internal tool is written and maintained almost exclusively using agentic AI. It's not part of any official release, not officially supported and purely internal.
Feel free to use if you find it useful though.

The cleanup core is also available as an internal Go library under `pkg/cleanup`. The library returns typed deployment, resource, action, and result data and intentionally does not write to stdout or stderr; command-line parsing and terminal output stay in the standalone CLI.

## Usage

Build:

```
task build
```

Provider selection uses `--provider=<name>[,<name>...]`. If omitted, all known providers (`aws`, `exoscale`, `stackit`, `azure`) are included. Location flags such as `--aws-region`, `--exoscale-zone`, `--stackit-region`, and `--azure-location` are multi-value too, so one provider can appear multiple times in the `Scope:` table - once per searched region or zone. Stackit cleanup also needs `--stackit-project-id` when the Stackit provider is selected; Azure needs `--azure-subscription-id`. Azure discovery is subscription-wide, so `--azure-location` is an optional filter rather than a separate search target (omit it to search every region). In command help, provider-specific flags are shown in separate provider option groups so they stay visually distinct from generic options such as `--owner`, `--json`, or `--verbose`.

Azure authenticates through the standard `DefaultAzureCredential` chain (environment variables, workload/managed identity, or the Azure CLI), and identifies Exasol Personal deployments by the same `Project`/`Deployment`/`Owner` tags the launcher applies. See [Set up Microsoft Azure](../../user-docs/content/cloud/azure.md) for account setup.

Discover deployments:

```
./bin/exasol-cleanup discover --owner=*
./bin/exasol-cleanup discover --provider=aws --owner=*
./bin/exasol-cleanup discover --provider=aws --aws-region=us-east-1,eu-central-1 --owner=*
./bin/exasol-cleanup discover --provider=stackit --stackit-project-id=<project-id> --stackit-region=eu01
./bin/exasol-cleanup discover --provider=azure --azure-subscription-id=<subscription-id> --owner=*
./bin/exasol-cleanup discover --provider=azure --azure-subscription-id=<subscription-id> --azure-location=westeurope
./bin/exasol-cleanup discover --legacy --owner=*
```

`--owner` is a global flag. When omitted, AWS commands default to the caller identity. If you want to inspect or delete a deployment owned by somebody else, pass the matching `--owner` filter explicitly, for example `--owner=*`.

`discover`, `show`, and `run` print a `Scope:` table first so you can always see every known provider, the effective location and owner filter for each one, a single status showing what happened with that provider, and an optional reason when something was skipped or failed.

Show details:

```
./bin/exasol-cleanup show exasol-123ad553
./bin/exasol-cleanup show exasol-123ad553 exasol-234bc664
./bin/exasol-cleanup show --owner=* exasol-123ad553
./bin/exasol-cleanup show --types=ec2-instance,ebs-volume exasol-123ad553
```

Run cleanup (dry-run by default):

```
./bin/exasol-cleanup run exasol-123ad553 
./bin/exasol-cleanup run exasol-123ad553 exasol-234bc664
# To actually delete resources (dangerous):
./bin/exasol-cleanup run exasol-123ad553 --execute
./bin/exasol-cleanup run --owner=* exasol-123ad553 --execute
./bin/exasol-cleanup run --types=ec2-instance,ebs-volume exasol-123ad553 --execute
```

`show` and `run` are intentionally different: `show` lists the resources currently associated with each deployment, while `run` plans ordered cleanup actions and optionally executes them. Because `run` is destructive in execute mode, `--execute` remains the safety switch.

In JSON mode, `show` and `run` always return a consistent batch-shaped envelope with a top-level `deployments` array, even when you request only a single deployment id.
