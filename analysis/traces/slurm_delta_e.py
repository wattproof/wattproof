"""Energy at equal wait (ΔE) from the Slurm scripts' outputs.

The rule is in docs/research/slurm-trace-plan.md (changes, 2026-10-16): ΔE is the forecast bound's
saving at SuspendTime 10 minutes minus the reactive saving at the reference SuspendTime of 30
minutes, at the same boot time and off draw, in points of the node class's measured energy. The
lowest of the four (off draw 10 and 30 W, boot 5 and 15 minutes) decides. Next to it: the
reference policy's delayed starts after re-placement, and its boot wait per 1,000 nodes per year.

Reads the saved text output of the three scripts; computes nothing from the traces itself:

    python -I slurm_delta_e.py --lisa-energy <slurm_energy.py output> \
        --lisa-idle <slurm_idle.py output> --m100 <slurm_m100.py output>
"""

import argparse
import re
import sys

REF, SHORT = 30, 10
BAR = 1.0
YEAR_DAYS = 365.25
num = lambda s: float(s.replace(",", ""))


def lisa_savings(text):
    """{class: (energy kWh, {(boot, suspend, off): (reactive, forecast) kWh})}"""
    out, cls = {}, None
    for line in text.splitlines():
        if m := re.match(r"== (CPU|GPU) nodes with draws", line):
            cls = m[1]
            out[cls] = [None, {}]
        elif cls and (m := re.search(r"measured energy of these nodes in the window: ([\d,]+) kWh", line)):
            out[cls][0] = num(m[1])
        elif cls and (m := re.search(r"boot\s+(\d+) min, SuspendTime\s+(\d+) min, off (\d+) W: saving\s+"
                                     r"([-\d,]+) kWh .*reactive,\s+([-\d,]+) kWh", line)):
            out[cls][1][int(m[1]), int(m[2]), int(m[3])] = (num(m[4]), num(m[5]))
    return out


def lisa_waits(text):
    """{class: (nodes, days, delayed share, wait node-hours at 5 and 15 min boot)}"""
    out, cls, days, section = {}, None, None, None
    for line in text.splitlines():
        if m := re.search(r"\(([\d.]+) days\)", line):
            days = days or float(m[1])
        if m := re.match(r"== (CPU|GPU) nodes: (\d+)", line):
            cls, nodes = m[1], int(m[2])
        elif line.startswith("-- "):
            section = "unavailable" in line
        elif cls and section and (m := re.search(
                rf"re-placement, SuspendTime {REF} min: .*\(([\d.]+)% of starts\); wait ([\d,]+) / ([\d,]+)", line)):
            out[cls] = (nodes, days, float(m[1]), num(m[2]), num(m[3]))
    return out


def m100(text):
    energy, sav, wait, boot = None, {}, None, None
    nodes = int(re.search(r"^nodes (\d+);", text, re.M)[1])
    by = re.search(r"complete days by: (\w+)", text)[1]
    days = int(re.search(rf"complete by {by} (\d+)", text)[1])
    section = text.split("-- 72 h+ gaps unavailable")[1].split("-- 72 h+ gaps idle")[0]
    energy = num(re.search(r"measured energy of these nodes: ([\d,]+) kWh", text)[1])
    for line in section.splitlines():
        if m := re.search(r"boot\s+(\d+) min, SuspendTime\s+(\d+) min:", line):
            boot = int(m[1]), int(m[2])
        elif m := re.search(r"off (\d+) W: saving\s+([-\d,]+) kWh .*reactive,\s+([-\d,]+) kWh", line):
            sav[boot[0], boot[1], int(m[1])] = (num(m[2]), num(m[3]))
        elif boot and boot[1] == REF and (m := re.search(r"\(([\d.]+)% of starts\); wait ([\d,]+) / ([\d,]+)", line)):
            wait = (nodes, days, float(m[1]), num(m[2]), num(m[3]))
    return {"GPU": [energy, sav]}, {"GPU": wait}, by


def report(site, savings, waits):
    lowest = {}
    for cls, (energy, sav) in savings.items():
        print(f"\n{site} {cls} nodes, measured energy {energy:,.0f} kWh")
        des = []
        for boot in (5, 15):
            for off in (10, 30):
                f10 = sav[boot, SHORT, off][1]
                r30 = sav[boot, REF, off][0]
                de = (f10 - r30) / energy * 100
                des.append(de)
                print(f"   boot {boot:2d} min, off {off} W: forecast at {SHORT} min {f10:9,.0f} kWh, "
                      f"reactive at {REF} min {r30:9,.0f} kWh, ΔE {de:+5.2f} points")
        lowest[cls] = min(des)
        print(f"   lowest ΔE {min(des):+.2f} points, highest {max(des):+.2f}")
        if cls in waits:
            nodes, days, share, w5, w15 = waits[cls]
            k = 1000 / nodes * YEAR_DAYS / days
            print(f"   reference ({REF} min): {share:.2f}% of starts delayed after re-placement; boot wait "
                  f"{w5 * k:,.0f} / {w15 * k:,.0f} node-hours per 1,000 nodes per year "
                  f"(5 / 15 min boot; {nodes} nodes, {days:g} days)")
    return lowest


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--lisa-energy", required=True)
    ap.add_argument("--lisa-idle", required=True)
    ap.add_argument("--m100", required=True)
    a = ap.parse_args()
    read = lambda p: open(p, encoding="utf-8").read()
    lowest = {f"Lisa {k}": v for k, v in report("Lisa", lisa_savings(read(a.lisa_energy)), lisa_waits(read(a.lisa_idle))).items()}
    sav, waits, by = m100(read(a.m100))
    lowest |= {f"Marconi100 {k}": v for k, v in report(f"Marconi100 2020 (complete by {by})", sav, waits).items()}
    worst = max(lowest.values())
    print(f"\nlowest ΔE per site and class: " + ", ".join(f"{k} {v:+.2f}" for k, v in lowest.items()))
    if worst < BAR:
        print(f"below {BAR:g} point on every site and class: by the rule, a Slurm planner is dropped")
    else:
        print(f"at least {BAR:g} point somewhere: the planner's target is the lowest ΔE where it clears the bar")


if __name__ == "__main__":
    sys.exit(main())
