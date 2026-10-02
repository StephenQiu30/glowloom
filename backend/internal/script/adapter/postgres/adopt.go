package postgres

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Adopt publishes only a fully confirmed own version and preserves prior adopted facts.
// Replacing an existing adopted version requires downstream owners that are not yet
// wired here; their absence is explicit rather than a fabricated empty impact set.
func (s *SourceStore) Adopt(ctx context.Context, actor identityapp.Principal, input application.AdoptCommand, now time.Time) (application.AdoptReceipt, error) {
	var result application.AdoptReceipt
	fingerprint := input
	fingerprint.RequestID = [16]byte{}
	hash, err := reviewHash("adopt", fingerprint)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := commandLock(tx, actor.ID, input.Key); err != nil {
			return err
		}
		replayed, err := replayReview(tx, actor, input.ProjectID, input.Key, "adopt", hash, &result)
		if err != nil || replayed {
			return err
		}
		state, err := readState(tx, actor.OrgID, input.ProjectID, true)
		if err != nil {
			return err
		}
		if state.Revision != input.ExpectedRevision {
			return application.ErrConflict
		}
		if err := rejectPendingWrite(tx, actor.OrgID, input.ProjectID); err != nil {
			return err
		}
		version, err := readVersion(tx, actor.OrgID, input.ProjectID, input.VersionID)
		if err != nil {
			return err
		}
		head, err := readVersionHead(tx, actor.OrgID, input.ProjectID, input.VersionID, true)
		if err != nil {
			return err
		}
		if head.SplitRevision != input.ExpectedSplitRevision {
			return application.ErrConflict
		}
		if head.ConfirmedSetID == nil {
			return application.ErrConfirmationRequired
		}
		episodes, err := readEpisodes(tx, actor.OrgID, input.ProjectID, input.VersionID, true)
		if err != nil {
			return err
		}
		var latest struct{ Preface string }
		read := tx.Raw(`SELECT preface::text FROM script.split_confirmation WHERE org_id=? AND project_id=? AND version_id=? AND formal_set_id=? ORDER BY revision DESC LIMIT 1`, actor.OrgID, input.ProjectID, input.VersionID, *head.ConfirmedSetID).Scan(&latest)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrContextUnavailable
		}
		var preface *domain.ScalarSpan
		if err := json.Unmarshal([]byte(latest.Preface), &preface); err != nil {
			return application.ErrUnavailable
		}
		boundaries := make([]domain.EpisodeBoundary, 0, len(episodes))
		for _, e := range episodes {
			if e.SplitSetID != *head.ConfirmedSetID {
				return application.ErrObjectMismatch
			}
			boundaries = append(boundaries, domain.EpisodeBoundary{SeqNo: e.SeqNo, Title: e.Title, Start: e.Start, End: e.End})
		}
		if err := domain.ValidateBoundaries(boundaries, preface, version.CharCount); err != nil {
			return err
		}
		result = application.AdoptReceipt{ScriptRevision: state.Revision, ProjectRevision: project.Revision, VersionID: input.VersionID, PreviousVersionID: state.AdoptedVersionID, SplitSetID: *head.ConfirmedSetID, Mappings: make([]application.EpisodeMapping, 0, len(episodes))}
		if state.AdoptedVersionID != nil && *state.AdoptedVersionID != input.VersionID {
			old, err := readEpisodes(tx, actor.OrgID, input.ProjectID, *state.AdoptedVersionID, true)
			if err != nil {
				return err
			}
			affected := make([]application.AffectedEpisode, 0)
			for _, e := range old {
				if e.ConfirmedStructureID != nil {
					affected = append(affected, application.AffectedEpisode{EpisodeID: e.ID, ConfirmedStructureID: *e.ConfirmedStructureID, Reason: "adopted_version_changed"})
				}
			}
			if len(affected) > 0 {
				return &application.ImpactError{Affected: affected, NeedsAck: !input.AckInvalidate}
			}
			return application.ErrContextUnavailable
		}
		result.Duplicate = state.AdoptedVersionID != nil
		if !result.Duplicate {
			if err := exactlyOne(tx.Exec(`UPDATE script.project_state SET adopted_version_id=?,revision=revision+1,updated_at=? WHERE org_id=? AND project_id=? AND revision=?`, input.VersionID, now, actor.OrgID, input.ProjectID, state.Revision)); err != nil {
				return err
			}
			projectRevision, err := access.TouchContent(ctx, actor, input.ProjectID, project.Revision)
			if err != nil {
				return err
			}
			result.ScriptRevision, result.ProjectRevision, result.Changed = state.Revision+1, projectRevision, true
		}
		for _, e := range episodes {
			status := "not_inherited"
			if result.Duplicate {
				status = "retained"
			}
			result.Mappings = append(result.Mappings, application.EpisodeMapping{EpisodeID: &e.ID, InheritStatus: status})
		}
		if err := saveReview(tx, actor, input.ProjectID, input.Key, "adopt", hash, result, now); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.version_adopted", result.ScriptRevision, input.VersionID, nil, now)
	})
	return result, err
}
