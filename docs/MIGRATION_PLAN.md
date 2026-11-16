# Migration plan: DayTrader 3 (Java EE 6 on Liberty) to Go + PostgreSQL

Target for this migration: **Go + PostgreSQL on AWS ECS Fargate**. Where `docs/TARGET_STATE.md` or
`docs/LAB.md` say Spring Boot, this plan wins. The AWS shape (ALB, Fargate, Aurora PostgreSQL, Secrets
Manager, SSM, SQS) is unchanged from `TARGET_STATE.md`.

What is being left: the application runs on **WebSphere Liberty** with embedded Derby and Liberty's embedded
messaging engine. The traditional WebSphere on AIX / IBM MQ / Db2 material in `legacy-ops/` is synthetic and
is mapped only so the inventory is complete. The code makes **no WebSphere-proprietary API calls**; the
coupling is in deployment descriptors, JNDI names, EJBs/MDBs and Liberty features.

## Done means

1. `make parity TARGET=http://localhost:8080/daytrader` is 13/13 against the Go binary and against the
   container, both on PostgreSQL, with `parity/transcripts/` untouched.
2. `go vet ./... && go test ./...` pass in `daytrader-go/`.
3. `terraform fmt -check`, `terraform init -backend=false` and `terraform validate` pass in `infra/terraform`.
   Nothing is applied and no AWS credentials are used.
4. Every row of `docs/WEBSPHERE_INVENTORY.md` has a replacement (146 of 146).
5. One open PR to `master`. A human merges it.

## Replacement mapping

The per-row mapping is generated: `REPLACEMENT_RULES` in `tools/ws_inventory.py` fills the last column of
`docs/WEBSPHERE_INVENTORY.md`. Summary:

| Legacy | Go / AWS replacement | Why |
| --- | --- | --- |
| `jdbc/TradeDataSource`, `jdbc/NoTxTradeDataSource` (Derby; synthetic Db2) | One `pgxpool.Pool` on Aurora PostgreSQL from `DATABASE_URL` | The XA / non-XA split existed for JTA with JMS. There is no two-phase commit left. |
| J2C alias `TradeDataSourceAuthData` | Secrets Manager secret, injected as an ECS task secret | Credentials leave server config. |
| J2C alias `TradeAdminAuthData` (messaging) | IAM task role | SQS has no password to store. |
| JMS queue `jms/TradeBrokerQueue` + `DTBroker3MDB` | SQS queue + Go consumer, only when `ORDER_PROCESSING_MODE=async` | Default stays synchronous, which is what the baseline and the transcripts use. |
| JMS topic `jms/TradeStreamerTopic` + `DTStreamer3MDB` | SNS topic fanned out to SQS, off (`PUBLISH_QUOTE_PRICE_CHANGE=false`) | The streamer MDB only fed benchmark statistics; no page reads it. |
| `TradeSLSBBean`, `DirectSLSBBean` (stateless session beans, container transactions) | Go services with explicit `pgx.Tx` (`Store.InTx`) | One implementation replaces the EJB3 / Direct runtime-mode switch. |
| JNDI lookups and `@Resource` / `@EJB` references | Constructor injection: `main` builds config, pool and `web.App` and passes them down | Nothing is looked up by name at run time. |
| JPA persistence unit (OpenJPA) | Hand-written SQL through pgx, schema in `daytrader-go/migrations/` | Six tables do not need an ORM. |
| `KEYGENEJB` key blocks | Kept: `Store.NextID` reserves blocks of 1000 with `SELECT ... FOR UPDATE` | Same id behaviour as the legacy allocator. |
| Liberty features (`ejbLite`, `jpa`, `jaxrs`, `jsf`, `jmsMdb`, `wasJmsClient`, `wasJmsServer`) | `net/http`, `html/template`, `encoding/json`, `pgx`, AWS SDK for Go v2 (SQS) | Standard library first. |
| JSP pages, `TradeAppServlet` | `html/template` pages behind one `/daytrader/app?action=...` front controller with a handler registry | Same URLs, forms and field names. |
| `ibm-*-bnd.xml`, `ibm-*-ext.xml` | Removed | They only bind names that no longer exist. |
| `server.xml` (`httpEndpoint`, `application`, `library`, `messagingEngine`) | Container image on Fargate behind an ALB; health check `/daytrader/health` | |
| Synthetic wsadmin + AIX scripts | Terraform modules `network`, `alb`, `ecs`, `rds` | Mapped for completeness; never executed. |

Not migrated, on purpose: the `web/prims` ping servlets, the JSF pages, `TradeScenarioServlet`, the run-stats
and config pages other than the two seed actions. None of them are in the parity scenarios; they are benchmark
tooling. The inventory marks each one.

## Data

- Schema: `daytrader-go/migrations/0001_schema.sql` is the Derby `Table.ddl` in PostgreSQL types
  (`DOUBLE` to `DOUBLE PRECISION`, same `DECIMAL(14,2)`, same indexes). Tables: `accountejb`,
  `accountprofileejb`, `holdingejb`, `orderejb`, `quoteejb`, `keygenejb`. `OPENJPASEQ` is dropped (OpenJPA only).
- Seed: `daytrader seed` (or `/daytrader/config?action=buildDBTables|buildDB`) reproduces `TradeBuildDB`:
  10,000 quotes, 15,000 users, 0 to 10 synchronous buys each, `uid:0` opening with 1,000,000.
- Production data move (not part of this demo): the demo estate has no production data. For a real estate the
  path is AWS DMS or `pgloader` from Db2 into the same six tables, then setting `keygenejb` above the highest ids.

## Steps

| # | Step | Owner | Gate |
| --- | --- | --- | --- |
| 0 | Liberty baseline, record that parity is 13/13 on `:9083`, regenerate inventory | parent | 13/13 |
| 1 | Fill the inventory mapping, write this plan, scaffold `daytrader-go/` on `migration/daytrader-go` | parent | `go vet`, seed, `/daytrader/health` |
| 2 | Seven parallel workstreams, one branch and PR each into `migration/daytrader-go` | children | per-area scenario ids |
| 3 | Combine the branches locally on `migration/daytrader-go-verify`, run the container on PostgreSQL | parent | 13/13 on `:8080`, at most two fix rounds |
| 4 | Browser walk-through of both apps, recording, one PR to `master` | parent | human review |

Workstreams in step 2 and the files each owns:

| Area | Scenarios | Owns |
| --- | --- | --- |
| auth | 01, 09, 10 | `internal/features/auth/`, `templates/auth_*.html` |
| market | 02, 03 | `internal/features/market/`, `templates/market_*.html` |
| trading | 05, 06, 07 | `internal/features/trading/`, `templates/trading_*.html` |
| account | 04, 08 | `internal/features/account/`, `templates/account_*.html` |
| rest | 11, 12, 13 | `internal/features/rest/` |
| tests | none (`go vet`, `go test`) | `*_test.go` files |
| infra | none (`terraform validate`) | `daytrader-go/Dockerfile`, `daytrader-go/.dockerignore`, `daytrader-go/compose.app.yaml`, `infra/terraform/` |

Scenarios cross areas (05-buy also registers a user and reads account, quote and portfolio pages), so an area
can only see its scenarios pass with the sibling branches combined locally. No child merges a PR.

## Behaviour to keep

These are recorded in the transcripts and are reproduced, not fixed:

- Invalid login answers **HTTP 500** with the error page.
- The account page prints **`Total Orders: 0`** whatever the order count.
- `GET /rest/addresses/{entryName}` answers **HTTP 415**.
- Dates print as `java.util.Date.toString()`, quantities as `Double.toString()` (`10.0`), money with scale 2.
- A user's closed orders are alerted once, on the next page, then marked `completed`.

Any deliberate difference is listed in the integration PR. None is planned.

## Risks

| Risk | Mitigation |
| --- | --- |
| Java number and date formatting differs from Go's | `internal/javafmt` owns it; unit tests ported from the EJB module's JUnit tests |
| `BigDecimal` arithmetic (`new BigDecimal(double)`, `ROUND_HALF_UP`) vs Go decimals | `shopspring/decimal`, no floats for money; parity checks the cash arithmetic |
| Parallel branches conflict | File ownership above; shared files belong to the scaffold |
| Sessions are in process memory | Fine for one task. More than one task needs ALB stickiness (set in the `alb` module) or a shared store later |
| Async order mode is not exercised by parity | It stays off; the consumer has unit tests only. Turning it on is a separate decision |
| Derby vs PostgreSQL ordering of unordered queries | Every list query has an explicit `ORDER BY` |
| Terraform is validated, never planned against an account | Stated in the PR; `plan` needs credentials this demo must not use |

## Rules for everyone

Nothing reaches `master` without a human. No PR is merged, approved or closed by an agent. `parity/transcripts/`
is never edited. A `parity/rules.yaml` change needs a reason in `parity/NORMALIZATION.md`. Legacy modules and
IBM licence headers stay as they are. Never `terraform apply`; no AWS credentials.
