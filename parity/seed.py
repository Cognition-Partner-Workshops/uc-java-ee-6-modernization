#!/usr/bin/env python3
"""Wait for DayTrader to come up, then create and populate its database over HTTP.

Uses the app's own config servlet: /config?action=buildDBTables then /config?action=buildDB.
"""
import argparse
import sys
import time

import requests


def waitForApp(baseUrl, timeoutSeconds):
    deadline = time.time() + timeoutSeconds
    while time.time() < deadline:
        try:
            if requests.get(baseUrl + "/", timeout=5).status_code == 200:
                return
        except requests.RequestException:
            pass
        time.sleep(2)
    sys.exit(f"DayTrader did not answer at {baseUrl}/ within {timeoutSeconds}s")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, help="base URL, e.g. http://localhost:9083/daytrader")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--skip-wait", action="store_true")
    args = parser.parse_args()
    baseUrl = args.target.rstrip("/")
    if not args.skip_wait:
        waitForApp(baseUrl, args.timeout)
    for action, marker in (("buildDBTables", "tables successfully created"), ("buildDB", "Registering")):
        started = time.time()
        response = requests.get(f"{baseUrl}/config", params={"action": action}, timeout=900)
        if response.status_code != 200 or marker not in response.text:
            sys.exit(f"{action} failed: HTTP {response.status_code}")
        print(f"{action}: ok in {time.time() - started:.1f}s")


if __name__ == "__main__":
    main()
