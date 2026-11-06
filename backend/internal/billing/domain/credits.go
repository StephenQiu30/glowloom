package domain

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	// ErrWalletInsufficient means eligible lots cannot cover the reservation.
	ErrWalletInsufficient = errors.New("insufficient eligible credits")
	// ErrWalletPriceUnavailable means the model lacks a reviewed supplier tariff.
	ErrWalletPriceUnavailable = errors.New("model has no approved commercial tariff")
	// ErrWalletPayerChanged means the project binding needs finance review.
	ErrWalletPayerChanged = errors.New("project payer changed")
)

// MicrosPerCredit is the CNY face value of one credit.
const MicrosPerCredit int64 = 100_000

// WholeCreditMicros rounds a nonnegative sale to an integer credit amount,
// represented in the same CNY micros used by the wallet and budget ledgers.
func WholeCreditMicros(sale int64) (int64, error) {
	if sale < 0 {
		return 0, errors.New("negative credit charge")
	}
	remainder := sale % MicrosPerCredit
	if remainder == 0 {
		return sale, nil
	}
	increase := MicrosPerCredit - remainder
	if sale > math.MaxInt64-increase {
		return 0, errors.New("credit charge overflows micros")
	}
	return sale + increase, nil
}

// Credits returns exact decimal credits from a nonnegative CNY micro amount.
func Credits(micros int64) string {
	whole, fraction := micros/MicrosPerCredit, micros%MicrosPerCredit
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%05d", whole, fraction), "0")
}
