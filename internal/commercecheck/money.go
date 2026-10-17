package commercecheck

import (
	"encoding/json"
	"math/big"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"golang.org/x/text/currency"
)

type Money struct {
	AmountMinor int64
	Currency    string
	Scale       int64 // zero means precision is not known locally
}

var decimal = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
var currencyCode = regexp.MustCompile(`^[A-Za-z]{3}$`)

// Decimal accepts finite JSON numbers or machine-readable decimal text, without
// binary floating-point arithmetic or unbounded scientific-notation expansion.
func Decimal(raw any) (*big.Rat, bool) {
	var text string
	switch value := raw.(type) {
	case string:
		text = value
	case json.Number:
		text = value.String()
	case float64, int, int64:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		text = string(encoded)
	default:
		return nil, false
	}
	if len(text) > 128 || !decimal.MatchString(text) {
		return nil, false
	}
	if at := strings.IndexAny(text, "eE"); at >= 0 {
		exponent, ok := new(big.Int).SetString(text[at+1:], 10)
		if !ok || !exponent.IsInt64() || exponent.Int64() < -18 || exponent.Int64() > 18 {
			return nil, false
		}
	}
	value, ok := new(big.Rat).SetString(text)
	return value, ok
}

func integer(raw any) (int64, bool) {
	number, ok := Decimal(raw)
	if !ok || !number.IsInt() || !number.Num().IsInt64() {
		return 0, false
	}
	// String values are not numeric JSON config fields.
	if _, text := raw.(string); text {
		return 0, false
	}
	return number.Num().Int64(), true
}

func (c *checker) money(raw any, path string) *Money {
	value := c.object(raw, path, "commerce.price")
	if value == nil {
		return nil
	}
	c.consume(path + ".amountMinor")
	c.consume(path + ".currency")
	amount, exists := value["amountMinor"]
	code, hasCode := value["currency"]
	if !exists || !hasCode {
		c.add("commerce.price", diagnostic.Warning, path, "Price snapshot needs both amountMinor and currency.", "Declare both for a fixed published amount, or omit price for a quote/variable offer. Extra pricing/tax/recurrence fields remain allowed.")
	}
	n, validAmount := integer(amount)
	if exists && (!validAmount || n < 0 || n > 9_007_199_254_740_991) {
		c.add("commerce.price", diagnostic.Error, path+".amountMinor", "amountMinor must be a nonnegative exact JSON integer within the safe numeric range.", "Read Stripe's minor-unit amount; do not store a decimal display price, currency symbol or numeric string here. Zero remains allowed.")
		validAmount = false
	}
	text, validCode := code.(string)
	validCode = validCode && currencyCode.MatchString(text)
	if hasCode && !validCode {
		c.add("commerce.price", diagnostic.Error, path+".currency", "Currency must be a three-letter code.", "Use the Stripe Price currency, such as EUR or JPY; uppercase/lowercase are both accepted in config.")
	}
	if !validAmount || !validCode {
		return nil
	}
	text = strings.ToUpper(text)
	scale := int64(0)
	if unit, err := currency.ParseISO(text); err == nil && unit != currency.XXX {
		digits, _ := currency.Standard.Rounding(unit)
		scale = 1
		for range digits {
			scale *= 10
		}
	} else {
		c.add("commerce.price", diagnostic.Warning, path+".currency", "Currency precision is not known in the bundled ISO table.", "Verify Stripe's supported charge currency/minor units. Price agreement is skipped rather than guessing a divisor.")
	}
	// Stripe's documented zero-decimal charge list, checked 2026-10-16.
	// MGA notably differs from the ISO minor-unit table.
	if strings.Contains(" BIF CLP DJF GNF JPY KMF KRW MGA PYG RWF VND VUV XAF XOF XPF ", " "+text+" ") {
		scale = 1
	}
	// Stripe charge units differ from ISO precision for these compatibility cases.
	if text == "ISK" || text == "UGX" {
		scale = 100
		if n%100 != 0 {
			c.add("commerce.price", diagnostic.Error, path+".amountMinor", "ISK/UGX Stripe charge amounts must be divisible by 100.", "Stripe represents whole units with a two-decimal compatibility amount; copy the actual Price amount, not ISO display units.")
		}
	}
	return &Money{AmountMinor: n, Currency: text, Scale: scale}
}

func (m Money) Matches(value any) (bool, bool) {
	price, ok := Decimal(value)
	if !ok || m.Scale == 0 {
		return false, false
	}
	want := new(big.Rat).SetFrac64(m.AmountMinor, m.Scale)
	return want.Cmp(price) == 0, true
}
