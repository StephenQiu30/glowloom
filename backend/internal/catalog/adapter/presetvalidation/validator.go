// Package presetvalidation reuses the model and price contracts for portable presets.
package presetvalidation

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// Validate checks the same model and price rules at preview, save and instantiation.
func Validate(doc domain.PresetDocument) error {
	if doc.Validate() != nil {
		return domain.ErrInvalidPreset
	}
	if len(doc.ModelTemplates) == 0 {
		return nil
	}
	params, err := paramvalidation.NewValidator()
	if err != nil {
		return err
	}
	price, err := pricevalidation.NewValidator()
	if err != nil {
		return err
	}
	for _, template := range doc.ModelTemplates {
		version := template.Version(uuid.NameSpaceOID, uuid.NameSpaceOID)
		var fields []struct {
			Field string `json:"field"`
		}
		if json.Unmarshal(template.ParamSchema, &fields) != nil {
			return domain.ErrInvalidPreset
		}
		for _, field := range fields {
			if !slices.Contains(doc.Config.ParameterFields(template.Capability), field.Field) {
				return domain.ErrInvalidPreset
			}
		}

		if params.Validate(version) != nil || price.Validate(template.PriceVersion(uuid.NameSpaceOID, uuid.NameSpaceOID, template.UpstreamCost.ReviewedAt)) != nil {
			return domain.ErrInvalidPreset
		}
	}
	return nil
}
