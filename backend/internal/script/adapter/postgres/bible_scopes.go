package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
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
		tx := s.tx.WithContext(ctx)
		state, err := readState(tx, actor.OrgID, project, true)
		if err != nil {
			return err
		}
		episode, err := readEpisode(tx, actor.OrgID, scope.EpisodeID, true)
		if err != nil {
			return err
		}
		if episode.ProjectID != project {
			return application.ErrNotFound
		}
		head, err := readVersionHead(tx, actor.OrgID, project, episode.VersionID, true)
		if err != nil {
			return err
		}
		current := state.DraftVersionID != nil && *state.DraftVersionID == episode.VersionID || state.AdoptedVersionID != nil && *state.AdoptedVersionID == episode.VersionID
		if !current || head.ConfirmedSetID == nil || *head.ConfirmedSetID != episode.SplitSetID {
			return application.ErrNotFound
		}
		if scope.SceneKey != nil {
			if episode.ConfirmedStructureID == nil {
				return application.ErrNotFound
			}
			var count int64
			if err := tx.Model(&scriptSceneRow{}).Where("org_id = ? AND project_id = ? AND episode_structure_id = ? AND scene_key = ?", actor.OrgID, project, *episode.ConfirmedStructureID, *scope.SceneKey).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return application.ErrNotFound
			}
		}
	}
	return nil
}
