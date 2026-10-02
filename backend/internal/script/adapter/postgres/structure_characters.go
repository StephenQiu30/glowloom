package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func (s *SourceStore) freezeStructureCharacters(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, document *domain.StructureDocument, pinnedOnly bool) error {
	// Freeze owned slices so version resolution cannot change a caller's original command hash.
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	var ownDocument domain.StructureDocument
	if err := json.Unmarshal(encoded, &ownDocument); err != nil {
		return err
	}
	*document = ownDocument
	var reader application.CharacterReferences
	for i := range document.Scenes {
		for j := range document.Scenes[i].Items {
			item := &document.Scenes[i].Items[j]
			if item.CharacterID == nil {
				if item.CharacterVersionID != nil {
					return domain.ErrInvalidStructure
				}
				continue
			}
			if s.characters == nil {
				return application.ErrContextUnavailable
			}
			if reader == nil {
				reader = s.characters(tx)
				if reader == nil {
					return application.ErrContextUnavailable
				}
			}
			if pinnedOnly && item.CharacterVersionID == nil {
				return application.ErrContextUnavailable
			}
			fact, err := reader.Reference(ctx, actor, project, *item.CharacterID, item.CharacterVersionID)
			if err != nil {
				return err
			}
			if fact.CharacterID == uuid.Nil || fact.VersionID == uuid.Nil || fact.Revision < 1 {
				return application.ErrContextUnavailable
			}
			if item.CharacterVersionID != nil && (fact.CharacterID != *item.CharacterID || fact.VersionID != *item.CharacterVersionID) {
				return application.ErrConflict
			}
			id, version := fact.CharacterID, fact.VersionID
			item.CharacterID = &id
			item.CharacterVersionID = &version
		}
	}
	return nil
}
