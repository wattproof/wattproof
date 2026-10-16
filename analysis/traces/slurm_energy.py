"""Energy of idle node time and what powering it off would save, from a Slurm job table and node
power samples. Implements the energy step of docs/research/slurm-trace-plan.md on SURF Lisa:
the job table as in slurm_idle.py, and node metrics reduced to the columns node, timestamp,
timestamp_seconds, up, node_power_usage, nvidia_gpu_power_usage_milliwatts_sum,
node_rapl_package_power_sum, node_boot_time_seconds (one or more parquet files).

    uv run --no-project --with duckdb python -I slurm_energy.py <job dir> <power glob> --tmp <dir>
"""

import argparse
import collections
import os
import statistics
import sys

import duckdb

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from slurm_idle import DAY, HOUR, UNAVAILABLE, expand, load, merge

EDGE = 5 * 60
SUSPEND_MIN = (10, 30, 60)
BOOT_MIN = (5, 15)
OFF_W = (10, 30)
OFFSETS_H = range(-3, 4)


def busy_intervals(jobs, w0, w1):
    busy = collections.defaultdict(list)
    for _, hl, s, e, _ in jobs:
        if s is not None and e is not None and e > s and e > w0 and s < w1:
            for n in expand(hl):
                busy[n].append((max(s, w0), min(e, w1)))
    return {n: merge(iv) for n, iv in busy.items()}


def idle_intervals(busy, nodes, w0, w1):
    out = {}
    for n in nodes:
        iv, t = [], w0
        for s, e in busy.get(n, []):
            if s > t:
                iv.append((t, s, True))
            t = max(t, e)
        if t < w1:
            iv.append((t, w1, False))
        out[n] = [x for x in iv if x[1] - x[0] < UNAVAILABLE]
    return out


def draws(con, idle, nodes, edge=EDGE):
    """Median power per node in idle minutes (at least edge inside an idle interval) and busy
    minutes (outside every idle interval), computed in DuckDB over the table ps."""
    con.sql("CREATE OR REPLACE TABLE iv (node VARCHAR, s DOUBLE, e DOUBLE)")
    con.executemany("INSERT INTO iv VALUES (?, ?, ?)", [(n, s, e) for n in nodes for s, e, _ in idle[n]])
    con.sql("CREATE OR REPLACE TABLE sel (node VARCHAR)")
    con.executemany("INSERT INTO sel VALUES (?)", [(n,) for n in nodes])
    rows = con.sql(f"""
        WITH c AS (
          SELECT ps.node, ps.p, ps.pg, ps.pr,
                 CASE WHEN iv.node IS NULL THEN 'busy'
                      WHEN ps.t >= iv.s + {edge} AND ps.t < iv.e - {edge} THEN 'idle' END AS st
          FROM ps JOIN sel USING (node)
          LEFT JOIN iv ON ps.node = iv.node AND ps.t >= iv.s AND ps.t < iv.e)
        SELECT node, st, median(p), median(pg), median(pr), COUNT(*) FROM c
        WHERE st IS NOT NULL GROUP BY node, st""").fetchall()
    d = collections.defaultdict(dict)
    for n, st, p, pg, pr, c in rows:
        if c >= 20:
            d[n][st] = (p, pg or 0.0, pr or 0.0)
    return {n: v for n, v in d.items() if "idle" in v and "busy" in v}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("jobdir")
    ap.add_argument("power")
    ap.add_argument("--tmp", required=True, help="directory DuckDB may spill to")
    a = ap.parse_args()

    jobs, gpu = load(a.jobdir)
    con = duckdb.connect()
    con.sql(f"SET threads=4; SET memory_limit='8GB'; SET temp_directory='{a.tmp}'")
    con.sql(f"""CREATE TABLE ps AS
        SELECT node, timestamp_seconds::DOUBLE AS t, node_power_usage AS p,
               nvidia_gpu_power_usage_milliwatts_sum / 1000 AS pg,
               node_rapl_package_power_sum AS pr, node_boot_time_seconds AS boot,
               epoch(timestamp) - timestamp_seconds AS naive
        FROM read_parquet('{a.power}')
        WHERE node_power_usage IS NOT NULL AND node_power_usage > 0""")
    n_s, p0, p1, naive = con.sql("SELECT COUNT(*), MIN(t), MAX(t), median(naive) FROM ps").fetchone()
    print(f"power samples {n_s:,}, {(p1 - p0) / DAY:.1f} days; naive timestamp minus epoch "
          f"seconds, median {naive / HOUR:+.2f} h")
    vals = con.sql("""SELECT p, COUNT(*) c FROM ps GROUP BY p ORDER BY c DESC LIMIT 8""").fetchall()
    print("most frequent node_power_usage values (W, count):", vals)

    starts = [s for _, _, s, e, _ in jobs if s is not None and e is not None and e > s]
    w0, w1 = max(min(starts) + 7 * DAY, p0), p1
    print(f"analysis window {(w1 - w0) / DAY:.1f} days")
    nodes = sorted(n for (n,) in con.sql("SELECT DISTINCT node FROM ps").fetchall() if n in gpu)

    # Align job time to power time; ties go to the smaller shift.
    best = None
    for oh in OFFSETS_H:
        off = oh * HOUR
        shifted = [(j, hl, s + off if s else s, e + off if e else e, sub) for j, hl, s, e, sub in jobs]
        idle = idle_intervals(busy_intervals(shifted, w0, w1), nodes, w0, w1)
        d = draws(con, idle, nodes[::5])
        gap = [v["busy"][0] - v["idle"][0] for v in d.values()]
        score = statistics.median(gap) if gap else float("-inf")
        print(f"   job offset {oh:+d} h: median busy minus idle draw {score:6.1f} W")
        if best is None or score > best[1] or (score == best[1] and abs(off) < abs(best[0])):
            best = (off, score, idle)
    off, _, idle = best
    print(f"job times shifted by {off / HOUR:+.0f} h")
    draw = draws(con, idle, nodes)
    reboots = dict(con.sql("""SELECT node, COUNT(DISTINCT round(boot / 600)) - 1 FROM ps
                              WHERE boot IS NOT NULL GROUP BY node""").fetchall())
    node_e = dict(con.sql(f"""
        SELECT node, SUM(p * dt) FROM (
          SELECT node, p, lead(t) OVER (PARTITION BY node ORDER BY t) - t AS dt FROM ps
          WHERE t >= {w0} AND t < {w1})
        WHERE dt <= 120 GROUP BY node""").fetchall())

    for cls in (False, True):
        name = "GPU" if cls else "CPU"
        ns = [n for n in nodes if gpu[n] == cls and n in draw]
        if not ns:
            continue
        idle_w = sorted(draw[n]["idle"][0] for n in ns)
        busy_w = sorted(draw[n]["busy"][0] for n in ns)
        q = lambda v, f: v[min(len(v) - 1, int(f * len(v)))]
        print(f"\n== {name} nodes with draws: {len(ns)} of {sum(1 for n in nodes if gpu[n] == cls)}")
        print(f"   idle draw median {q(idle_w, .5):.0f} W (10th-90th {q(idle_w, .1):.0f}-"
              f"{q(idle_w, .9):.0f}); busy median {q(busy_w, .5):.0f} W "
              f"({q(busy_w, .1):.0f}-{q(busy_w, .9):.0f}); idle/busy "
              f"{q(idle_w, .5) / q(busy_w, .5):.0%}")
        if cls:
            g = sorted(draw[n]["idle"][1] for n in ns)
            print(f"   GPU boards at idle (NVML, class D): median {q(g, .5):.0f} W")
        r = sorted(draw[n]["idle"][2] for n in ns)
        print(f"   CPU packages at idle (RAPL, class D): median {q(r, .5):.0f} W")
        rb = [reboots.get(n, 0) for n in ns]
        print(f"   reboots per node in power window: median {statistics.median(rb):.0f}, max {max(rb)}")
        e_tot = sum(node_e.get(n, 0) for n in ns) / 3.6e6
        print(f"   measured energy of these nodes in the window: {e_tot:,.0f} kWh")
        for b in BOOT_MIN:
            for sm in SUSPEND_MIN:
                S, B = sm * 60, b * 60
                for ow in OFF_W:
                    save_r = save_f = 0.0
                    for n in ns:
                        pi, pb = draw[n]["idle"][0], draw[n]["busy"][0]
                        for s, e, ends in idle[n]:
                            L = e - s
                            cyc = B * max(0.0, pb - pi) if ends else 0.0
                            if L > S:
                                save_r += (L - S) * (pi - ow) - cyc
                            if L - B > S:
                                save_f += (L - S - (B if ends else 0)) * (pi - ow) - cyc
                    print(f"   boot {b:2d} min, SuspendTime {sm:2d} min, off {ow} W: saving "
                          f"{save_r / 3.6e6:8,.0f} kWh ({save_r / 3.6e6 / e_tot:5.1%}) reactive, "
                          f"{save_f / 3.6e6:8,.0f} kWh ({save_f / 3.6e6 / e_tot:5.1%}) forecast")


if __name__ == "__main__":
    sys.exit(main())
