package pricevalidation

import (
	"encoding/json"
	"slices"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ValidateModel checks the frozen combination at preset import and formal release.
// Individual price publication cannot establish coverage without a model version.
func ValidateModel(config domain.ProviderConfig, capability string, version domain.ModelVersion, price domain.PriceRuleVersion) error {
	validator, err := NewValidator()
	if err != nil {
		return err
	}
	if validator.Validate(price) != nil {
		return ErrInvalidConfiguration
	}
	fields, err := parseObject(price.Rule)
	if err != nil {
		return ErrInvalidConfiguration
	}
	var strategy string
	if raw := fields["pricing_strategy"]; raw != nil {
		if json.Unmarshal(raw, &strategy) != nil {
			return ErrInvalidConfiguration
		}
	}
	if capability == "video.generate" && price.Unit == domain.PricePer1KTokens {
		if config.AdapterKey != "volcengine_ark" || !domain.IsSeedanceTokenModel(version.ProviderModelID) ||
			string(fields["input_micros_per_1k"]) != "0" || fields["cached_input_micros_per_1k"] != nil || fields["time_pricing"] != nil ||
			(strategy != "" && strategy != domain.PricingSeedanceVideoTokensV1) {
			return ErrInvalidConfiguration
		}
	} else if strategy == domain.PricingSeedanceVideoTokensV1 {
		return ErrInvalidConfiguration
	}
	if strategy == domain.PricingTokenIOV1 && capability != "text.generate" {
		return ErrInvalidConfiguration
	}
	if strategy == domain.PricingImageFlatV1 && capability != "image.generate" {
		return ErrInvalidConfiguration
	}
	if raw := fields["by_mode"]; raw != nil {
		matrix, err := parseObject(raw)
		if err != nil || !covers(matrix, version.Modes) {
			return ErrInvalidConfiguration
		}
	}
	if raw := fields["by_resolution"]; raw != nil {
		matrix, err := parseObject(raw)
		if err != nil {
			return ErrInvalidConfiguration
		}
		var schema []struct {
			Field string   `json:"field"`
			Enum  []string `json:"enum"`
		}
		if json.Unmarshal(version.ParamSchema, &schema) != nil {
			return ErrInvalidConfiguration
		}
		dimensions := 0
		for _, field := range schema {
			if field.Field == "resolution" || field.Field == "size" {
				dimensions++
				if len(field.Enum) == 0 || !covers(matrix, field.Enum) {
					return ErrInvalidConfiguration
				}
			}
		}
		if dimensions != 1 {
			return ErrInvalidConfiguration
		}
	}
	// The protocol's parameter allowlist is independent of the model's tighter SKU contract.
	var schema []struct {
		Field string `json:"field"`
	}
	if json.Unmarshal(version.ParamSchema, &schema) != nil {
		return ErrInvalidConfiguration
	}
	for _, field := range schema {
		if !slices.Contains(config.ParameterFields(capability), field.Field) {
			return ErrInvalidConfiguration
		}
	}
	return nil
}

func covers(matrix map[string]json.RawMessage, values []string) bool {
	for _, value := range values {
		if matrix[value] == nil {
			return false
		}
	}
	return len(values) > 0
}
