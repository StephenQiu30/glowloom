package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Episodes retains proposed and formal states separately in authorized project scope.
func (s *SourceStore) Episodes(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID) (application.EpisodeView, error) {
	var result application.EpisodeView
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		head, err := readVersionHead(tx, actor.OrgID, project, version, false)
		if err != nil {
			return err
		}
		candidate, err := readSplitSet(tx, actor.OrgID, project, head.CandidateSetID)
		if err != nil {
			return err
		}
		episodes, err := readEpisodes(tx, actor.OrgID, project, version, false)
		if err != nil {
			return err
		}
		result = application.EpisodeView{VersionID: version, Head: head, Candidate: candidate, Episodes: episodes}
		return nil
	})
	return result, err
}

// Episode resolves only own organization metadata before current project authorization.
func (s *SourceStore) Episode(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.Episode, error) {
	if s == nil || s.db == nil {
		return domain.Episode{}, application.ErrUnavailable
	}
	var row struct{ ProjectID uuid.UUID }
	read := s.db.WithContext(ctx).Raw(`SELECT project_id FROM script.episode WHERE org_id=? AND id=?`, actor.OrgID, id).Scan(&row)
	if read.Error != nil {
		return domain.Episode{}, read.Error
	}
	if read.RowsAffected != 1 {
		return domain.Episode{}, application.ErrNotFound
	}
	var result domain.Episode
	err := s.transaction(ctx, actor, row.ProjectID, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var err error
		result, err = readEpisode(tx, actor.OrgID, id, false)
		return err
	})
	return result, err
}

func reviewHash(action string, input any) (string, error) {
	data, err := json.Marshal(struct {
		Action string `json:"action"`
		Input  any    `json:"input"`
	}{action, input})
	if err != nil {
		return "", err
	}
	return domain.ContentSHA(data), nil
}
func replayReview(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string, out any) (bool, error) {
	if err := checkRequestScope(tx, actor, project, key, action, hash); err != nil {
		return false, err
	}
	var row struct {
		OrgID, ProjectID              uuid.UUID
		Action, RequestHash, Response string
	}
	read := tx.Raw(`SELECT org_id,project_id,action,request_hash,response::text FROM script.review_command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return false, read.Error
	}
	if read.RowsAffected == 0 {
		return false, nil
	}
	if row.OrgID != actor.OrgID || row.ProjectID != project || row.Action != action || row.RequestHash != hash {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal([]byte(row.Response), out); err != nil {
		return false, application.ErrUnavailable
	}
	return true, nil
}
func saveReview(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string, result any, now time.Time) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := registerRequest(tx, actor, project, key, action, hash, now); err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO script.review_command(actor_id,request_id,org_id,project_id,action,request_hash,response,created_at) VALUES(?,?,?,?,?,?,?::jsonb,?)`, actor.ID, key, actor.OrgID, project, action, hash, string(data), now))
}

func rejectPendingWrite(tx *gorm.DB, org, project uuid.UUID) error {
	if err := rejectPendingSourceExcept(tx, org, project, uuid.Nil); err != nil {
		return err
	}
	return rejectPendingImport(tx, org, project, uuid.Nil)
}

func reviewState(tx *gorm.DB, actor identityapp.Principal, project, version uuid.UUID, revision int64) (domain.ProjectState, error) {
	state, err := readState(tx, actor.OrgID, project, true)
	if err != nil {
		return state, err
	}
	if state.Revision != revision || state.DraftVersionID == nil || *state.DraftVersionID != version {
		return state, application.ErrConflict
	}
	if err := rejectPendingWrite(tx, actor.OrgID, project); err != nil {
		return state, err
	}
	return state, nil
}
func advanceReview(ctx context.Context, tx *gorm.DB, access application.ProjectAccess, actor identityapp.Principal, project workspaceapp.ProjectContentAccess, scriptRevision int64, now time.Time) (int64, error) {
	updated := tx.Exec(`UPDATE script.project_state SET revision=revision+1,updated_at=? WHERE org_id=? AND project_id=? AND revision=?`, now, actor.OrgID, project.ProjectID, scriptRevision)
	if updated.Error != nil {
		return 0, updated.Error
	}
	if updated.RowsAffected != 1 {
		return 0, application.ErrConflict
	}
	return access.TouchContent(ctx, actor, project.ProjectID, project.Revision)
}

// Resplit saves actual rules output as another immutable candidate set.
func (s *SourceStore) Resplit(ctx context.Context, actor identityapp.Principal, input application.SplitCommand, split domain.SplitResult, now time.Time) (application.SplitReceipt, error) {
	var result application.SplitReceipt
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash("resplit", fingerprint)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := commandLock(tx, actor.ID, input.Key); err != nil {
			return err
		}
		found, err := replayReview(tx, actor, input.ProjectID, input.Key, "resplit", hash, &result)
		if err != nil || found {
			return err
		}
		state, err := reviewState(tx, actor, input.ProjectID, input.VersionID, input.ExpectedRevision)
		if err != nil {
			return err
		}
		head, err := readVersionHead(tx, actor.OrgID, input.ProjectID, input.VersionID, true)
		if err != nil {
			return err
		}
		if head.SplitRevision != input.ExpectedSplitRevision || head.CandidateSetID != input.CandidateSetID {
			return application.ErrConflict
		}
		v, err := readVersion(tx, actor.OrgID, input.ProjectID, input.VersionID)
		if err != nil {
			return err
		}
		if v.CharCount > 0 && len(split.Episodes) > 0 {
			if err := domain.ValidateBoundaries(split.Episodes, split.Preface, v.CharCount); err != nil {
				return err
			}
		}
		set := domain.SplitSet{ID: uuid.NewSHA1(input.Key, []byte("rules/"+actor.ID.String()+"/"+input.ProjectID.String())), VersionID: input.VersionID, OrgID: actor.OrgID, ProjectID: input.ProjectID, Kind: "candidate", Origin: "rules", Preface: split.Preface, Boundaries: split.Episodes, Warnings: split.Warnings, CreatedAt: now}
		if err := insertSplitSet(tx, set); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`UPDATE script.version_head SET split_revision=split_revision+1,candidate_split_set_id=? WHERE version_id=? AND org_id=? AND project_id=?`, set.ID, input.VersionID, actor.OrgID, input.ProjectID)); err != nil {
			return err
		}
		revision, err := advanceReview(ctx, tx, access, actor, project, state.Revision, now)
		if err != nil {
			return err
		}
		result = application.SplitReceipt{ScriptRevision: state.Revision + 1, ProjectRevision: revision, SplitRevision: head.SplitRevision + 1, SplitSetID: set.ID, Episodes: []domain.Episode{}, Mappings: []application.EpisodeMapping{}, RenamedIDs: []uuid.UUID{}}
		if err := saveReview(tx, actor, input.ProjectID, input.Key, "resplit", hash, result, now); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.rules_split_saved", result.ScriptRevision, input.VersionID, nil, now)
	})
	return result, err
}

// ConfirmSplit freezes every current boundary and formal ID, keeping unchanged episodes.
func (s *SourceStore) ConfirmSplit(ctx context.Context, actor identityapp.Principal, input application.SplitCommand, now time.Time) (application.SplitReceipt, error) {
	var result application.SplitReceipt
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash("confirm_split", fingerprint)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := commandLock(tx, actor.ID, input.Key); err != nil {
			return err
		}
		found, err := replayReview(tx, actor, input.ProjectID, input.Key, "confirm_split", hash, &result)
		if err != nil || found {
			return err
		}
		state, err := reviewState(tx, actor, input.ProjectID, input.VersionID, input.ExpectedRevision)
		if err != nil {
			return err
		}
		head, err := readVersionHead(tx, actor.OrgID, input.ProjectID, input.VersionID, true)
		if err != nil {
			return err
		}
		if head.SplitRevision != input.ExpectedSplitRevision || head.CandidateSetID != input.CandidateSetID {
			return application.ErrConflict
		}
		version, err := readVersion(tx, actor.OrgID, input.ProjectID, input.VersionID)
		if err != nil {
			return err
		}
		if err := domain.ValidateBoundaries(input.Boundaries, input.Preface, version.CharCount); err != nil {
			return err
		}
		sources, err := readSources(tx, actor.OrgID, input.ProjectID, version.SourceIDs)
		if err != nil {
			return err
		}
		for _, boundary := range input.Boundaries {
			if boundary.SourceLineageID != nil && !slices.ContainsFunc(sources, func(r domain.SourceRecord) bool { return r.LineageID == *boundary.SourceLineageID }) {
				return domain.ErrInvalidSpan
			}
		}
		previous, err := readEpisodes(tx, actor.OrgID, input.ProjectID, input.VersionID, true)
		if err != nil {
			return err
		}
		changed := make([]domain.Episode, 0)
		for _, episode := range previous {
			if !slices.ContainsFunc(input.Boundaries, func(b domain.EpisodeBoundary) bool {
				return b.SeqNo == episode.SeqNo && b.Start == episode.Start && b.End == episode.End
			}) {
				changed = append(changed, episode)
			}
		}
		affected := make([]application.AffectedEpisode, 0)
		for _, episode := range changed {
			if episode.ConfirmedStructureID != nil {
				affected = append(affected, application.AffectedEpisode{EpisodeID: episode.ID, ConfirmedStructureID: *episode.ConfirmedStructureID, Reason: "boundary_changed"})
			}
		}
		if len(affected) > 0 {
			return &application.ImpactError{Affected: affected, NeedsAck: !input.AckInvalidate}
		}
		setID := head.ConfirmedSetID
		if setID == nil {
			id := uuid.NewSHA1(input.Key, []byte("formal/"+actor.ID.String()+"/"+input.ProjectID.String()))
			setID = &id
			set := domain.SplitSet{ID: id, VersionID: input.VersionID, OrgID: actor.OrgID, ProjectID: input.ProjectID, Kind: "formal", Origin: "manual", Preface: input.Preface, Boundaries: input.Boundaries, CreatedAt: now}
			if err := insertSplitSet(tx, set); err != nil {
				return err
			}
		}
		// Release the unique active sequence before inserting any changed head.
		for _, episode := range changed {
			if err := exactlyOne(tx.Exec(`UPDATE script.episode SET is_delete=true,revision=revision+1 WHERE id=? AND org_id=? AND project_id=?`, episode.ID, actor.OrgID, input.ProjectID)); err != nil {
				return err
			}
		}
		result = application.SplitReceipt{SplitSetID: *setID, SplitRevision: head.SplitRevision + 1, Episodes: make([]domain.Episode, 0, len(input.Boundaries)), Mappings: make([]application.EpisodeMapping, 0), RenamedIDs: make([]uuid.UUID, 0)}
		for _, boundary := range input.Boundaries {
			index := slices.IndexFunc(previous, func(e domain.Episode) bool {
				return e.SeqNo == boundary.SeqNo && e.Start == boundary.Start && e.End == boundary.End
			})
			if index >= 0 {
				episode := previous[index]
				if episode.Title != boundary.Title {
					episode.Title = boundary.Title
					episode.Revision++
					if err := exactlyOne(tx.Exec(`UPDATE script.episode SET title=?,revision=revision+1 WHERE id=? AND org_id=? AND project_id=?`, episode.Title, episode.ID, actor.OrgID, input.ProjectID)); err != nil {
						return err
					}
					result.RenamedIDs = append(result.RenamedIDs, episode.ID)
				}
				result.Episodes = append(result.Episodes, episode)
				result.Mappings = append(result.Mappings, application.EpisodeMapping{PreviousID: &episode.ID, EpisodeID: &episode.ID, InheritStatus: "retained"})
				continue
			}
			id := uuid.NewSHA1(input.Key, []byte("episode/"+actor.ID.String()+"/"+input.ProjectID.String()+fmt.Sprint(boundary.SeqNo)))
			episode := domain.Episode{ID: id, OrgID: actor.OrgID, ProjectID: input.ProjectID, VersionID: input.VersionID, SplitSetID: *setID, SeqNo: boundary.SeqNo, Title: boundary.Title, Start: boundary.Start, End: boundary.End, Revision: 1, InheritStatus: "not_inherited"}
			if err := insertEpisode(tx, episode); err != nil {
				return err
			}
			result.Episodes = append(result.Episodes, episode)
			result.Mappings = append(result.Mappings, application.EpisodeMapping{EpisodeID: &id, InheritStatus: "not_inherited"})
		}
		for _, episode := range changed {
			result.Mappings = append(result.Mappings, application.EpisodeMapping{PreviousID: &episode.ID, InheritStatus: "invalidated"})
		}
		confirmationID := uuid.NewSHA1(input.Key, []byte("confirmation/"+actor.ID.String()+"/"+input.ProjectID.String()))
		result.ConfirmationID = &confirmationID
		preface, err := json.Marshal(input.Preface)
		if err != nil {
			return err
		}
		episodes, err := json.Marshal(result.Episodes)
		if err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.split_confirmation(id,org_id,project_id,version_id,candidate_set_id,formal_set_id,actor_id,revision,preface,episodes,created_at) VALUES(?,?,?,?,?,?,?,?,?::jsonb,?::jsonb,?)`, confirmationID, actor.OrgID, input.ProjectID, input.VersionID, head.CandidateSetID, *setID, actor.ID, result.SplitRevision, string(preface), string(episodes), now)); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`UPDATE script.version_head SET split_revision=split_revision+1,confirmed_split_set_id=coalesce(confirmed_split_set_id,?) WHERE version_id=? AND org_id=? AND project_id=?`, *setID, input.VersionID, actor.OrgID, input.ProjectID)); err != nil {
			return err
		}
		projectRevision, err := advanceReview(ctx, tx, access, actor, project, state.Revision, now)
		if err != nil {
			return err
		}
		result.ScriptRevision, result.ProjectRevision = state.Revision+1, projectRevision
		if err := saveReview(tx, actor, input.ProjectID, input.Key, "confirm_split", hash, result, now); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.split_confirmed", result.ScriptRevision, input.VersionID, nil, now)
	})
	return result, err
}

// ReplaySplit returns the first result with current read authority before any body I/O.
func (s *SourceStore) ReplaySplit(ctx context.Context, actor identityapp.Principal, input application.SplitCommand, action string) (*application.SplitReceipt, error) {
	if action != "resplit" && action != "confirm_split" {
		return nil, domain.ErrInvalidSpan
	}
	fingerprint := input
	fingerprint.RequestID = uuid.Nil
	hash, err := reviewHash(action, fingerprint)
	if err != nil {
		return nil, err
	}
	var result application.SplitReceipt
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
