"""Idle node time and the cost of powering it off, from a Slurm job table.

Implements docs/research/slurm-trace-plan.md on SURF Lisa's job table (Zenodo 11028934,
CC-BY-4.0): slurm_table_cleaned.parquet and node_hardware_info.parquet. Reads parquet with
DuckDB; everything else is the standard library.

    uv run --no-project --with duckdb python -I slurm_idle.py <dir with the two parquet files>
"""

import argparse
import bisect
import collections
import datetime as dt
import sys

import duckdb

DAY = 86400
HOUR = 3600
UNAVAILABLE = 72 * HOUR
SUSPEND_MIN = (10, 30, 60)
BOOT_MIN = (5, 15)


def load(d):
    con = duckdb.connect()
    jobs = con.sql(
        f"""SELECT id, node, epoch(start_date) AS s, epoch(end_date) AS e,
                   TRY_CAST(submit AS BIGINT) AS sub
            FROM '{d}/slurm_table_cleaned.parquet'"""
    ).fetchall()
    gpu = dict(
        con.sql(
            f"SELECT node, gpu_count > 0 FROM '{d}/node_hardware_info.parquet'"
        ).fetchall()
    )
    return jobs, gpu


def expand(hostlist):
    """Slurm host list to node names: 'r25n[18,20-22],r27n4' -> r25n18 r25n20 r25n21 r25n22 r27n4."""
    parts, depth, cur = [], 0, ""
    for ch in hostlist:
        if ch == "," and depth == 0:
            parts.append(cur)
            cur = ""
            continue
        depth += ch == "["
        depth -= ch == "]"
        cur += ch
    parts.append(cur)
    out = []
    for part in parts:
        if "[" not in part:
            out.append(part)
            continue
        prefix, body = part.split("[", 1)
        for r in body.rstrip("]").split(","):
            if "-" in r:
                lo, hi = r.split("-")
                out += [f"{prefix}{i:0{len(lo)}d}" for i in range(int(lo), int(hi) + 1)]
            else:
                out.append(prefix + r)
    return out


def merge(iv):
    iv.sort()
    out = []
    for s, e in iv:
        if out and s <= out[-1][1]:
            out[-1][1] = max(out[-1][1], e)
        else:
            out.append([s, e])
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("dir")
    a = ap.parse_args()
    jobs, gpu = load(a.dir)

    kept, dropped = [], 0
    for j, n, s, e, sub in jobs:
        if s is None or e is None or n is None or e <= s:
            dropped += 1
        else:
            kept.append((j, n, s, e, sub))
    longest = max(e - s for _, _, s, e, _ in kept)
    warm = max(7 * DAY, longest)
    w0 = min(s for _, _, s, _, _ in kept) + warm
    w1 = max(e for _, _, _, e, _ in kept) - DAY
    print(f"rows {len(jobs)}, dropped {dropped} (missing or end <= start)")
    print(f"longest row {longest / DAY:.2f} days; warm-up {warm / DAY:.2f} days")
    print(f"window {dt.datetime.utcfromtimestamp(w0)} to {dt.datetime.utcfromtimestamp(w1)}"
          f" ({(w1 - w0) / DAY:.1f} days)")

    # Time zone rule: submit times (epoch, so UTC) by hour; minimum in 01-06 UTC means the
    # timestamps are UTC. Also report start minus submit, which shows the timestamps' offset.
    subh = collections.Counter()
    lag = []
    for _, _, s, _, sub in kept:
        if sub:
            subh[dt.datetime.utcfromtimestamp(sub).hour] += 1
            lag.append(s - sub)
    hmin = min(range(24), key=lambda h: subh[h])
    lag.sort()
    print(f"submissions by UTC hour: minimum at {hmin:02d}:00")
    print(f"start_date minus submit epoch, median {lag[len(lag) // 2] / HOUR:.2f} h, "
          f"10th percentile {lag[len(lag) // 10] / HOUR:.2f} h")
    utc = 1 <= hmin < 6
    print("timestamps taken as", "UTC" if utc else "local time")

    busy = collections.defaultdict(list)
    starts = collections.defaultdict(list)  # node -> [(start, job id)]
    multi = 0
    for j, hl, s, e, _ in kept:
        if e > w0 and s < w1:
            names = expand(hl)
            multi += len(names) > 1
            for n in names:
                busy[n].append((max(s, w0), min(e, w1)))
                if w0 <= s < w1:
                    starts[n].append((s, j))
    for n in starts:
        starts[n].sort()
    nodes = sorted(n for n in busy if n in gpu)
    unknown = sorted(n for n in busy if n not in gpu)
    print(f"multi-node rows expanded: {multi}")
    print(f"nodes with jobs in window: {len(nodes)} known "
          f"({sum(gpu[n] for n in nodes)} GPU); not in hardware table, excluded: {unknown}")
    jobs_started = {j for n in nodes for _, j in starts[n]}

    # Idle intervals per node; an interval ends at a job start unless it ends at w1.
    idle = collections.defaultdict(list)  # class -> [(node, start, end, ends_in_job)]
    for n in nodes:
        t = w0
        for s, e in merge(busy[n]):
            if s > t:
                idle[gpu[n]].append((n, t, s, True))
            t = max(t, e)
        if t < w1:
            idle[gpu[n]].append((n, t, w1, False))

    for cls in (False, True):
        name = "GPU" if cls else "CPU"
        nn = sum(1 for n in nodes if gpu[n] == cls)
        total = nn * (w1 - w0)
        ivs = idle[cls]
        unav = sum(e - s for _, s, e, _ in ivs if e - s >= UNAVAILABLE)
        avail = total - unav
        print(f"\n== {name} nodes: {nn}, node-hours {total / HOUR:,.0f}")
        for label, use_unav in (("72 h+ gaps unavailable", False), ("72 h+ gaps idle", True)):
            base = total if use_unav else avail
            iv = [x for x in ivs if use_unav or x[2] - x[1] < UNAVAILABLE]
            idle_s = sum(e - s for _, s, e, _ in iv)
            print(f"-- {label}: available node-hours {base / HOUR:,.0f}, "
                  f"idle {idle_s / base:.1%}")
            for b in BOOT_MIN:
                for sm in SUSPEND_MIN:
                    S, B = sm * 60, b * 60
                    off_r = off_f = 0
                    wakes, delayed = 0, set()
                    for n, s, e, ends_in_job in iv:
                        L = e - s
                        if L > S:
                            off_r += L - S
                            if ends_in_job:
                                wakes += 1
                                k = bisect.bisect_left(starts[n], (e, -1))
                                while k < len(starts[n]) and starts[n][k][0] == e:
                                    delayed.add(starts[n][k][1])
                                    k += 1
                        if L - B > S:
                            off_f += L - S - (B if ends_in_job else 0)
                    print(f"   boot {b:2d} min, SuspendTime {sm:2d} min: off "
                          f"{off_r / base:5.1%} reactive, {off_f / base:5.1%} forecast; "
                          f"wakes {wakes:6d}, jobs delayed {len(delayed):6d} "
                          f"({len(delayed) / len(jobs_started):.2%} of starts), "
                          f"wait {wakes * b / 60:,.0f} node-hours")

        # Idle share by hour of week (Monday 00:00 first), 72 h+ gaps unavailable.
        tz = dt.timezone(dt.timedelta(hours=1)) if utc else dt.timezone.utc
        idle_h = [0.0] * 168
        for _, s, e, _ in ivs:
            if e - s >= UNAVAILABLE:
                continue
            t = s
            while t < e:
                nxt = min(e, (t // HOUR + 1) * HOUR)
                d = dt.datetime.fromtimestamp(t, tz)
                idle_h[d.weekday() * 24 + d.hour] += nxt - t
                t = nxt
        weeks = (w1 - w0) / (7 * DAY)
        share = [x / (nn * HOUR * weeks) for x in idle_h]
        print("   idle share by day (CET, mean over hours), Mon..Sun:",
              " ".join(f"{sum(share[d * 24:(d + 1) * 24]) / 24:.0%}" for d in range(7)))
        print("   idle share by hour of day (CET, mean over days):",
              " ".join(f"{sum(share[d * 24 + h] for d in range(7)) / 7:.0%}" for h in range(24)))
    print(f"\njob starts in window: {len(jobs_started)}")


if __name__ == "__main__":
    sys.exit(main())
