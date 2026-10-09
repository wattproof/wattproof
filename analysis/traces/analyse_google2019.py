"""Measures from the Google 2019 trace, as defined in docs/research/trace-analysis-plan.md.

Input: the CSV of google2019_cell.sql's result (columns kind, k, t, cpu_use, cpu_use_max5,
cpu_req, mem_use, mem_req, n, priority, sched_class, vscale, ctype), and the cell's CPU and memory
capacity. Standard library only.

    python3 -I analyse_google2019.py cell_a.csv --cpu-cap 6390.5 --mem-cap 5733.2
"""

import argparse
import array
import bisect
import collections
import csv
import math
import statistics

HOURS_PER_DAY = 24
HOURS = 744  # May 2019, hours from 00:00 PT on 1 May
NAN = float("nan")


def num(x):
    return float(x) if x not in ("", "NULL", None) else None


def pearson(a, b):
    ma, mb = statistics.fmean(a), statistics.fmean(b)
    da = [x - ma for x in a]
    db = [y - mb for y in b]
    sa = math.sqrt(sum(x * x for x in da))
    sb = math.sqrt(sum(y * y for y in db))
    if sa == 0 or sb == 0:
        return 0.0
    return sum(x * y for x, y in zip(da, db)) / (sa * sb)


def weighted_quantiles(values, weights, qs):
    pairs = sorted(zip(values, weights))
    total = sum(w for _, w in pairs)
    cum, out, i = 0.0, [], 0
    cums = []
    for v, w in pairs:
        cum += w
        cums.append(cum / total)
    for q in qs:
        i = min(bisect.bisect_left(cums, q), len(pairs) - 1)
        out.append(pairs[i][0])
    return out


class Series:
    """Hourly values in a fixed array; NaN where the service has no row. Dict-like reads."""

    __slots__ = ("a",)

    def __init__(self):
        self.a = array.array("d", [NAN]) * HOURS

    def __setitem__(self, t, v):
        self.a[t] = v

    def __getitem__(self, t):
        return self.a[t]

    def get(self, t, default=None):
        v = self.a[t] if 0 <= t < HOURS else NAN
        return default if v != v else v

    def __iter__(self):
        return (t for t in range(HOURS) if self.a[t] == self.a[t])

    def items(self):
        return ((t, v) for t, v in enumerate(self.a) if v == v)

    def values(self):
        return (v for v in self.a if v == v)


class Service:
    __slots__ = ("cpu", "max5", "req", "mem", "memreq", "priority", "vscale")

    def __init__(self, priority, vscale):
        self.cpu, self.max5, self.req, self.mem, self.memreq = (Series() for _ in range(5))
        self.priority, self.vscale = priority, vscale

    def group(self):
        if self.vscale == 1:
            return "manual"
        if self.vscale in (2, 3):
            return "autopilot"
        return "unknown"

    def peak_hour(self):
        return max(self.cpu, key=self.cpu.__getitem__)

    def daily_cycle(self):
        """The plan's robust test: correlation of each full day with the median day."""
        days = collections.defaultdict(dict)
        for t, v in self.cpu.items():
            days[t // HOURS_PER_DAY][t % HOURS_PER_DAY] = v
        full = [[d[h] for h in range(HOURS_PER_DAY)] for d in days.values() if len(d) == HOURS_PER_DAY]
        if len(full) < 7:
            return False, 0.0
        median_day = [statistics.median(col) for col in zip(*full)]
        top = max(median_day)
        swing = (top - min(median_day)) / top if top > 0 else 0.0
        good = sum(1 for day in full if pearson(day, median_day) >= 0.8)
        return good / len(full) >= 0.7 and swing >= 0.2, swing

    def fixed_requests(self):
        reqs = sorted(self.req.values())
        if not reqs:
            return True
        p10 = reqs[int(0.1 * (len(reqs) - 1))]
        p90 = reqs[int(0.9 * (len(reqs) - 1))]
        return p90 == 0 or p10 / p90 >= 0.9


def load(path):
    services, tiers, pdus = {}, collections.defaultdict(dict), collections.defaultdict(dict)
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            kind, k, t = r["kind"], r["k"], int(r["t"])
            if kind == "svc" and not 0 <= t < HOURS:
                continue
            if kind == "svc":
                s = services.get(k)
                if s is None:
                    s = services[k] = Service(int(r["priority"]), int(r["vscale"] or 0))
                s.cpu[t] = num(r["cpu_use"]) or 0.0
                s.max5[t] = num(r["cpu_use_max5"]) or 0.0
                s.req[t] = num(r["cpu_req"]) or 0.0
                s.mem[t] = num(r["mem_use"]) or 0.0
                s.memreq[t] = num(r["mem_req"]) or 0.0
            elif kind == "tier":
                tiers[k][t] = (num(r["cpu_use"]), num(r["cpu_req"]), num(r["mem_use"]), num(r["mem_req"]))
            elif kind == "pdu":
                pdus[k][t] = num(r["cpu_use"])
    return services, tiers, pdus


def checks(tiers, pdus):
    print("== Checks (plan section 'Checks that can fail')")
    prod = tiers.get("prod", {})
    cu = sum(v[0] for v in prod.values())
    cr = sum(v[1] for v in prod.values())
    mu = sum(v[2] for v in prod.values())
    mr = sum(v[3] for v in prod.values())
    print(f"1. prod CPU use/limit {cu / cr:.3f} (expected 0.20-0.40); memory {mu / mr:.3f} (0.50-0.80)")
    total_tier = sum(v[0] for tier in tiers.values() for t, v in tier.items() if t >= 0)
    total_pdu = sum(v for p in pdus.values() for t, v in p.items() if t >= 0)
    print(f"4. power-domain CPU total / tier total = {total_pdu / total_tier:.4f} (expected within 0.99-1.01)")


def summarise(services, cpu_cap, mem_cap):
    print(f"\n== Services alive at least 168 h: {len(services)}")
    groups = collections.defaultdict(list)
    for s in services.values():
        h = s.peak_hour()
        ratio = s.cpu[h] / s.req[h] if s.req[h] > 0 else None
        if ratio is None:
            continue
        cyc, swing = s.daily_cycle()
        weight = statistics.fmean(list(s.req.values()))
        max5 = [s.max5[t] / s.cpu[t] for t in s.cpu if s.cpu[t] > 0]
        memratio = s.mem[h] / s.memreq[h] if s.memreq[h] > 0 else None
        peak5 = statistics.median(max5) if max5 else None  # None: the service never used CPU
        rec = (ratio, weight, cyc, swing, s.fixed_requests(), peak5, memratio)
        groups[s.group()].append(rec)
        groups["all"].append(rec)
    qs = (0.1, 0.25, 0.5, 0.75, 0.9)
    print("Peak-hour CPU use/request, weighted by requested CPU (quantiles 10/25/50/75/90):")
    for g in ("all", "manual", "autopilot"):
        for label, keep in (("every service", lambda r: True), ("daily cycle", lambda r: r[2])):
            rs = [r for r in groups.get(g, []) if keep(r)]
            if not rs:
                continue
            q = weighted_quantiles([r[0] for r in rs], [r[1] for r in rs], qs)
            share = sum(r[1] for r in rs) / sum(r[1] for r in groups["all"])
            fixed = sum(r[1] for r in rs if r[4]) / sum(r[1] for r in rs)
            peak5 = statistics.median(r[5] for r in rs if r[5] is not None)
            mems = [(r[6], r[1]) for r in rs if r[6] is not None]
            memq = weighted_quantiles([m for m, _ in mems], [w for _, w in mems], (0.5,))[0]
            print(f"  {g:9} {label:13} n={len(rs):6} req share={share:5.2f} "
                  f"ratio={' '.join(f'{x:.2f}' for x in q)} fixed-req share={fixed:.2f} "
                  f"max5/mean={peak5:.2f} mem ratio median={memq:.2f}")

    # Cluster-level series of services: shape, aggregate ratio, fixed share, memory against CPU.
    hours = sorted({t for s in services.values() for t in s.cpu})
    use = [sum(s.cpu.get(t, 0.0) for s in services.values()) for t in hours]
    req = [sum(s.req.get(t, 0.0) for s in services.values()) for t in hours]
    mreq = [sum(s.memreq.get(t, 0.0) for s in services.values()) for t in hours]
    peak = max(range(len(hours)), key=use.__getitem__)
    shape = [u / use[peak] for u in use]
    s_min = min(shape)
    f_est = (min(req) / max(req) - s_min) / (1 - s_min) if s_min < 1 else float("nan")
    print("\n== Cluster-level, services only")
    print(f"peak-hour use/request {use[peak] / req[peak]:.3f}; month mean {sum(use) / sum(req):.3f}")
    print(f"shape: mean/peak {statistics.fmean(shape):.3f}, min/peak {s_min:.3f}")
    print(f"fixed share estimate F {f_est:.3f}")
    cpu_share = [r / cpu_cap for r in req]
    mem_share = [m / mem_cap for m in mreq]
    binds = sum(1 for c, m in zip(cpu_share, mem_share) if m > c) / len(hours)
    print(f"requests as share of cell capacity at peak: CPU {cpu_share[peak]:.3f}, memory {mem_share[peak]:.3f}; "
          f"memory requests exceed CPU requests (as shares) in {binds:.0%} of hours")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("csv")
    ap.add_argument("--cpu-cap", type=float, required=True)
    ap.add_argument("--mem-cap", type=float, required=True)
    a = ap.parse_args()
    services, tiers, pdus = load(a.csv)
    checks(tiers, pdus)
    summarise(services, a.cpu_cap, a.mem_cap)


if __name__ == "__main__":
    main()
