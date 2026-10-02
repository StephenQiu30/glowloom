package postgres

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func reuseOrNumberVersion(tx *gorm.DB, actor identityapp.Principal, state domain.ProjectState, p *application.WritePlan) error {
	var row struct{ ID uuid.UUID }
	q := tx.Raw(`SELECT id FROM script.script_version WHERE org_id=? AND project_id=? AND source_manifest_sha256=?`, actor.OrgID, p.Command.ProjectID, p.Version.SourceManifestSHA256).Scan(&row)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 0 {
		var latest int64
		if err := tx.Raw(`SELECT coalesce(max(version_no),0) FROM script.script_version WHERE org_id=? AND project_id=?`, actor.OrgID, p.Command.ProjectID).Scan(&latest).Error; err != nil {
			return err
		}
		p.Version.VersionNo = latest + 1
		return nil
	}
	if len(p.NewSources) != 0 {
		return application.ErrObjectMismatch
	}
	v, err := readVersion(tx, actor.OrgID, p.Command.ProjectID, row.ID)
	if err != nil {
		return err
	}
	if !slices.Equal(v.SourceIDs, p.Version.SourceIDs) || v.ContentHash != p.Version.ContentHash || v.DocumentSHA256 != p.Version.DocumentSHA256 || v.CharCount != p.Version.CharCount {
		return application.ErrObjectMismatch
	}
	head, err := readVersionHead(tx, actor.OrgID, p.Command.ProjectID, v.ID, false)
	if err != nil {
		return err
	}
	split, err := readSplitSet(tx, actor.OrgID, p.Command.ProjectID, head.CandidateSetID)
	if err != nil {
		return err
	}
	p.Version = v
	p.Candidate = split
	p.Objects = []domain.ObjectFact{}
	p.Reuse = true
	p.HeadUnchanged = state.DraftVersionID != nil && *state.DraftVersionID == v.ID
	return nil
}
func requirePendingSource(tx *gorm.DB, p application.WritePlan) error {
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM script.command_state WHERE actor_id=? AND request_id=? AND status='pending' AND NOT cancellation_requested AND io_owner_id IS NULL AND io_state='idle'`, p.ActorID, p.Command.Key).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return application.ErrConflict
	}
	return nil
}
func finishReusedWrite(ctx context.Context, tx *gorm.DB, access application.ProjectAccess, actor identityapp.Principal, project workspaceapp.ProjectContentAccess, state domain.ProjectState, p application.WritePlan) (application.SourceReceipt, error) {
	result := application.SourceReceipt{ScriptRevision: state.Revision, ProjectRevision: project.Revision, VersionID: p.Version.ID, SplitSetID: p.Candidate.ID, Mappings: p.Mappings, Changed: !p.HeadUnchanged, Duplicate: true}
	if !p.HeadUnchanged {
		if err := exactlyOne(tx.Exec(`UPDATE script.project_state SET draft_version_id=?,revision=revision+1,updated_at=? WHERE org_id=? AND project_id=? AND revision=?`, p.Version.ID, p.CreatedAt, actor.OrgID, p.Command.ProjectID, state.Revision)); err != nil {
			return result, err
		}
		revision, err := access.TouchContent(ctx, actor, p.Command.ProjectID, project.Revision)
		if err != nil {
			return result, err
		}
		result.ProjectRevision = revision
		result.ScriptRevision++
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if err := exactlyOne(tx.Exec(`INSERT INTO script.command_result(actor_id,request_id,response,created_at) VALUES(?,?,?::jsonb,?)`, actor.ID, p.Command.Key, string(bytes), p.CreatedAt)); err != nil {
		return result, err
	}
	if err := exactlyOne(tx.Exec(`UPDATE script.command_state SET status='completed',revision=revision+1,updated_at=? WHERE actor_id=? AND request_id=? AND status='pending' AND NOT cancellation_requested`, p.CreatedAt, actor.ID, p.Command.Key)); err != nil {
		return result, err
	}
	return result, sourceAudit(tx, actor, p, result)
}
