package pgsource

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func freezeClosedDetails(ctx context.Context, tx *gorm.DB, state source.ProjectClosureState, project domain.Project) (source.ProjectCloseDetails, error) {
	details := source.ProjectCloseDetails{Project: project}
	var defaults string
	if err := tx.Raw(`SELECT default_models::text FROM workspace.project WHERE id=? AND org_id=?`, project.ID, project.OrgID).Scan(&defaults).Error; err != nil {
		return details, err
	}
	if projectJSON([]byte(defaults), &details.DefaultModels) != nil || details.DefaultModels == nil {
		return details, writefence.ErrConflict
	}
	var activity struct {
		ModifiedAt time.Time
		Basis      string
	}
	read := tx.Raw(`SELECT COALESCE(a.modified_at,p.create_time) AS modified_at,COALESCE(a.basis,'created') AS basis FROM workspace.project p LEFT JOIN workspace.project_activity a ON a.project_id=p.id WHERE p.id=?`, project.ID).Scan(&activity)
	if read.Error != nil {
		return details, read.Error
	}
	if read.RowsAffected != 1 {
		return details, writefence.ErrConflict
	}
	details.LastModifiedAt = &activity.ModifiedAt
	details.LastModifiedBasis = &activity.Basis
	if project.CoverAssetID != nil {
		proof, ok := source.ProjectClosureCover(ctx)
		if !ok || proof.Request != state.Spec.CoverRequest() {
			return details, source.ErrProjectClosureUnknown
		}
		details.Cover = &proof
		details.CoverUnavailable = !proof.Available
	}
	return details, details.Validate()
}

func decodeClosedDetails(raw json.RawMessage, state *source.ProjectClosureState) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return writefence.ErrConflict
	}
	if _, wrapped := fields["Project"]; wrapped {
		details := new(source.ProjectCloseDetails)
		if projectJSON(raw, details) != nil || details.Validate() != nil {
			return writefence.ErrConflict
		}
		if details.Cover != nil && details.Cover.Request != state.Spec.CoverRequest() {
			return writefence.ErrConflict
		}
		state.Details = details
		state.Result = &details.Project
	} else {
		// Earlier committed source-only receipts predate the public detail writer.
		// Read their exact original Project; never manufacture absent frozen facts.
		state.Result = new(domain.Project)
		if projectJSON(raw, state.Result) != nil || state.Result.Validate() != nil {
			return writefence.ErrConflict
		}
	}
	if state.Result.ID != state.Spec.ProjectID || state.Result.OrgID != state.Spec.OrganizationID {
		return writefence.ErrConflict
	}
	return nil
}
