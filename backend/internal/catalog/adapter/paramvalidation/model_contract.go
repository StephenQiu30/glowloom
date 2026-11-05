package paramvalidation

import (
	"encoding/json"
	"math/big"
	"slices"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func validateModelContract(version domain.ModelVersion) error {
	contract, known := domain.ModelParameterContract(version.ProviderModelID)
	if !known {
		return nil
	}
	var fields []struct {
		Field string       `json:"field"`
		Type  string       `json:"type"`
		Enum  []string     `json:"enum"`
		Min   *json.Number `json:"min"`
		Max   *json.Number `json:"max"`
		Step  *json.Number `json:"step"`
	}
	if json.Unmarshal(version.ParamSchema, &fields) != nil {
		return ErrInvalidConfiguration
	}
	for _, field := range fields {
		rule, exists := contract[field.Field]
		if !exists || rule.Type != field.Type {
			return ErrInvalidConfiguration
		}
		if len(rule.Values) > 0 {
			if len(field.Enum) == 0 {
				return ErrInvalidConfiguration
			}
			for _, value := range field.Enum {
				if !slices.Contains(rule.Values, value) {
					return ErrInvalidConfiguration
				}
			}
		}
		if rule.Type == "integer" {
			if field.Min == nil || field.Max == nil || field.Step == nil {
				return ErrInvalidConfiguration
			}
			low, lowOK := new(big.Rat).SetString(field.Min.String())
			high, highOK := new(big.Rat).SetString(field.Max.String())
			step, stepOK := new(big.Rat).SetString(field.Step.String())
			if !lowOK || !highOK || !stepOK || !low.IsInt() || !high.IsInt() || !step.IsInt() ||
				low.Cmp(big.NewRat(rule.Minimum, 1)) < 0 || high.Cmp(big.NewRat(rule.Maximum, 1)) > 0 ||
				new(big.Int).Mod(low.Num(), big.NewInt(rule.Step)).Sign() != 0 ||
				new(big.Int).Mod(high.Num(), big.NewInt(rule.Step)).Sign() != 0 ||
				step.Sign() <= 0 || new(big.Int).Mod(step.Num(), big.NewInt(rule.Step)).Sign() != 0 {
				return ErrInvalidConfiguration
			}
		}
	}
	return nil
}
