<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The Analytics Database for Agentic AI — Free for Personal Use**

*Run a full Exasol database locally on your Mac or deploy to your own cloud*

[![Documentation](https://img.shields.io/badge/docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

[Changelog](CHANGELOG.md)

</div>

## 🔥 Key Features

- 💻 **Runs Locally in Seconds** — Spin up a full Exasol database on your own Mac with one command (macOS today; Windows & Linux coming soon)
- 🤖 **Built for Agentic AI** — Connect AI agents and LLM tools directly through a scriptable CLI
- 🧠 **Built-in AI Functions** — Leverage native AI/ML capabilities with GPU acceleration, right where your data lives
- 🏎️ **In-Memory Performance** — Run complex analytics at in-memory speed with Exasol's industry-leading analytics engine
- 🏢 **Full Enterprise Features** — Access all enterprise-scale capabilities of the Exasol database, completely free for personal use
- ♾️ **Unlimited Data** — Store and analyze unlimited amounts of data with no artificial limits
- 📈 **Scalable Architecture** — Scale up to any number of nodes using Exasol's MPP (Massively Parallel Processing) architecture
- ☁️ **Or Deploy to Your Cloud** — Provision to AWS, Azure, Exoscale, or STACKIT with just a few commands when you need scale or a shared instance
- 🖥️ **Cross-Platform CLI** — Install and manage Exasol using the Exasol Launcher on macOS, Linux, or Windows


## ✅ Prerequisites

**Local deployment (recommended — fastest):** a Mac with at least 8 GB RAM.

> Local deployment is currently **macOS only**. Windows and Linux support is coming soon — until then, use a cloud deployment on those platforms.

**Cloud deployment:** an account on one of the supported providers, with permission to provision compute instances:

- **AWS** — [Set up Amazon Web Services](./user-docs/content/cloud/aws.md)
- **Azure** — [Set up Microsoft Azure](./user-docs/content/cloud/azure.md)
- **Exoscale** — [Set up Exoscale](./user-docs/content/cloud/exoscale.md)
- **STACKIT** — [Set up STACKIT](./user-docs/content/cloud/stackit.md)


## ⬇️ Install the Launcher

The **Exasol Launcher** (`exasol`) is the command-line tool that deploys and manages your Exasol database, locally or in the cloud. It runs on macOS, Linux, and Windows.

On macOS and Linux, install it with:
```bash
curl https://www.exasol.com/install/ | sh
```
This places the `exasol` binary in `~/.local/bin`. On most Linux distributions this directory is already in your `PATH`; on macOS, or if the installer reports that `~/.local/bin` is not in your `PATH`, follow the instructions it prints.

On Windows, download the launcher from the [Exasol Download Portal](https://downloads.exasol.com/exasol-personal) and copy the `exasol` binary to a directory in your `PATH`.

Verify the installation with `exasol version`.


## 🏎️ Quick Start — Run Exasol Locally

The fastest way to try Exasol: a full database running on your own Mac.

> **Currently macOS only.** Windows and Linux support is coming soon — to run Exasol on those platforms today, use a cloud deployment (see the **Deploy to the Cloud** section below).

With the launcher installed (see **Install the Launcher** above), start a local Exasol database:
```bash
exasol install local
```

In a few seconds you'll have a local Exasol instance running. Run `exasol info` at any time to see how to connect, then head to the **Load Sample Data** section to start querying.

Prefer to run in your own cloud? See the **Deploy to the Cloud** section below.


## ☁️ Deploy to the Cloud

Deploy Exasol Personal to your own cloud account when you need more scale or a shared instance. Supported providers: AWS, Azure, Exoscale, and STACKIT.

With the launcher installed (see **Install the Launcher** above):

1. Configure authentication for your provider. See the relevant account setup guide in the **Prerequisites** section for the environment variables and credentials required.

2. To install Exasol Personal, run the following command with the preset for your cloud provider:
   ```bash
   exasol install aws        # Amazon Web Services
   exasol install azure      # Microsoft Azure
   exasol install exoscale   # Exoscale
   exasol install stackit    # STACKIT
   ```
   The `exasol install` command does the following:
   - Generates backend files in the deployment directory
   - Provisions the necessary deployment resources
   - Starts the deployment
   - Downloads and installs Exasol Personal on that infrastructure

   The whole process normally takes about 10 to 20 minutes to complete.

   When the deployment process has finished, you will see instructions on how to connect to your Exasol database using a client of your choice. You can also find this information at any time by using `exasol info` in the terminal.

By default, Exasol Personal stores deployment state in `~/.exasol/personal/deployments/default`. If you run a command from an existing deployment directory, Exasol Personal uses that directory instead. Pass `--deployment-dir <path>` to choose a different deployment directory explicitly, or pass `--deployment <name>` (`-d <name>`) to use `~/.exasol/personal/deployments/<name>` — useful for running more than one deployment side by side without picking your own path. `--deployment-dir` and `--deployment` cannot be used together. Deployment names are matched case-sensitively, but may collide on case-insensitive filesystems (the default on macOS and Windows). Use `exasol deployments list` to see every default and named deployment directory, its status, and which one is currently active.

Keep the deployment directory until deployment resources have been destroyed. Deleting the directory does not remove those resources and can make cleanup harder.

An initialized deployment directory is tied to the selected infrastructure and installation presets. Rerun `exasol install <preset>` with the same presets to retry a failed deployment safely, or use `exasol config get`, `exasol config set`, and `exasol config reset` to inspect or change parameters for the existing presets without deleting local state. To switch presets in the same deployment directory, run `exasol destroy --remove` before initializing again, or run `exasol remove` if the deployment resources are already gone.

Runtime tools such as OpenTofu are downloaded on demand and reused from a per-user runtime artifact cache. Use `exasol cache list` to inspect cached artifacts, `exasol cache clean` to remove stale artifacts, `exasol cache clean --invalid` to remove artifacts that fail integrity checks, `exasol cache clean --partial-downloads` to remove staged partial downloads, `exasol cache clean --all` to wipe cached artifacts, and `exasol diag cache` to inspect cache health without changing it. Add `--dry-run` to a cleanup command to preview what would be removed.

## 📊 Load Sample Data

To get started quickly, Exasol provides a sample dataset hosted on S3 that you can import using SQL.

You can load it directly by executing this command in your deployment directory (e.g. `~/.exasol/personal/deployments/default` for a default deployment):

```bash
exasol connect -f sample.sql
```

Alternatively, connect with a SQL client of your choice and paste the statements below:

```sql
CREATE OR REPLACE TABLE PRODUCTS AS (
    IMPORT FROM PARQUET
    AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
    FILE 'online_products.parquet'
);
```

```sql
CREATE OR REPLACE TABLE PRODUCT_REVIEWS AS (
    IMPORT FROM PARQUET
    AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
    FILE 'product_reviews.parquet'
);
```

Exasol infers the table schema directly from the Parquet files, so there's no need to define columns up front.

| Table | Rows | Size |
|---|---|---|
| `PRODUCTS` | 1,000,000 | 27.3 MiB |
| `PRODUCT_REVIEWS` | 1,822,007 | 154.5 MiB |

## ⏯️ Start and stop Exasol Personal

To save costs, you can temporarily stop Exasol Personal by using the following command:
```bash
exasol stop
```
This stops the compute instance(s) that Exasol Personal is running on.

Networking and the data volumes where the database data is stored will continue to incur costs when compute instances are stopped.

To start Exasol Personal again, use the following command:
```bash
exasol start
```
The IP addresses of the nodes will change when you restart Exasol Personal. Check the output of the `start` command to know how to connect to the deployment after a restart.

For local deployments, which currently require macOS with at least 8 GB RAM, the launcher manages a local VM runtime and an internal deployment share inside the deployment directory. If you do not configure local VM memory explicitly, it defaults to about 50% of detected host memory. Configured local VM memory must be at least 4096 MB. The initial local database credentials are `sys` / `exasol`. `exasol shell host` opens the local VM shell, and `exasol shell container` opens a shell inside the local database container.

## 🗑️ Remove Exasol Personal

To completely remove an Exasol Personal deployment, use `exasol destroy`. This command deletes the deployment resources and all associated data.

To learn more about this command, use `exasol destroy --help`.

By default, `exasol destroy` keeps the local deployment files so you can inspect the deployment or recreate the same preset. To also remove the local deployment directory after deployment resources have been destroyed, run:
```bash
exasol destroy --remove
```

If deployment resources were already deleted manually or you no longer have access to destroy them through the launcher, use the local recovery command:
```bash
exasol remove
```
This removes the local deployment directory. It does not destroy deployment resources.

Deleting the deployment directory and the Exasol Launcher will not remove the resources that were created in the target environment. To completely remove a deployment, you must use the `exasol destroy` command before deleting the deployment directory.

If you have already deleted the deployment directory and the Exasol Launcher, you must remove the resources manually in the target environment.

For local deployments, `exasol destroy` deletes the local VM disk/data and launcher-managed share for that deployment.

## ⚙️ Cloud: Choosing cluster size and compute instance types

By default the launcher deploys a single-node cluster on a memory-optimized instance in the cloud (e.g. `r6i.xlarge` on AWS, `Standard_E4s_v3` on Azure, `standard.extra-large` on Exoscale, `m2i.4` on STACKIT). To change the number of nodes or the instance type, use the `--cluster-size` and `--instance-type` options:
```bash
exasol install <preset> --cluster-size <number> --instance-type <string>
```
If the deployment process is interrupted, resources that were already created will not be removed automatically and cloud resources may continue to accrue cost. In that case, use `exasol destroy` to clean up the deployment, or remove the resources manually in the target environment.


## 🔜 Next steps

Once the deployment process is complete, use `exasol info` for information about how to connect to your Exasol database. The credentials for connecting to the database from a client are stored in the file `secrets.json` in the deployment directory.

You can also use the built-in SQL client in Exasol Launcher to connect directly to the database from the command line:
```bash
exasol connect
```

To run SQL without entering the interactive shell, pass it directly. Both flags accept multiple `;`-separated statements and exit when done, which is convenient for scripting and automation:
```bash
exasol connect -c "SELECT 1; SELECT 2"   # run inline statement(s)
exasol connect -f script.sql             # run statements from a file
```
`--command` and `--file` are mutually exclusive. In this non-interactive mode, execution stops at the first failing statement and `connect` exits with a non-zero status so scripts can detect errors. Combine with `--json` for machine-readable output, or use `--csv` for CSV output:
```bash
exasol connect --csv -c "SELECT * FROM PRODUCTS" > products.csv
```
With `--json`, non-interactive execution writes exactly one JSON document to stdout for the full invocation, including multi-statement runs and SQL errors. Interactive `exasol connect --json` continues to emit one JSON document per executed statement.

In an interactive session, query output is capped at 100 rows by default so a large `SELECT` doesn't flood the terminal; a note is printed when output is truncated. Piped or `--command`/`--file` (non-interactive) execution returns the full result set. Use `--max-rows N` to set the cap explicitly, or `--max-rows 0` for unlimited:
```bash
exasol connect --max-rows 0        # return all rows, even interactively
echo "SELECT * FROM PRODUCTS;" | exasol connect --max-rows 1000
```

See also...
- To learn more about how you can connect to your Exasol database and start loading data using the many supported tools and integrations, see [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) and [Load Data](https://docs.exasol.com/db/latest/loading_data.htm).
- To learn how to use the SQL statements, data types, functions, and other SQL language elements that are supported in Exasol, see [SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm).

## 🧑‍💻 Exasol Admin

Exasol Admin is an easy-to-use web interface that you can use to administer your new Exasol database. Instructions on how to access Exasol Admin are shown in the terminal output at the end of the install process.

- To find the Exasol Admin URL after the installation has completed, use `exasol info`.
- The credentials for connecting to Exasol Admin are stored in the file `secrets.json` in the deployment directory.

Your browser may show a security warning when connecting to Exasol Admin because of the self-signed certificate. Accept this warning and continue.

Currently, Exasol Admin is only available on cloud deployments.

## 🔒 Connect using SSH

To connect with SSH to your deployment use one of the following commands:

```bash
# Connect to the compute instance your database is running on:
exasol shell host
```

```bash
# Connect to the COS container your node is running on:
exasol shell container
```

## 📦 Presets

Exasol Personal uses **presets** — self-contained directories of templates and config files — to provision infrastructure and install Exasol. Each deployment combines two presets:

- **Infrastructure preset** — provisions deployment resources for a cloud provider or local system. Built-in: `aws`, `azure`, `exoscale`, `stackit`, `local`.
- **Installation preset** — installs and configures Exasol on the provisioned nodes. Built-in: `ubuntu` (used by default).

```bash
exasol install <infra-preset> [install-preset]

exasol install aws                                       # built-in preset by name
exasol install ./my-preset                               # local preset by path (starts with . / ~ or contains /)
exasol install https://github.com/org/preset.git         # git repository (HEAD)
exasol install https://github.com/org/preset.git@v1.0    # git repository at branch or tag
exasol install https://example.com/preset.tar.gz         # remote archive (re-fetched on every run)
exasol install file:///path/to/preset-dir                # local directory via URI
exasol install file:///path/to/preset.tar.gz             # local archive (re-extracted on every run)
```

List all available built-in presets, including cloud and local targets:

```bash
exasol presets
```

### Local presets

You can store your own preset directories anywhere on your filesystem and pass the path directly to `exasol install`. This lets you target additional infrastructure platforms or customize provisioning without modifying the launcher.

### External presets

In addition to built-in names and local paths, `exasol install` accepts external sources:

- **Git repository** — `https://github.com/org/preset.git` or `git@github.com:org/preset.git`. Append `@branch-or-tag` to pin a specific ref. The repository is cloned once per commit and cached; repeated runs at the same commit reuse the cache.
- **Remote archive** — An `https://` or `http://` URL ending in `.tar.gz`, `.tgz`, or `.zip`. The archive is re-downloaded on every run because no checksum is provided.
- **Local directory via URI** — `file:///absolute/path`. The directory is used directly without copying.
- **Local archive via URI** — `file:///absolute/path/to/preset.tar.gz`. The archive is re-extracted on every run.

See [doc/presets.md](doc/presets.md) for the full preset contract, caching behavior, and troubleshooting.

### Building your own preset

See [doc/presets.md](doc/presets.md) for the full preset contract: manifest schema, required output artifacts, variable channels, and the reference implementations in `assets/infrastructure`.

## 🖥️ System Requirements

The Exasol Launcher runs on:

- **macOS** — 12 (Monterey) or later.
- **Linux** — amd64 or arm64.
- **Windows** — 10 or later, amd64.

Where the database runs depends on the deployment type:

- **Cloud deployments** — the database runs on your provider's infrastructure, so any supported launcher platform above works. The launcher provisions **Ubuntu 22.04 LTS (x86-64)** compute instances on all cloud providers.
- **Local deployments** — the database runs in a VM on your machine, which requires macOS 15 (Sequoia) or later, with at least 8 GB RAM.

## 🚧 Limitations

Local deployments are intended for development and exploration and do not yet support every feature of a cloud deployment:

- **UDFs** — user-defined functions are not available yet (coming soon).
- **Virtual schemas** — virtual schemas are not available yet (coming soon).
- **Admin UI** — the Administration UI is not available yet (coming soon).
- **Multi-node clusters** — a local deployment is a single VM on your machine by design and always runs as a single node.

Cloud deployments support all of the above.

## 🔎 Version Check

Exasol Personal periodically checks for newer versions so it can let you know when an update is available. The launcher checks for a newer `exasol` binary, and cloud deployments additionally check for a newer database version. These checks send a small amount of information (such as the current version, operating system, and architecture) to Exasol.

Both checks are opt-out. Pass the flags to `exasol install` to disable them:

```bash
exasol install <preset> --no-launcher-version-check   # disable the launcher update check
exasol install <preset> --no-db-version-check         # disable the database version check (cloud deployments)
```

For details on what data is collected and how it is used, see the [Exasol Privacy Policy](https://www.exasol.com/privacy-policy/).

## ⚖️ Licensing

The Exasol Launcher source code in this repository is open-source software licensed under the [MIT License](./LICENSE). You are free to use, modify, and distribute it. Contributions are made under the same terms.

The launcher installs the **Exasol Database**, which is proprietary software provided by Exasol AG, free for personal use. By deploying it with `exasol install`, you accept the [Exasol Personal End User License Agreement (EULA)](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement).

## 📚 Resources & Documentation

Get the most out of Exasol with these resources:

- [Exasol Documentation](https://docs.exasol.com/db/latest/home.htm) — Complete database documentation
- [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) — Driver downloads and client setup
- [Load Data](https://docs.exasol.com/db/latest/loading_data.htm) — Import data into your database
- [SQL Reference](https://docs.exasol.com/db/latest/sql_reference.htm) — Complete SQL syntax reference
- [Exasol Community](https://community.exasol.com) — Ask questions and share knowledge (use tag `exasol-personal`)
