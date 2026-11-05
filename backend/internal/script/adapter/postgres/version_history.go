package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Confirmations preserves each immutable boundary/identity decision on the selected version.
func (s *SourceStore) Confirmations(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID, before int64, limit int) (application.ConfirmationPage, error) {
	result := application.ConfirmationPage{Items: []application.ConfirmationSummary{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if _, err := readVersion(tx, actor.OrgID, project, version); err != nil {
			return err
		}
		query := tx.Model(&scriptConfirmationSummaryRow{}).Where("org_id = ? AND project_id = ? AND version_id = ?", actor.OrgID, project, version)
		if before > 0 {
			query = query.Where("revision < ?", before)
		}
		var rows []scriptConfirmationSummaryRow
		if err := query.Order("revision DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var episodes []domain.Episode
			if err := json.Unmarshal(row.Episodes, &episodes); err != nil {
				return application.ErrUnavailable
			}
			result.Items = append(result.Items, application.ConfirmationSummary{ID: row.ID, Revision: row.Revision, CandidateSetID: row.CandidateSetID, FormalSetID: row.FormalSetID, EpisodeCount: len(episodes), CreatedAt: row.CreatedAt.UTC()})
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
		var row scriptSplitConfirmationRow
		if err := tx.Where("id = ? AND org_id = ? AND project_id = ? AND version_id = ?", id, actor.OrgID, project, version).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return err
		}
		confirmation, err := row.confirmation()
		if err != nil {
			return err
		}
		if confirmation.ID == uuid.Nil {
			return application.ErrNotFound
		}
		result = confirmation
		return nil
	})
	return result, err
}
