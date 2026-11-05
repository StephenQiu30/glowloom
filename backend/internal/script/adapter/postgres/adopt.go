package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
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
		if err := registerCommand(tx, actor, input.ProjectID, input.Key, "adopt", hash, now); err != nil {
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
		var latest scriptSplitConfirmationRow
		if err := tx.Where("org_id = ? AND project_id = ? AND version_id = ? AND formal_set_id = ?", actor.OrgID, input.ProjectID, input.VersionID, *head.ConfirmedSetID).
			Order("revision DESC").Take(&latest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrContextUnavailable
			}
			return err
		}
		var preface *domain.ScalarSpan
		if len(latest.Preface) > 0 && string(latest.Preface) != "null" {
			if err := json.Unmarshal(latest.Preface, &preface); err != nil {
				return application.ErrUnavailable
			}
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
			if err := exactlyOne(tx.Model(&scriptProjectStateRow{}).Where("org_id = ? AND project_id = ? AND revision = ?", actor.OrgID, input.ProjectID, state.Revision).Updates(map[string]any{"adopted_version_id": input.VersionID, "revision": state.Revision + 1, "updated_at": now})); err != nil {
				return err
			}
			projectRevision, err := access.TouchContent(ctx, actor, input.ProjectID, project.Revision)
			if err != nil {
				return err
			}
			result.ScriptRevision, result.ProjectRevision, result.Changed = state.Revision+1, projectRevision, true
		}
		for _, e := range episodes {
			status := domain.EpisodeInheritStatusNotInherited
			if result.Duplicate {
				status = domain.EpisodeInheritStatusRetained
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
