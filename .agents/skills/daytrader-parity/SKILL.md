---
name: daytrader-parity
description: Run and interpret the DayTrader behavioural parity harness (parity/). Use when checking a migrated DayTrader (e.g. Spring Boot) against the legacy Liberty baseline, when make parity fails, or before touching parity transcripts or normalisation rules.
---

# DayTrader parity

## Run it

1. Legacy baseline (reference): `make up && make build && make run`. Wait for `buildDB: ok`.
2. Target: start the migrated app, with its DB seeded the same way (15,000 users `uid:0..14999` with password `xxx`, and 10,000 quotes `s:0..9999`; the legacy app does this through `/config?action=buildDBTables` then `buildDB`).
3. `make parity TARGET=<base-url-including-/daytrader>`. If the REST API is not at `<scheme>://<host>/rest`, run `.venv/bin/python parity/compare.py --target <url> --rest-target <rest-url>`.
4. Read `work/parity/report.md`: one row per scenario, then a unified diff per failing scenario (`-` recorded, `+` target). The fresh transcripts are in `work/parity/transcripts/`.

## Interpret a diff

- `status` / `contentType` differ: routing, error handling or a missing endpoint. Example: invalid login is **500** on legacy.
- `text` differs: the visible page text changed (labels, order, missing sections). A Thymeleaf port must render the same text nodes in the same order.
- `inputs` differ: form field names or prefilled values changed (the account profile form).
- `checks` differ: the business arithmetic is wrong (cash debit/credit, holdings, opening balance). Treat this as a real defect.
- `error` key: the scenario crashed (connection refused, an expected table was missing). Check that the target is up and seeded.

## Rules

- Never edit `parity/transcripts/*.json` to make a target pass. Never re-record against anything except the Liberty baseline.
- Adding or loosening a rule in `parity/rules.yaml` changes what parity means. Add a row to `parity/NORMALIZATION.md` with the reason, re-record from Liberty, and call it out in the PR.
- A deliberate behaviour change (for example, returning 401 for a bad password) should stay a visible diff and be listed as a decision. Don't hide it.
- Scenarios use freshly registered users, so they can run repeatedly against the same DB. Only `uid:0` scenarios depend on the seed, and they are masked by rule N7.
