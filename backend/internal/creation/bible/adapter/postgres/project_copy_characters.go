package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/bible/application"
)

// ResolveCharacterMappings verifies exact frozen stable/pinned history under trusted caller-owned SQL.
func (s *ProjectCopyStore) ResolveCharacterMappings(ctx context.Context, actor application.Actor, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, refs []application.CharacterCopyReference) ([]application.CharacterCopyMapping, error) {
	var result []application.CharacterCopyMapping
	err := s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		m, err := readBibleManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		result, err = application.ResolveCopyCharacters(m, refs)
		return err
	})
	return result, err
}
