"""Data check for slurm_m100.py: what nodes draw on days the job table shows them with no job.

If the job table is complete, such node-days are idle and draw the idle level (about 540 W on
Marconi100). Busy-level power on them means jobs are missing. Prints, per month: node-days with no
job, their median power, the share of them above 700 W, and the median power of all node-days.

    uv run --no-project --with duckdb python -I slurm_m100_nodeday.py <dir> <spill dir>
"""
import sys, duckdb
d = sys.argv[1]
c = duckdb.connect()
c.sql(f"SET threads=4; SET memory_limit='8GB'; SET temp_directory='{sys.argv[2]}'")
jt = f"{d}/year_month=*/plugin=job_table/*/*.parquet"
ps = f"{d}/year_month=*/plugin=ipmi_pub/metric=ps%d_input_power/*.parquet"
c.sql(f"""CREATE TABLE jn AS SELECT DISTINCT trim(n) AS node, floor(epoch(start_time)/86400)*86400 AS d0,
          floor(epoch(end_time)/86400)*86400 AS d1
          FROM (SELECT unnest(string_split(trim(nodes,'[]'),',')) AS n, start_time, end_time
                FROM read_parquet('{jt}') WHERE nodes IS NOT NULL AND nodes<>'[]' AND end_time>start_time)""")
c.sql(f"""CREATE TABLE nd AS
  WITH p AS (SELECT node, floor(epoch(timestamp)/86400)*86400 AS d, AVG(value) w FROM read_parquet('{ps % 0}') GROUP BY ALL
             UNION ALL
             SELECT node, floor(epoch(timestamp)/86400)*86400, AVG(value) FROM read_parquet('{ps % 1}') GROUP BY ALL)
  SELECT node, d, SUM(w) w FROM p GROUP BY ALL HAVING COUNT(*)=2""")
r = c.sql("""SELECT strftime(to_timestamp(nd.d),'%y-%m') m,
       COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM jn WHERE jn.node=nd.node AND nd.d BETWEEN jn.d0 AND jn.d1)) AS empty_days,
       median(w) FILTER (WHERE NOT EXISTS (SELECT 1 FROM jn WHERE jn.node=nd.node AND nd.d BETWEEN jn.d0 AND jn.d1)) AS empty_w,
       AVG((w>700)::INT) FILTER (WHERE NOT EXISTS (SELECT 1 FROM jn WHERE jn.node=nd.node AND nd.d BETWEEN jn.d0 AND jn.d1)) AS empty_hi,
       median(w) all_w, COUNT(*) n
       FROM nd GROUP BY 1 ORDER BY 1""").fetchall()
for x in r:
    print(x)
