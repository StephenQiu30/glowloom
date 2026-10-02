package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// BibleScopes reads only formal script identities under the caller's current project lock.
type BibleScopes struct {
	tx     *gorm.DB
	access application.ProjectAccess
}

// NewBibleScopes injects the caller transaction and current workspace authority.
func NewBibleScopes(tx *gorm.DB, access application.ProjectAccess) *BibleScopes {
	return &BibleScopes{tx: tx, access: access}
}

// ValidateLookScopes verifies current formal episodes and confirmed stable scene keys.
func (s *BibleScopes) ValidateLookScopes(ctx context.Context, actor identityapp.Principal, project uuid.UUID, scopes []application.BibleLookScope) error {
	if s == nil || s.tx == nil || s.access == nil {
		return application.ErrContextUnavailable
	}
	if _, ok := s.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrContextUnavailable
	}
	facts, err := s.access.Authorize(ctx, actor, project, false)
	if err != nil {
		return err
	}
	if facts.ProjectID != project || facts.OrgID != actor.OrgID {
		return application.ErrContextUnavailable
	}
	for _, scope := range scopes {
		if scope.EpisodeID == uuid.Nil || scope.SceneKey != nil && *scope.SceneKey == uuid.Nil {
			return domain.ErrInvalidStructure
		}
		var row struct {
			ID                   uuid.UUID
			ConfirmedStructureID *uuid.UUID
		}
		read := s.tx.WithContext(ctx).Raw(`SELECT e.id,e.confirmed_structure_id FROM script.episode e JOIN script.version_head h ON h.version_id=e.script_version_id AND h.org_id=e.org_id AND h.project_id=e.project_id AND h.confirmed_split_set_id=e.split_set_id JOIN script.project_state s ON s.project_id=e.project_id AND s.org_id=e.org_id AND COALESCE(s.adopted_version_id,s.draft_version_id)=e.script_version_id WHERE e.org_id=? AND e.project_id=? AND e.id=? AND NOT e.is_delete FOR SHARE OF e,h,s`, actor.OrgID, project, scope.EpisodeID).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		if scope.SceneKey != nil {
			if row.ConfirmedStructureID == nil {
				return application.ErrNotFound
			}
			var count int64
			if err := s.tx.Raw(`SELECT count(*) FROM script.scene WHERE org_id=? AND project_id=? AND episode_structure_id=? AND scene_key=?`, actor.OrgID, project, *row.ConfirmedStructureID, *scope.SceneKey).Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return application.ErrNotFound
			}
		}
	}
	return nil
}
