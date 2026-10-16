"""Check 2 of docs/research/trace-analysis-plan.md: hourly usage by tier, ours against Google's.

Compares the tier rows of google2019_cell.sql (5-minute buckets aligned to PT) with the output of
google2019_notebook_tiers.sql (Google's notebook method, hours of raw trace time). Our buckets are
summed into the notebook's hours (bucket b covers raw time 300 b + 600 s) and divided by the same
cell capacity. The first and last hour are partial and left out.

    uv run --no-project --with duckdb python -I google2019_check2.py <cell_a.csv> <notebook_tiers.csv>
"""
import sys, duckdb

ours_csv, nb_csv = sys.argv[1], sys.argv[2]
c = duckdb.connect()
c.sql(f"""CREATE TABLE nb AS SELECT hour_index h, substr(tier, 3) tier, avg_cpu_usage cpu,
  avg_memory_usage mem, cpu_capacity cc, memory_capacity mc FROM read_csv('{nb_csv}', header=true)""")
cc, mc = c.sql("SELECT any_value(cc), any_value(mc) FROM nb").fetchone()
cols = {"kind": "VARCHAR", "k": "VARCHAR", "t": "BIGINT", "cpu_use": "DOUBLE", "cpu_use_max5": "DOUBLE",
        "cpu_req": "DOUBLE", "mem_use": "DOUBLE", "mem_req": "DOUBLE", "vscale": "VARCHAR"}
c.sql(f"""CREATE TABLE ours AS SELECT floor((t + 2) / 12)::INT h, k tier,
  SUM(cpu_use) / 12 / {cc} cpu, SUM(mem_use) / 12 / {mc} mem
  FROM read_csv('{ours_csv}', header=true, columns={cols}) WHERE kind = 'tier' GROUP BY 1, 2""")
c.sql("""CREATE TABLE j AS SELECT tier, h, n.cpu nc, o.cpu oc, n.mem nm, o.mem om
  FROM nb n JOIN ours o USING (tier, h)""")
c.sql("DELETE FROM j WHERE h = (SELECT min(h) FROM j) OR h = (SELECT max(h) FROM j)")

print("tier   hours  r CPU   r mem   ours/Google CPU  mem   (pass: r >= 0.95, totals within 10%)")
for tier, n, rc, rm, qc, qm in c.sql("""SELECT tier, count(*), corr(nc, oc), corr(nm, om),
    sum(oc) / sum(nc), sum(om) / sum(nm) FROM j GROUP BY 1 ORDER BY 1""").fetchall():
    ok = "" if tier not in ("mid", "prod") else (
        "pass" if min(rc, rm) >= 0.95 and all(0.9 <= q <= 1.1 for q in (qc, qm)) else "FAIL")
    print(f"{tier:5} {n:6} {rc:7.4f} {rm:7.4f} {qc:13.3f} {qm:7.3f}   {ok}")

print("\nproduction memory, daily: Google's method, ours, ratio")
for d, a, b in c.sql("""SELECT h // 24, avg(nm), avg(om) FROM j WHERE tier = 'prod'
    GROUP BY 1 ORDER BY 1""").fetchall():
    print(f"  day {d:2}: {a:.4f} {b:.4f} {b / a:.3f}")
sd = c.sql("SELECT stddev_pop(nm), stddev_pop(om - nm) FROM j WHERE tier = 'prod'").fetchone()
print(f"production memory: hourly SD {sd[0]:.4f} of capacity, SD of the difference {sd[1]:.4f}")
