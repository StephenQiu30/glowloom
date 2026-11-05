package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func readStructure(tx *gorm.DB, org, project, id uuid.UUID) (domain.EpisodeStructure, error) {
	var row scriptEpisodeStructureRow
	if err := tx.Where("id = ? AND org_id = ? AND project_id = ?", id, org, project).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeStructure{}, application.ErrNotFound
		}
		return domain.EpisodeStructure{}, err
	}
	return row.structure()
}

func episodeReviewState(tx *gorm.DB, actor identityapp.Principal, input application.StructureCommand) (domain.ProjectState, domain.Episode, error) {
	state, err := readState(tx, actor.OrgID, input.ProjectID, true)
	if err != nil {
		return state, domain.Episode{}, err
	}
	if state.Revision != input.ExpectedRevision {
		return state, domain.Episode{}, application.ErrConflict
	}
	if err := rejectPendingWrite(tx, actor.OrgID, input.ProjectID); err != nil {
		return state, domain.Episode{}, err
	}
	episode, err := readEpisode(tx, actor.OrgID, input.EpisodeID, true)
	if err != nil {
		return state, episode, err
	}
	if episode.ProjectID != input.ProjectID {
		return state, episode, application.ErrNotFound
	}
	if episode.Revision != input.ExpectedEpisodeRevision {
		return state, episode, application.ErrConflict
	}
	current := state.DraftVersionID != nil && *state.DraftVersionID == episode.VersionID || state.AdoptedVersionID != nil && *state.AdoptedVersionID == episode.VersionID
	if !current {
		return state, episode, application.ErrConflict
	}
	head, err := readVersionHead(tx, actor.OrgID, input.ProjectID, episode.VersionID, false)
	if err != nil {
		return state, episode, err
	}
	if head.ConfirmedSetID == nil || *head.ConfirmedSetID != episode.SplitSetID {
		return state, episode, application.ErrConflict
	}
	return state, episode, nil
}

func insertStructure(tx *gorm.DB, s domain.EpisodeStructure) error {
	data, err := json.Marshal(s.Document)
	if err != nil {
		return err
	}
	structure := scriptEpisodeStructureRow{ID: s.ID, OrgID: s.OrgID, ProjectID: s.ProjectID, EpisodeID: s.EpisodeID, VersionNo: s.VersionNo, SourceHash: s.SourceHash, Document: scriptJSON(data), ActorID: s.ActorID, CreatedAt: s.CreatedAt}
	if err := exactlyOne(tx.Create(&structure)); err != nil {
		return err
	}
	for _, scene := range s.Document.Scenes {
		id := uuid.NewSHA1(s.ID, []byte("scene/"+scene.Key.String()))
		sceneRow := scriptSceneRow{ID: id, OrgID: s.OrgID, ProjectID: s.ProjectID, StructureID: s.ID, SceneKey: scene.Key, SeqNo: scene.SeqNo, Heading: scene.Heading, Location: scene.LocationText, TimeOfDay: scene.TimeOfDay, Start: scene.Start, End: scene.End}
		if err := exactlyOne(tx.Create(&sceneRow)); err != nil {
			return err
		}
		for i, item := range scene.Items {
			lineID := uuid.NewSHA1(s.ID, []byte("line/"+scene.Key.String()+"/"+item.Key.String()))
			if item.Type == domain.StructureItemTypeDialogue {
				line := scriptDialogueLineRow{ID: lineID, OrgID: s.OrgID, ProjectID: s.ProjectID, SceneID: id, LineKey: item.Key, SeqNo: i + 1, Kind: item.Kind, Content: item.Content, Speaker: item.Speaker, CharacterID: item.CharacterID, CharacterVersionID: item.CharacterVersionID, Emotion: item.Emotion, ContentHash: domain.ContentSHA([]byte(item.Content)), Start: item.Start, End: item.End}
				if err := exactlyOne(tx.Create(&line)); err != nil {
					return err
				}
			} else {
				line := scriptActionLineRow{ID: lineID, OrgID: s.OrgID, ProjectID: s.ProjectID, SceneID: id, LineKey: item.Key, SeqNo: i + 1, Content: item.Content, Start: item.Start, End: item.End}
				if err := exactlyOne(tx.Create(&line)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SaveStructure commits one new immutable manual version without overwriting confirmation.
func (s *SourceStore) SaveStructure(ctx context.Context, actor identityapp.Principal, input application.StructureCommand, now time.Time) (application.StructureReceipt, error) {
	var result application.StructureReceipt
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash("save_structure", fingerprint)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := registerCommand(tx, actor, input.ProjectID, input.Key, "save_structure", hash, now); err != nil {
			return err
		}
		found, err := replayReview(tx, actor, input.ProjectID, input.Key, "save_structure", hash, &result)
		if err != nil || found {
			return err
		}
		state, episode, err := episodeReviewState(tx, actor, input)
		if err != nil {
			return err
		}
		if err := input.Document.Validate(episode.Start, episode.End); err != nil {
			return err
		}
		if err := s.freezeStructureCharacters(ctx, tx, actor, input.ProjectID, &input.Document, false); err != nil {
			return err
		}
		currentVersion := int64(0)
		if episode.CurrentStructureID != nil {
			current, err := readStructure(tx, actor.OrgID, input.ProjectID, *episode.CurrentStructureID)
			if err != nil {
				return err
			}
			currentVersion = current.VersionNo
		}
		if currentVersion != input.BaseStructureVersionNo {
			return application.ErrConflict
		}
		version, err := readVersion(tx, actor.OrgID, input.ProjectID, episode.VersionID)
		if err != nil {
			return err
		}
		id := uuid.NewSHA1(input.Key, []byte("structure/"+actor.ID.String()+"/"+input.ProjectID.String()+"/"+episode.ID.String()))
		structure := domain.EpisodeStructure{ID: id, OrgID: actor.OrgID, ProjectID: input.ProjectID, EpisodeID: episode.ID, VersionNo: currentVersion + 1, SourceHash: version.ContentHash, Document: input.Document, ActorID: actor.ID, CreatedAt: now}
		if err := insertStructure(tx, structure); err != nil {
			return err
		}
		if err := exactlyOne(tx.Model(&scriptEpisodeRow{}).Where("id = ? AND org_id = ? AND project_id = ? AND revision = ?", episode.ID, actor.OrgID, input.ProjectID, episode.Revision).Updates(map[string]any{"current_structure_id": id, "revision": episode.Revision + 1})); err != nil {
			return err
		}
		projectRevision, err := advanceReview(ctx, tx, access, actor, project, state.Revision, now)
		if err != nil {
			return err
		}
		result = application.StructureReceipt{ScriptRevision: state.Revision + 1, ProjectRevision: projectRevision, EpisodeRevision: episode.Revision + 1, StructureID: id, VersionNo: structure.VersionNo, ReviewStatus: domain.StructureReviewStatusCandidate}
		if err := saveReview(tx, actor, input.ProjectID, input.Key, "save_structure", hash, result, now); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.structure_saved", result.ScriptRevision, episode.VersionID, &episode.ID, now)
	})
	return result, err
}

// ConfirmStructure requires current formal identity and no unresolved source lines.
func (s *SourceStore) ConfirmStructure(ctx context.Context, actor identityapp.Principal, input application.StructureCommand, now time.Time) (application.StructureReceipt, error) {
	var result application.StructureReceipt
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash("confirm_structure", fingerprint)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := registerCommand(tx, actor, input.ProjectID, input.Key, "confirm_structure", hash, now); err != nil {
			return err
		}
		found, err := replayReview(tx, actor, input.ProjectID, input.Key, "confirm_structure", hash, &result)
		if err != nil || found {
			return err
		}
		state, episode, err := episodeReviewState(tx, actor, input)
		if err != nil {
			return err
		}
		if episode.CurrentStructureID == nil {
			return domain.ErrInvalidStructure
		}
		structure, err := readStructure(tx, actor.OrgID, input.ProjectID, *episode.CurrentStructureID)
		if err != nil {
			return err
		}
		if structure.EpisodeID != episode.ID || structure.VersionNo != input.BaseStructureVersionNo {
			return application.ErrConflict
		}
		if err := structure.Document.Validate(episode.Start, episode.End); err != nil {
			return err
		}
		if len(structure.Document.Unassigned) != 0 {
			return domain.ErrInvalidStructure
		}
		if err := s.freezeStructureCharacters(ctx, tx, actor, input.ProjectID, &structure.Document, true); err != nil {
			return err
		}
		if episode.ConfirmedStructureID != nil && *episode.ConfirmedStructureID != structure.ID {
			return &application.ImpactError{Affected: []application.AffectedEpisode{{EpisodeID: episode.ID, ConfirmedStructureID: *episode.ConfirmedStructureID, Reason: "structure_replaced"}}, NeedsAck: !input.AckInvalidate}
		}
		if err := exactlyOne(tx.Model(&scriptEpisodeRow{}).Where("id = ? AND org_id = ? AND project_id = ? AND revision = ?", episode.ID, actor.OrgID, input.ProjectID, episode.Revision).Updates(map[string]any{"confirmed_structure_id": structure.ID, "revision": episode.Revision + 1})); err != nil {
			return err
		}
		projectRevision, err := advanceReview(ctx, tx, access, actor, project, state.Revision, now)
		if err != nil {
			return err
		}
		result = application.StructureReceipt{ScriptRevision: state.Revision + 1, ProjectRevision: projectRevision, EpisodeRevision: episode.Revision + 1, StructureID: structure.ID, VersionNo: structure.VersionNo, ReviewStatus: domain.StructureReviewStatusConfirmed}
		if err := saveReview(tx, actor, input.ProjectID, input.Key, "confirm_structure", hash, result, now); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.structure_confirmed", result.ScriptRevision, episode.VersionID, &episode.ID, now)
	})
	return result, err
}

// ReplayStructure returns a current-authorized permanent result without fetching bodies.
func (s *SourceStore) ReplayStructure(ctx context.Context, actor identityapp.Principal, input application.StructureCommand, action string) (*application.StructureReceipt, error) {
	if action != "save_structure" && action != "confirm_structure" {
		return nil, domain.ErrInvalidStructure
	}
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash(action, fingerprint)
	if err != nil {
		return nil, err
	}
	var result application.StructureReceipt
	found := false
	err = s.transaction(ctx, actor, input.ProjectID, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var err error
		found, err = replayReview(tx, actor, input.ProjectID, input.Key, action, hash, &result)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &result, nil
}
