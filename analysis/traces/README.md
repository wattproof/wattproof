# Trace queries

Queries for the [trace analysis plan](../../docs/research/trace-analysis-plan.md). They run on the
public Google 2019 cluster trace in BigQuery (`google.com:google-cluster-data`), CC-BY 4.0.

| File | What it does | Bytes read (cell a) |
|---|---|---|
| `google2019_cell_check.sql` | Tier totals on a 1% block sample, to check the query before the full scan | about 5 GB |
| `google2019_cell.sql` | One scan: per-service hourly usage and requests, tier totals and power-domain totals per 5 minutes | about 507 GB |
| `google2019_notebook_tiers.sql` | Check 2: hourly usage by tier with the method of Google's analysis notebook, unchanged | about 547 GB |

`google2019_check2.py` compares the two (check 2):

    uv run --no-project --with duckdb python -I google2019_check2.py <cell_a.csv> <notebook_tiers.csv>

The cell is written into each query as `clusterdata_2019_a`; replace it for another cell.

    bq query --use_legacy_sql=false --dry_run < google2019_cell.sql
    bq query --use_legacy_sql=false --maximum_bytes_billed=520000000000 \
      --destination_table=<project>:<dataset>.cell_a < google2019_cell.sql

Always dry-run first and keep the byte cap. Queries are passed on standard input, because `bq`
reads a leading `#` or `--` line on the command line as a flag.

## Slurm job tables

`slurm_idle.py` implements the [Slurm trace analysis plan](../../docs/research/slurm-trace-plan.md)
on SURF Lisa's job table (Zenodo 11028934, CC-BY-4.0). It reads parquet through DuckDB:

    uv run --no-project --with duckdb python -I slurm_idle.py <dir with the two parquet files>

`slurm_energy.py` adds Lisa's node power metrics (the energy step); it takes the job directory, a
glob of power parquet files and a spill directory:

    uv run --no-project --with duckdb python -I slurm_energy.py <dir> '<power glob>' --tmp <dir>

`slurm_m100.py` runs idle time, power and savings on CINECA Marconi100 (M100 ExaData, CC-BY-4.0),
from the monthly archives' `job_table`, `ipmi_pub` (`ps0_input_power`, `ps1_input_power`) and,
where present, `slurm_pub` node counts, laid out as in the archive:

    uv run --no-project --with duckdb python -I slurm_m100.py <dir> --tmp <dir>

`slurm_replace.py` is the re-placement check both idle scripts use: whether a start that finds its
node powered off could have used an idle node of the same hardware type that was still on.

`slurm_m100_nodeday.py` checks a Marconi100 period's job table against power: node-days with no
job should draw the idle level. It is the evidence that the 2021 job table misses running work.
