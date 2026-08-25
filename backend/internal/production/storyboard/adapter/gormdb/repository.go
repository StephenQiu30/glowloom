package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) DraftInput(ctx context.Context, actor application.Actor, episodeID string, write bool) (storyboarddomain.DraftInput, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.DraftInput{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.DraftInput{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, write); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	if episode.CurrentScriptVersionID == nil {
		return storyboarddomain.DraftInput{}, conflict("Episode has no published script version")
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).First(&project, "id = ?", episode.ProjectID).Error; err != nil {
		return storyboarddomain.DraftInput{}, normalizeNotFound(err)
	}
	var structure model.EpisodeStructure
	if err = repo.database.WithContext(ctx).
		Where("episode_id = ? AND script_version_id = ? AND status = ?", episode.ID, *episode.CurrentScriptVersionID, "confirmed").
		Order("created_at DESC").First(&structure).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storyboarddomain.DraftInput{}, conflict("Episode structure must be confirmed before storyboard drafting")
		}
		return storyboarddomain.DraftInput{}, err
	}
	var scenes []domain.Scene
	if err = json.Unmarshal(structure.Scenes, &scenes); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	var bible model.ProductionBible
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND status = ?", episode.ProjectID, "confirmed").Order("confirmed_at DESC").First(&bible).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storyboarddomain.DraftInput{}, conflict("Production bible must be confirmed before storyboard drafting")
		}
		return storyboarddomain.DraftInput{}, err
	}
	var candidate struct {
		WorldEntries []map[string]any `json:"world_entries"`
	}
	if err = json.Unmarshal(bible.Candidate, &candidate); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	units := flattenUnits(scenes)
	if len(units) == 0 {
		return storyboarddomain.DraftInput{}, conflict("Confirmed episode structure contains no narrative units")
	}
	resultHash := ""
	if bible.ResultHash != nil {
		resultHash = *bible.ResultHash
	}
	return storyboarddomain.DraftInput{
		WorkspaceID: bible.WorkspaceID.String(), ProjectID: episode.ProjectID.String(), EpisodeID: episode.ID.String(),
		StructureID: structure.ID.String(), ScriptVersionID: episode.CurrentScriptVersionID.String(),
		BibleID: bible.ID.String(), BibleRevision: bible.Revision, BibleResultHash: resultHash,
		TargetDurationMS: episode.TargetDurationMS, AspectRatio: project.AspectRatio, VisualStyle: project.VisualStyle,
		Units: units, WorldEntries: candidate.WorldEntries,
	}, nil
}

func flattenUnits(scenes []domain.Scene) []storyboarddomain.Unit {
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].Position < scenes[j].Position })
	units := make([]storyboarddomain.Unit, 0)
	position := 1
	for _, scene := range scenes {
		units = append(units, storyboarddomain.Unit{ID: scene.ID, SceneID: scene.ID, Kind: "scene_heading", Text: scene.Heading, Position: position, Required: true})
		position++
		type item struct {
			id, kind, text string
			dialogueID     *string
			start          int
		}
		items := make([]item, 0, len(scene.NarrativeUnits)+len(scene.Dialogues))
		for _, unit := range scene.NarrativeUnits {
			items = append(items, item{id: unit.ID, kind: unit.Kind, text: unit.Text, start: unit.SourceStart})
		}
		for _, dialogue := range scene.Dialogues {
			id := dialogue.ID
			items = append(items, item{id: id, kind: "dialogue", text: dialogue.Speaker + ": " + dialogue.Text, dialogueID: &id, start: dialogue.SourceStart})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].start < items[j].start })
		for _, value := range items {
			units = append(units, storyboarddomain.Unit{ID: value.id, SceneID: scene.ID, Kind: value.kind, Text: value.text, DialogueID: value.dialogueID, Position: position, Required: true})
			position++
		}
	}
	return units
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	id, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", id, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return platformcommand.Receipt{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation, IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(), Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt}, nil
}

func (repo *repository) CreateReceipt(ctx context.Context, value platformcommand.Receipt) error {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(value.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: value.Operation, IdempotencyKey: value.IdempotencyKey, InputHash: value.InputHash, ResourceID: resourceID, Result: datatypes.JSON(value.Result), CreatedBy: createdBy, CreatedAt: value.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict("Idempotency key is already in use")
		}
		return err
	}
	return nil
}

func (repo *repository) CreateWorkflow(ctx context.Context, batch storyboarddomain.Batch, invocation storyboarddomain.Invocation) error {
	batchRecord, err := batchRecord(batch)
	if err != nil {
		return err
	}
	invocationRecord, err := invocationRecord(invocation)
	if err != nil {
		return err
	}
	scope, err := json.Marshal(map[string]any{"workspace_id": batch.WorkspaceID, "project_id": batch.ProjectID, "episode_id": batch.EpisodeID})
	if err != nil {
		return err
	}
	task := model.WorkflowTask{ID: batchRecord.TaskID, WorkspaceID: batchRecord.WorkspaceID, TaskType: "storyboard_draft", RequestType: "storyboard_draft_batch", RequestID: batchRecord.ID, Scope: datatypes.JSON(scope), Status: "queued", ProgressStage: "queued", CancelStatus: "none", Revision: 1, CreatedAt: batch.CreatedAt, UpdatedAt: batch.UpdatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&task).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&batchRecord).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&invocationRecord).Error
}

func (repo *repository) GetBatch(ctx context.Context, actor application.Actor, batchID string, forUpdate bool) (storyboarddomain.Batch, error) {
	id, err := uuid.Parse(batchID)
	if err != nil {
		return storyboarddomain.Batch{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.StoryboardDraftBatch
	if err = query.First(&record).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return storyboarddomain.Batch{}, err
	}
	return batchDomain(record)
}

func (repo *repository) GetLatestBatch(ctx context.Context, actor application.Actor, episodeID string) (storyboarddomain.Batch, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.Batch{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return storyboarddomain.Batch{}, err
	}
	var record model.StoryboardDraftBatch
	if err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("created_at DESC").Order("id DESC").First(&record).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	return batchDomain(record)
}

func (repo *repository) SaveBatch(ctx context.Context, value storyboarddomain.Batch) error {
	record, err := batchRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "candidate": record.Candidate, "decisions": record.Decisions, "error": record.Error, "revision": record.Revision, "approved_by": record.ApprovedBy, "approved_at": record.ApprovedAt, "applied_at": record.AppliedAt, "updated_at": record.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) CreateShots(ctx context.Context, batch storyboarddomain.Batch, shots []storyboarddomain.Shot) error {
	record, err := batchRecord(batch)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.StoryboardShot{}).Where("episode_id = ? AND status = ?", record.EpisodeID, "active").Updates(map[string]any{"status": "archived", "updated_at": record.UpdatedAt}).Error; err != nil {
		return err
	}
	records := make([]model.StoryboardShot, len(shots))
	for index, shot := range shots {
		records[index], err = shotRecord(shot)
		if err != nil {
			return err
		}
	}
	if len(records) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error; err != nil {
			return err
		}
	}
	return repo.database.WithContext(ctx).Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "applied_at": record.AppliedAt, "revision": record.Revision, "updated_at": record.UpdatedAt}).Error
}

func (repo *repository) ListShots(ctx context.Context, actor application.Actor, episodeID string) ([]storyboarddomain.Shot, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return nil, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return nil, err
	}
	var records []model.StoryboardShot
	if err = repo.database.WithContext(ctx).Where("episode_id = ? AND status = ?", id, "active").Order("position").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	values := make([]storyboarddomain.Shot, len(records))
	for index, record := range records {
		values[index], err = shotDomain(record)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (repo *repository) CreateExport(ctx context.Context, value storyboarddomain.Export) error {
	record, err := exportRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) GetExport(ctx context.Context, actor application.Actor, exportID string) (storyboarddomain.Export, error) {
	id, err := uuid.Parse(exportID)
	if err != nil {
		return storyboarddomain.Export{}, application.ErrNotFound
	}
	var record model.StoryboardExport
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return storyboarddomain.Export{}, err
	}
	return exportDomain(record)
}

func (repo *repository) GetLatestExport(ctx context.Context, actor application.Actor, episodeID string) (storyboarddomain.Export, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.Export{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return storyboarddomain.Export{}, err
	}
	var record model.StoryboardExport
	if err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("created_at DESC").Order("id DESC").First(&record).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	return exportDomain(record)
}

func (store *Store) ClaimNext(ctx context.Context, now time.Time) (storyboarddomain.Invocation, bool, error) {
	var result storyboarddomain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND kind = ?", "queued", "storyboard_draft").Order("created_at").First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = transaction.Model(&record).Updates(map[string]any{"status": "running", "attempts": gorm.Expr("attempts + 1"), "started_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err = transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.RequestID).Updates(map[string]any{"status": "running", "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		if err = transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", record.RequestType, record.RequestID).Updates(map[string]any{"status": "running", "progress_stage": "agent_invocation", "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		record.Status, record.StartedAt, record.Attempts = "running", &now, record.Attempts+1
		result = invocationDomain(record)
		found = true
		return nil
	})
	return result, found, err
}

func (store *Store) CompleteInvocation(ctx context.Context, invocationID string, result contract.Result, candidate storyboarddomain.Candidate, now time.Time) error {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.ErrNotFound
	}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if invocation.Status != "running" || invocation.Kind != "storyboard_draft" {
			return fmt.Errorf("storyboard invocation is not running")
		}
		if err := transaction.Model(&invocation).Updates(map[string]any{"status": "succeeded", "result_hash": result.ResultHash, "error": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{"status": "needs_review", "result_hash": result.ResultHash, "candidate": datatypes.JSON(candidateJSON), "error": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", invocation.RequestType, invocation.RequestID).Updates(map[string]any{"status": "succeeded", "progress_stage": "candidate_ready", "error": nil, "next_action": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
}

func (store *Store) FailInvocation(ctx context.Context, invocationID, outcome, code, summary string, retryable bool, now time.Time) error {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "unknown"
	}
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.ErrNotFound
	}
	errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if err := transaction.Model(&invocation).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		nextAction := "retry_agent"
		if !retryable {
			nextAction = "review_input"
		}
		if err := transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return transaction.Model(&model.WorkflowTask{}).Where("request_type = ? AND request_id = ?", invocation.RequestType, invocation.RequestID).Updates(map[string]any{"status": outcome, "progress_stage": "agent_result", "error": datatypes.JSON(errorJSON), "next_action": nextAction, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
}

func authorizeProject(ctx context.Context, database *gorm.DB, actor application.Actor, projectID uuid.UUID, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	var project model.Project
	if err = database.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return application.ErrNotFound
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return application.ErrNotFound
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return application.ErrNotFound
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func batchRecord(value storyboarddomain.Batch) (model.StoryboardDraftBatch, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	structureID, err := uuid.Parse(value.StructureID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	versionID, err := uuid.Parse(value.ScriptVersionID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	taskID, err := uuid.Parse(value.TaskID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	var approvedBy *uuid.UUID
	if value.ApprovedBy != nil {
		parsed, parseErr := uuid.Parse(*value.ApprovedBy)
		if parseErr != nil {
			return model.StoryboardDraftBatch{}, parseErr
		}
		approvedBy = &parsed
	}
	candidate, err := json.Marshal(value.Candidate)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	decisions, err := json.Marshal(value.Decisions)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	return model.StoryboardDraftBatch{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, StructureID: structureID, ScriptVersionID: versionID, TaskID: taskID, Status: value.Status, InputHash: value.InputHash, ResultHash: value.ResultHash, Candidate: datatypes.JSON(candidate), Decisions: datatypes.JSON(decisions), Error: datatypes.JSON(value.Error), Revision: value.Revision, ApprovedBy: approvedBy, ApprovedAt: value.ApprovedAt, AppliedAt: value.AppliedAt, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func batchDomain(record model.StoryboardDraftBatch) (storyboarddomain.Batch, error) {
	candidate := storyboarddomain.Candidate{Shots: []storyboarddomain.DraftShot{}}
	if len(record.Candidate) > 0 {
		if err := json.Unmarshal(record.Candidate, &candidate); err != nil {
			return storyboarddomain.Batch{}, err
		}
	}
	decisions := map[string]string{}
	if len(record.Decisions) > 0 {
		if err := json.Unmarshal(record.Decisions, &decisions); err != nil {
			return storyboarddomain.Batch{}, err
		}
	}
	var approvedBy *string
	if record.ApprovedBy != nil {
		value := record.ApprovedBy.String()
		approvedBy = &value
	}
	return storyboarddomain.Batch{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), EpisodeID: record.EpisodeID.String(), StructureID: record.StructureID.String(), ScriptVersionID: record.ScriptVersionID.String(), TaskID: record.TaskID.String(), Status: record.Status, InputHash: record.InputHash, ResultHash: record.ResultHash, Candidate: candidate, Decisions: decisions, Error: append([]byte(nil), record.Error...), Revision: record.Revision, ApprovedBy: approvedBy, ApprovedAt: record.ApprovedAt, AppliedAt: record.AppliedAt, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func shotRecord(value storyboarddomain.Shot) (model.StoryboardShot, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	batchID, err := uuid.Parse(value.BatchID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	units, err := json.Marshal(value.NarrativeUnitIDs)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	spec, err := json.Marshal(value.Spec)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	return model.StoryboardShot{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, BatchID: batchID, ProposalKey: value.ProposalKey, Position: value.Position, Title: value.Title, NarrativeUnitIDs: datatypes.JSON(units), Spec: datatypes.JSON(spec), ContentHash: value.ContentHash, Status: value.Status, Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func shotDomain(record model.StoryboardShot) (storyboarddomain.Shot, error) {
	var units []string
	if err := json.Unmarshal(record.NarrativeUnitIDs, &units); err != nil {
		return storyboarddomain.Shot{}, err
	}
	var spec map[string]any
	if err := json.Unmarshal(record.Spec, &spec); err != nil {
		return storyboarddomain.Shot{}, err
	}
	return storyboarddomain.Shot{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), EpisodeID: record.EpisodeID.String(), BatchID: record.BatchID.String(), ProposalKey: record.ProposalKey, Title: record.Title, Position: record.Position, Revision: record.Revision, NarrativeUnitIDs: units, Spec: spec, ContentHash: record.ContentHash, Status: record.Status, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func exportRecord(value storyboarddomain.Export) (model.StoryboardExport, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	manifest, err := json.Marshal(value.Manifest)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	files, err := json.Marshal(value.Files)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	return model.StoryboardExport{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, Status: value.Status, InputHash: value.InputHash, ContentHash: value.ContentHash, Manifest: datatypes.JSON(manifest), Files: datatypes.JSON(files), Package: append([]byte(nil), value.Package...), Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func exportDomain(record model.StoryboardExport) (storyboarddomain.Export, error) {
	var manifest map[string]any
	if err := json.Unmarshal(record.Manifest, &manifest); err != nil {
		return storyboarddomain.Export{}, err
	}
	var files []storyboarddomain.ExportFile
	if err := json.Unmarshal(record.Files, &files); err != nil {
		return storyboarddomain.Export{}, err
	}
	return storyboarddomain.Export{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), EpisodeID: record.EpisodeID.String(), Status: record.Status, InputHash: record.InputHash, ContentHash: record.ContentHash, Manifest: manifest, Files: files, Package: append([]byte(nil), record.Package...), Revision: record.Revision, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func invocationRecord(value storyboarddomain.Invocation) (model.AgentInvocation, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	requestID, err := uuid.Parse(value.RequestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	return model.AgentInvocation{ID: id, WorkspaceID: workspaceID, RequestType: "storyboard_draft_batch", RequestID: requestID, Kind: value.Kind, InputHash: value.InputHash, Payload: datatypes.JSON(value.Payload), Status: value.Status, Attempts: 0, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt}, nil
}

func invocationDomain(record model.AgentInvocation) storyboarddomain.Invocation {
	return storyboarddomain.Invocation{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), RequestID: record.RequestID.String(), Kind: record.Kind, InputHash: record.InputHash, Payload: append([]byte(nil), record.Payload...), Status: record.Status, CreatedAt: record.CreatedAt}
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func conflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}
