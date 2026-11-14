package domain

import (
	"encoding/json"

	"github.com/google/uuid"
)

// ProcurementVersionField names the frozen supplier tariff in sales-rule JSON.
const ProcurementVersionField = "procurement_price_version_id"

// ParseProcurementVersion accepts an optional immutable tariff reference.
func ParseProcurementVersion(raw json.RawMessage) (uuid.UUID, error) {
	if len(raw) == 0 {
		return uuid.Nil, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return uuid.Nil, ErrInvalidPriceRule
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return uuid.Nil, ErrInvalidPriceRule
	}
	return id, nil
}

// ProcurementVersion returns the immutable supplier tariff behind a sales rule.
func (p PriceRuleVersion) ProcurementVersion() (uuid.UUID, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(p.Rule, &fields); err != nil {
		return uuid.Nil, ErrInvalidPriceRule
	}
	id, err := ParseProcurementVersion(fields[ProcurementVersionField])
	if err != nil || id == p.ID {
		return uuid.Nil, ErrInvalidPriceRule
	}
	return id, nil
}
