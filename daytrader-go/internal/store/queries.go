package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const (
	accountColumns = "accountid, logincount, logoutcount, lastlogin, creationdate, balance, openbalance, profile_userid"
	profileColumns = "userid, passwd, fullname, address, email, creditcard"
	quoteColumns   = "symbol, companyname, volume, price, open1, low, high, change1"
	holdingColumns = "holdingid, account_accountid, quantity, purchaseprice, purchasedate, quote_symbol"
	orderColumns   = "orderid, ordertype, orderstatus, opendate, completiondate, quantity, price, orderfee, account_accountid, quote_symbol, holding_holdingid"
)

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func ScanAccount(row pgx.Row) (*Account, error) {
	a := &Account{}
	err := row.Scan(&a.AccountID, &a.LoginCount, &a.LogoutCount, &a.LastLogin, &a.CreationDate, &a.Balance, &a.OpenBalance, &a.ProfileID)
	return a, notFound(err)
}

func ScanProfile(row pgx.Row) (*Profile, error) {
	p := &Profile{}
	err := row.Scan(&p.UserID, &p.Password, &p.FullName, &p.Address, &p.Email, &p.CreditCard)
	return p, notFound(err)
}

func ScanQuote(row pgx.Row) (*Quote, error) {
	q := &Quote{}
	err := row.Scan(&q.Symbol, &q.CompanyName, &q.Volume, &q.Price, &q.Open, &q.Low, &q.High, &q.Change)
	return q, notFound(err)
}

func ScanHolding(row pgx.Row) (*Holding, error) {
	h := &Holding{}
	err := row.Scan(&h.HoldingID, &h.AccountID, &h.Quantity, &h.PurchasePrice, &h.PurchaseDate, &h.QuoteID)
	return h, notFound(err)
}

func ScanOrder(row pgx.Row) (*Order, error) {
	o := &Order{}
	err := row.Scan(&o.OrderID, &o.OrderType, &o.OrderStatus, &o.OpenDate, &o.CompletionDate, &o.Quantity, &o.Price, &o.OrderFee, &o.AccountID, &o.Symbol, &o.HoldingID)
	return o, notFound(err)
}

func collect[T any](rows pgx.Rows, err error, scan func(pgx.Row) (*T, error)) ([]*T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// AccountByUser is getAccountData(userID).
func AccountByUser(ctx context.Context, q Querier, userID string) (*Account, error) {
	return ScanAccount(q.QueryRow(ctx, "SELECT "+accountColumns+" FROM accountejb WHERE profile_userid = $1", userID))
}

// ProfileByUser is getAccountProfileData(userID).
func ProfileByUser(ctx context.Context, q Querier, userID string) (*Profile, error) {
	return ScanProfile(q.QueryRow(ctx, "SELECT "+profileColumns+" FROM accountprofileejb WHERE userid = $1", userID))
}

// QuoteBySymbol is getQuote(symbol).
func QuoteBySymbol(ctx context.Context, q Querier, symbol string) (*Quote, error) {
	return ScanQuote(q.QueryRow(ctx, "SELECT "+quoteColumns+" FROM quoteejb WHERE symbol = $1", symbol))
}

// HoldingByID is getHolding(holdingID).
func HoldingByID(ctx context.Context, q Querier, holdingID int) (*Holding, error) {
	return ScanHolding(q.QueryRow(ctx, "SELECT "+holdingColumns+" FROM holdingejb WHERE holdingid = $1", holdingID))
}

// HoldingsByUser is getHoldings(userID).
func HoldingsByUser(ctx context.Context, q Querier, userID string) ([]*Holding, error) {
	rows, err := q.Query(ctx, "SELECT "+holdingColumns+" FROM holdingejb h WHERE h.account_accountid = "+
		"(SELECT a.accountid FROM accountejb a WHERE a.profile_userid = $1) ORDER BY h.holdingid", userID)
	return collect(rows, err, ScanHolding)
}

// OrdersByUser is getOrders(userID).
func OrdersByUser(ctx context.Context, q Querier, userID string) ([]*Order, error) {
	rows, err := q.Query(ctx, "SELECT "+orderColumns+" FROM orderejb o WHERE o.account_accountid = "+
		"(SELECT a.accountid FROM accountejb a WHERE a.profile_userid = $1) ORDER BY o.orderid DESC", userID)
	return collect(rows, err, ScanOrder)
}

// ClosedOrders is getClosedOrders(userID): returns the user's orders in status "closed" and
// marks them "completed" in the same transaction, so each is alerted once. Every signed-in
// page shows this list through the shared "closedOrdersAlert" template.
func (s *Store) ClosedOrders(ctx context.Context, userID string) ([]*Order, error) {
	var orders []*Order
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+orderColumns+" FROM orderejb o WHERE o.orderstatus = 'closed' AND o.account_accountid = "+
			"(SELECT a.accountid FROM accountejb a WHERE a.profile_userid = $1) ORDER BY o.orderid", userID)
		orders, err = collect(rows, err, ScanOrder)
		if err != nil || len(orders) == 0 {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE orderejb SET orderstatus = 'completed' WHERE orderstatus = 'closed' AND account_accountid = "+
			"(SELECT a.accountid FROM accountejb a WHERE a.profile_userid = $1)", userID)
		// The legacy bean returns the orders with the new status already set.
		for _, order := range orders {
			order.OrderStatus = "completed"
		}
		return err
	})
	return orders, err
}
