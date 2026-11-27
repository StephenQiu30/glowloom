package domain

import (
	"math/big"
	"regexp"
	"strings"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
)

const arkWeekdaySchedule = "ark_weekday_cn"

var decimalRate = regexp.MustCompile(`^[0-9]{1,8}(\.[0-9]{1,24})?$`)
var vercelProviderSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// TextRates contains exact original-currency rates per million text tokens.
type TextRates struct {
	Input      string `json:"input"`
	CacheRead  string `json:"cache_read"`
	CacheWrite string `json:"cache_write"`
	Output     string `json:"output"`
	Reasoning  string `json:"reasoning,omitempty"`
}

// TextTier applies to all the tokens of requests meeting the inclusive input threshold.
type TextTier struct {
	FromInput int64 `json:"from_input"`
	TextRates
}

// TextTariff is frozen with the sale or purchase version at onboarding.
type TextTariff struct {
	Tiers          []TextTier `json:"tiers"`
	MaxInput       int64      `json:"max_input,omitempty"`
	Schedule       string     `json:"schedule,omitempty"`
	OffPeak        *TextRates `json:"off_peak,omitempty"`
	VercelProvider string     `json:"vercel_provider,omitempty"`
	SourceURL      string     `json:"source_url,omitempty"`
	Observed       string     `json:"observed_on,omitempty"`
}

func (r TextRates) parse() ([5]*big.Rat, bool) {
	var rates [5]*big.Rat
	var positive bool
	reasoning := r.Reasoning
	if reasoning == "" {
		reasoning = r.Output
	}
	for i, value := range [...]string{r.Input, r.CacheRead, r.CacheWrite, r.Output, reasoning} {
		if !decimalRate.MatchString(value) {
			return rates, false
		}
		rates[i], _ = new(big.Rat).SetString(value)
		positive = positive || rates[i].Sign() > 0
	}
	return rates, positive
}

func (t TextTariff) valid() bool {
	if len(t.Tiers) == 0 || len(t.Tiers) > 16 || t.MaxInput < 0 || t.MaxInput > 1<<20 {
		return false
	}
	last := int64(-1)
	for _, tier := range t.Tiers {
		if _, ok := tier.parse(); !ok || tier.FromInput <= last || tier.FromInput > 1<<20 ||
			t.MaxInput > 0 && tier.FromInput > t.MaxInput {
			return false
		}
		last = tier.FromInput
	}
	if t.Tiers[0].FromInput != 0 || t.VercelProvider != "" && !vercelProviderSlug.MatchString(t.VercelProvider) {
		return false
	}
	if t.Schedule == "" {
		return t.OffPeak == nil
	}
	if len(t.Tiers) != 1 || t.OffPeak == nil || t.Schedule != arkWeekdaySchedule && t.Schedule != catalog.DeepSeekCN2026 {
		return false
	}
	_, ok := t.OffPeak.parse()
	return ok
}

func (t TextTariff) rates(u Usage) (TextRates, error) {
	if t.MaxInput > 0 && u.Input > t.MaxInput {
		return TextRates{}, ErrUnknown
	}
	if t.Schedule != "" {
		offPeak, err := t.offPeak(u)
		if err != nil {
			return TextRates{}, err
		}
		if offPeak {
			return *t.OffPeak, nil
		}
	}
	rates := t.Tiers[0].TextRates
	for _, tier := range t.Tiers[1:] {
		if u.Input < tier.FromInput {
			break
		}
		rates = tier.TextRates
	}
	return rates, nil
}

func (t TextTariff) offPeak(u Usage) (bool, error) {
	start, end := u.BillingStartedAt, u.BillingReceivedAt
	// The supplier bills at a point during this interval. Less than one hour
	// permits at most one schedule transition; different bands are ambiguous.
	if start.IsZero() || end.Before(start) || end.Sub(start) >= time.Hour {
		return false, ErrUnknown
	}
	at := func(stamp time.Time) (bool, error) {
		if t.Schedule == catalog.DeepSeekCN2026 {
			return catalog.DeepSeekOffPeak2026(stamp)
		}
		beijing := stamp.UTC().Add(8 * time.Hour)
		hour := beijing.Hour()
		return beijing.Weekday() == time.Saturday || beijing.Weekday() == time.Sunday ||
			hour < 9 || hour >= 12 && hour < 14 || hour >= 18, nil
	}
	from, err := at(start)
	to, endErr := at(end)
	if err != nil || endErr != nil || from != to {
		return false, ErrUnknown
	}
	return to, nil
}

func (r TextRates) amountMicros(u Usage, bound bool) *big.Rat {
	rates, _ := r.parse()
	units := [...]int64{u.Input - u.CacheRead - u.CacheWrite, u.CacheRead, u.CacheWrite, u.Output - u.Reasoning, u.Reasoning}
	if bound {
		if rates[4].Cmp(rates[3]) > 0 {
			rates[3] = rates[4]
		}
		for _, rate := range rates[1:3] {
			if rate.Cmp(rates[0]) > 0 {
				rates[0] = rate
			}
		}
	}
	var amount big.Rat
	for i, rate := range rates {
		amount.Add(&amount, new(big.Rat).Mul(rate, new(big.Rat).SetInt64(units[i])))
	}
	return &amount
}

// UpperCost bounds the full range of possible input lengths and cache usage.
func (p Price) UpperCost(input, output int64) (int64, error) {
	usage := Usage{Input: input, Output: output}
	if p.Validate() != nil || usage.Validate() != nil {
		return 0, ErrInvalid
	}
	if p.MediaTariff != nil {
		m := p.MediaTariff
		if m.UsesAuthorizedCeiling() {
			return 0, ErrInvalid
		}
		return p.convert(m.upperMicros())
	}
	if p.TextTariff == nil {
		p.Input = max(p.Input, p.CacheRead, p.CacheWrite)
		return p.Cost(usage)
	}
	t := p.TextTariff
	if t.MaxInput > 0 && input > t.MaxInput {
		return 0, ErrInvalid
	}
	var high big.Rat
	for _, tier := range t.Tiers {
		if tier.FromInput > input {
			break
		}
		if amount := tier.amountMicros(usage, true); amount.Cmp(&high) > 0 {
			high.Set(amount)
		}
	}
	if t.OffPeak != nil {
		if amount := t.OffPeak.amountMicros(usage, true); amount.Cmp(&high) > 0 {
			high.Set(amount)
		}
	}
	return p.convert(&high)
}

// AcceptsTextTariffRoute ensures that provider pinning applies only to the
// official Vercel Chat Completions endpoint.
func (r Route) AcceptsTextTariffRoute() bool {
	t := r.Purchase.TextTariff
	return t == nil || t.VercelProvider == "" ||
		r.Protocol == "chat" && strings.TrimRight(r.BaseURL, "/") == "https://ai-gateway.vercel.sh/v1"
}

// AvailableAt reports whether a finite supplier calendar covers the request.
func (p Price) AvailableAt(at time.Time) bool {
	if p.TextTariff == nil || p.TextTariff.Schedule != catalog.DeepSeekCN2026 {
		return true
	}
	_, err := catalog.DeepSeekOffPeak2026(at)
	return err == nil
}
