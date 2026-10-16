"""Idle node time, its power, and what powering it off would save on CINECA's Marconi100.

Implements docs/research/slurm-trace-plan.md on M100 ExaData (Zenodo 7588815 for 2020, 7589131 for 2021; CC-BY-4.0):
per month, `plugin=job_table/metric=job_info_marconi100` and `plugin=ipmi_pub/metric=ps0_input_power`
and `ps1_input_power` (the node's two power supplies at the BMC, class C), under one directory
laid out as in the archive (`year_month=YY-MM/plugin=.../metric=.../a_0.parquet`).

    uv run --no-project --with duckdb python -I slurm_m100.py <dir>
"""

import argparse
import collections
import os
import statistics
import sys

import duckdb

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from slurm_idle import BOOT_MIN, DAY, HOUR, SUSPEND_MIN, UNAVAILABLE, merge
from slurm_replace import replace

EDGE = 5 * 60
OFF_W = (10, 30)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("dir")
    ap.add_argument("--threads", type=int, default=4)
    ap.add_argument("--memory", default="8GB")
    ap.add_argument("--tmp", required=True, help="directory DuckDB may spill to")
    ap.add_argument("--check", choices=("auto", "power"), default="auto",
                    help="complete days by Slurm's counts where present (auto), or by power only")
    a = ap.parse_args()
    con = duckdb.connect()
    con.sql(f"SET threads={a.threads}; SET memory_limit='{a.memory}'; SET temp_directory='{a.tmp}'")
    jt = f"{a.dir}/year_month=*/plugin=job_table/*/*.parquet"
    ps = f"{a.dir}/year_month=*/plugin=ipmi_pub/metric=ps%d_input_power/*.parquet"

    # Jobs: one row per job id (monthly tables overlap), nodes as a list of indices.
    con.sql(f"""CREATE TABLE jobs AS
        SELECT job_id, ANY_VALUE(nodes) AS nodes, ANY_VALUE(epoch(start_time)) AS s,
               ANY_VALUE(epoch(end_time)) AS e, ANY_VALUE(epoch(submit_time)) AS sub
        FROM read_parquet('{jt}') GROUP BY job_id""")
    n_all = con.sql("SELECT COUNT(*) FROM jobs").fetchone()[0]
    rows = con.sql("""SELECT job_id, nodes, s, e FROM jobs
                      WHERE s IS NOT NULL AND e IS NOT NULL AND e > s AND nodes IS NOT NULL
                        AND nodes <> '[]'""").fetchall()
    print(f"jobs {n_all:,}; used {len(rows):,}; dropped {n_all - len(rows):,}")
    longest = max(e - s for _, _, s, e in rows)
    p_rng = con.sql(f"SELECT MIN(epoch(timestamp)), MAX(epoch(timestamp)) FROM read_parquet('{ps % 0}')").fetchone()
    warm = max(7 * DAY, longest)
    w0 = max(min(s for _, _, s, _ in rows) + warm, p_rng[0])
    w1 = min(max(e for _, _, _, e in rows) - DAY, p_rng[1])
    print(f"longest job {longest / DAY:.2f} days; window {(w1 - w0) / DAY:.1f} days")

    busy = collections.defaultdict(list)
    starts = collections.defaultdict(set)
    for j, nodes, s, e in rows:
        if e > w0 and s < w1:
            for n in nodes.strip("[]").split(","):
                n = n.strip()
                busy[n].append((max(s, w0), min(e, w1)))
                if w0 <= s < w1:
                    starts[n].add((s, j))
    nodes = sorted(busy, key=int)
    raw = {}
    for n in nodes:
        iv, t = [], w0
        for s, e in merge(busy[n]):
            if s > t:
                iv.append((t, s, True))
            t = max(t, e)
        if t < w1:
            iv.append((t, w1, False))
        raw[n] = iv

    # Power: ps0 + ps1 per node and minute, over the whole window.
    con.sql(f"""CREATE TABLE pm AS
        WITH p AS (
          SELECT node, floor(epoch(timestamp) / 60) * 60 AS m, AVG(value) AS w
          FROM read_parquet('{ps % 0}') WHERE epoch(timestamp) >= {w0} AND epoch(timestamp) < {w1}
          GROUP BY ALL
          UNION ALL
          SELECT node, floor(epoch(timestamp) / 60) * 60, AVG(value)
          FROM read_parquet('{ps % 1}') WHERE epoch(timestamp) >= {w0} AND epoch(timestamp) < {w1}
          GROUP BY ALL)
        SELECT node, m, SUM(w) AS w FROM p GROUP BY node, m HAVING COUNT(*) = 2""")
    con.sql("CREATE TABLE iv (node VARCHAR, s DOUBLE, e DOUBLE)")
    con.sql("CREATE TABLE spans (s DOUBLE, e DOUBLE)")

    def draws(intervals, restrict):
        """Median power per node in idle minutes (at least EDGE inside an idle interval) and in
        busy minutes (outside every idle interval); only minutes inside spans if restrict."""
        con.sql("DELETE FROM iv")
        con.executemany("INSERT INTO iv VALUES (?, ?, ?)",
                        [(n, s, e) for n in nodes for s, e, _ in intervals[n] if e - s < UNAVAILABLE])
        where = "WHERE EXISTS (SELECT 1 FROM spans WHERE pm.m >= spans.s AND pm.m < spans.e)" if restrict else ""
        out = collections.defaultdict(dict)
        for n, st, w, c in con.sql(f"""
            WITH c AS (
              SELECT pm.node, pm.w,
                     CASE WHEN iv.node IS NULL THEN 'busy'
                          WHEN pm.m >= iv.s + {EDGE} AND pm.m < iv.e - {EDGE} THEN 'idle' END AS st
              FROM pm LEFT JOIN iv ON pm.node = iv.node AND pm.m >= iv.s AND pm.m < iv.e {where})
            SELECT node, st, median(w), COUNT(*) FROM c WHERE st IS NOT NULL GROUP BY node, st""").fetchall():
            if c >= 20:
                out[n][st] = w
        return {n: v for n, v in out.items() if "idle" in v and "busy" in v}

    # Complete days. The job table has collection gaps, and a missing job reads as idle time
    # (plan change of 2026-10-16). Two checks per day:
    #  counts: the job table's busy node-days within 25% of Slurm's allocated plus mixed nodes
    #          (slurm_pub counts; 2020 only);
    #  power:  the measured mean node power within 10% of what the job table predicts from each
    #          node class's idle and busy draw (all periods; calibrated against counts on 2020).
    pre = draws(raw, False)
    mi = statistics.median(v["idle"] for v in pre.values())
    mb = statistics.median(v["busy"] for v in pre.values())
    con.sql(f"""CREATE TABLE days AS
        SELECT floor(m / 86400) * 86400 AS d, AVG(w) AS mean_w, COUNT(DISTINCT node) AS nn
        FROM pm GROUP BY 1""")
    con.sql("""CREATE TABLE jb AS
        SELECT days.d, SUM(len(string_split(trim(j.nodes, '[]'), ',')) *
               GREATEST(0, LEAST(j.e, days.d + 86400) - GREATEST(j.s, days.d))) / 86400 AS busy
        FROM days JOIN jobs j ON j.s < days.d + 86400 AND j.e > days.d AND j.e > j.s GROUP BY 1""")
    sp = f"{a.dir}/year_month=*/plugin=slurm_pub/metric=%s/*.parquet"
    try:
        con.sql(f"""CREATE TABLE sc AS
            SELECT floor(epoch(a.timestamp) / 86400) * 86400 AS d, AVG(a.value) + AVG(m.value) AS sb
            FROM read_parquet('{sp % "total_nodes_alloc"}') a
            JOIN read_parquet('{sp % "total_nodes_mixed"}') m USING (timestamp) GROUP BY 1""")
    except duckdb.IOException:
        con.sql("CREATE TABLE sc (d DOUBLE, sb DOUBLE)")
    dl = con.sql("""SELECT days.d, mean_w, COALESCE(busy, 0), sb FROM days
                    LEFT JOIN jb USING (d) LEFT JOIN sc USING (d) ORDER BY 1""").fetchall()
    N = len(nodes)
    days_in = [(d, mw, jb, sb) for d, mw, jb, sb in dl if w0 <= d and d + 86400 <= w1]
    ok_p = {d for d, mw, jb, sb in days_in if abs(mw / (mi + (mb - mi) * jb / N) - 1) <= 0.10}
    ok_c = {d for d, mw, jb, sb in days_in if sb and 0.8 <= jb / sb <= 1.25}
    with_c = [d for d, *_, sb in days_in if sb]
    print(f"days in window {len(days_in)}; complete by power {len(ok_p)}; with Slurm counts "
          f"{len(with_c)}, complete by counts {len(ok_c)}")
    if with_c:
        agree = sum((d in ok_p) == (d in ok_c) for d in with_c)
        print(f"power check agrees with counts check on {agree} of {len(with_c)} days "
              f"({agree / len(with_c):.0%}); power-complete days that counts reject: "
              f"{len([d for d in with_c if d in ok_p and d not in ok_c])}")
    by_counts = bool(with_c) and a.check == "auto"
    good = sorted(ok_c) if by_counts else sorted(ok_p)
    print("complete days by:", "counts" if by_counts else "power")
    spans = merge([[d, d + 86400] for d in good])
    cover = sum(e - s for s, e in spans)
    con.executemany("INSERT INTO spans VALUES (?, ?)", spans)

    def clip(iv):
        """Parts of idle intervals on complete days; a part keeps 'ends in a job' only if it
        ends where the interval did."""
        out = []
        for s, e, ends in iv:
            for cs, ce in spans:
                lo, hi = max(s, cs), min(e, ce)
                if lo < hi:
                    out.append((lo, hi, ends and hi == e))
        return out

    idle = {n: clip(raw[n]) for n in nodes}
    jobs_started = {j for n in nodes for t, j in starts[n] if any(cs <= t < ce for cs, ce in spans)}
    print(f"nodes {len(nodes)}; job starts on complete days {len(jobs_started):,}")

    # Idle share by calendar month (all gaps counted), to compare with Slurm's own node counts.
    import datetime as dt
    mon = collections.Counter()
    tot = collections.Counter()
    for n in nodes:
        for s, e, _ in idle[n]:
            t = s
            while t < e:
                d = dt.datetime.fromtimestamp(t, dt.timezone.utc)
                nxt = dt.datetime(d.year + (d.month == 12), d.month % 12 + 1, 1, tzinfo=dt.timezone.utc).timestamp()
                mon[d.strftime("%y-%m")] += min(e, nxt) - t
                t = min(e, nxt)
    for cs, ce in spans:
        t = cs
        while t < ce:
            d = dt.datetime.fromtimestamp(t, dt.timezone.utc)
            nxt = dt.datetime(d.year + (d.month == 12), d.month % 12 + 1, 1, tzinfo=dt.timezone.utc).timestamp()
            tot[d.strftime("%y-%m")] += (min(ce, nxt) - t) * len(nodes)
            t = min(ce, nxt)
    print("idle share by month on complete days, all gaps idle:",
          ", ".join(f"{m} {mon[m] / tot[m]:.0%}" for m in sorted(tot)))
    sl = collections.defaultdict(list)
    for d, mw, jb, sb in days_in:
        if sb and d in set(good):
            sl[dt.datetime.fromtimestamp(d, dt.timezone.utc).strftime("%y-%m")].append(1 - sb / N)
    if sl:
        print("Slurm's own idle-or-down share on the same days:",
              ", ".join(f"{m} {statistics.fmean(v):.0%}" for m, v in sorted(sl.items())))

    d = draws(idle, True)
    energy = dict(con.sql("""SELECT node, SUM(w) * 60 FROM pm
        WHERE EXISTS (SELECT 1 FROM spans WHERE pm.m >= spans.s AND pm.m < spans.e) GROUP BY node""").fetchall())
    ns = [n for n in nodes if n in d]
    q = lambda v, f: sorted(v)[min(len(v) - 1, int(f * len(v)))]
    iw = [d[n]["idle"] for n in ns]
    bw = [d[n]["busy"] for n in ns]
    print(f"\nnodes with both draws: {len(ns)}")
    print(f"idle draw median {q(iw, .5):.0f} W (10th-90th {q(iw, .1):.0f}-{q(iw, .9):.0f}); "
          f"busy {q(bw, .5):.0f} W ({q(bw, .1):.0f}-{q(bw, .9):.0f}); idle/busy {q(iw, .5) / q(bw, .5):.0%}")
    e_tot = sum(energy.get(n, 0) for n in ns) / 3.6e6
    print(f"measured energy of these nodes: {e_tot:,.0f} kWh")

    total = len(nodes) * cover
    for label, use_unav in (("72 h+ gaps unavailable", False), ("72 h+ gaps idle", True)):
        unav = sum(e - s for n in nodes for s, e, _ in idle[n] if e - s >= UNAVAILABLE)
        base = total if use_unav else total - unav
        ivs = {n: [x for x in idle[n] if use_unav or x[1] - x[0] < UNAVAILABLE] for n in nodes}
        idle_s = sum(e - s for n in nodes for s, e, _ in ivs[n])
        print(f"\n-- {label}: available node-hours {base / HOUR:,.0f}, idle {idle_s / base:.1%}")
        for b in BOOT_MIN:
            for sm in SUSPEND_MIN:
                S, B = sm * 60, b * 60
                off_r = off_f = wakes = 0
                delayed = set()
                for n in nodes:
                    st = sorted(starts[n])
                    at = collections.defaultdict(list)
                    for t, j in st:
                        at[t].append(j)
                    for s, e, ends in ivs[n]:
                        L = e - s
                        if L > S:
                            off_r += L - S
                            if ends:
                                wakes += 1
                                delayed.update(at.get(e, []))
                        if L - B > S:
                            off_f += L - S - (B if ends else 0)
                print(f"   boot {b:2d} min, SuspendTime {sm:2d} min: off {off_r / base:5.1%} reactive, "
                      f"{off_f / base:5.1%} forecast; jobs delayed {len(delayed):6d} "
                      f"({len(delayed) / len(jobs_started):.2%}), wait {wakes * b / 60:,.0f} node-hours")
                if not use_unav and b == BOOT_MIN[0]:
                    at = {n: collections.defaultdict(list) for n in nodes}
                    for n in nodes:
                        for t, j in starts[n]:
                            at[n][t].append(j)
                    dl, av, left = replace(ivs, {n: "M100" for n in nodes}, at, S)
                    rest = len(dl) - len(av)
                    print(f"      re-placement: delayed {len(dl)} -> {rest} ({rest / len(jobs_started):.2%} "
                          f"of starts); wait " + " / ".join(f"{left * bb / 60:,.0f}" for bb in BOOT_MIN)
                          + f" node-hours ({' / '.join(str(bb) for bb in BOOT_MIN)} min boot)")
                if use_unav:
                    continue
                for ow in OFF_W:
                    sr = sf = 0.0
                    for n in ns:
                        pi, pb = d[n]["idle"], d[n]["busy"]
                        for s, e, ends in ivs[n]:
                            L = e - s
                            cyc = B * max(0.0, pb - pi) if ends else 0.0
                            if L > S:
                                sr += (L - S) * (pi - ow) - cyc
                            if L - B > S:
                                sf += (L - S - (B if ends else 0)) * (pi - ow) - cyc
                    print(f"      off {ow} W: saving {sr / 3.6e6:9,.0f} kWh ({sr / 3.6e6 / e_tot:5.1%}) "
                          f"reactive, {sf / 3.6e6:9,.0f} kWh ({sf / 3.6e6 / e_tot:5.1%}) forecast")


if __name__ == "__main__":
    sys.exit(main())
