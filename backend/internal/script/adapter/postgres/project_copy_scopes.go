package postgres

import (
	"context"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// RemapLookScopes plans complete historical formal identities under trusted caller-owned admission SQL.
// It neither reads remote objects nor creates content, jobs or confirmation facts.
func (s *ProjectCopyStore) RemapLookScopes(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, scopes []application.BibleLookScope) ([]application.CopyLookScopeMapping, error) {
	var result []application.CopyLookScopeMapping
	err := s.ownTransaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		if err := rejectPendingWrite(tx, b.OrgID, b.SourceProjectID); err != nil {
			return err
		}
		history, err := readCopyHistory(tx, b.OrgID, b.SourceProjectID)
		if err != nil {
			return err
		}
		result, err = application.PlanProjectCopyLookScopes(b, history, scopes)
		return err
	})
	return result, err
}
