package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

const (
	maxMediaReferencePlans        = 50000
	maxMediaReferencePlanBytes    = 1 << 20
	maxMediaReferenceHistoryBytes = 64 << 20
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
	var found bool
	read := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.script_source WHERE org_id=? AND project_id=? AND media_asset_id=?) OR EXISTS(SELECT 1 FROM script.import_file f JOIN script.import_job j ON j.id=f.job_id JOIN script.import_state s ON s.job_id=j.id WHERE j.org_id=? AND j.project_id=? AND f.project_id=? AND f.asset_id=? AND (s.status NOT IN ('succeeded','cancelled') OR s.needs_reconciliation OR s.io_owner_id IS NOT NULL OR s.io_state IN ('running','unknown')))`, actor.OrgID, project, asset, actor.OrgID, project, project, asset).Scan(&found)
	if read.Error != nil {
		return false, read.Error
	}
	if read.RowsAffected != 1 {
		return false, application.ErrUnavailable
	}
	if found {
		return true, nil
	}
	// Bound the body in PostgreSQL before any oversized JSON reaches the
	// process. Stream the retained history rather than collecting the entire
	// row limit before enforcing its aggregate memory budget.
	rows, err := tx.Raw(`SELECT c.actor_id,c.request_id,CASE WHEN octet_length(c.plan::text) BETWEEN 1 AND ? THEN c.plan::text ELSE '' END AS plan FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND s.status='pending' ORDER BY c.created_at,c.actor_id,c.request_id LIMIT ?`, maxMediaReferencePlanBytes, actor.OrgID, project, maxMediaReferencePlans+1).Rows()
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
		var actorID, requestID uuid.UUID
		var raw string
		if err := rows.Scan(&actorID, &requestID, &raw); err != nil {
			return false, err
		}
		bytesRead += len(raw)
		if len(raw) == 0 || len(raw) > maxMediaReferencePlanBytes || bytesRead > maxMediaReferenceHistoryBytes {
			return false, application.ErrUnavailable
		}
		var plan application.WritePlan
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&plan) != nil || decoder.Decode(new(any)) != io.EOF || plan.Command.ProjectID != project || plan.Command.Key != requestID || validatePlan(identityapp.Principal{ID: actorID, OrgID: actor.OrgID}, plan) != nil {
			return false, application.ErrUnavailable
		}
		for _, source := range plan.NewSources {
			if source.Origin == "file" {
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
