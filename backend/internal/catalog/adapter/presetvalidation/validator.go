// Package presetvalidation reuses the model and price contracts for portable presets.
package presetvalidation

import (
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
	for _, template := range doc.ModelTemplates {
		version := template.Version(uuid.NameSpaceOID, uuid.NameSpaceOID)
		price := template.PriceVersion(uuid.NameSpaceOID, uuid.NameSpaceOID, template.UpstreamCost.ReviewedAt)
		if id, err := price.ProcurementVersion(); err != nil || id != uuid.Nil {
			return domain.ErrInvalidPreset
		}
		if params.Validate(version) != nil || pricevalidation.ValidateModel(doc.Config, template.Capability, version, price) != nil {
			return domain.ErrInvalidPreset
		}
	}
	return nil
}
