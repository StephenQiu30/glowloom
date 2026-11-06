package postgres

import (
	"strings"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// saveSelectedModelDrafts instantiates only the submitted candidates. Imported
// templates contribute model configuration but do not create price or release records.
func saveSelectedModelDrafts(tx *gorm.DB, actor identityapp.Principal, providerID uuid.UUID, catalogVersion string, templates []domain.PresetModelTemplate, candidates []domain.ModelCandidatePreset) ([]uuid.UUID, error) {
	type modelIdentity struct{ capability, upstreamID string }
	seen := make(map[modelIdentity]bool, len(candidates))
	byKey := make(map[string]domain.PresetModelTemplate, len(templates))
	for _, template := range templates {
		byKey[template.Key] = template
	}

	ids := make([]uuid.UUID, 0, len(candidates))
	for _, candidate := range candidates {
		identity := modelIdentity{candidate.Capability, candidate.ProviderModelID}
		if candidate.RequiresModelSelection || identity.upstreamID == "" || len(identity.upstreamID) > 200 ||
			strings.ContainsFunc(identity.upstreamID, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) || seen[identity] {
			return nil, domain.ErrInvalidPreset
		}
		seen[identity] = true
		id, err := createPresetCandidate(tx, providerID, catalogVersion, candidate)
		if err != nil {
			return nil, err
		}
		template, hasTemplate := byKey[candidate.Key]
		if hasTemplate {
			if err := appendModelVersionInTx(tx, template.Version(id, actor.ID), 1); err != nil {
				return nil, err
			}
		} else if candidate.SourceURL != "custom://admin" {
			// Some candidates only describe discoverable IDs. Preserve those
			// as drafts until an administrator supplies the missing contract.
			if version, err := candidate.OnboardingVersion(id, actor.ID); err == nil {
				if err := appendModelVersionInTx(tx, version, 1); err != nil {
					return nil, err
				}
			}
		}
		ids = append(ids, id)
	}
	return ids, nil
}
