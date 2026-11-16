# Parity report

- Target: `http://localhost:8080/daytrader` (REST: `http://localhost:8080/rest`)
- Seed: 42
- Result: **13/13 scenarios match the recorded Liberty baseline**

| Scenario | What it does | Steps | Checks | Time | Result |
| --- | --- | --- | --- | --- | --- |
| 01-login | Log in as the seeded user uid:0 | 1 | 0 | 0.0s | **PASS** |
| 02-home | Home page for uid:0 | 2 | 0 | 0.0s | **PASS** |
| 03-quotes | Quotes for three seeded-random symbols | 2 | 3 | 0.0s | **PASS** |
| 04-register | Register a new user with a 100000 opening balance | 3 | 5 | 0.1s | **PASS** |
| 05-buy | New user buys one symbol; cash debit = price x qty + 24.95 | 6 | 4 | 0.1s | **PASS** |
| 06-sell | New user buys then sells; cash credit = price x qty - 24.95 | 7 | 3 | 0.1s | **PASS** |
| 07-portfolio | Portfolio after two buys and one sell | 6 | 3 | 0.1s | **PASS** |
| 08-account-update | Update profile, check it, log in with new password | 5 | 2 | 0.1s | **PASS** |
| 09-invalid-login | Wrong password for uid:0 | 1 | 0 | 0.0s | **PASS** |
| 10-logout | Log out, then request home without a session | 3 | 0 | 0.0s | **PASS** |
| 11-rest-list | GET /rest/addresses | 1 | 0 | 0.0s | **PASS** |
| 12-rest-search | GET /rest/addresses/search/Entry1 | 1 | 0 | 0.0s | **PASS** |
| 13-rest-entry | GET /rest/addresses/{entryName}, existing and unknown | 2 | 0 | 0.0s | **PASS** |

