// Package javafmt reproduces the Java formatting the legacy JSPs print, so pages match the
// recorded transcripts: java.util.Date.toString(), Double.toString() and FinancialUtils.
package javafmt

import (
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var hundred = decimal.NewFromInt(100)

// Date formats like java.util.Date.toString(), e.g. "Wed Nov 04 07:42:47 UTC 2026".
func Date(t time.Time) string {
	return t.UTC().Format("Mon Jan 02 15:04:05 MST 2006")
}

// Double formats like Double.toString() for the magnitudes DayTrader shows: 10 -> "10.0".
func Double(value float64) string {
	text := strconv.FormatFloat(value, 'f', -1, 64)
	if !strings.Contains(text, ".") {
		text += ".0"
	}
	return text
}

// Money prints a DECIMAL(14,2) value the way BigDecimal.toString() does for scale 2.
func Money(value decimal.Decimal) string {
	return value.StringFixed(2)
}

// ComputeGain is FinancialUtils.computeGain.
func ComputeGain(currentBalance, openBalance decimal.Decimal) decimal.Decimal {
	return currentBalance.Sub(openBalance)
}

// ComputeGainPercent is FinancialUtils.computeGainPercent: divide at the dividend's scale (2),
// ROUND_HALF_UP, minus 1, times 100.
func ComputeGainPercent(currentBalance, openBalance decimal.Decimal) decimal.Decimal {
	if openBalance.IsZero() {
		return decimal.Zero
	}
	return currentBalance.DivRound(openBalance, 2).Sub(decimal.NewFromInt(1)).Mul(hundred)
}

// HoldingsTotal is FinancialUtils.computeHoldingsTotal: sum of purchase price x quantity.
func HoldingsTotal(purchasePrices []decimal.Decimal, quantities []float64) decimal.Decimal {
	total := decimal.Zero
	for i, price := range purchasePrices {
		total = total.Add(price.Mul(decimal.NewFromFloat(quantities[i])))
	}
	return total.Round(2)
}

func arrow(negative bool) (string, string) {
	if negative {
		return "#ff0000", "arrowdown.gif"
	}
	return "#009900", "arrowup.gif"
}

func image(name string) string {
	return `<IMG src="images/` + name + `" width="10" height="10" border="0"></IMG>`
}

// PrintGainHTML is FinancialUtils.printGainHTML.
func PrintGainHTML(gain decimal.Decimal) template.HTML {
	color, img := arrow(gain.IsNegative())
	return template.HTML(`<FONT color="` + color + `">` + gain.StringFixed(2) + `</FONT>` + image(img))
}

// PrintChangeHTML is FinancialUtils.printChangeHTML (prints the double as Double.toString()).
func PrintChangeHTML(change float64) template.HTML {
	color, img := arrow(change < 0)
	return template.HTML(`<FONT color="` + color + `">` + Double(change) + `</FONT>` + image(img))
}

// PrintGainPercentHTML is FinancialUtils.printGainPercentHTML.
func PrintGainPercentHTML(gain decimal.Decimal) template.HTML {
	color, img := arrow(gain.IsNegative())
	sign := "+"
	if gain.IsNegative() {
		sign = ""
	}
	return template.HTML(`(<B><FONT color="` + color + `">` + sign + gain.StringFixed(2) + `%</FONT></B>)` + image(img))
}

// PrintQuoteLink is FinancialUtils.printQuoteLink.
func PrintQuoteLink(symbol string) template.HTML {
	escaped := template.HTMLEscapeString(symbol)
	return template.HTML(`<A href="app?action=quotes&symbols=` + escaped + `">` + escaped + `</A>`)
}
