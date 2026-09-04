# UDFs and script language containers

User-defined functions (UDFs) execute in a script language container (SLC), which supplies a runtime
such as Python, Java, or R.

Cloud deployments include the standard SLCs. Local deployments do not install one by default.
Install the language you need with `exasol slc`:

```bash
exasol slc list
exasol slc install python3
exasol slc install java
exasol slc install r
exasol slc install rust
exasol slc update python3
exasol slc remove python3
```

Official SLCs can be installed, updated, or removed using either an alias or the flavor shown by
`exasol slc list`. Matching is case-insensitive. Prefer aliases because they remain stable when the
catalog flavor changes. For example, the Python container enables the `PYTHON3` and `PYTHON312`
aliases, the Java container enables `JAVA` and `JAVA17`, and the R container enables `R` and `R44`.

The alias `rust` is reserved and is not part of the official catalog. Instead of resolving a catalog
entry, it installs the latest release of
[language-container-rs](https://github.com/exasol-labs/language-container-rs) that matches this
machine's CPU architecture, under the alias `RUST`. `exasol slc update rust` re-resolves the latest
release and applies it if it has changed. To install a specific Rust container instead of whatever
`rust` currently resolves to, use `exasol slc custom install --alias RUST --language rust --source
<path-or-url>`.

`exasol slc list` displays aliases first, followed by each container's flavor, version, and
installation state.

Installing, updating, or removing an SLC normally restarts the local database. This drops open
connections and aborts running statements. The command asks for confirmation first:

- Use `--auto-approve` to skip the prompt. A command with no terminal attached also skips it and
  restarts without asking.
- Use `--no-restart` to record the change and activate it the next time the deployment starts.

Official `install`, `update`, and `remove` operations and custom SLC management apply only to local
deployments.

Script language containers also provide runtimes for features such as virtual schema adapter
scripts. Install the language required by the adapter before creating it. See
[Virtual schemas](virtual-schemas.md) for a JDBC example.

## Install a custom container

Install a custom SLC when the official catalog does not provide the language or packages you need:

```bash
exasol slc custom install --source ./my-python.tar.gz --alias MYPY3 --language python
exasol slc custom update --source ./my-python-v2.tar.gz --alias MYPY3
exasol slc remove MYPY3
```

`--source` accepts a local tar archive or an HTTPS URL. `--alias` defines the name used in
`CREATE <alias> ... SCRIPT`. `--language` identifies the custom client language. The launcher trims
whitespace, converts the identifier to lowercase, and requires it to start with an ASCII letter or
digit and contain only ASCII letters, digits, dots, hyphens, or underscores. A custom client can use
another identifier, such as `rust`, when it implements the Exasol UDF protocol.

Custom `install` and `update` restart the database and support `--auto-approve` and `--no-restart`.
If the container cannot be activated, the database still starts but the command reports that the
container is recorded as pending.

Removing a custom container takes effect immediately and does not accept the restart options. The
deployment must be running to remove an active alias through the database. A container recorded but
never activated can be removed while the deployment is stopped.

`exasol slc list` shows custom containers separately, including their alias, language, status, and
source. JSON output marks them as custom and includes availability information.
