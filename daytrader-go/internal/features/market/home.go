package market

import (
	"html/template"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/javafmt"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

const defaultResults = "Ready to Trade"

// accountSummary is the arithmetic tradehome.jsp does in its scriptlet.
type accountSummary struct {
	HoldingCount      int
	HoldingsTotal     decimal.Decimal
	SumOfCashHoldings decimal.Decimal
	Gain              decimal.Decimal
	GainPercent       decimal.Decimal
}

func summarizeAccount(account *store.Account, holdings []*store.Holding) accountSummary {
	prices := make([]decimal.Decimal, len(holdings))
	quantities := make([]float64, len(holdings))
	for i, holding := range holdings {
		prices[i], quantities[i] = holding.PurchasePrice, holding.Quantity
	}
	holdingsTotal := javafmt.HoldingsTotal(prices, quantities)
	sum := account.Balance.Add(holdingsTotal)
	return accountSummary{
		HoldingCount:      len(holdings),
		HoldingsTotal:     holdingsTotal,
		SumOfCashHoldings: sum,
		Gain:              javafmt.ComputeGain(sum, account.OpenBalance),
		GainPercent:       javafmt.ComputeGainPercent(sum, account.OpenBalance),
	}
}

// summaryQuote is one top gainer / top loser row of marketSummary.jsp.
type summaryQuote struct {
	SymbolLink template.HTML
	Price      string
	Change     template.HTML
}

// summaryView is marketSummary.jsp's data, formatted.
type summaryView struct {
	SummaryDate string
	TSIA        string
	GainPercent template.HTML
	Volume      string
	TopGainers  []summaryQuote
	TopLosers   []summaryQuote
}

func summaryQuotes(quotes []*store.Quote) []summaryQuote {
	rows := []summaryQuote{}
	for i, quote := range quotes {
		if i == topCount {
			break
		}
		rows = append(rows, summaryQuote{
			SymbolLink: javafmt.PrintQuoteLink(quote.Symbol),
			Price:      javafmt.Money(quote.Price),
			Change:     javafmt.PrintGainHTML(quote.Price.Sub(quote.Open)),
		})
	}
	return rows
}

func newSummaryView(summary *Summary) summaryView {
	return summaryView{
		// java.sql.Date.toString(): yyyy-mm-dd.
		SummaryDate: summary.SummaryDate.UTC().Format("2006-01-02"),
		TSIA:        javafmt.Money(summary.TSIA),
		GainPercent: javafmt.PrintGainPercentHTML(summary.GainPercent),
		Volume:      javaDouble(summary.Volume),
		TopGainers:  summaryQuotes(summary.TopGainers),
		TopLosers:   summaryQuotes(summary.TopLosers),
	}
}

type homeData struct {
	Page    web.Page
	Results string
	Account *store.Account
	Session *web.Session
	Totals  accountSummary
	Market  summaryView
}

// home is TradeServletAction.doHome + tradehome.jsp + marketSummary.jsp.
func home(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	ctx := r.Context()
	results, ok := web.Attr(r, "results")
	if !ok {
		results = defaultResults
	}
	account, err := store.AccountByUser(ctx, app.Store.Pool, session.UserID)
	if err != nil {
		app.Error(w, r, err)
		return
	}
	holdings, err := store.HoldingsByUser(ctx, app.Store.Pool, session.UserID)
	if err != nil {
		app.Error(w, r, err)
		return
	}
	summary, err := MarketSummary(ctx, app.Store.Pool, app.Config.MarketSummaryInterval)
	if err != nil {
		app.Error(w, r, err)
		return
	}
	app.Render(w, r, http.StatusOK, "market_home", homeData{
		Page:    app.NewPage(r, session, "Welcome to DayTrader", "DayTrader Home"),
		Results: results,
		Account: account,
		Session: session,
		Totals:  summarizeAccount(account, holdings),
		Market:  newSummaryView(summary),
	})
}
