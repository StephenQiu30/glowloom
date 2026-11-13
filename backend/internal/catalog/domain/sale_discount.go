package domain

import (
	"encoding/json"
	"math/big"
	"regexp"

	"github.com/google/uuid"
)

// DiscountFactorField is the fraction of the regular sale price payable by the buyer.
const DiscountFactorField = "discount_factor"

var discountDecimal = regexp.MustCompile(`^(0\.[0-9]{1,6}|1(\.0{1,6})?)$`)

// ParseSaleDiscount accepts a positive decimal fraction up to one. An absent
// field is a full-price historical version; an explicit null is invalid.
func ParseSaleDiscount(raw json.RawMessage) (*big.Rat, bool) {
	if raw == nil {
		return big.NewRat(1, 1), true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !discountDecimal.MatchString(value) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(value)
	return r, ok && r.Sign() > 0
}

// SalePriceFromProcurement derives a sale version without modifying the
// reviewed supplier tariff. Used by publication and read-only draft previews.
func SalePriceFromProcurement(price PriceRuleVersion, markup, discount string) (PriceRuleVersion, error) {
	var rule map[string]json.RawMessage
	if json.Unmarshal(price.Rule, &rule) != nil || rule == nil || rule[ProcurementVersionField] != nil || rule["markup_multiplier"] != nil || rule[DiscountFactorField] != nil || rule["credit_ratio"] != nil {
		return PriceRuleVersion{}, ErrInvalidPriceRule
	}
	if markup == "" {
		markup = "1"
	}
	link, err := json.Marshal(price.ID)
	if err != nil {
		return PriceRuleVersion{}, err
	}
	rule[ProcurementVersionField] = link
	rule["markup_multiplier"], err = json.Marshal(markup)
	if err != nil {
		return PriceRuleVersion{}, err
	}
	if discount != "" {
		raw, err := json.Marshal(discount)
		if err != nil {
			return PriceRuleVersion{}, err
		}
		if _, ok := ParseSaleDiscount(raw); !ok {
			return PriceRuleVersion{}, ErrInvalidPriceRule
		}
		rule[DiscountFactorField] = raw
	}
	sale := price
	sale.ID = uuid.NewSHA1(price.ID, []byte("official-sale"))
	sale.VersionNo++
	sale.Rule, err = json.Marshal(rule)
	return sale, err
}
