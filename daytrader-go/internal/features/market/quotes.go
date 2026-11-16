package market

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/javafmt"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

const noSymbolProvided = "no_stock_symbol_provided"

// quoteRow is one displayQuote.jsp row, formatted the way the JSP prints it.
type quoteRow struct {
	Symbol      string
	SymbolLink  template.HTML
	CompanyName string
	Volume      string
	Low         string
	High        string
	Open        string
	Price       string
	Gain        template.HTML
	GainPercent template.HTML
}

type quotesData struct {
	Page    web.Page
	Results string
	Quotes  []quoteRow
}

// splitSymbols is new StringTokenizer(symbols, " ,"): blanks and commas separate, empty tokens vanish.
func splitSymbols(symbols string) []string {
	return strings.FieldsFunc(symbols, func(r rune) bool { return r == ' ' || r == ',' })
}

// GetQuote is TradeAction.getQuote: an empty symbol or one longer than ten characters answers
// the "Invalid symbol" placeholder without touching the database; an unknown symbol is
// store.ErrNotFound.
func GetQuote(ctx context.Context, q store.Querier, symbol string) (*store.Quote, error) {
	if symbol == "" || len(symbol) > 10 {
		return &store.Quote{Symbol: "Invalid symbol"}, nil
	}
	return store.QuoteBySymbol(ctx, q, symbol)
}

func newQuoteRow(quote *store.Quote) quoteRow {
	return quoteRow{
		Symbol:      quote.Symbol,
		SymbolLink:  javafmt.PrintQuoteLink(quote.Symbol),
		CompanyName: quote.CompanyName,
		Volume:      javaDouble(quote.Volume),
		Low:         javafmt.Money(quote.Low),
		High:        javafmt.Money(quote.High),
		Open:        javafmt.Money(quote.Open),
		Price:       javafmt.Money(quote.Price),
		Gain:        javafmt.PrintGainHTML(exactDecimal(quote.Change)),
		GainPercent: javafmt.PrintGainPercentHTML(javafmt.ComputeGainPercent(quote.Price, quote.Open)),
	}
}

// loadQuoteRows reads one quote per requested symbol, in request order. A symbol that does
// not exist is skipped, as displayQuote.jsp swallows the exception and prints no row.
func loadQuoteRows(ctx context.Context, q store.Querier, symbols []string) ([]quoteRow, error) {
	rows := []quoteRow{}
	for _, symbol := range symbols {
		quote, err := GetQuote(ctx, q, symbol)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, newQuoteRow(quote))
	}
	return rows, nil
}

// quotes is TradeServletAction.doQuotes + quote.jsp + displayQuote.jsp.
func quotes(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	symbols := noSymbolProvided
	if err := r.ParseForm(); err == nil && r.Form.Has("symbols") {
		symbols = r.FormValue("symbols")
	}
	rows, err := loadQuoteRows(r.Context(), app.Store.Pool, splitSymbols(symbols))
	if err != nil {
		app.Error(w, r, err)
		return
	}
	results, _ := web.Attr(r, "results")
	app.Render(w, r, http.StatusOK, "market_quotes", quotesData{
		Page:    app.NewPage(r, session, "DayTrader: Quotes and Trading", "DayTrader Quotes"),
		Results: results,
		Quotes:  rows,
	})
}
