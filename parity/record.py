#!/usr/bin/env python3
"""Record reference transcripts from the legacy DayTrader app into parity/transcripts/.

Only run this against the Liberty baseline (make run). Never re-record to make a migrated app pass.
"""
import argparse
import sys

from harness import TRANSCRIPT_DIR, Runner, restBaseFor, runScenario, writeTranscript
from scenarios import SCENARIOS


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", default="http://localhost:9083/daytrader")
    parser.add_argument("--rest-target", help="REST base URL (default: <scheme>://<host>/rest)")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()
    runner = Runner(args.target, args.rest_target or restBaseFor(args.target), args.seed)
    failed = False
    for scenario in SCENARIOS:
        transcript, seconds = runScenario(runner, scenario)
        writeTranscript(TRANSCRIPT_DIR, transcript)
        status = "ERROR " + transcript["error"] if "error" in transcript else "ok"
        failed |= "error" in transcript
        print(f"{scenario['id']:<20} {len(transcript['steps']):>2} steps {seconds:5.1f}s  {status}")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
