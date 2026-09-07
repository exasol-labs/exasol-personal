# Load sample data

Exasol provides sample product and review data in Parquet files hosted on Amazon S3.

Run the bundled sample script from a deployment directory:

```bash
exasol connect -f sample.sql
```

For a named deployment, add `-d <name>`.

Alternatively, connect with a SQL client and run:

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

Exasol infers the table schemas from the Parquet files.

Run the statement through `exasol connect -c` or place it in a file passed to `exasol connect -f`.
The `LOCAL` form reads the file from the computer running the client. It does not change the
database engine's object-storage Parquet behavior.

| Table | Rows | Size |
| --- | ---: | ---: |
| `PRODUCTS` | 1,000,000 | 27.3 MiB |
| `PRODUCT_REVIEWS` | 1,822,007 | 154.5 MiB |

See [Load data](https://docs.exasol.com/db/latest/loading_data.htm) for the other supported import
methods.
