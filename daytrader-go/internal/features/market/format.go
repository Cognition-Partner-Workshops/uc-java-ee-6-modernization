package market

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// javaDouble formats like Double.toString(), including the computerized scientific notation
// Java switches to below 1e-3 and from 1e7 up (a traded volume of 12,500,000 prints "1.25E7").
func javaDouble(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	}
	magnitude := math.Abs(value)
	if magnitude == 0 || (magnitude >= 1e-3 && magnitude < 1e7) {
		text := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.Contains(text, ".") {
			text += ".0"
		}
		return text
	}
	text := strconv.FormatFloat(value, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(text, "e")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	exponent = strings.TrimPrefix(exponent, "+")
	sign := ""
	if strings.HasPrefix(exponent, "-") {
		sign, exponent = "-", exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	return mantissa + "E" + sign + exponent
}

// exactDecimal is new BigDecimal(double): the exact binary value of the double, not its
// shortest decimal representation, so HALF_UP rounding matches Java (1.005 rounds to 1.00).
func exactDecimal(value float64) decimal.Decimal {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero
	}
	rational, _ := new(big.Float).SetFloat64(value).Rat(nil)
	if rational == nil {
		return decimal.Zero
	}
	// A float64 is n / 2^k, so it terminates in base 10; 1100 digits cover the whole range.
	return decimal.NewFromBigRat(rational, 1100)
}
