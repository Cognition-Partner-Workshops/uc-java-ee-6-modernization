package market

import (
	"bytes"
	"errors"
	"html/template"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	daytradergo "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func dec(text string) decimal.Decimal { return decimal.RequireFromString(text) }

func TestJavaDouble(t *testing.T) {
	cases := map[float64]string{
		0: "0.0", 100: "100.0", 12.5: "12.5", -3.25: "-3.25", 9999999: "9999999.0",
		1e7: "1.0E7", 12500000: "1.25E7", 0.001: "0.001", 0.0001: "1.0E-4", 1.5e-10: "1.5E-10",
	}
	for value, want := range cases {
		if got := javaDouble(value); got != want {
			t.Errorf("javaDouble(%v) = %q, want %q", value, got, want)
		}
	}
}

func TestExactDecimalRoundsLikeJava(t *testing.T) {
	// new BigDecimal(1.005) is 1.00499999999999989..., so HALF_UP at scale 2 gives 1.00.
	cases := map[float64]string{1.005: "1.00", 0.125: "0.13", -2.5: "-2.50", 0: "0.00", 3: "3.00"}
	for value, want := range cases {
		if got := exactDecimal(value).StringFixed(2); got != want {
			t.Errorf("exactDecimal(%v) at scale 2 = %q, want %q", value, got, want)
		}
	}
}

func TestSplitSymbols(t *testing.T) {
	cases := map[string][]string{
		"s:7,s:80,s:89":           {"s:7", "s:80", "s:89"},
		"s:0, s:1, s:2, s:3, s:4": {"s:0", "s:1", "s:2", "s:3", "s:4"},
		" ,, s:1 ,":               {"s:1"},
		"":                        {},
	}
	for input, want := range cases {
		if got := splitSymbols(input); !reflect.DeepEqual(got, want) {
			t.Errorf("splitSymbols(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestGetQuoteInvalidSymbol(t *testing.T) {
	for _, symbol := range []string{"", "no_stock_symbol_provided"} {
		quote, err := GetQuote(t.Context(), nil, symbol)
		if err != nil || quote.Symbol != "Invalid symbol" {
			t.Errorf("GetQuote(%q) = %+v, %v", symbol, quote, err)
		}
	}
}

func indexQuotes(count int) []*store.Quote {
	quotes := make([]*store.Quote, count)
	for i := range quotes {
		quotes[i] = &store.Quote{
			Symbol: "s:" + string(rune('a'+i)), Price: dec("10.00").Add(decimal.NewFromInt(int64(i))),
			Open: dec("10.00"), Volume: 100,
		}
	}
	return quotes
}

func TestBuildSummary(t *testing.T) {
	quotes := indexQuotes(7) // prices 10..16, open 10
	summary := buildSummary(quotes, time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC))
	if got := summary.TSIA.StringFixed(2); got != "13.00" {
		t.Errorf("TSIA = %s", got)
	}
	if got := summary.OpenTSIA.StringFixed(2); got != "10.00" {
		t.Errorf("openTSIA = %s", got)
	}
	if summary.Volume != 700 {
		t.Errorf("volume = %v", summary.Volume)
	}
	// 13.00 / 10.00 at scale 2 = 1.30, minus 1, times 100.
	if got := summary.GainPercent.StringFixed(2); got != "30.00" {
		t.Errorf("gainPercent = %s", got)
	}
	if len(summary.TopGainers) != 5 || summary.TopGainers[0] != quotes[0] || summary.TopGainers[4] != quotes[4] {
		t.Errorf("top gainers = %v", summary.TopGainers)
	}
	if len(summary.TopLosers) != 5 || summary.TopLosers[0] != quotes[6] || summary.TopLosers[4] != quotes[2] {
		t.Errorf("top losers = %v", summary.TopLosers)
	}
	view := newSummaryView(summary)
	if view.SummaryDate != "2026-11-04" || view.TSIA != "13.00" || view.Volume != "700.0" {
		t.Errorf("view = %+v", view)
	}
}

func TestBuildSummaryRoundsHalfUp(t *testing.T) {
	quotes := indexQuotes(6)
	for _, quote := range quotes {
		quote.Price = dec("1.00")
	}
	quotes[0].Price = dec("1.03") // 6.03 / 6 = 1.005 -> 1.01
	if got := buildSummary(quotes, time.Now()).TSIA.StringFixed(2); got != "1.01" {
		t.Errorf("TSIA = %s", got)
	}
}

func TestBuildSummaryFiveQuotesOrFewer(t *testing.T) {
	summary := buildSummary(indexQuotes(5), time.Now())
	if !summary.TSIA.IsZero() || !summary.GainPercent.IsZero() || summary.Volume != 0 ||
		len(summary.TopGainers) != 0 || len(summary.TopLosers) != 0 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestSummaryCache(t *testing.T) {
	loads := 0
	load := func() (*Summary, error) { loads++; return &Summary{}, nil }
	start := time.Now()
	c := &summaryCache{}
	for _, offset := range []time.Duration{0, 5 * time.Second, 20 * time.Second, 21 * time.Second} {
		if _, err := c.get(20*time.Second, start.Add(offset), load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 2 {
		t.Errorf("interval 20s: %d loads, want 2", loads)
	}
	loads, c = 0, &summaryCache{}
	_, _ = c.get(0, start, load)
	_, _ = c.get(0, start, load)
	if loads != 2 {
		t.Errorf("interval 0: %d loads, want 2", loads)
	}
	loads, c = 0, &summaryCache{}
	_, _ = c.get(-time.Second, start, load)
	_, _ = c.get(-time.Second, start.Add(time.Hour), load)
	if loads != 1 {
		t.Errorf("negative interval: %d loads, want 1", loads)
	}
	failing := errors.New("down")
	if _, err := (&summaryCache{}).get(time.Second, start, func() (*Summary, error) { return nil, failing }); !errors.Is(err, failing) {
		t.Errorf("error not returned: %v", err)
	}
}

func TestSummarizeAccount(t *testing.T) {
	account := &store.Account{Balance: dec("900.00"), OpenBalance: dec("1000.00")}
	holdings := []*store.Holding{
		{PurchasePrice: dec("10.50"), Quantity: 10},
		{PurchasePrice: dec("2.25"), Quantity: 4},
	}
	totals := summarizeAccount(account, holdings)
	if totals.HoldingCount != 2 || totals.HoldingsTotal.StringFixed(2) != "114.00" ||
		totals.SumOfCashHoldings.StringFixed(2) != "1014.00" || totals.Gain.StringFixed(2) != "14.00" ||
		totals.GainPercent.StringFixed(2) != "1.00" {
		t.Errorf("totals = %+v", totals)
	}
}

func TestNewQuoteRow(t *testing.T) {
	row := newQuoteRow(&store.Quote{
		Symbol: "s:7", CompanyName: "S7 Incorporated", Volume: 250, Price: dec("110.00"), Open: dec("100.00"),
		Low: dec("90.00"), High: dec("120.00"), Change: -1.005,
	})
	if row.Volume != "250.0" || row.Low != "90.00" || row.High != "120.00" || row.Open != "100.00" || row.Price != "110.00" {
		t.Errorf("row = %+v", row)
	}
	if !strings.Contains(string(row.Gain), `<FONT color="#ff0000">-1.00</FONT>`) {
		t.Errorf("gain = %s", row.Gain)
	}
	if !strings.Contains(string(row.GainPercent), "+10.00%") {
		t.Errorf("gain percent = %s", row.GainPercent)
	}
}

func render(t *testing.T, name string, data any) string {
	t.Helper()
	templates, err := template.New("daytrader").Funcs(web.TemplateFuncs).ParseFS(daytradergo.Templates, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := templates.ExecuteTemplate(&buffer, name, data); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}

func TestQuotesTemplate(t *testing.T) {
	page := render(t, "market_quotes", quotesData{
		Page: web.Page{Title: "DayTrader: Quotes and Trading", Banner: "DayTrader Quotes"},
		Quotes: []quoteRow{newQuoteRow(&store.Quote{
			Symbol: "s:7", CompanyName: "S7 Incorporated", Price: dec("11.00"), Open: dec("10.00"), Low: dec("9.00"), High: dec("12.00"),
		})},
	})
	for _, want := range []string{
		"<TITLE>DayTrader: Quotes and Trading</TITLE>", `<A href="app?action=quotes&symbols=s:7">s:7</A>`,
		"S7 Incorporated", "9.00 - 12.00", "$ 11.00", `name="action" value="buy"`,
		`type="hidden" name="symbol" value="s:7"`, `name="quantity" value="100"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("quotes page misses %q", want)
		}
	}
}

func TestHomeTemplate(t *testing.T) {
	created := time.Date(2026, 11, 4, 7, 42, 47, 0, time.UTC)
	account := &store.Account{AccountID: 42, LoginCount: 3, CreationDate: created, Balance: dec("900.00"), OpenBalance: dec("1000.00"), ProfileID: "uid:0"}
	page := render(t, "market_home", homeData{
		Page:    web.Page{Title: "Welcome to DayTrader", Banner: "DayTrader Home"},
		Results: defaultResults,
		Account: account,
		Session: &web.Session{CreationDate: created},
		Totals:  summarizeAccount(account, nil),
		Market:  newSummaryView(buildSummary(indexQuotes(7), created)),
	})
	for _, want := range []string{
		"Welcome &nbsp;uid:0,", "Wed Nov 04 07:42:47 UTC 2026", "$ 900.00<BR>", "$ 1000.00<BR>", "-100.00", "-10.00%",
		"Market Summary<BR>", "2026-11-04", "13.00 (<B>", "+30.00%", "700.0", "Top Gainers", "Top Losers",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("home page misses %q", want)
		}
	}
	// tradehome.jsp never prints the results bean.
	if strings.Contains(page, defaultResults) {
		t.Errorf("home page prints the status line, legacy does not")
	}
}
