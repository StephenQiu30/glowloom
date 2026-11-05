package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

const (
	maxMediaReferencePlans        = 50000
	maxMediaReferencePlanBytes    = domain.MaxWritePlanBytes
	maxMediaReferenceHistoryBytes = domain.MaxWritePlanBytes * 2
)

// MediaReferenceGuard protects complete source history and recoverable original-document pins.
type MediaReferenceGuard struct {
	tx     *gorm.DB
	access application.ProjectAccess
}

// NewMediaReferenceGuard injects the caller transaction and current owning project authority.
func NewMediaReferenceGuard(tx *gorm.DB, access application.ProjectAccess) *MediaReferenceGuard {
	return &MediaReferenceGuard{tx: tx, access: access}
}

// HasMediaReferences reads only script-owned historical and durable recovery facts.
// Media object state and Library visibility do not substitute for these references.
func (g *MediaReferenceGuard) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (referenced bool, resultErr error) {
	if g == nil || g.tx == nil || g.tx.Statement == nil || g.access == nil || project == uuid.Nil || asset == uuid.Nil {
		return false, application.ErrUnavailable
	}
	if _, ok := g.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrUnavailable
	}
	facts, err := g.access.Authorize(ctx, actor, project, false)
	if err != nil {
		return false, err
	}
	if facts.ProjectID != project || facts.OrgID != actor.OrgID || facts.Revision < 1 {
		return false, application.ErrUnavailable
	}
	tx := g.tx.WithContext(ctx)
	var sourceReferences int64
	if err := tx.Model(&sourceRow{}).
		Where("org_id = ? AND project_id = ? AND media_asset_id = ?", actor.OrgID, project, asset).
		Count(&sourceReferences).Error; err != nil {
		return false, err
	}
	if sourceReferences != 0 {
		return true, nil
	}
	var activeImports int64
	if err := tx.Model(&scriptImportFileRow{}).
		Joins("JOIN script.import_job AS import_job ON import_job.id = script.import_file.job_id").
		Joins("JOIN script.import_state AS import_state ON import_state.job_id = import_job.id").
		Where("import_job.org_id = ? AND import_job.project_id = ? AND script.import_file.project_id = ? AND script.import_file.asset_id = ?", actor.OrgID, project, project, asset).
		Where("(import_state.status NOT IN ? OR import_state.needs_reconciliation = ? OR import_state.io_owner_id IS NOT NULL OR import_state.io_state IN ?)", []domain.ImportStatus{domain.ImportStatusSucceeded, domain.ImportStatusCancelled}, true, []domain.ImportIOState{domain.ImportIOStateRunning, domain.ImportIOStateUnknown}).
		Count(&activeImports).Error; err != nil {
		return false, err
	}
	if activeImports != 0 {
		return true, nil
	}
	// Bound the body in PostgreSQL before any oversized JSON reaches the
	// process. Stream the retained history rather than collecting the entire
	// row limit before enforcing its aggregate memory budget.
	rows, err := tx.Model(&scriptCommandRow{}).
		Select("script.command.actor_id, script.command.request_id, CASE WHEN octet_length(script.command.plan::text) <= ? THEN script.command.plan ELSE NULL END AS plan", maxMediaReferencePlanBytes).
		Joins("JOIN script.command_state AS command_state ON command_state.actor_id = script.command.actor_id AND command_state.request_id = script.command.request_id").
		Where("script.command.org_id = ? AND script.command.project_id = ? AND command_state.status = ?", actor.OrgID, project, scriptCommandPending).
		Order("script.command.created_at").Order("script.command.actor_id").Order("script.command.request_id").
		Limit(maxMediaReferencePlans + 1).Rows()
	if err != nil {
		return false, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, rows.Close())
		if resultErr != nil {
			referenced = false
		}
	}()
	var count, bytesRead int
	for rows.Next() {
		count++
		if count > maxMediaReferencePlans {
			return false, application.ErrUnavailable
		}
		var row scriptCommandRow
		if err := rows.Scan(&row.ActorID, &row.RequestID, &row.Plan); err != nil {
			return false, err
		}
		bytesRead += len(row.Plan)
		if len(row.Plan) == 0 || len(row.Plan) > maxMediaReferencePlanBytes || bytesRead > maxMediaReferenceHistoryBytes {
			return false, application.ErrUnavailable
		}
		var plan application.WritePlan
		decoder := json.NewDecoder(strings.NewReader(string(row.Plan)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&plan) != nil || decoder.Decode(new(any)) != io.EOF || plan.Command.ProjectID != project || plan.Command.Key != row.RequestID || validatePlan(identityapp.Principal{ID: row.ActorID, OrgID: actor.OrgID}, plan) != nil {
			return false, application.ErrUnavailable
		}
		for _, source := range plan.NewSources {
			if source.Origin == domain.SourceOriginFile {
				if source.MediaAssetID == nil || *source.MediaAssetID == uuid.Nil || source.MediaRevision == nil || *source.MediaRevision < 1 || source.MediaSHA256 == nil || *source.MediaSHA256 != source.Original.SHA256 {
					return false, application.ErrUnavailable
				}
			} else if source.MediaAssetID != nil || source.MediaRevision != nil || source.MediaSHA256 != nil {
				return false, application.ErrUnavailable
			}
			if source.MediaAssetID != nil && *source.MediaAssetID == asset {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}
