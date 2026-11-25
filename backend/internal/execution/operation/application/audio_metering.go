package application

import (
	"bytes"
	"encoding/json"
	"math/big"
)

// ParseAudioDurationSeconds preserves the provider's original decimal seconds.
// The bounded JSON number is never converted through binary floating point or
// rounded to milliseconds before pricing. Strings and ambiguous values fail closed.
func ParseAudioDurationSeconds(raw json.RawMessage, maximum int64) (json.Number, error) {
	text := string(bytes.TrimSpace(raw))
	if len(text) == 0 || len(text) > 64 || text[0] < '0' || text[0] > '9' || !json.Valid([]byte(text)) {
		return "", ErrInvalidPricingInput
	}
	number, err := exactFingerprintNumber(text)
	if err != nil || number.Sign() <= 0 || number.Cmp(big.NewRat(maximum, 1)) > 0 {
		return "", ErrInvalidPricingInput
	}
	return json.Number(text), nil
}

// AudioDurationMillisCeiling is for playback validation and historical receipts,
// not the original_duration_v1 monetary quantity.
func AudioDurationMillisCeiling(seconds json.Number) (int64, error) {
	value, err := ParseAudioDurationSeconds(json.RawMessage(seconds.String()), 240)
	if err != nil {
		return 0, err
	}
	amount, _ := new(big.Rat).SetString(value.String())
	return finishPriceAmount(amount, big.NewRat(1000, 1))
}
