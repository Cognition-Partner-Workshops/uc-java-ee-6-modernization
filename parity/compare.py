#!/usr/bin/env python3
"""Replay every parity scenario against --target and diff with the recorded transcripts.

Writes work/parity/report.md (and the fresh transcripts under work/parity/transcripts/).
Exits 1 if any scenario differs, errors, or has no recorded transcript.
"""
import argparse
import difflib
import json
import os
import sys

from harness import HERE, TRANSCRIPT_DIR, Runner, restBaseFor, runScenario, writeTranscript
from scenarios import SCENARIOS

REPO = os.path.dirname(HERE)
MAX_DIFF_LINES = 60


def pretty(transcript):
    return json.dumps(transcript, indent=2, sort_keys=True).splitlines()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, help="app base URL, e.g. http://localhost:9083/daytrader")
    parser.add_argument("--rest-target", help="REST base URL (default: <scheme>://<host>/rest)")
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--out", default=os.path.join(REPO, "work", "parity"))
    args = parser.parse_args()
    restTarget = args.rest_target or restBaseFor(args.target)
    runner = Runner(args.target, restTarget, args.seed)
    rows, details, failures = [], [], 0
    for scenario in SCENARIOS:
        expectedPath = os.path.join(TRANSCRIPT_DIR, scenario["id"] + ".json")
        actual, seconds = runScenario(runner, scenario)
        writeTranscript(os.path.join(args.out, "transcripts"), actual)
        if not os.path.exists(expectedPath):
            result, diff = "MISSING", ["no recorded transcript: run make record against the Liberty baseline"]
        else:
            with open(expectedPath) as handle:
                expected = json.load(handle)
            diff = list(difflib.unified_diff(pretty(expected), pretty(actual), "recorded", "target", lineterm=""))
            result = "PASS" if not diff else ("ERROR" if "error" in actual else "DIFF")
        failures += result != "PASS"
        checks = actual.get("checks", {})
        rows.append(f"| {scenario['id']} | {scenario['description']} | {len(actual['steps'])} | "
                    f"{len(checks)} | {seconds:.1f}s | **{result}** |")
        if result != "PASS":
            shown = diff[:MAX_DIFF_LINES] + ([f"... {len(diff) - MAX_DIFF_LINES} more lines"]
                                             if len(diff) > MAX_DIFF_LINES else [])
            details.append(f"### {scenario['id']}\n\n```diff\n" + "\n".join(shown) + "\n```\n")
        print(f"{scenario['id']:<20} {result}")
    report = [
        "# Parity report", "",
        f"- Target: `{args.target}` (REST: `{restTarget}`)",
        f"- Seed: {args.seed}",
        f"- Result: **{len(SCENARIOS) - failures}/{len(SCENARIOS)} scenarios match the recorded Liberty baseline**",
        "", "| Scenario | What it does | Steps | Checks | Time | Result |", "| --- | --- | --- | --- | --- | --- |",
        *rows, "",
    ]
    if details:
        report += ["## Differences", "", *details]
    os.makedirs(args.out, exist_ok=True)
    with open(os.path.join(args.out, "report.md"), "w") as handle:
        handle.write("\n".join(report) + "\n")
    print(f"report: {os.path.relpath(os.path.join(args.out, 'report.md'), os.getcwd())}")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
