# Trace queries

Queries for the [trace analysis plan](../../docs/research/trace-analysis-plan.md). They run on the
public Google 2019 cluster trace in BigQuery (`google.com:google-cluster-data`), CC-BY 4.0.

| File | What it does | Bytes read (cell a) |
|---|---|---|
| `google2019_cell_check.sql` | Tier totals on a 1% block sample, to check the query before the full scan | about 5 GB |
| `google2019_cell.sql` | One scan: per-service hourly usage and requests, tier totals and power-domain totals per 5 minutes | about 507 GB |

The cell is written into each query as `clusterdata_2019_a`; replace it for another cell.

    bq query --use_legacy_sql=false --dry_run < google2019_cell.sql
    bq query --use_legacy_sql=false --maximum_bytes_billed=520000000000 \
      --destination_table=<project>:<dataset>.cell_a < google2019_cell.sql

Always dry-run first and keep the byte cap. Queries are passed on standard input, because `bq`
reads a leading `#` or `--` line on the command line as a flag.
