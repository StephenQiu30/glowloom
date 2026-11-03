package domain

import "time"

// DeepSeekCN2026 identifies the reviewed Beijing-time tariff and holiday
// calendar. Keep historical identifiers unchanged when adding a later year.
const DeepSeekCN2026 = "deepseek_cn_2026"

// DeepSeekOffPeak2026 applies the supplier's weekday hours and the State
// Council's 2026 holiday notice. Adjusted working weekends remain off-peak.
// An unreviewed year is unknown, never silently treated as an ordinary weekday.
func DeepSeekOffPeak2026(at time.Time) (bool, error) {
	beijing := at.UTC().Add(8 * time.Hour)
	if at.IsZero() || beijing.Year() != 2026 {
		return false, ErrInvalidPriceRule
	}
	date := int(beijing.Month())*100 + beijing.Day()
	holiday := date >= 101 && date <= 103 || date >= 215 && date <= 223 ||
		date >= 404 && date <= 406 || date >= 501 && date <= 505 ||
		date >= 619 && date <= 621 || date >= 925 && date <= 927 ||
		date >= 1001 && date <= 1007
	if holiday || beijing.Weekday() == time.Saturday || beijing.Weekday() == time.Sunday {
		return true, nil
	}
	hour := beijing.Hour()
	return hour < 9 || hour >= 12 && hour < 14 || hour >= 18, nil
}
