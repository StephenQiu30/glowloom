package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	command "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planning "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	review "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) AuthorizedRun(ctx context.Context, actor app.Actor, id string, write bool) (domain.Run, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Run{}, app.ErrNotFound
	}
	var run domain.Run
	err := s.WithinTransaction(ctx, func(repo app.Repository) error {
		var err error
		run, err = repo.Get(ctx, id)
		if err != nil {
			return err
		}
		_, err = repo.Authorize(ctx, actor, run.Command.ProjectID, write)
		return err
	})
	return run, err
}
func (s *Store) MachineRun(ctx context.Context, id, hash string) (domain.Run, error) {
	var run domain.Run
	err := s.WithinTransaction(ctx, func(repo app.Repository) error {
		var err error
		run, err = repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if run.PayloadHash != hash {
			return app.Problem("command_payload_mismatch", 409)
		}
		_, err = repo.Authorize(ctx, app.Actor{UserID: run.Command.ActorID, TokenVersion: run.TokenVersion}, run.Command.ProjectID, true)
		return err
	})
	return run, err
}
func (s *Store) SaveProposal(ctx context.Context, actor app.Actor, run domain.Run, proposal domain.Proposal, now time.Time) (domain.Proposal, error) {
	var result domain.Proposal
	err := database.WithinTransaction(ctx, s.database, func(tx *gorm.DB) error {
		repo := &repository{database: tx}
		if _, err := repo.Authorize(ctx, actor, run.Command.ProjectID, true); err != nil {
			return err
		}
		var project model.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", run.Command.ProjectID).Error; err != nil {
			return err
		}
		var stored model.CreationProposal
		err := tx.First(&stored, "id = ?", proposal.ID).Error
		if err == nil {
			if stored.RunID.String() != run.Command.RunID || stored.ResultHash != proposal.ResultHash || stored.StepID.String() != proposal.StepID {
				return app.Problem("proposal_revision_conflict", 409)
			}
			result, err = proposalDomain(run, stored)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var revision model.DocumentRevision
		if err = tx.Select("id", "normalized_hash", "normalized_text", "document_id").First(&revision, "id = ?", run.Command.Source.RevisionID).Error; err != nil {
			return err
		}
		var task domain.TextTask
		if json.Unmarshal(proposal.Draft.Task, &task) != nil || revision.NormalizedText != task.Source.Text || revision.NormalizedHash != proposal.SourceHash || revision.DocumentID.String() != run.Command.Source.DocumentID {
			return app.Problem("source_hash_drift", 409)
		}
		reviewService := review.NewService(reviewgorm.New(tx), review.Config{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 5 * time.Minute})
		humanTask, err := reviewService.Open(ctx, review.OpenCommand{WorkspaceID: run.Command.WorkspaceID, ProjectID: run.Command.ProjectID, WorkflowRunID: run.Command.RunID, NodeRunID: proposal.StepID, SubjectType: "creation_text_proposal", SubjectID: proposal.ID, SubjectRevision: proposal.Revision, SubjectHash: proposal.ResultHash, CandidateIDs: []string{}, RubricVersion: "creation-text-review-production", AllowedDecisions: []string{"approved", "rejected", "changes_requested"}})
		if err != nil {
			return err
		}
		raw, err := json.Marshal(proposal.Draft)
		if err != nil {
			return err
		}
		stored = model.CreationProposal{ID: uuid.MustParse(proposal.ID), RunID: uuid.MustParse(run.Command.RunID), StepID: uuid.MustParse(proposal.StepID), Stage: proposal.Stage, ResultHash: proposal.ResultHash, Draft: datatypes.JSON(raw), HumanTaskID: uuid.MustParse(humanTask.ID), ProjectRevision: project.Revision, CreatedAt: now}
		if err = tx.Omit(clause.Associations).Create(&stored).Error; err != nil {
			return err
		}
		result, err = proposalDomain(run, stored)
		return err
	})
	return result, err
}
func (s *Store) Proposals(ctx context.Context, actor app.Actor, runID string) ([]domain.Proposal, error) {
	run, err := s.AuthorizedRun(ctx, actor, runID, false)
	if err != nil {
		return nil, err
	}
	return loadProposals(ctx, s.database, run)
}
func loadProposals(ctx context.Context, tx *gorm.DB, run domain.Run) ([]domain.Proposal, error) {
	var rows []model.CreationProposal
	if err := tx.WithContext(ctx).Where("run_id = ?", run.Command.RunID).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Proposal, 0, len(rows))
	for _, row := range rows {
		p, err := proposalDomain(run, row)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}
func proposalDomain(run domain.Run, row model.CreationProposal) (domain.Proposal, error) {
	var draft domain.DraftEnvelope
	if err := json.Unmarshal(row.Draft, &draft); err != nil {
		return domain.Proposal{}, err
	}
	p, err := domain.ValidateDraft(run, draft)
	if err != nil {
		return p, err
	}
	if p.ID != row.ID.String() || p.StepID != row.StepID.String() || p.Stage != row.Stage || p.ResultHash != row.ResultHash {
		return p, app.Problem("persisted_proposal_drift", 409)
	}
	p.HumanTaskID = row.HumanTaskID.String()
	p.CreatedAt = row.CreatedAt
	if len(row.Acceptance) > 0 {
		if err = json.Unmarshal(row.Acceptance, &p.Acceptance); err != nil {
			return p, err
		}
		if p.Acceptance == nil || p.Acceptance.ProposalID != p.ID || p.Acceptance.RunID != run.Command.RunID || p.Acceptance.ProposalHash != p.ResultHash || p.Acceptance.CandidateHash != p.CandidateHash || p.Acceptance.ProposalRevision != p.Revision || len(p.Acceptance.OwnerReceipts) == 0 || len(p.Acceptance.FormalRefs) == 0 {
			return p, app.Problem("persisted_acceptance_drift", 409)
		}
		p.Status = "accepted"
	}
	return p, nil
}
func (s *Store) AdoptProposal(ctx context.Context, actor app.Actor, runID, proposalID string, input app.AdoptCommand, now time.Time) (domain.AdoptionReceipt, error) {
	var result domain.AdoptionReceipt
	err := database.WithinTransaction(ctx, s.database, func(tx *gorm.DB) error {
		repo := &repository{database: tx}
		run, err := repo.Get(ctx, runID)
		if err != nil {
			return err
		}
		if _, err = repo.Authorize(ctx, actor, run.Command.ProjectID, true); err != nil {
			return err
		}
		// Serialize owner effects for this project, including competing runs.
		var project model.Project
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", run.Command.ProjectID).Error; err != nil {
			return err
		}
		var row model.CreationProposal
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ? AND run_id = ?", proposalID, runID).Error; err != nil {
			return notFound(err)
		}
		proposal, err := proposalDomain(run, row)
		if err != nil {
			return err
		}
		if proposal.Revision != input.ExpectedRevision {
			return app.Problem("proposal_revision_conflict", 409)
		}
		inputHash, err := command.InputHash(struct {
			ActorID    string
			ProposalID string
			Input      app.AdoptCommand
		}{actor.UserID, proposalID, input})
		if err != nil {
			return err
		}
		if proposal.Acceptance != nil {
			if row.AdoptionInputHash != inputHash || row.AdoptionKey != input.IdempotencyKey {
				return app.Problem("idempotency_conflict", 409)
			}
			result = *proposal.Acceptance
			return verifyOwnerReceipts(ctx, tx, result)
		}
		var decision model.ReviewDecision
		if err = tx.First(&decision, "id = ? AND human_task_id = ? AND workspace_id = ?", input.DecisionID, row.HumanTaskID, run.Command.WorkspaceID).Error; err != nil {
			return app.Problem("review_decision_required", 409)
		}
		var humanTask model.HumanTask
		if err = tx.First(&humanTask, "id = ?", row.HumanTaskID).Error; err != nil {
			return err
		}
		if decision.Decision != "approved" || decision.SubjectHash != proposal.ResultHash || decision.SubjectRevision != proposal.Revision || humanTask.Status != "COMPLETED" || humanTask.SubjectID != row.ID || humanTask.SubjectType != "creation_text_proposal" || humanTask.WorkflowRunID.String() != runID || humanTask.NodeRunID != row.StepID {
			return app.Problem("review_binding_mismatch", 409)
		}
		if err = domain.ValidateResolutions(proposal.Issues, input.RiskResolutions); err != nil {
			return app.Problem("unresolved_proposal_blockers", 409)
		}
		all, err := loadProposals(ctx, tx, run)
		if err != nil {
			return err
		}
		for _, previous := range all {
			if previous.Acceptance != nil {
				if err = verifyOwnerReceipts(ctx, tx, *previous.Acceptance); err != nil {
					return err
				}
			}
		}
		mapping, err := acceptedDependencies(proposal, all)
		if err != nil {
			return err
		}
		result = domain.AdoptionReceipt{Schema: "creation-adoption-production", SubmissionID: uuid.NewString(), RunID: runID, ProposalID: proposalID, StepID: proposal.StepID, Gate: proposal.Stage, ProposalRevision: proposal.Revision, ProposalHash: proposal.ResultHash, CandidateHash: proposal.CandidateHash, DecisionID: input.DecisionID, OwnerReceipts: []domain.OwnerReceipt{}, FormalRefs: []domain.ResourceRef{}, IDMapping: map[string]string{}, RiskResolutions: input.RiskResolutions, AcceptedAt: now}
		var task domain.TextTask
		if err = json.Unmarshal(proposal.Draft.Task, &task); err != nil {
			return err
		}
		receiptID := uuid.NewString()
		owner, operation, err := applyTextOwner(ctx, tx, run, proposal, row.ProjectRevision, actor, input, task, mapping, &result, receiptID, now)
		if err != nil {
			return normalizeAdoptionError(err)
		}
		result.OwnerReceipts = append(result.OwnerReceipts, domain.OwnerReceipt{ID: receiptID, Owner: owner, Operation: operation})
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err = commandgorm.Create(ctx, tx, command.Receipt{ID: receiptID, WorkspaceID: run.Command.WorkspaceID, Operation: operation, IdempotencyKey: input.IdempotencyKey, InputHash: inputHash, ResourceID: proposalID, Result: raw, CreatedBy: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		updated := tx.Model(&row).Where("acceptance IS NULL").Updates(map[string]any{"acceptance": datatypes.JSON(raw), "adoption_input_hash": inputHash, "adoption_key": input.IdempotencyKey})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return app.Problem("proposal_revision_conflict", 409)
		}
		return nil
	})
	return result, err
}
func applyTextOwner(ctx context.Context, tx *gorm.DB, run domain.Run, p domain.Proposal, projectRevision int, actor app.Actor, input app.AdoptCommand, task domain.TextTask, mapping map[string]string, result *domain.AdoptionReceipt, receiptID string, now time.Time) (string, string, error) {
	if p.Stage == "map_manuscript" || p.Stage == "analyze_episode" {
		applied, err := planninggorm.AcceptTextPlanning(ctx, tx, planning.TextPlanningInput{SourceReceiptID: receiptID, Stage: p.Stage, WorkspaceID: run.Command.WorkspaceID, ProjectID: run.Command.ProjectID, SourceRevisionID: p.SourceRevisionID, SourceHash: p.SourceHash, SourceText: task.Source.Text, Candidate: p.Candidate, Blocks: domain.SourceBlocks(task.Source.Text), IDMapping: mapping, ExpectedProjectRevision: projectRevision, CreatedBy: actor.UserID, CreatedAt: now, NewID: uuid.NewString})
		if err != nil {
			return "", "", err
		}
		result.IDMapping = applied.IDMapping
		for i, e := range applied.Episodes {
			v := applied.Versions[i]
			result.FormalRefs = append(result.FormalRefs, domain.ResourceRef{Owner: "production/planning", Type: "episode", ID: e.ID, Revision: 1, ContentHash: v.ContentHash}, domain.ResourceRef{Owner: "production/script", Type: "episode_script_version", ID: v.ID, Revision: 1, ContentHash: v.ContentHash})
		}
		if applied.Structure != nil {
			v := applied.Structure
			result.FormalRefs = append(result.FormalRefs, domain.ResourceRef{Owner: "production/planning", Type: "episode_structure", ID: v.ID, Revision: v.Revision, ContentHash: v.ResultHash})
		}
		return "production/planning", "creation.text." + p.Stage, nil
	}
	return applyWorldOrIntent(ctx, tx, run, p, actor, input, mapping, result, now)
}
func acceptedDependencies(p domain.Proposal, all []domain.Proposal) (map[string]string, error) {
	mapping := map[string]string{}
	accepted := map[string]domain.Proposal{}
	for _, item := range all {
		if item.Acceptance != nil {
			accepted[item.Stage+":"+item.CandidateHash] = item
			for key, id := range item.Acceptance.IDMapping {
				if previous, ok := mapping[key]; ok && previous != id {
					return nil, app.Problem("owner_mapping_conflict", 409)
				}
				mapping[key] = id
			}
		}
	}
	var task domain.TextTask
	if json.Unmarshal(p.Draft.Task, &task) != nil {
		return nil, app.Problem("proposal_task_invalid", 409)
	}
	require := func(stage string, raw json.RawMessage) error {
		hash, err := canonical.Hash(raw)
		if err != nil {
			return app.Problem("upstream_proposal_invalid", 409)
		}
		if _, ok := accepted[stage+":"+hash]; !ok {
			return app.Problem("upstream_not_adopted", 409)
		}
		return nil
	}
	if p.Stage != "map_manuscript" {
		if err := require("map_manuscript", task.EpisodeMap); err != nil {
			return nil, err
		}
	}
	if p.Stage == "build_world" || p.Stage == "direct_scene" {
		var episodeMap struct {
			Episodes []struct {
				Key string `json:"key"`
			} `json:"episodes"`
		}
		if json.Unmarshal(task.EpisodeMap, &episodeMap) != nil || len(task.Analyses) == 0 || len(task.Analyses) != len(episodeMap.Episodes) {
			return nil, app.Problem("upstream_not_adopted", 409)
		}
		remaining := map[string]bool{}
		for _, episode := range episodeMap.Episodes {
			if remaining[episode.Key] {
				return nil, app.Problem("upstream_scope_mismatch", 409)
			}
			remaining[episode.Key] = true
		}
		for _, raw := range task.Analyses {
			var analysis struct {
				EpisodeKey string `json:"episode_key"`
			}
			if json.Unmarshal(raw, &analysis) != nil || !remaining[analysis.EpisodeKey] {
				return nil, app.Problem("upstream_scope_mismatch", 409)
			}
			delete(remaining, analysis.EpisodeKey)
			if err := require("analyze_episode", raw); err != nil {
				return nil, err
			}
		}
	}
	if p.Stage == "direct_scene" {
		if err := require("build_world", task.World); err != nil {
			return nil, err
		}
	}
	return mapping, nil
}
func (s *Store) GateState(ctx context.Context, actor app.Actor, runID, gate string, refs []domain.DraftRef) (domain.GateResult, error) {
	result := domain.GateResult{Status: "accepted", Gate: gate, HumanTaskIDs: []string{}, Receipts: []domain.AdoptionReceipt{}, SelectedScenes: []domain.SelectedScene{}}
	proposals, err := s.Proposals(ctx, actor, runID)
	if err != nil {
		return result, err
	}
	byID := map[string]domain.Proposal{}
	for _, p := range proposals {
		if p.Acceptance != nil {
			if err = verifyOwnerReceipts(ctx, s.database, *p.Acceptance); err != nil {
				return result, err
			}
		}
		byID[p.ID] = p
	}
	scopes := map[string]bool{}
	for _, ref := range refs {
		p, ok := byID[ref.DraftID]
		if !ok || p.Stage != gate || p.StepID != ref.StepID || p.ResultHash != ref.ResultHash || p.CandidateHash != ref.CandidateHash {
			return result, app.Problem("gate_proposal_mismatch", 409)
		}
		result.HumanTaskIDs = append(result.HumanTaskIDs, p.HumanTaskID)
		scopes[p.StepKey] = true
		if p.Acceptance == nil {
			if result.Status != "rejected" {
				result.Status = "pending"
			}
			var decision model.ReviewDecision
			e := s.database.WithContext(ctx).First(&decision, "human_task_id = ?", p.HumanTaskID).Error
			if e == nil && (decision.Decision == "rejected" || decision.Decision == "changes_requested") {
				result.Status = "rejected"
			} else if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return result, e
			}
		} else {
			if err = verifyOwnerReceipts(ctx, s.database, *p.Acceptance); err != nil {
				return result, err
			}
			result.Receipts = append(result.Receipts, *p.Acceptance)
		}
	}
	expected := map[string]bool{}
	for _, p := range proposals {
		if p.Stage == "map_manuscript" && p.Acceptance != nil {
			var m struct {
				Episodes []struct {
					Key string `json:"key"`
				} `json:"episodes"`
			}
			if json.Unmarshal(p.Candidate, &m) != nil {
				return result, app.Problem("persisted_proposal_drift", 409)
			}
			for _, ep := range m.Episodes {
				if gate == "analyze_episode" {
					expected["analyze_episode/"+ep.Key] = true
				}
			}
		}
		if p.Stage == "analyze_episode" && p.Acceptance != nil {
			var a struct {
				EpisodeKey string `json:"episode_key"`
				Scenes     []struct {
					Key string `json:"key"`
				} `json:"scenes"`
			}
			if json.Unmarshal(p.Candidate, &a) != nil {
				return result, app.Problem("persisted_proposal_drift", 409)
			}
			for _, scene := range a.Scenes {
				result.SelectedScenes = append(result.SelectedScenes, domain.SelectedScene{EpisodeKey: a.EpisodeKey, SceneKey: scene.Key})
				if gate == "direct_scene" {
					expected["direct_scene/"+a.EpisodeKey+"/"+scene.Key] = true
				}
			}
		}
	}
	if gate == "map_manuscript" || gate == "build_world" {
		expected[gate] = true
	}
	if len(expected) != len(scopes) {
		return result, app.Problem("gate_scope_incomplete", 409)
	}
	for key := range expected {
		if !scopes[key] {
			return result, app.Problem("gate_scope_incomplete", 409)
		}
	}
	sort.Slice(result.SelectedScenes, func(i, j int) bool {
		a, b := result.SelectedScenes[i], result.SelectedScenes[j]
		return a.EpisodeKey+"/"+a.SceneKey < b.EpisodeKey+"/"+b.SceneKey
	})
	return result, nil
}
func verifyOwnerReceipts(ctx context.Context, tx *gorm.DB, result domain.AdoptionReceipt) error {
	for _, owner := range result.OwnerReceipts {
		var row model.CommandReceipt
		if err := tx.WithContext(ctx).First(&row, "id = ? AND operation = ?", owner.ID, owner.Operation).Error; err != nil {
			return app.Problem("owner_receipt_missing", 409)
		}
		var value domain.AdoptionReceipt
		expected, err := json.Marshal(result)
		expectedHash, hashErr := canonical.Hash(expected)
		actualHash, actualErr := canonical.Hash(json.RawMessage(row.Result))
		if err != nil || hashErr != nil || actualErr != nil || actualHash != expectedHash || json.Unmarshal(row.Result, &value) != nil || value.SubmissionID != result.SubmissionID || value.ProposalHash != result.ProposalHash {
			return app.Problem("owner_receipt_drift", 409)
		}
	}
	for _, ref := range result.FormalRefs {
		if err := verifyFormalReference(ctx, tx, result, ref); err != nil {
			return err
		}
	}
	return nil
}

func verifyFormalReference(ctx context.Context, tx *gorm.DB, receipt domain.AdoptionReceipt, ref domain.ResourceRef) error {
	fail := app.Problem("formal_owner_drift", 409)
	query := tx.WithContext(ctx)
	switch ref.Type {
	case "episode":
		var value model.Episode
		if err := query.First(&value, "id = ?", ref.ID).Error; err != nil {
			return ownerReadError(err)
		}
		if ref.Owner != "production/planning" || value.Revision != ref.Revision || value.Status != "active" || value.CurrentScriptVersionID == nil {
			return fail
		}
		var script model.EpisodeScriptVersion
		if err := query.First(&script, "id = ? AND episode_id = ?", value.CurrentScriptVersionID, value.ID).Error; err != nil {
			return ownerReadError(err)
		}
		if script.ContentHash != ref.ContentHash || script.Status != "published" {
			return fail
		}
	case "episode_script_version":
		var value model.EpisodeScriptVersion
		if err := query.First(&value, "id = ?", ref.ID).Error; err != nil {
			return ownerReadError(err)
		}
		if ref.Owner != "production/script" || value.VersionNo != ref.Revision || value.ContentHash != ref.ContentHash || value.Status != "published" || domain.TextHash(value.Content) != value.ContentHash {
			return fail
		}
	case "episode_structure":
		var value model.EpisodeStructure
		if err := query.First(&value, "id = ?", ref.ID).Error; err != nil {
			return ownerReadError(err)
		}
		hash, err := canonical.Hash(json.RawMessage(value.Scenes))
		if err != nil || ref.Owner != "production/planning" || value.Revision != ref.Revision || value.ResultHash != ref.ContentHash || hash != value.ResultHash || value.Status != "confirmed" {
			return fail
		}
	case "text_world_version":
		var value model.TextWorldVersion
		if err := query.First(&value, "id = ? AND run_id = ? AND proposal_id = ?", ref.ID, receipt.RunID, receipt.ProposalID).Error; err != nil {
			return ownerReadError(err)
		}
		if ref.Owner != "production/bible" || value.Revision != ref.Revision || value.ContentHash != ref.ContentHash || !validFormalBody(json.RawMessage(value.Body), ref) {
			return fail
		}
	case "text_intent_version":
		var value model.TextIntentVersion
		if err := query.First(&value, "id = ? AND run_id = ? AND proposal_id = ?", ref.ID, receipt.RunID, receipt.ProposalID).Error; err != nil {
			return ownerReadError(err)
		}
		if ref.Owner != "production/storyboard" || value.Revision != ref.Revision || value.ContentHash != ref.ContentHash || value.AssetReadiness != "needs_asset" || !validFormalBody(json.RawMessage(value.Body), ref) {
			return fail
		}
	default:
		return fail
	}
	return nil
}
func validFormalBody(raw json.RawMessage, ref domain.ResourceRef) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	var id, hash string
	if json.Unmarshal(value["id"], &id) != nil || json.Unmarshal(value["content_hash"], &hash) != nil || id != ref.ID || hash != ref.ContentHash {
		return false
	}
	value["content_hash"] = json.RawMessage(`""`)
	reencoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	actual, err := canonical.Hash(reencoded)
	return err == nil && actual == ref.ContentHash
}
func ownerReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.Problem("formal_owner_missing", 409)
	}
	return err
}
func normalizeAdoptionError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var p *planning.Error
	if errors.As(err, &p) {
		return app.Problem(p.Code, p.Status)
	}
	var problem *app.Error
	if errors.As(err, &problem) {
		return err
	}
	if strings.Contains(err.Error(), "conflict") {
		return app.Problem("owner_revision_conflict", 409)
	}
	return app.Problem("owner_validation_failed", 422)
}
