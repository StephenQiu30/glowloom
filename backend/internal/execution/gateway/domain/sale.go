package domain

import (
	"reflect"
	"slices"
)

// SameSaleMeter checks that a rate change preserves the supported usage contract.
func (p Price) SameSaleMeter(next Price) bool {
	if p.Currency != next.Currency || (p.TextTariff == nil) != (next.TextTariff == nil) || (p.MediaTariff == nil) != (next.MediaTariff == nil) {
		return false
	}
	if p.TextTariff != nil {
		a, b := *p.TextTariff, *next.TextTariff
		if !slices.EqualFunc(a.Tiers, b.Tiers, func(a, b TextTier) bool { return a.FromInput == b.FromInput }) ||
			(a.OffPeak == nil) != (b.OffPeak == nil) {
			return false
		}
		a.Tiers, b.Tiers, a.OffPeak, b.OffPeak = nil, nil, nil, nil
		return reflect.DeepEqual(a, b)
	}
	if p.MediaTariff != nil {
		a, b := *p.MediaTariff, *next.MediaTariff
		a.UnitPrice, b.UnitPrice = "", ""
		a.OutputTokenPrice, b.OutputTokenPrice = "", ""
		a.HighPrice, b.HighPrice = "", ""
		a.ExtraReferencePrice, b.ExtraReferencePrice = "", ""
		return reflect.DeepEqual(a, b)
	}
	return true
}
