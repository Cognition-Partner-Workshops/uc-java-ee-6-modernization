// Package store is the shared data layer: the six DayTrader tables as Go structs, the pgx pool,
// the KEYGENEJB key allocator and the reads that more than one feature area needs.
// Feature areas keep their own SQL in their own package and use Store.Pool directly.
package store

import (
	"time"

	"github.com/shopspring/decimal"
)

// Account is a row of accountejb (AccountDataBean).
type Account struct {
	AccountID    int
	LoginCount   int
	LogoutCount  int
	LastLogin    time.Time
	CreationDate time.Time
	Balance      decimal.Decimal
	OpenBalance  decimal.Decimal
	ProfileID    string
}

// Profile is a row of accountprofileejb (AccountProfileDataBean).
type Profile struct {
	UserID     string
	Password   string
	FullName   string
	Address    string
	Email      string
	CreditCard string
}

// Quote is a row of quoteejb (QuoteDataBean).
type Quote struct {
	Symbol      string
	CompanyName string
	Volume      float64
	Price       decimal.Decimal
	Open        decimal.Decimal
	Low         decimal.Decimal
	High        decimal.Decimal
	Change      float64
}

// Holding is a row of holdingejb (HoldingDataBean).
type Holding struct {
	HoldingID     int
	AccountID     int
	Quantity      float64
	PurchasePrice decimal.Decimal
	PurchaseDate  time.Time
	QuoteID       string
}

// Order is a row of orderejb (OrderDataBean).
type Order struct {
	OrderID        int
	OrderType      string // buy | sell
	OrderStatus    string // open | processing | closed | completed | cancelled
	OpenDate       time.Time
	CompletionDate *time.Time
	Quantity       float64
	Price          decimal.Decimal
	OrderFee       decimal.Decimal
	AccountID      int
	Symbol         string
	HoldingID      *int
}
