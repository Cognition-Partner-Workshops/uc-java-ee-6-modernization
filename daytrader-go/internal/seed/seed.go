// Package seed is the Go replacement for TradeBuildDB: BuildTables is buildDBTables (drop and
// recreate the schema from migrations/*.sql) and BuildDB is buildDB (quotes, users, holdings).
package seed

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	daytradergo "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// Result counts what BuildDB wrote.
type Result struct{ Quotes, Users, Holdings, Orders int }

// BuildTables applies migrations/*.sql in name order. The first migration drops the tables.
func BuildTables(ctx context.Context, db *store.Store) error {
	names, err := fs.Glob(daytradergo.Migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		script, err := fs.ReadFile(daytradergo.Migrations, name)
		if err != nil {
			return err
		}
		if _, err := db.Pool.Exec(ctx, string(script)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	db.ResetKeys()
	return nil
}

var (
	pennyStockPrice = decimal.RequireFromString("0.01")
	pennyMultiplier = decimal.NewFromInt(600)
)

// RandomPriceChangeFactor is TradeConfig.getRandomPriceChangeFactor: between 0.80 and 1.20.
func RandomPriceChangeFactor(rng *rand.Rand) decimal.Decimal {
	percentGain := float64(rng.Float32()) * 0.2
	if rng.Float64() < 0.5 {
		percentGain *= -1
	}
	factor := decimal.NewFromFloat(percentGain + 1).Round(2)
	if !factor.IsPositive() {
		return decimal.NewFromInt(1)
	}
	return factor
}

// BuildDB repopulates the tables like TradeBuildDB: MAX_QUOTES quotes "s:<n>" priced 1..200,
// MAX_USERS users "uid:<n>" with password "xxx" (uid:0 opens with 1,000,000, the others with
// 200,000..299,999), and 0..MAX_HOLDINGS synchronous buys per user. Each buy debits the
// account, creates a closed order and a holding, and moves the quote's price and volume.
// The rows are computed in memory and loaded with COPY, so 15,000 users take seconds.
func BuildDB(ctx context.Context, db *store.Store, cfg *config.Config, rng *rand.Rand) (*Result, error) {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	type quote struct {
		price, open decimal.Decimal
		volume      float64
	}
	quotes := make([]quote, cfg.MaxQuotes)
	for i := range quotes {
		price := decimal.NewFromInt(int64(rng.Intn(200) + 1)).Round(2)
		quotes[i] = quote{price: price, open: price}
	}
	var profileRows, accountRows, holdingRows, orderRows [][]any
	nextHolding, nextOrder := 0, 0
	for i := 0; i < cfg.MaxUsers; i++ {
		userID := fmt.Sprintf("uid:%d", i)
		fullName := fmt.Sprintf("first:%d last:%d", rng.Intn(1000), rng.Intn(5000))
		email := fmt.Sprintf("%s@%d.com", userID, rng.Intn(100))
		address := fmt.Sprintf("%d Oak St.", rng.Intn(1000))
		creditCard := fmt.Sprintf("%d-%d-%d-%d", rng.Intn(100), rng.Intn(1000), rng.Intn(1000), rng.Intn(1000))
		openBalance := decimal.NewFromInt(int64(rng.Intn(100000) + 200000))
		if i == 0 {
			openBalance = decimal.NewFromInt(1000000)
		}
		balance := openBalance
		profileRows = append(profileRows, []any{userID, "xxx", fullName, address, email, creditCard})
		holdings := 0
		if cfg.MaxQuotes > 1 {
			holdings = rng.Intn(cfg.MaxHoldings + 1)
		}
		for j := 0; j < holdings; j++ {
			index := rng.Intn(cfg.MaxQuotes - 1)
			symbol := fmt.Sprintf("s:%d", index)
			quantity := float64(rng.Intn(200) + 1)
			price := quotes[index].price
			balance = balance.Sub(decimal.NewFromFloat(quantity).Mul(price).Add(cfg.OrderFee))
			holdingRows = append(holdingRows, []any{nextHolding, i, quantity, price, now, symbol})
			orderRows = append(orderRows, []any{nextOrder, "buy", "closed", now, now, quantity, price, cfg.OrderFee, i, symbol, nextHolding})
			nextHolding++
			nextOrder++
			if cfg.UpdateQuotePrices {
				factor := RandomPriceChangeFactor(rng)
				if price.Equal(pennyStockPrice) {
					factor = pennyMultiplier
				}
				quotes[index].price = factor.Mul(price).Round(2)
				quotes[index].volume += quantity
			}
		}
		accountRows = append(accountRows, []any{i, 0, 0, now, now, balance.Round(2), openBalance, userID})
	}
	quoteRows := make([][]any, len(quotes))
	for i, q := range quotes {
		quoteRows[i] = []any{fmt.Sprintf("s:%d", i), fmt.Sprintf("S%d Incorporated", i), q.volume, q.price, q.open, q.open, q.open, 0.0}
	}
	// The next key block starts above everything loaded, as the legacy allocator would leave it.
	blockAfter := func(used int) int {
		return int(math.Ceil(float64(used)/float64(cfg.KeyBlockSize))) * cfg.KeyBlockSize
	}
	keyRows := [][]any{
		{"account", blockAfter(cfg.MaxUsers)},
		{"holding", blockAfter(nextHolding)},
		{"order", blockAfter(nextOrder)},
	}
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "TRUNCATE accountejb, accountprofileejb, holdingejb, orderejb, quoteejb, keygenejb"); err != nil {
			return err
		}
		loads := []struct {
			table   string
			columns []string
			rows    [][]any
		}{
			{"quoteejb", []string{"symbol", "companyname", "volume", "price", "open1", "low", "high", "change1"}, quoteRows},
			{"accountprofileejb", []string{"userid", "passwd", "fullname", "address", "email", "creditcard"}, profileRows},
			{"accountejb", []string{"accountid", "logincount", "logoutcount", "lastlogin", "creationdate", "balance", "openbalance", "profile_userid"}, accountRows},
			{"holdingejb", []string{"holdingid", "account_accountid", "quantity", "purchaseprice", "purchasedate", "quote_symbol"}, holdingRows},
			{"orderejb", []string{"orderid", "ordertype", "orderstatus", "opendate", "completiondate", "quantity", "price", "orderfee", "account_accountid", "quote_symbol", "holding_holdingid"}, orderRows},
			{"keygenejb", []string{"keyname", "keyval"}, keyRows},
		}
		for _, load := range loads {
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{load.table}, load.columns, pgx.CopyFromRows(load.rows)); err != nil {
				return fmt.Errorf("load %s: %w", load.table, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	db.ResetKeys()
	return &Result{Quotes: len(quoteRows), Users: len(accountRows), Holdings: len(holdingRows), Orders: len(orderRows)}, nil
}
