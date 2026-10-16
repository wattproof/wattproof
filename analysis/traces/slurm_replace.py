"""Re-placement check of docs/research/slurm-trace-plan.md (change of 2026-10-16).

Slurm allocates powered-up nodes before powered-down ones. A start that the trace placement puts on
a powered-off node is avoidable if each such node can be swapped for an idle, still powered-on
node of the same hardware type. Optimistic: the skipped node stays off and its next job is not
followed, so the result is an upper bound on what Slurm's placement removes.
"""

import collections


def replace(ivs, pool, starts_at, S):
    """ivs: node -> [(start, end, ends_in_job)] idle intervals, unavailable ones already removed.
    pool: node -> hardware type. starts_at: node -> {time: [job ids]}. S: SuspendTime in seconds.

    Returns (delayed, avoided, wakes_left): the job ids that start on at least one powered-off
    node, those whose powered-off nodes can all be swapped, and the boots left after the swaps
    (one per node and time, however many jobs start on it together)."""
    off = collections.defaultdict(list)  # (t, job) -> [its powered-off nodes]
    cand = []
    for n, iv in ivs.items():
        for s, e, ends in iv:
            cand.append((s, e, n))
            if ends and e - s > S:
                for j in starts_at.get(n, {}).get(e, ()):
                    off[(e, j)].append(n)
    cand.sort()
    active = collections.defaultdict(dict)  # pool -> node -> (start, end)
    i, avoided, boots = 0, set(), set()
    for t, j in sorted(off):
        while i < len(cand) and cand[i][0] <= t:
            s, e, n = cand[i]
            active[pool[n]][n] = (s, e)
            i += 1
        need = collections.Counter(pool[n] for n in off[(t, j)])
        pick = {}
        for p, k in need.items():
            a = active[p]
            for n in [n for n, (s, e) in a.items() if e <= t or t - s >= S]:
                del a[n]  # busy again, or powered off: stale for every later t
            if len(a) < k:
                break
            pick[p] = sorted(a, key=lambda n: -a[n][1])[:k]  # longest remaining idle first
        if len(pick) == len(need):
            avoided.add(j)
            for p, ns in pick.items():
                for n in ns:
                    del active[p][n]
        else:
            boots.update((n, t) for n in off[(t, j)])
    return {j for _, j in off}, avoided, len(boots)
