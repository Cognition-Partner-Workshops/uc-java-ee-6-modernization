package seed_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/seed"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// openTestStore connects to TEST_DATABASE_URL (skips the test when it is unset) inside a
// schema of this package's own, so packages can run in parallel and a seeded database
// (make go-run) is left alone.
func openTestStore(t *testing.T, keyBlockSize int) *store.Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; start PostgreSQL with `make go-db` to run the database tests")
	}
	const schema = "daytrader_test_seed"
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	_, err = admin.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema)
	_ = admin.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := store.Open(ctx, parsed.String(), 4, keyBlockSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}

func smallConfig(t *testing.T, overrides map[string]string) *config.Config {
	t.Helper()
	values := map[string]string{"MAX_USERS": "25", "MAX_QUOTES": "12", "MAX_HOLDINGS": "4", "KEY_BLOCK_SIZE": "10"}
	for name, value := range overrides {
		values[name] = value
	}
	cfg, err := config.FromEnv(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func count(t *testing.T, db *store.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TradeConfigTest.priceChangeFactorStaysWithinTwentyPercent
func TestPriceChangeFactorStaysWithinTwentyPercent(t *testing.T) {
	low, high := decimal.RequireFromString("0.80"), decimal.RequireFromString("1.20")
	rng := rand.New(rand.NewSource(1))
	sawLoss, sawGain := false, false
	for i := 0; i < 5000; i++ {
		factor := seed.RandomPriceChangeFactor(rng)
		if factor.LessThan(low) || factor.GreaterThan(high) {
			t.Fatalf("factor %s is outside 0.80..1.20", factor)
		}
		if factor.Exponent() != -2 {
			t.Fatalf("factor %s has scale %d, want 2", factor, -factor.Exponent())
		}
		sawLoss = sawLoss || factor.LessThan(decimal.NewFromInt(1))
		sawGain = sawGain || factor.GreaterThan(decimal.NewFromInt(1))
	}
	if !sawLoss || !sawGain {
		t.Errorf("sawLoss %v sawGain %v", sawLoss, sawGain)
	}
}

// EntityMappingTest.entitiesMapToTheDerbyTablesWithNonNullIds, against the live schema.
func TestBuildTablesCreatesTheLegacyTablesWithNonNullIDs(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{
		"accountejb": "accountid", "accountprofileejb": "userid", "holdingejb": "holdingid",
		"orderejb": "orderid", "quoteejb": "symbol", "keygenejb": "keyname",
	}
	if got := count(t, db, "SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()"); got != len(ids) {
		t.Errorf("%d tables, want %d", got, len(ids))
	}
	for table, idColumn := range ids {
		var nullable string
		err := db.Pool.QueryRow(ctx, "SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema() "+
			"AND table_name = $1 AND column_name = $2", table, idColumn).Scan(&nullable)
		if err != nil || nullable != "NO" {
			t.Errorf("%s.%s: is_nullable %q, %v", table, idColumn, nullable, err)
		}
		primaryKey := count(t, db, "SELECT count(*) FROM information_schema.table_constraints c JOIN information_schema.key_column_usage k "+
			"ON k.constraint_name = c.constraint_name AND k.table_schema = c.table_schema "+
			"WHERE c.table_schema = current_schema() AND c.constraint_type = 'PRIMARY KEY' AND c.table_name = $1 AND k.column_name = $2", table, idColumn)
		if primaryKey != 1 {
			t.Errorf("%s: %s is not the primary key", table, idColumn)
		}
		if rows := count(t, db, "SELECT count(*) FROM "+table); rows != 0 {
			t.Errorf("%s has %d rows after BuildTables", table, rows)
		}
	}
}

func TestBuildTablesDropsTheDataAndTheKeyBlocks(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.BuildDB(ctx, db, smallConfig(t, nil), rand.New(rand.NewSource(3))); err != nil {
		t.Fatal(err)
	}
	if id, err := db.NextID(ctx, "account"); err != nil || id != 30 {
		t.Fatalf("NextID(account) after BuildDB = %d, %v; want 30", id, err)
	}
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accountejb", "accountprofileejb", "holdingejb", "orderejb", "quoteejb", "keygenejb"} {
		if rows := count(t, db, "SELECT count(*) FROM "+table); rows != 0 {
			t.Errorf("%s has %d rows", table, rows)
		}
	}
	if id, err := db.NextID(ctx, "account"); err != nil || id != 0 {
		t.Errorf("NextID(account) after BuildTables = %d, %v; want 0", id, err)
	}
}

func TestBuildDBPopulatesUsersQuotesHoldingsAndOrders(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	cfg := smallConfig(t, nil)
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	result, err := seed.BuildDB(ctx, db, cfg, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Users != 25 || result.Quotes != 12 || result.Holdings != result.Orders || result.Holdings == 0 {
		t.Fatalf("result = %+v", result)
	}
	for table, want := range map[string]int{
		"accountejb": 25, "accountprofileejb": 25, "quoteejb": 12, "holdingejb": result.Holdings, "orderejb": result.Orders, "keygenejb": 3,
	} {
		if got := count(t, db, "SELECT count(*) FROM "+table); got != want {
			t.Errorf("%s has %d rows, want %d", table, got, want)
		}
	}

	// Users: uid:<n>, password xxx, account id n, uid:0 opens with 1,000,000, the others 200,000..299,999.
	for i := 0; i < cfg.MaxUsers; i++ {
		userID := "uid:" + strconv.Itoa(i)
		profile, err := store.ProfileByUser(ctx, db.Pool, userID)
		if err != nil {
			t.Fatalf("%s: %v", userID, err)
		}
		if profile.Password != "xxx" || !strings.HasPrefix(profile.FullName, "first:") || !strings.HasSuffix(profile.Address, " Oak St.") ||
			!strings.HasPrefix(profile.Email, userID+"@") || !strings.HasSuffix(profile.Email, ".com") || strings.Count(profile.CreditCard, "-") != 3 {
			t.Errorf("profile = %+v", profile)
		}
		account, err := store.AccountByUser(ctx, db.Pool, userID)
		if err != nil {
			t.Fatalf("%s: %v", userID, err)
		}
		if account.AccountID != i || account.LoginCount != 0 || account.LogoutCount != 0 {
			t.Errorf("account = %+v", account)
		}
		open := account.OpenBalance.IntPart()
		if i == 0 && open != 1000000 || i > 0 && (open < 200000 || open > 299999) {
			t.Errorf("%s open balance = %s", userID, account.OpenBalance)
		}
		// Each seeded buy debits quantity x price + 24.95.
		holdings, err := store.HoldingsByUser(ctx, db.Pool, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(holdings) > cfg.MaxHoldings {
			t.Errorf("%s has %d holdings, MAX_HOLDINGS is %d", userID, len(holdings), cfg.MaxHoldings)
		}
		orders, err := store.OrdersByUser(ctx, db.Pool, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(orders) != len(holdings) {
			t.Errorf("%s: %d orders, %d holdings", userID, len(orders), len(holdings))
		}
		spent := decimal.Zero
		for _, holding := range holdings {
			spent = spent.Add(decimal.NewFromFloat(holding.Quantity).Mul(holding.PurchasePrice)).Add(cfg.OrderFee)
			// TradeConfigTest.randomSymbolsStayInsideTheQuotePopulation: rndSymbol never picks the last quote.
			n, err := strconv.Atoi(strings.TrimPrefix(holding.QuoteID, "s:"))
			if err != nil || n < 0 || n >= cfg.MaxQuotes-1 {
				t.Errorf("holding %d has symbol %q", holding.HoldingID, holding.QuoteID)
			}
			if holding.Quantity < 1 || holding.Quantity > 200 || holding.Quantity != float64(int(holding.Quantity)) {
				t.Errorf("holding %d has quantity %v", holding.HoldingID, holding.Quantity)
			}
		}
		if want := account.OpenBalance.Sub(spent); !account.Balance.Equal(want) {
			t.Errorf("%s balance = %s, want %s", userID, account.Balance, want)
		}
		for _, order := range orders {
			if order.OrderType != "buy" || order.OrderStatus != "closed" || order.OrderFee.StringFixed(2) != "24.95" ||
				order.CompletionDate == nil || order.HoldingID == nil || order.AccountID != i {
				t.Errorf("order = %+v", order)
				continue
			}
			holding, err := store.HoldingByID(ctx, db.Pool, *order.HoldingID)
			if err != nil || holding.AccountID != i || holding.QuoteID != order.Symbol || holding.Quantity != order.Quantity ||
				!holding.PurchasePrice.Equal(order.Price) {
				t.Errorf("order %d does not match holding %+v (%v)", order.OrderID, holding, err)
			}
		}
	}

	// Quotes: s:<n>, "S<n> Incorporated", opened at 1..200, volume = shares bought.
	for i := 0; i < cfg.MaxQuotes; i++ {
		symbol := "s:" + strconv.Itoa(i)
		quote, err := store.QuoteBySymbol(ctx, db.Pool, symbol)
		if err != nil {
			t.Fatalf("%s: %v", symbol, err)
		}
		open := quote.Open.IntPart()
		if quote.CompanyName != fmt.Sprintf("S%d Incorporated", i) || open < 1 || open > 200 || !quote.Open.Equal(decimal.NewFromInt(open)) ||
			!quote.Low.Equal(quote.Open) || !quote.High.Equal(quote.Open) || quote.Change != 0 || !quote.Price.IsPositive() {
			t.Errorf("quote = %+v", quote)
		}
		var bought float64
		if err := db.Pool.QueryRow(ctx, "SELECT coalesce(sum(quantity), 0) FROM holdingejb WHERE quote_symbol = $1", symbol).Scan(&bought); err != nil {
			t.Fatal(err)
		}
		if quote.Volume != bought {
			t.Errorf("%s volume = %v, shares bought = %v", symbol, quote.Volume, bought)
		}
		if bought == 0 && !quote.Price.Equal(quote.Open) {
			t.Errorf("%s was never bought but price %s differs from open %s", symbol, quote.Price, quote.Open)
		}
	}

	// keygenejb: the next block starts above everything loaded.
	blockAfter := func(used int) int { return (used + cfg.KeyBlockSize - 1) / cfg.KeyBlockSize * cfg.KeyBlockSize }
	for keyName, used := range map[string]int{"account": 25, "holding": result.Holdings, "order": result.Orders} {
		if got := count(t, db, "SELECT keyval FROM keygenejb WHERE keyname = $1", keyName); got != blockAfter(used) {
			t.Errorf("keygenejb %s = %d, want %d", keyName, got, blockAfter(used))
		}
		id, err := db.NextID(ctx, keyName)
		if err != nil || id < used || id != blockAfter(used) {
			t.Errorf("NextID(%s) = %d, %v; %d ids are in use", keyName, id, err, used)
		}
	}
	if maxOrder := count(t, db, "SELECT max(orderid) FROM orderejb"); maxOrder != result.Orders-1 {
		t.Errorf("max(orderid) = %d, want %d", maxOrder, result.Orders-1)
	}
}

func accountBalances(t *testing.T, db *store.Store) string {
	t.Helper()
	var text string
	err := db.Pool.QueryRow(context.Background(), "SELECT string_agg(accountid || '=' || balance, ',' ORDER BY accountid) FROM accountejb").Scan(&text)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestBuildDBIsRepeatableForTheSameRandomSeedAndReplacesTheRows(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	cfg := smallConfig(t, nil)
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	first, err := seed.BuildDB(ctx, db, cfg, rand.New(rand.NewSource(11)))
	if err != nil {
		t.Fatal(err)
	}
	firstBalances := accountBalances(t, db)
	second, err := seed.BuildDB(ctx, db, cfg, rand.New(rand.NewSource(11)))
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second || accountBalances(t, db) != firstBalances {
		t.Errorf("same seed, different data: %+v vs %+v", first, second)
	}
	if got := count(t, db, "SELECT count(*) FROM accountejb"); got != 25 {
		t.Errorf("%d accounts after the second run, want 25 (rows are replaced, not added)", got)
	}
	if _, err := seed.BuildDB(ctx, db, cfg, rand.New(rand.NewSource(12))); err != nil {
		t.Fatal(err)
	}
	if accountBalances(t, db) == firstBalances {
		t.Error("a different seed produced the same balances")
	}
	// A nil rng seeds from the clock.
	if result, err := seed.BuildDB(ctx, db, cfg, nil); err != nil || result.Users != 25 {
		t.Errorf("nil rng: %+v, %v", result, err)
	}
}

func TestBuildDBWithoutQuotePriceUpdatesKeepsTheOpeningPrices(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	result, err := seed.BuildDB(ctx, db, smallConfig(t, map[string]string{"UPDATE_QUOTE_PRICES": "false"}), rand.New(rand.NewSource(5)))
	if err != nil || result.Holdings == 0 {
		t.Fatalf("%+v, %v", result, err)
	}
	if moved := count(t, db, "SELECT count(*) FROM quoteejb WHERE price <> open1 OR volume <> 0"); moved != 0 {
		t.Errorf("%d quotes moved", moved)
	}
}

func TestBuildDBWithOneQuoteCreatesNoHoldings(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	result, err := seed.BuildDB(ctx, db, smallConfig(t, map[string]string{"MAX_USERS": "3", "MAX_QUOTES": "1"}), rand.New(rand.NewSource(5)))
	if err != nil {
		t.Fatal(err)
	}
	if *result != (seed.Result{Quotes: 1, Users: 3}) {
		t.Errorf("result = %+v", result)
	}
	if unchanged := count(t, db, "SELECT count(*) FROM accountejb WHERE balance = openbalance"); unchanged != 3 {
		t.Errorf("%d accounts kept their open balance, want 3", unchanged)
	}
	if got := count(t, db, "SELECT keyval FROM keygenejb WHERE keyname = 'account'"); got != 10 {
		t.Errorf("account key = %d, want 10", got)
	}
}

func TestBuildDBFailsWithoutTables(t *testing.T) {
	db := openTestStore(t, 10)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "DROP TABLE quoteejb"); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.BuildDB(ctx, db, smallConfig(t, nil), rand.New(rand.NewSource(1))); err == nil {
		t.Error("no error without quoteejb")
	}
	if err := seed.BuildTables(ctx, db); err != nil {
		t.Errorf("BuildTables must recover a partly dropped schema: %v", err)
	}
}
