"""Per-minute totals of the Azure LLM inference trace 2024 (DynamoLLM, HPCA 2025).

Input: the two one-week CSVs (TIMESTAMP, ContextTokens, GeneratedTokens), downloaded from
https://github.com/Azure/AzurePublicDataset/blob/master/AzureLLMInferenceDataset2024.md
(CC-BY 4.0). Output: one row per minute from 2024-05-10 00:00 UTC, with requests, context
tokens and generated tokens per service. Each file covers seven days from its own first day
(code: 10-16 May, conversation: 12-18 May), so minute m is counted from each file's start.
Standard library only.

    python3 -I azure2024_minutes.py code_1week.csv conv_1week.csv > testdata/azure2024-llm-minutes.csv
"""

import argparse
import datetime
import sys

MINUTES = 7 * 24 * 60


def minutes(path):
    req = [0] * MINUTES
    ctx = [0] * MINUTES
    gen = [0] * MINUTES
    day0 = None
    with open(path, encoding="ascii") as f:
        if f.readline().strip() != "TIMESTAMP,ContextTokens,GeneratedTokens":
            sys.exit(f"{path}: unexpected header")
        for line in f:
            ts, c, g = line.rstrip("\n").split(",")
            # "2024-05-10 00:00:00.009930+00:00": every timestamp is UTC.
            if not ts.endswith("+00:00"):
                sys.exit(f"{path}: not UTC: {ts}")
            day = datetime.date(int(ts[0:4]), int(ts[5:7]), int(ts[8:10]))
            if day0 is None:
                day0, first = day.toordinal(), day
            d = day.toordinal() - day0
            m = d * 1440 + int(ts[11:13]) * 60 + int(ts[14:16])
            if not 0 <= m < MINUTES:
                sys.exit(f"{path}: outside the week: {ts}")
            req[m] += 1
            ctx[m] += int(c)
            gen[m] += int(g)
    return req, ctx, gen, first


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("code")
    ap.add_argument("conv")
    a = ap.parse_args()
    code, conv = minutes(a.code), minutes(a.conv)
    out = sys.stdout
    out.write("# Azure LLM inference trace 2024, per-minute totals. Source: Microsoft Azure,\n")
    out.write("# https://github.com/Azure/AzurePublicDataset (AzureLLMInferenceDataset2024), CC-BY 4.0.\n")
    out.write("# Cite: Stojkovic et al., DynamoLLM, HPCA 2025. Derived by analysis/traces/azure2024_minutes.py.\n")
    out.write(f"# Minute 0 is 00:00 UTC on each service's first day: code {code[3]:%Y-%m-%d (%A)}, "
              f"conversation {conv[3]:%Y-%m-%d (%A)}.\n")
    out.write("minute,code_requests,code_context_tokens,code_generated_tokens,"
              "conv_requests,conv_context_tokens,conv_generated_tokens\n")
    for m in range(MINUTES):
        out.write(f"{m},{code[0][m]},{code[1][m]},{code[2][m]},{conv[0][m]},{conv[1][m]},{conv[2][m]}\n")


if __name__ == "__main__":
    main()
