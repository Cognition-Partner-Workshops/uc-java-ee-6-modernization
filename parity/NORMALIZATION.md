# Parity normalisation rules

The parity harness compares the legacy app (recorded once into `parity/transcripts/`) with any target URL. Some values legitimately change on every run. Each one is masked by a rule in [`rules.yaml`](rules.yaml), in the order listed below. Nothing else is masked. If a target needs a new rule to pass, that is a design decision: write it down here and re-record from the Liberty baseline. **Never edit the transcripts by hand to make parity pass.**

## What a transcript holds

For every HTTP step:

| Field | Content |
| --- | --- |
| `request` | Method, path and parameters, normalised. |
| `status` | HTTP status after redirects. |
| `contentType` | MIME type without its charset. |
| `title` | HTML `<title>` (page steps). |
| `text` | The visible text of the page, one line per text node, in document order, whitespace collapsed. Comments, scripts and styles are removed. |
| `inputs` | Form input `name -> value` (not submit buttons), so profile values in `<input value=…>` are compared too. |
| `containsPhrases` | Only on steps whose full body can't be compared (the invalid-login error page carries a JVM stack trace). It records whether each listed phrase appears. |
| `json` | REST bodies with strings normalised and lists sorted, because the REST sample fills its list from a `HashMap`. |

Each scenario also has **`checks`**: values the harness works out from the raw, un-normalised pages. They prove that the numbers hidden by rule N5 are still right.

| Scenario | Checks |
| --- | --- |
| 03-quotes | Requested symbols = returned symbols, company names |
| 04-register | Opening balance `100000.00`, cash = opening balance, 0 holdings, profile name and email |
| 05-buy | Order type/symbol/quantity/fee/status, cash debit = quote price × qty + 24.95, purchase price = quote price |
| 06-sell | Sell order facts, cash credit = current price × qty − 24.95, no holdings left |
| 07-portfolio | Holdings after two buys and after one sell, purchase basis = price × qty |
| 08-account-update | Updated profile fields, login with the new password |

## Rules

| Id | Masks | Replacement | Why |
| --- | --- | --- | --- |
| P1 | Before each `uid:0` scenario, an unrecorded `uid:0` login, home and logout | Not in transcript | `buildDB` creates `uid:0`'s holdings with buy orders. Their "The following Order(s) have completed" alert waits for `uid:0`'s next page view, so without this only the first run after a seed would show it. |
| S1 | Tables whose header row is `symbol / price / change` (Top Gainers / Top Losers on the home page) | Table removed | These lists rank all 10,000 quotes. Every trade anyone makes changes them. |
| N1 | `java.util.Date#toString()` timestamps, e.g. `Wed Nov 04 07:42:47 UTC 2026` | `<TIMESTAMP>` | Clock line, order dates, account created/last-login dates. |
| N2 | `jsessionid=…` | `jsessionid=<SESSION_ID>` | Servlet session ids. Cookie values are never written to transcripts. |
| N3 | `ru:p<seed>-<runToken>-<n>` | `ru:<NEW_USER>` | Users the harness registers. The run token keeps re-runs on the same DB from colliding. |
| N4 | `Order 74881` in confirmation text | `Order <ID>` | Order ids come from the key generator and depend on DB history. |
| N4b | ISO dates `2026-11-04` | `<DATE>` | Market summary date. |
| N5 | Signed decimals, e.g. `145.06`, `-3.2`, `+53.00` | `<DEC>` | `buildDB` gives every quote a random price (`TradeConfig.rndPrice`), and each trade moves it by a random ±20% (`getRandomPriceChangeFactor`). The RNG is seeded from the server clock and can't be pinned over HTTP. So amounts are masked in `text` and checked in `checks` instead. |
| N6 | Integers with 4+ digits that are not part of a word, symbol, decimal or dashed number | `<ID>` | Account, order and holding ids (≥ 1000 once seeded). |
| N7 | Every remaining integer, and any `<ID>` from N6, **only on steps marked `volatileAccount`** | `<N>` | The seeded `uid:0` account gets random holdings and balances from `buildDB`, and its login count goes up on every run. N6 is folded in too, because whether one of those counts reaches 4 digits depends on how long the DB has been used. Only the login, home and quotes steps for `uid:0` use this scope. |

## Fixed seed

`--seed` (default 42) feeds a Python `random.Random("<seed>:<scenario id>")` for each scenario. It picks the quoted symbols, the traded symbols and the quantities, so every run and every target gets the same requests. The legacy server's own RNG (`TradeConfig.r0`, seeded with `System.currentTimeMillis()`) has no HTTP or config hook. Pinning it would mean changing app code, which this preparation step doesn't do. The scenarios avoid depending on it: trades use freshly registered users, and money is checked arithmetically.

## REST base URL

REST scenarios call `<scheme>://<host>/rest` by default (the legacy app serves REST from a separate WAR at `/rest`). If the target serves it elsewhere, pass `--rest-target`.
