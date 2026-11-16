package trading

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	daytradergo "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

const testUser = "uid:test"

// testStore gives a Store on a private schema of the PostgreSQL named by
// TRADING_TEST_DATABASE_URL (for example the compose database), with the real migrations, one
// user with 10000.00 and two quotes. Without the variable the database tests are skipped.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	databaseURL := os.Getenv("TRADING_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TRADING_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("trading_test_%d", time.Now().UnixNano())
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	entries, err := daytradergo.Migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		script, err := daytradergo.Migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, statement := range []string{
		"INSERT INTO accountprofileejb (userid, passwd, fullname, address, email, creditcard) VALUES ('" + testUser + "', 'xxx', 'Test', 'a', 'e', 'c')",
		"INSERT INTO accountejb (accountid, profile_userid, balance, openbalance, logincount, logoutcount, creationdate, lastlogin) VALUES (1, '" + testUser + "', 10000.00, 10000.00, 0, 0, now(), now())",
		"INSERT INTO quoteejb (symbol, companyname, volume, price, open1, low, high, change1) VALUES ('s:1', 'S1', 100, 50.00, 50.00, 50.00, 50.00, 0)",
		"INSERT INTO quoteejb (symbol, companyname, volume, price, open1, low, high, change1) VALUES ('s:2', 'S2', 0, 0.01, 0.01, 0.01, 0.01, 0)",
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	return store.New(pool, 10)
}

func testConfig(mode string) *config.Config {
	return &config.Config{OrderProcessingMode: mode, UpdateQuotePrices: true, OrderFee: dec("24.95")}
}

func balanceOf(t *testing.T, db *store.Store) decimal.Decimal {
	t.Helper()
	account, err := store.AccountByUser(context.Background(), db.Pool, testUser)
	if err != nil {
		t.Fatal(err)
	}
	return account.Balance
}

func orderByID(t *testing.T, db *store.Store, orderID int) *store.Order {
	t.Helper()
	order, err := store.ScanOrder(db.Pool.QueryRow(context.Background(), "SELECT "+orderColumns+" FROM orderejb WHERE orderid = $1", orderID))
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func TestBuyAndSellSync(t *testing.T) {
	db := testStore(t)
	ctx := context.Background()
	trades := newService(db, testConfig(config.OrderProcessingSync), nil)

	order, err := trades.buy(ctx, testUser, "s:1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if order.OrderStatus != statusClosed || order.CompletionDate == nil || order.HoldingID == nil || !order.Price.Equal(dec("50.00")) {
		t.Fatalf("buy order = %+v", order)
	}
	if got := balanceOf(t, db); !got.Equal(dec("9475.05")) { // 10000 - (10 x 50 + 24.95)
		t.Errorf("balance after buy = %s, want 9475.05", got)
	}
	holdings, err := store.HoldingsByUser(ctx, db.Pool, testUser)
	if err != nil || len(holdings) != 1 {
		t.Fatalf("holdings = %v, %v", holdings, err)
	}
	if holdings[0].HoldingID != *order.HoldingID || holdings[0].Quantity != 10 || !holdings[0].PurchasePrice.Equal(dec("50.00")) {
		t.Errorf("holding = %+v", holdings[0])
	}
	quote, err := store.QuoteBySymbol(ctx, db.Pool, "s:1")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Volume != 110 {
		t.Errorf("volume = %v, want 110", quote.Volume)
	}
	if quote.Price.LessThan(dec("35.00")) || quote.Price.GreaterThan(dec("65.00")) || !quote.Price.Equal(quote.Price.Round(2)) {
		t.Errorf("price moved to %s", quote.Price)
	}

	sold, err := trades.sell(ctx, testUser, holdings[0].HoldingID)
	if err != nil {
		t.Fatal(err)
	}
	if sold.OrderStatus != statusClosed || sold.OrderType != orderTypeSell || sold.Quantity != 10 || sold.HoldingID != nil {
		t.Fatalf("sell order = %+v", sold)
	}
	want := dec("9475.05").Add(quote.Price.Mul(dec("10"))).Sub(dec("24.95"))
	if got := balanceOf(t, db); !got.Equal(want) {
		t.Errorf("balance after sell = %s, want %s", got, want)
	}
	if holdings, _ = store.HoldingsByUser(ctx, db.Pool, testUser); len(holdings) != 0 {
		t.Errorf("holdings after sell = %v", holdings)
	}

	// Selling it again: cancelled, nothing changes.
	again, err := trades.sell(ctx, testUser, *order.HoldingID)
	if err != nil || again.OrderStatus != statusCancelled {
		t.Fatalf("second sell = %+v, %v", again, err)
	}
	if got := balanceOf(t, db); !got.Equal(want) {
		t.Errorf("balance after cancelled sell = %s, want %s", got, want)
	}
}

func TestBuyRejectsBadInputAndRollsBack(t *testing.T) {
	db := testStore(t)
	ctx := context.Background()
	trades := newService(db, testConfig(config.OrderProcessingSync), nil)
	if _, err := trades.buy(ctx, testUser, "s:1", 0); !errors.Is(err, errIllegalArgument) {
		t.Errorf("zero quantity: %v", err)
	}
	if _, err := trades.buy(ctx, testUser, "s:none", 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown symbol: %v", err)
	}
	if got := balanceOf(t, db); !got.Equal(dec("10000.00")) {
		t.Errorf("balance = %s, want 10000.00", got)
	}
	var orders int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM orderejb").Scan(&orders); err != nil || orders != 0 {
		t.Errorf("orders = %d, %v", orders, err)
	}
}

func TestPennyStockRecovers(t *testing.T) {
	db := testStore(t)
	ctx := context.Background()
	trades := newService(db, testConfig(config.OrderProcessingSync), nil)
	if _, err := trades.buy(ctx, testUser, "s:2", 5); err != nil {
		t.Fatal(err)
	}
	quote, err := store.QuoteBySymbol(ctx, db.Pool, "s:2")
	if err != nil || !quote.Price.Equal(dec("6.00")) {
		t.Errorf("penny stock price = %v, %v, want 6.00", quote, err)
	}
}

// Async mode: buy and sell leave the order open and queue its id; the consumer completes it.
func TestBuyAndSellAsync(t *testing.T) {
	db := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue := newMemoryQueue(8)
	trades := newService(db, testConfig(config.OrderProcessingAsync), queue)

	order, err := trades.buy(ctx, testUser, "s:1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if order.OrderStatus != statusOpen || order.CompletionDate != nil || order.HoldingID != nil {
		t.Fatalf("async buy order = %+v", order)
	}
	if got := balanceOf(t, db); !got.Equal(dec("9475.05")) {
		t.Errorf("balance = %s, want 9475.05", got)
	}
	if holdings, _ := store.HoldingsByUser(ctx, db.Pool, testUser); len(holdings) != 0 {
		t.Fatalf("holding exists before the consumer ran: %v", holdings)
	}
	if len(queue.messages) != 1 {
		t.Fatalf("queued = %d, want 1", len(queue.messages))
	}

	consumer := newOrderConsumer(queue, trades.completeOrder, quietLog())
	consumer.start(ctx)
	waitFor(t, func() bool { return orderByID(t, db, order.OrderID).OrderStatus == statusClosed })
	holdings, err := store.HoldingsByUser(ctx, db.Pool, testUser)
	if err != nil || len(holdings) != 1 || holdings[0].Quantity != 10 {
		t.Fatalf("holdings = %v, %v", holdings, err)
	}
	if closed := orderByID(t, db, order.OrderID); closed.CompletionDate == nil || closed.HoldingID == nil || *closed.HoldingID != holdings[0].HoldingID {
		t.Errorf("closed order = %+v", closed)
	}

	sold, err := trades.sell(ctx, testUser, holdings[0].HoldingID)
	if err != nil || sold.OrderStatus != statusOpen {
		t.Fatalf("async sell = %+v, %v", sold, err)
	}
	waitFor(t, func() bool { return orderByID(t, db, sold.OrderID).OrderStatus == statusClosed })
	if holdings, _ = store.HoldingsByUser(ctx, db.Pool, testUser); len(holdings) != 0 {
		t.Errorf("holdings after async sell = %v", holdings)
	}

	// Completing twice is refused and changes nothing.
	if _, err := trades.completeOrder(ctx, order.OrderID); !errors.Is(err, errOrderAlreadyCompleted) {
		t.Errorf("second completion: %v", err)
	}
}

// A failed queue send rolls the whole trade back.
func TestAsyncSendFailureRollsBack(t *testing.T) {
	db := testStore(t)
	trades := newService(db, testConfig(config.OrderProcessingAsync), failingQueue{})
	if _, err := trades.buy(context.Background(), testUser, "s:1", 10); err == nil {
		t.Fatal("buy succeeded although the queue send failed")
	}
	if got := balanceOf(t, db); !got.Equal(dec("10000.00")) {
		t.Errorf("balance = %s, want 10000.00", got)
	}
}

type failingQueue struct{}

func (failingQueue) send(context.Context, int) error { return errors.New("queue down") }
func (failingQueue) receive(context.Context) ([]queueMessage, error) {
	return nil, errors.New("queue down")
}
func (failingQueue) remove(context.Context, queueMessage) error { return nil }

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the order consumer")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
