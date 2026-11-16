package trading

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/javafmt"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

const (
	orderTitle      = "DayTrader Order information"
	orderBanner     = "DayTrader New Orders"
	portfolioTitle  = "DayTrader Portfolio"
	portfolioBanner = "DayTrader Portfolio"
)

type orderPage struct {
	Page  web.Page
	Order *store.Order
}

type holdingRow struct {
	Holding      *store.Holding
	CurrentPrice string
	Basis        string
	MarketValue  string
	Gain         template.HTML
}

type portfolioPage struct {
	Page        web.Page
	Holdings    []holdingRow
	TotalBasis  string
	TotalValue  string
	TotalGain   template.HTML
	GainPercent template.HTML
}

// scaleOf is BigDecimal.scale() for the values DayTrader shows: the number of decimal places,
// never negative (new BigDecimal(10.0) has scale 0).
func scaleOf(value decimal.Decimal) int32 {
	if scale := -value.Exponent(); scale > 0 {
		return scale
	}
	return 0
}

// javaDecimal is a BigDecimal as portfolio.jsp prints it: toString() keeps the scale, so
// 9.00 x 10 is "90.00" and an empty total is "0". Multiplying adds the scales, adding keeps
// the larger one.
type javaDecimal struct {
	value decimal.Decimal
	scale int32
}

func newJavaDecimal(value decimal.Decimal) javaDecimal {
	return javaDecimal{value: value, scale: scaleOf(value)}
}

func (d javaDecimal) mul(other javaDecimal) javaDecimal {
	return javaDecimal{value: d.value.Mul(other.value), scale: d.scale + other.scale}
}

func (d javaDecimal) add(other javaDecimal) javaDecimal {
	return javaDecimal{value: d.value.Add(other.value), scale: max(d.scale, other.scale)}
}

func (d javaDecimal) sub(other javaDecimal) javaDecimal {
	return javaDecimal{value: d.value.Sub(other.value), scale: max(d.scale, other.scale)}
}

func (d javaDecimal) String() string { return d.value.StringFixed(d.scale) }

// buy is TradeServletAction.doBuy.
func buy(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	current, err := runtimeFor(app)
	if err != nil {
		app.Error(w, r, err)
		return
	}
	symbol := web.Param(r, "symbol")
	// new Double(quantity): anything that is not a number is a NumberFormatException, an
	// IllegalArgumentException, which the legacy servlet answers with the home page.
	quantity, err := strconv.ParseFloat(strings.TrimSpace(web.Param(r, "quantity")), 64)
	if err != nil {
		app.Log.Error("trading: buy with illegal quantity", "user", session.UserID, "symbol", symbol)
		app.Forward(w, r, session, "home", map[string]string{"results": "illegal argument:"})
		return
	}
	// The page's closed-order alert is read before the trade: the order placed by this request
	// is alerted on the next page, as recorded from the legacy app.
	page := app.NewPage(r, session, orderTitle, orderBanner)
	order, err := current.service.buy(r.Context(), session.UserID, symbol, quantity)
	if errors.Is(err, errIllegalArgument) {
		app.Log.Error("trading: buy with illegal argument", "user", session.UserID, "symbol", symbol, "error", err)
		app.Forward(w, r, session, "home", map[string]string{"results": "illegal argument:"})
		return
	}
	if err != nil {
		app.Error(w, r, fmt.Errorf("TradeServletAction.buy(...) exception buying stock %s for user %s: %w", symbol, session.UserID, err))
		return
	}
	app.Render(w, r, http.StatusOK, "trading_order", orderPage{Page: page, Order: order})
}

// sell is TradeServletAction.doSell.
func sell(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	current, err := runtimeFor(app)
	if err != nil {
		app.Error(w, r, err)
		return
	}
	holdingID, err := strconv.Atoi(web.Param(r, "holdingID"))
	if err != nil {
		app.Error(w, r, fmt.Errorf("sell: holdingID: %w", err))
		return
	}
	page := app.NewPage(r, session, orderTitle, orderBanner)
	order, err := current.service.sell(r.Context(), session.UserID, holdingID)
	if err != nil {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doSell(...) exception selling holding %d for user =%s: %w", holdingID, session.UserID, err))
		return
	}
	app.Render(w, r, http.StatusOK, "trading_order", orderPage{Page: page, Order: order})
}

// portfolio is TradeServletAction.doPortfolio + portfolio.jsp.
func portfolio(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	ctx := r.Context()
	holdings, err := store.HoldingsByUser(ctx, app.Store.Pool, session.UserID)
	if err != nil {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doPortfolio(...) exception user =%s: %w", session.UserID, err))
		return
	}
	data := portfolioPage{}
	totalBasis, totalValue, totalGain := javaDecimal{}, javaDecimal{}, javaDecimal{}
	prices := map[string]decimal.Decimal{}
	for _, holding := range holdings {
		price, known := prices[holding.QuoteID]
		if !known {
			quote, err := store.QuoteBySymbol(ctx, app.Store.Pool, holding.QuoteID)
			if err != nil {
				app.Error(w, r, fmt.Errorf("TradeServletAction.doPortfolio(...) quote %s: %w", holding.QuoteID, err))
				return
			}
			price = quote.Price
			prices[holding.QuoteID] = price
		}
		quantity := newJavaDecimal(decimal.NewFromFloat(holding.Quantity))
		basis := newJavaDecimal(holding.PurchasePrice).mul(quantity)
		marketValue := newJavaDecimal(price).mul(quantity)
		gain := marketValue.sub(basis)
		totalBasis = totalBasis.add(basis)
		totalValue = totalValue.add(marketValue)
		totalGain = totalGain.add(gain)
		data.Holdings = append(data.Holdings, holdingRow{
			Holding:      holding,
			CurrentPrice: newJavaDecimal(price).String(),
			Basis:        basis.String(),
			MarketValue:  marketValue.String(),
			Gain:         javafmt.PrintGainHTML(gain.value),
		})
	}
	data.TotalBasis = totalBasis.String()
	data.TotalValue = totalValue.String()
	data.TotalGain = javafmt.PrintGainHTML(totalGain.value)
	data.GainPercent = javafmt.PrintGainPercentHTML(javafmt.ComputeGainPercent(totalValue.value, totalBasis.value))
	data.Page = app.NewPage(r, session, portfolioTitle, portfolioBanner)
	app.Render(w, r, http.StatusOK, "trading_portfolio", data)
}
