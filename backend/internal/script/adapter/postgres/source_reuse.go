package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func reuseOrNumberVersion(tx *gorm.DB, actor identityapp.Principal, state domain.ProjectState, p *application.WritePlan) error {
	var existing versionRow
	err := tx.Where("org_id = ? AND project_id = ? AND source_manifest_sha256 = ?", actor.OrgID, p.Command.ProjectID, p.Version.SourceManifestSHA256).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var latest versionRow
		latestErr := tx.Select("version_no").Where("org_id = ? AND project_id = ?", actor.OrgID, p.Command.ProjectID).Order("version_no DESC").Take(&latest).Error
		if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
			return latestErr
		}
		p.Version.VersionNo = latest.VersionNo + 1
		return nil
	}
	if err != nil {
		return err
	}
	if len(p.NewSources) != 0 {
		return application.ErrObjectMismatch
	}
	v, err := readVersion(tx, actor.OrgID, p.Command.ProjectID, existing.ID)
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
	var row scriptCommandStateRow
	err := tx.Where("actor_id = ? AND request_id = ? AND status = ? AND cancellation_requested = ? AND io_owner_id IS NULL AND io_state = ?", p.ActorID, p.Command.Key, scriptCommandPending, false, scriptIOIdle).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrConflict
	}
	if err != nil {
		return err
	}
	return nil
}
func finishReusedWrite(ctx context.Context, tx *gorm.DB, access application.ProjectAccess, actor identityapp.Principal, project workspaceapp.ProjectContentAccess, state domain.ProjectState, p application.WritePlan) (application.SourceReceipt, error) {
	result := application.SourceReceipt{ScriptRevision: state.Revision, ProjectRevision: project.Revision, VersionID: p.Version.ID, SplitSetID: p.Candidate.ID, Mappings: p.Mappings, Changed: !p.HeadUnchanged, Duplicate: true}
	if !p.HeadUnchanged {
		if err := exactlyOne(tx.Model(&scriptProjectStateRow{}).Where("org_id = ? AND project_id = ? AND revision = ?", actor.OrgID, p.Command.ProjectID, state.Revision).
			Updates(map[string]any{"draft_version_id": p.Version.ID, "revision": state.Revision + 1, "updated_at": p.CreatedAt})); err != nil {
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
	commandResult := scriptCommandResultRow{ActorID: actor.ID, RequestID: p.Command.Key, Response: scriptJSON(bytes), CreatedAt: p.CreatedAt}
	if err := exactlyOne(tx.Create(&commandResult)); err != nil {
		return result, err
	}
	var commandState scriptCommandStateRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ?", actor.ID, p.Command.Key).Take(&commandState).Error; err != nil {
		return result, err
	}
	if commandState.Status != scriptCommandPending || commandState.CancellationRequested {
		return result, application.ErrConflict
	}
	if err := exactlyOne(tx.Model(&scriptCommandStateRow{}).Where("actor_id = ? AND request_id = ? AND status = ? AND cancellation_requested = ?", actor.ID, p.Command.Key, scriptCommandPending, false).
		Updates(map[string]any{"status": scriptCommandCompleted, "needs_reconciliation": false, "revision": commandState.Revision + 1, "updated_at": p.CreatedAt})); err != nil {
		return result, err
	}
	return result, sourceAudit(tx, actor, p, result)
}
