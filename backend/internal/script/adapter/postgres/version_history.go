package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Confirmations preserves each immutable boundary/identity decision on the selected version.
func (s *SourceStore) Confirmations(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID, before int64, limit int) (application.ConfirmationPage, error) {
	result := application.ConfirmationPage{Items: []application.ConfirmationSummary{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if _, err := readVersion(tx, actor.OrgID, project, version); err != nil {
			return err
		}
		if err := tx.Raw(`SELECT id,revision,candidate_set_id,formal_set_id,jsonb_array_length(episodes) AS episode_count,created_at FROM script.split_confirmation WHERE org_id=? AND project_id=? AND version_id=? AND (?=0 OR revision<?) ORDER BY revision DESC LIMIT ?`, actor.OrgID, project, version, before, before, limit+1).Scan(&result.Items).Error; err != nil {
			return err
		}
		if len(result.Items) > limit {
			result.Items = result.Items[:limit]
			next := result.Items[limit-1].Revision
			result.NextRevision = &next
		}
		for i := range result.Items {
			result.Items[i].CreatedAt = result.Items[i].CreatedAt.UTC()
		}
		return nil
	})
	return result, err
}

// Confirmation reads the recorded past snapshot rather than today's episode pointers.
func (s *SourceStore) Confirmation(ctx context.Context, actor identityapp.Principal, project, version, id uuid.UUID) (domain.SplitConfirmation, error) {
	var result domain.SplitConfirmation
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var row struct {
			ID, OrgID, ProjectID, VersionID, CandidateSetID, FormalSetID, ActorID uuid.UUID
			Revision                                                              int64
			Preface, Episodes                                                     string
			CreatedAt                                                             time.Time
		}
		read := tx.Raw(`SELECT id,org_id,project_id,version_id,candidate_set_id,formal_set_id,actor_id,revision,preface::text,episodes::text,created_at FROM script.split_confirmation WHERE id=? AND org_id=? AND project_id=? AND version_id=?`, id, actor.OrgID, project, version).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		result = domain.SplitConfirmation{ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID, VersionID: row.VersionID, CandidateSetID: row.CandidateSetID, FormalSetID: row.FormalSetID, ActorID: row.ActorID, Revision: row.Revision, CreatedAt: row.CreatedAt.UTC()}
		if err := json.Unmarshal([]byte(row.Preface), &result.Preface); err != nil {
			return application.ErrUnavailable
		}
		if err := json.Unmarshal([]byte(row.Episodes), &result.Episodes); err != nil {
			return application.ErrUnavailable
		}
		return nil
	})
	return result, err
}
