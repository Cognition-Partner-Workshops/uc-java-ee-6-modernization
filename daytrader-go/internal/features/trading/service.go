package trading

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/seed"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

const (
	orderTypeBuy  = "buy"
	orderTypeSell = "sell"

	statusOpen      = "open"
	statusClosed    = "closed"
	statusCancelled = "cancelled"

	orderColumns = "orderid, ordertype, orderstatus, opendate, completiondate, quantity, price, orderfee, account_accountid, quote_symbol, holding_holdingid"
)

var (
	// errIllegalArgument is the legacy IllegalArgumentException: a user error, not an HTTP 500.
	errIllegalArgument = errors.New("illegal argument")
	// errOrderAlreadyCompleted is the legacy "attempt to complete Order that is already completed".
	errOrderAlreadyCompleted = errors.New("attempt to complete order that is already completed")

	pennyStockPrice = decimal.RequireFromString("0.01")
	pennyMultiplier = decimal.NewFromInt(600)
)

// service is the Go replacement for the trading half of TradeSLSBBean / TradeAction: buy, sell,
// completeOrder, cancelOrder, queueOrder and updateQuotePriceVolume. Every public method is one
// explicit transaction (store.InTx) instead of a container-managed one.
type service struct {
	db    *store.Store
	cfg   *config.Config
	queue orderQueue // nil in sync mode

	rngMutex sync.Mutex
	rng      *rand.Rand
}

func newService(db *store.Store, cfg *config.Config, queue orderQueue) *service {
	return &service{db: db, cfg: cfg, queue: queue, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (s *service) async() bool {
	return s.cfg.OrderProcessingMode == config.OrderProcessingAsync && s.queue != nil
}

func (s *service) priceChangeFactor() decimal.Decimal {
	s.rngMutex.Lock()
	defer s.rngMutex.Unlock()
	return seed.RandomPriceChangeFactor(s.rng)
}

func timestampNow() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// orderTotal is the legacy new BigDecimal(quantity).multiply(price), before the fee is added
// (buy) or subtracted (sell).
func orderTotal(quantity float64, price decimal.Decimal) decimal.Decimal {
	return decimal.NewFromFloat(quantity).Mul(price)
}

// buyTotal is what a buy debits: quantity x price + order fee.
func buyTotal(quantity float64, price, orderFee decimal.Decimal) decimal.Decimal {
	return orderTotal(quantity, price).Add(orderFee)
}

// sellTotal is what a sell credits: quantity x price - order fee.
func sellTotal(quantity float64, price, orderFee decimal.Decimal) decimal.Decimal {
	return orderTotal(quantity, price).Sub(orderFee)
}

// newQuotePrice is the price half of updateQuotePriceVolume: a penny stock recovers by x600,
// anything else moves by the change factor, rounded half up to two places.
func newQuotePrice(oldPrice, changeFactor decimal.Decimal) decimal.Decimal {
	if oldPrice.Equal(pennyStockPrice) {
		changeFactor = pennyMultiplier
	}
	return changeFactor.Mul(oldPrice).Round(2)
}

// buy is TradeAction.buy + TradeSLSBBean.buy: debit the account, create the order, complete it
// (sync) or queue it (async), then move the quote's price and volume. One transaction.
func (s *service) buy(ctx context.Context, userID, symbol string, quantity float64) (*store.Order, error) {
	if math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity <= 0 {
		return nil, fmt.Errorf("%w: quantity %v", errIllegalArgument, quantity)
	}
	var order *store.Order
	err := s.db.InTx(ctx, func(tx pgx.Tx) error {
		quote, err := store.ScanQuote(tx.QueryRow(ctx,
			"SELECT symbol, companyname, volume, price, open1, low, high, change1 FROM quoteejb WHERE symbol = $1 FOR UPDATE", symbol))
		if err != nil {
			return fmt.Errorf("quote %q: %w", symbol, err)
		}
		account, err := store.AccountByUser(ctx, tx, userID)
		if err != nil {
			return fmt.Errorf("account of %q: %w", userID, err)
		}
		order, err = s.createOrder(ctx, tx, account.AccountID, quote, nil, orderTypeBuy, quantity)
		if err != nil {
			return err
		}
		if err := adjustBalance(ctx, tx, account.AccountID, buyTotal(quantity, quote.Price, order.OrderFee).Neg()); err != nil {
			return err
		}
		if err := s.finishOrder(ctx, tx, order); err != nil {
			return err
		}
		return s.updateQuotePriceVolume(ctx, tx, quote, s.priceChangeFactor(), quantity)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// sell is TradeAction.sell + TradeSLSBBean.sell. A holding that is gone (already sold) gives a
// cancelled order and changes nothing, as in the legacy bean.
func (s *service) sell(ctx context.Context, userID string, holdingID int) (*store.Order, error) {
	var order *store.Order
	err := s.db.InTx(ctx, func(tx pgx.Tx) error {
		account, err := store.AccountByUser(ctx, tx, userID)
		if err != nil {
			return fmt.Errorf("account of %q: %w", userID, err)
		}
		holding, err := store.ScanHolding(tx.QueryRow(ctx,
			"SELECT holdingid, account_accountid, quantity, purchaseprice, purchasedate, quote_symbol FROM holdingejb "+
				"WHERE holdingid = $1 AND account_accountid = $2 FOR UPDATE", holdingID, account.AccountID))
		if errors.Is(err, store.ErrNotFound) {
			order = &store.Order{OrderStatus: statusCancelled}
			return nil
		}
		if err != nil {
			return err
		}
		quote, err := store.ScanQuote(tx.QueryRow(ctx,
			"SELECT symbol, companyname, volume, price, open1, low, high, change1 FROM quoteejb WHERE symbol = $1 FOR UPDATE", holding.QuoteID))
		if err != nil {
			return fmt.Errorf("quote %q: %w", holding.QuoteID, err)
		}
		order, err = s.createOrder(ctx, tx, account.AccountID, quote, &holding.HoldingID, orderTypeSell, holding.Quantity)
		if err != nil {
			return err
		}
		// The legacy bean marks the holding "in flight" by setting its purchase date to the epoch.
		if _, err := tx.Exec(ctx, "UPDATE holdingejb SET purchasedate = $2 WHERE holdingid = $1", holding.HoldingID, time.Unix(0, 0).UTC()); err != nil {
			return err
		}
		if err := adjustBalance(ctx, tx, account.AccountID, sellTotal(holding.Quantity, quote.Price, order.OrderFee)); err != nil {
			return err
		}
		if err := s.finishOrder(ctx, tx, order); err != nil {
			return err
		}
		return s.updateQuotePriceVolume(ctx, tx, quote, s.priceChangeFactor(), holding.Quantity)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// finishOrder completes the order in this transaction (sync) or leaves it open and queues its
// id (async, the legacy queueOrder). The send is the last step before commit: when it fails the
// whole trade rolls back, which is what the legacy two-phase commit gave.
func (s *service) finishOrder(ctx context.Context, tx pgx.Tx, order *store.Order) error {
	if s.async() {
		return s.queue.send(ctx, order.OrderID)
	}
	return s.completeOrderTx(ctx, tx, order)
}

// createOrder inserts an open order at the quote's current price with the configured fee.
func (s *service) createOrder(ctx context.Context, tx pgx.Tx, accountID int, quote *store.Quote, holdingID *int, orderType string, quantity float64) (*store.Order, error) {
	orderID, err := s.db.NextID(ctx, "order")
	if err != nil {
		return nil, fmt.Errorf("order id: %w", err)
	}
	order := &store.Order{
		OrderID:     orderID,
		OrderType:   orderType,
		OrderStatus: statusOpen,
		OpenDate:    timestampNow(),
		Quantity:    quantity,
		Price:       quote.Price.Round(2),
		OrderFee:    s.cfg.OrderFeeFor(orderType),
		AccountID:   accountID,
		Symbol:      quote.Symbol,
		HoldingID:   holdingID,
	}
	_, err = tx.Exec(ctx, "INSERT INTO orderejb ("+orderColumns+") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
		order.OrderID, order.OrderType, order.OrderStatus, order.OpenDate, order.CompletionDate, order.Quantity, order.Price,
		order.OrderFee, order.AccountID, order.Symbol, order.HoldingID)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	return order, nil
}

func adjustBalance(ctx context.Context, tx pgx.Tx, accountID int, amount decimal.Decimal) error {
	_, err := tx.Exec(ctx, "UPDATE accountejb SET balance = balance + $2 WHERE accountid = $1", accountID, amount.Round(2))
	return err
}

// completeOrder is TradeSLSBBean.completeOrder for the queue consumer: load the order by id and
// complete it in its own transaction.
func (s *service) completeOrder(ctx context.Context, orderID int) (*store.Order, error) {
	var order *store.Order
	err := s.db.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		order, err = store.ScanOrder(tx.QueryRow(ctx, "SELECT "+orderColumns+" FROM orderejb WHERE orderid = $1 FOR UPDATE", orderID))
		if err != nil {
			return err
		}
		return s.completeOrderTx(ctx, tx, order)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// completeOrderTx completes an open order: a buy gets its holding, a sell loses it (or is
// cancelled when the holding is already gone), then the order is closed.
func (s *service) completeOrderTx(ctx context.Context, tx pgx.Tx, order *store.Order) error {
	if order.OrderStatus != statusOpen && order.OrderStatus != "processing" {
		return fmt.Errorf("%w: order %d is %s", errOrderAlreadyCompleted, order.OrderID, order.OrderStatus)
	}
	switch order.OrderType {
	case orderTypeBuy:
		holdingID, err := s.db.NextID(ctx, "holding")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO holdingejb (holdingid, account_accountid, quantity, purchaseprice, purchasedate, quote_symbol) "+
			"VALUES ($1, $2, $3, $4, $5, $6)", holdingID, order.AccountID, order.Quantity, order.Price, timestampNow(), order.Symbol)
		if err != nil {
			return fmt.Errorf("create holding: %w", err)
		}
		order.HoldingID = &holdingID
	case orderTypeSell:
		removed := int64(0)
		if order.HoldingID != nil {
			tag, err := tx.Exec(ctx, "DELETE FROM holdingejb WHERE holdingid = $1", *order.HoldingID)
			if err != nil {
				return err
			}
			removed = tag.RowsAffected()
		}
		order.HoldingID = nil
		if removed == 0 {
			order.OrderStatus = statusCancelled
			_, err := tx.Exec(ctx, "UPDATE orderejb SET orderstatus = $2, holding_holdingid = NULL WHERE orderid = $1", order.OrderID, order.OrderStatus)
			return err
		}
	}
	completed := timestampNow()
	order.OrderStatus = statusClosed
	order.CompletionDate = &completed
	_, err := tx.Exec(ctx, "UPDATE orderejb SET orderstatus = $2, completiondate = $3, holding_holdingid = $4 WHERE orderid = $1",
		order.OrderID, order.OrderStatus, order.CompletionDate, order.HoldingID)
	return err
}

// cancelOrder is TradeSLSBBean.cancelOrder.
func (s *service) cancelOrder(ctx context.Context, orderID int) error {
	return s.db.InTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE orderejb SET orderstatus = $2 WHERE orderid = $1", orderID, statusCancelled)
		if err == nil && tag.RowsAffected() == 0 {
			return store.ErrNotFound
		}
		return err
	})
}

// updateQuotePriceVolume moves the (already locked) quote's price and adds the traded shares to
// its volume. Like the legacy session bean it leaves open, low, high and change alone.
func (s *service) updateQuotePriceVolume(ctx context.Context, tx pgx.Tx, quote *store.Quote, changeFactor decimal.Decimal, sharesTraded float64) error {
	if !s.cfg.UpdateQuotePrices {
		return nil
	}
	_, err := tx.Exec(ctx, "UPDATE quoteejb SET price = $2, volume = volume + $3 WHERE symbol = $1",
		quote.Symbol, newQuotePrice(quote.Price, changeFactor), sharesTraded)
	return err
}
