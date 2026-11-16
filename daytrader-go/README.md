# daytrader-go

Go + PostgreSQL replacement for the DayTrader 3 EAR. One binary: `net/http`, `html/template`, `pgx`.
Plan and decisions: [docs/MIGRATION_PLAN.md](../docs/MIGRATION_PLAN.md).

## Run

```bash
make go-run                                            # PostgreSQL (compose), seed, serve on :8080
make parity TARGET=http://localhost:8080/daytrader     # 13 scenarios against the Go target
make go-test                                           # go vet + go test
make go-down
```

`daytrader seed` is `buildDBTables` + `buildDB` (15,000 users, 10,000 quotes, about one second).
The parity seeder's `/daytrader/config?action=buildDBTables|buildDB` works too.

| Variable | Default | Legacy source |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://daytrader:daytrader@localhost:55432/daytrader?sslmode=disable` | `jdbc/TradeDataSource` + J2C alias |
| `LISTEN_ADDR` | `:8080` | `<httpEndpoint>` |
| `CONTEXT_PATH` / `REST_PATH` | `/daytrader` / `/rest` | EAR context roots |
| `DB_MAX_CONNS` | `20` | connection pool settings |
| `MAX_USERS` / `MAX_QUOTES` / `MAX_HOLDINGS` | `15000` / `10000` / `10` | `TradeConfig` |
| `KEY_BLOCK_SIZE` | `1000` | `TradeConfig.KEYBLOCKSIZE` |
| `ORDER_PROCESSING_MODE` | `sync` | `TradeConfig.orderProcessingMode`; `async` = SQS consumer (MDB replacement) |
| `ORDER_QUEUE_URL` | empty | `jms/TradeBrokerQueue` |
| `PUBLISH_QUOTE_PRICE_CHANGE` | `false` | `TradeConfig.publishQuotePriceChange` |
| `UPDATE_QUOTE_PRICES` | `true` | `TradeConfig.updateQuotePrices` |
| `MARKET_SUMMARY_INTERVAL` | `20` | `TradeConfig.marketSummaryInterval` |

## Layout and ownership

| Path | What | Owner |
| --- | --- | --- |
| `cmd/daytrader` | `serve`, `seed`, `migrate` | scaffold |
| `internal/config` | TradeConfig from env | scaffold |
| `internal/store` | table structs, pgx pool, `InTx`, `NextID` (KEYGENEJB), shared reads | scaffold |
| `internal/javafmt` | Java-compatible formatting, FinancialUtils | scaffold |
| `internal/seed` | buildDBTables, buildDB | scaffold |
| `internal/web` | front controller, registry, sessions, rendering, health | scaffold |
| `templates/layout.html` | shared blocks `pageStart`, `pageEnd`, `closedOrdersAlert` | scaffold |
| `internal/features/<area>` + `templates/<area>_*.html` | one feature area | that area only |
| `static/` | legacy static files, unchanged | scaffold |

Feature areas: `auth`, `market`, `trading`, `account`, `rest`. **A feature branch only adds or edits files
in its own `internal/features/<area>/` and its own `templates/<area>_*.html`.** Scaffold files are shared;
if one needs to change, say so in the PR instead of editing it. This is what keeps the parallel branches
conflict-free.

## Adding a handler

```go
// internal/features/market/register.go
package market

import "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"

func init() {
	web.RegisterAction("home", home)             // /daytrader/app?action=home, login required
	// web.RegisterPublicAction("login", login)  // no login required
	// web.RegisterWelcome(welcome)              // page for no action / not logged in / unknown action
	// web.RegisterErrorPage(errorPage)          // HTTP 500 page
	// web.RegisterRoute("GET /welcome.jsp", ...)          // under /daytrader
	// web.RegisterRestRoute("GET /addresses", ...)        // under /rest
}

type homeData struct {
	Page    web.Page
	Account *store.Account
}

func home(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	account, err := store.AccountByUser(r.Context(), app.Store.Pool, session.UserID)
	if err != nil {
		app.Error(w, r, err) // HTTP 500
		return
	}
	app.Render(w, r, http.StatusOK, "market_home", homeData{
		Page:    app.NewPage(r, session, "Welcome to DayTrader", "DayTrader Home"),
		Account: account,
	})
}
```

```html
{{/* templates/market_home.html */}}
{{define "market_home"}}{{template "pageStart" .Page}}
  ... {{money .Account.Balance}} ...
{{template "pageEnd" .Page}}{{end}}
```

- Template names are global: prefix them with the area (`market_home`).
- Template functions: `date`, `double`, `money`, `gainHTML`, `changeHTML`, `gainPercentHTML`, `quoteLink`,
  `now`, `safeHTML`, `computeGain`, `computeGainPct`, `formatNullableTS` (see `internal/web/app.go`).
- Writes go through `app.Store.InTx(ctx, func(tx pgx.Tx) error { ... })`; new ids through
  `app.Store.NextID(ctx, "account" | "holding" | "order")`.
- A session is logged in when `session.UserID != ""`. Login sets `session.UserID` and
  `session.CreationDate`; logout calls `app.Sessions.Invalidate(w, session)`.
- Another area's service code can be imported (`internal/features/trading` may import
  `internal/features/market`), but not edited. Avoid import cycles.
