package domain

import (
	"errors"
	"fmt"
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

// Credits returns exact decimal credits from a nonnegative CNY micro amount.
func Credits(micros int64) string {
	whole, fraction := micros/100_000, micros%100_000
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%05d", whole, fraction), "0")
}
