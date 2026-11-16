package market

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/javafmt"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

const (
	quoteColumns = "symbol, companyname, volume, price, open1, low, high, change1"
	topCount     = 5
)

// Summary is MarketSummaryDataBean.
type Summary struct {
	TSIA        decimal.Decimal // Trade Stock Index Average
	OpenTSIA    decimal.Decimal
	Volume      float64
	TopGainers  []*store.Quote
	TopLosers   []*store.Quote
	SummaryDate time.Time
	GainPercent decimal.Decimal
}

// buildSummary is the arithmetic of TradeSLSBBean.getMarketSummary. quotes are the index
// quotes ordered by change, highest first. With five quotes or fewer the legacy code leaves
// everything at zero and the top lists empty.
func buildSummary(quotes []*store.Quote, now time.Time) *Summary {
	summary := &Summary{
		TSIA:        decimal.Zero,
		OpenTSIA:    decimal.Zero,
		TopGainers:  []*store.Quote{},
		TopLosers:   []*store.Quote{},
		SummaryDate: now,
	}
	if len(quotes) > topCount {
		summary.TopGainers = append(summary.TopGainers, quotes[:topCount]...)
		for i := len(quotes) - 1; i >= len(quotes)-topCount; i-- {
			summary.TopLosers = append(summary.TopLosers, quotes[i])
		}
		for _, quote := range quotes {
			summary.TSIA = summary.TSIA.Add(quote.Price)
			summary.OpenTSIA = summary.OpenTSIA.Add(quote.Open)
			summary.Volume += quote.Volume
		}
		count := decimal.NewFromInt(int64(len(quotes)))
		summary.TSIA = summary.TSIA.DivRound(count, 2)
		summary.OpenTSIA = summary.OpenTSIA.DivRound(count, 2)
	}
	summary.GainPercent = javafmt.ComputeGainPercent(summary.TSIA, summary.OpenTSIA)
	return summary
}

// loadSummary is getMarketSummary: the index is the 100 quotes s:100..s:199
// (named query quoteejb.quotesByChange). The symbol tie-break makes the order stable.
func loadSummary(ctx context.Context, q store.Querier) (*Summary, error) {
	rows, err := q.Query(ctx, "SELECT "+quoteColumns+" FROM quoteejb WHERE symbol LIKE 's:1__' ORDER BY change1 DESC, symbol")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	quotes := []*store.Quote{}
	for rows.Next() {
		quote, err := store.ScanQuote(rows)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildSummary(quotes, time.Now()), nil
}

// summaryCache is the TradeAction market summary cache: interval 0 reads every time, a
// positive interval re-reads at most once per interval, a negative one keeps the first read.
type summaryCache struct {
	mutex  sync.Mutex
	cached *Summary
	next   time.Time
}

var cache = &summaryCache{}

func (c *summaryCache) get(interval time.Duration, now time.Time, load func() (*Summary, error)) (*Summary, error) {
	if interval == 0 {
		return load()
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.cached != nil && (interval < 0 || !now.After(c.next)) {
		return c.cached, nil
	}
	summary, err := load()
	if err != nil {
		return nil, err
	}
	c.cached, c.next = summary, now.Add(interval)
	return summary, nil
}

// MarketSummary is TradeAction.getMarketSummary (cached for MARKET_SUMMARY_INTERVAL seconds).
func MarketSummary(ctx context.Context, q store.Querier, intervalSeconds int) (*Summary, error) {
	return cache.get(time.Duration(intervalSeconds)*time.Second, time.Now(), func() (*Summary, error) {
		return loadSummary(ctx, q)
	})
}
