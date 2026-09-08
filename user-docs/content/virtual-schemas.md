# Virtual schemas on local deployments

Virtual schemas let you query another database as if its objects were an Exasol schema. The
[Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
describes the concepts, the available adapters, and the properties each one supports. Follow it for
the SQL that creates connections, adapter scripts, and virtual schemas.

This guide covers only what a local deployment needs before that standard SQL flow works: an adapter
runtime and the adapter's files staged where the database can read them. Cloud deployments already
include the script language containers and driver locations, so use the Exasol documentation
directly for those.

## Where local deployments keep database files

Your deployment directory exposes the database's `/exa` directory at `local/runtime/exa` on Linux,
macOS, and Windows. Stage adapter files by writing them there from your computer.

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
EXA_DATA="$DEPLOY_DIR/local/runtime/exa"
```

This command requires `jq`. Pass the same `--deployment` or `--deployment-dir` selector to `info`
when working with a named or explicitly selected deployment.

Two locations below `EXA_DATA` matter for a JDBC virtual schema:

| Files | Path on your computer | Read by |
| --- | --- | --- |
| Adapter and driver JARs | `$EXA_DATA/bucketfs/<service>/<bucket>/<dir>/` | the adapter script, through its `%jar` lines |
| Driver JAR and `settings.cfg` | `$EXA_DATA/jdbc/<DRIVERNAME>/` | the ETL layer, for `IMPORT` and `EXPORT` |

`$EXA_DATA/bucketfs` is the deployment's
[BucketFS](https://docs.exasol.com/db/latest/database_concepts/bucketfs/bucketfs.htm). Creating a
directory below it creates the matching bucket, and SQL addresses its contents under `/buckets`. A
JAR written to `$EXA_DATA/bucketfs/bfsdefault/default/vs/example.jar` is therefore referenced by an
adapter script as `/buckets/bfsdefault/default/vs/example.jar`.

The driver JAR belongs in both locations because the adapter reads it for metadata while the ETL
layer uses the registered copy to transfer data. Exasol's
[Add JDBC Driver](https://docs.exasol.com/db/latest/administration/on-premise/manage_drivers/add_jdbc_driver.htm)
guide keeps the driver and `settings.cfg` in BucketFS; local deployments use
`$EXA_DATA/jdbc/<DRIVERNAME>/` instead, with the same `settings.cfg` keys and meanings.

## Install the adapter runtime

JDBC adapters run as Java UDFs, and JDBC data transfer needs a Java runtime. Install the Java SLC
without restarting yet:

```bash
exasol slc install java --no-restart --deployment-dir "$DEPLOY_DIR"
```

`--no-restart` records the container and applies it on the next start. Stage the adapter files
before that start so the whole setup needs only one restart. Skip this step if the required SLC is
already installed, and use the matching language for an adapter implemented in another one. A
[custom SLC](udfs.md#install-a-custom-container) can supply packages missing from the official
container.

## PostgreSQL example

Check the [PostgreSQL virtual schema releases](https://github.com/exasol/postgresql-virtual-schema)
and [PostgreSQL JDBC downloads](https://jdbc.postgresql.org/download/) for current versions. The
versions below are examples; update them together when using newer compatible artifacts, and
substitute the resulting filenames throughout.

### Stage the adapter and driver

Write both JARs into BucketFS and register the driver for the ETL layer:

```bash
ADAPTER_VERSION=4.0.2
ADAPTER_JAR=virtual-schema-dist-14.0.5-postgresql-4.0.2.jar
DRIVER_JAR=postgresql-42.7.13.jar

BUCKET_DIR="$EXA_DATA/bucketfs/bfsdefault/default/vs"
JDBC_DIR="$EXA_DATA/jdbc/POSTGRESQL"
mkdir -p "$BUCKET_DIR" "$JDBC_DIR"

curl -L -o "$BUCKET_DIR/$ADAPTER_JAR" \
  "https://github.com/exasol/postgresql-virtual-schema/releases/download/$ADAPTER_VERSION/$ADAPTER_JAR"
curl -L -o "$BUCKET_DIR/$DRIVER_JAR" \
  "https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/$DRIVER_JAR"

cp "$BUCKET_DIR/$DRIVER_JAR" "$JDBC_DIR/"
```

Create `$JDBC_DIR/settings.cfg` with an empty final line:

```text
DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR=postgresql-42.7.13.jar

```

`DRIVERNAME` is the name used in `IMPORT ... DRIVER = '...'`.

### Restart once

Apply the pending script language container and the driver registration:

```bash
exasol stop --deployment-dir "$DEPLOY_DIR"
exasol start --deployment-dir "$DEPLOY_DIR"
```

### Create the connection and virtual schema

The PostgreSQL host must be reachable from the Exasol runtime. `localhost` and `127.0.0.1` refer to
that runtime, not to the computer running PostgreSQL, so use an address the runtime can reach and
verify connectivity before creating the virtual schema.

Create `$DEPLOY_DIR/create-vs.sql`, replacing the host, credentials, and source schema:

```sql
CREATE OR REPLACE CONNECTION POSTGRES_CONN
  TO 'jdbc:postgresql://<postgres-host>:5432/<database>'
  USER '<user>' IDENTIFIED BY '<password>';

CREATE SCHEMA IF NOT EXISTS VS;

CREATE OR REPLACE JAVA ADAPTER SCRIPT VS.POSTGRES_ADAPTER AS
  %scriptclass com.exasol.adapter.RequestDispatcher;
  %jvmoption -Duser.timezone=UTC;
  %jar /buckets/bfsdefault/default/vs/virtual-schema-dist-14.0.5-postgresql-4.0.2.jar;
  %jar /buckets/bfsdefault/default/vs/postgresql-42.7.13.jar;
/

CREATE VIRTUAL SCHEMA VS_POSTGRES
  USING VS.POSTGRES_ADAPTER
  WITH CONNECTION_NAME = 'POSTGRES_CONN'
       SCHEMA_NAME = '<source-schema>';
```

Run it against the selected deployment:

```bash
exasol connect --deployment-dir "$DEPLOY_DIR" -f "$DEPLOY_DIR/create-vs.sql"
```

Query the virtual schema like any other schema, and refresh it after the source schema changes:

```sql
ALTER VIRTUAL SCHEMA VS_POSTGRES REFRESH;
```

## Other adapters

For another JDBC dialect, substitute the adapter JAR, driver JAR, and the `DRIVERNAME`, `PREFIX`,
`DRIVERMAIN`, and `JAR` settings. For a non-JDBC adapter, install its required SLC and follow the
adapter's dependency and script-definition instructions. The
[Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
lists available adapters and their properties.

## Troubleshooting

**`ETL-1014: No default DRIVER registered`**: Check that `$EXA_DATA/jdbc/<DRIVERNAME>/` contains the
JAR and `settings.cfg`, that the configuration ends with an empty line, and that you restarted the
deployment.

**The adapter script cannot find its class**: Check that every `%jar` path resolves below `/buckets`
and that the files exist in the corresponding BucketFS directory.

**Adapter execution or `IMPORT` reports no runtime**: Run `exasol slc list` and confirm that the
adapter's SLC is installed. JDBC adapters require the Java SLC.
