package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformcommandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/review/application"
	"github.com/StephenQiu30/lanverse/backend/internal/review/domain"
)

const (
	claimOperation   = "review.human_task.claim"
	renewOperation   = "review.human_task.renew"
	releaseOperation = "review.human_task.release"
	decideOperation  = "review.human_task.decide"
)

type Store struct {
	database *gorm.DB
}

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) ListTasks(
	ctx context.Context,
	actor application.Actor,
	query application.ListTasksQuery,
	now time.Time,
) (domain.HumanTaskPage, error) {
	projectID, err := uuid.Parse(query.ProjectID)
	if err != nil {
		return domain.HumanTaskPage{}, application.ErrNotFound
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return domain.HumanTaskPage{}, application.ErrNotFound
	}
	var page domain.HumanTaskPage
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var project model.Project
		if loadErr := transaction.First(&project, "id = ?", projectID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, project.WorkspaceID, actorID, actor.TokenVersion, false); authorizeErr != nil {
			return authorizeErr
		}
		queryBuilder := transaction.WithContext(ctx).Where("project_id = ?", projectID)
		switch query.Status {
		case "active":
			queryBuilder = queryBuilder.Where("status IN ?", []string{"OPEN", "CLAIMED"})
		case "OPEN", "CLAIMED", "COMPLETED", "CANCELLED", "STALE":
			queryBuilder = queryBuilder.Where("status = ?", query.Status)
		}
		if query.SubjectType != "" {
			queryBuilder = queryBuilder.Where("subject_type = ?", query.SubjectType)
		}
		if query.After != "" {
			afterID, parseErr := uuid.Parse(query.After)
			if parseErr != nil {
				return parseErr
			}
			var cursor model.HumanTask
			if loadErr := transaction.Where("id = ? AND project_id = ?", afterID, projectID).First(&cursor).Error; loadErr != nil {
				return normalizeNotFound(loadErr)
			}
			queryBuilder = queryBuilder.Where(
				"created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID,
			)
		}
		var records []model.HumanTask
		if loadErr := queryBuilder.Order("created_at DESC").Order("id DESC").Limit(query.Limit + 1).Find(&records).Error; loadErr != nil {
			return loadErr
		}
		hasMore := len(records) > query.Limit
		if hasMore {
			records = records[:query.Limit]
		}
		page.Tasks = make([]domain.HumanTask, len(records))
		for index, record := range records {
			mapped, mapErr := taskDomain(record)
			if mapErr != nil {
				return mapErr
			}
			mapped.ClaimToken = nil
			page.Tasks[index] = mapped
		}
		if hasMore && len(page.Tasks) != 0 {
			cursor := page.Tasks[len(page.Tasks)-1].ID
			page.NextAfter = &cursor
		}
		return nil
	})
	return page, err
}

func (store *Store) GetTask(
	ctx context.Context,
	actor application.Actor,
	taskID string,
	now time.Time,
) (domain.HumanTaskDetail, error) {
	id, err := uuid.Parse(taskID)
	if err != nil {
		return domain.HumanTaskDetail{}, application.ErrNotFound
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return domain.HumanTaskDetail{}, application.ErrNotFound
	}
	var detail domain.HumanTaskDetail
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.HumanTask
		if loadErr := transaction.First(&record, "id = ?", id).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, record.WorkspaceID, actorID, actor.TokenVersion, false); authorizeErr != nil {
			return authorizeErr
		}
		mapped, mapErr := taskDomain(record)
		if mapErr != nil {
			return mapErr
		}
		if record.Status != "CLAIMED" || record.ClaimedBy == nil || *record.ClaimedBy != actorID ||
			record.ClaimExpiresAt == nil || !record.ClaimExpiresAt.After(now) {
			mapped.ClaimToken = nil
		}
		detail.Task = mapped
		var decision model.ReviewDecision
		loadErr := transaction.Where("human_task_id = ?", record.ID).First(&decision).Error
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		mappedDecision := decisionDomain(decision)
		detail.Decision = &mappedDecision
		return nil
	})
	return detail, err
}

func (store *Store) GetDecision(ctx context.Context, actor application.Actor, decisionID string) (domain.DecisionResult, error) {
	decisionUUID, err := uuid.Parse(decisionID)
	if err != nil {
		return domain.DecisionResult{}, application.ErrNotFound
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return domain.DecisionResult{}, application.ErrNotFound
	}
	var result domain.DecisionResult
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var decision model.ReviewDecision
		if loadErr := transaction.WithContext(ctx).First(&decision, "id = ?", decisionUUID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var task model.HumanTask
		if loadErr := transaction.WithContext(ctx).First(&task, "id = ?", decision.HumanTaskID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, task.WorkspaceID, actorID, actor.TokenVersion, false); authorizeErr != nil {
			return authorizeErr
		}
		persistedTask, mapErr := taskDomain(task)
		if mapErr != nil {
			return mapErr
		}
		persistedDecision := decisionDomain(decision)
		if validationErr := validateDecisionResult(persistedTask, persistedDecision); validationErr != nil {
			return validationErr
		}
		result = domain.DecisionResult{Task: persistedTask, Decision: persistedDecision}
		return nil
	})
	return result, err
}

func (store *Store) FindTaskByNode(ctx context.Context, workspaceID, nodeRunID string) (domain.HumanTask, error) {
	workspace, node, err := taskScope(workspaceID, nodeRunID)
	if err != nil {
		return domain.HumanTask{}, application.ErrNotFound
	}
	var record model.HumanTask
	if err = store.database.WithContext(ctx).Where("workspace_id = ? AND node_run_id = ?", workspace, node).First(&record).Error; err != nil {
		return domain.HumanTask{}, normalizeNotFound(err)
	}
	return taskDomain(record)
}

func (store *Store) EnsureTask(ctx context.Context, desired domain.HumanTask) (domain.HumanTask, error) {
	record, err := taskRecord(desired)
	if err != nil {
		return domain.HumanTask{}, err
	}
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}, {Name: "node_run_id"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&record).Error; createErr != nil {
			return createErr
		}
		return transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND node_run_id = ?", record.WorkspaceID, record.NodeRunID).First(&record).Error
	})
	if err != nil {
		return domain.HumanTask{}, err
	}
	persisted, err := taskDomain(record)
	if err != nil {
		return domain.HumanTask{}, err
	}
	if !domain.SameTaskBinding(persisted, desired) {
		return domain.HumanTask{}, errors.New("persisted human task binding has drifted")
	}
	return persisted, nil
}

func (store *Store) Claim(
	ctx context.Context,
	actor application.Actor,
	command application.ClaimCommand,
	claimToken string,
	expiresAt time.Time,
	now time.Time,
) (domain.ClaimResult, error) {
	taskID, token, actorID, err := claimIdentities(command.TaskID, claimToken, actor.UserID)
	if err != nil {
		return domain.ClaimResult{}, application.ErrNotFound
	}
	var result domain.ClaimResult
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var task model.HumanTask
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id = ?", taskID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, task.WorkspaceID, actorID, actor.TokenVersion, true); authorizeErr != nil {
			return authorizeErr
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID string
			Command application.ClaimCommand
		}{ActorID: actor.UserID, Command: command})
		if hashErr != nil {
			return hashErr
		}
		if receipt, receiptErr := platformcommandgorm.Find(ctx, transaction, task.WorkspaceID.String(), claimOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[domain.ClaimResult](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different claim input")
			}
			result = replayed
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if task.Revision != command.ExpectedRevision || task.Status == "COMPLETED" || task.Status == "CANCELLED" ||
			(task.Status == "CLAIMED" && task.ClaimExpiresAt != nil && task.ClaimExpiresAt.After(now)) {
			return conflict("Human task changed before claim")
		}
		task.Status, task.ClaimedBy, task.ClaimToken, task.ClaimExpiresAt = "CLAIMED", &actorID, &token, &expiresAt
		task.Revision++
		task.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.HumanTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status": task.Status, "claimed_by": actorID, "claim_token": token, "claim_expires_at": expiresAt,
			"revision": task.Revision, "updated_at": task.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		persisted, mapErr := taskDomain(task)
		if mapErr != nil {
			return mapErr
		}
		result = domain.ClaimResult{Task: persisted, ClaimToken: claimToken}
		return storeResultReceipt(ctx, transaction, task.WorkspaceID, claimOperation, command.IdempotencyKey, inputHash, task.ID, actorID, result, now)
	})
	return result, err
}

func (store *Store) Renew(
	ctx context.Context,
	actor application.Actor,
	command application.RenewCommand,
	expiresAt time.Time,
	now time.Time,
) (domain.ClaimResult, error) {
	taskID, token, actorID, err := claimIdentities(command.TaskID, command.ClaimToken, actor.UserID)
	if err != nil {
		return domain.ClaimResult{}, application.ErrNotFound
	}
	var result domain.ClaimResult
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var task model.HumanTask
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id = ?", taskID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, task.WorkspaceID, actorID, actor.TokenVersion, true); authorizeErr != nil {
			return authorizeErr
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID string
			Command application.RenewCommand
		}{ActorID: actor.UserID, Command: command})
		if hashErr != nil {
			return hashErr
		}
		if receipt, receiptErr := platformcommandgorm.Find(ctx, transaction, task.WorkspaceID.String(), renewOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[domain.ClaimResult](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different renewal input")
			}
			result = replayed
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if task.Revision != command.ExpectedRevision || task.Status != "CLAIMED" || task.ClaimedBy == nil ||
			*task.ClaimedBy != actorID || task.ClaimToken == nil || *task.ClaimToken != token ||
			task.ClaimExpiresAt == nil || !task.ClaimExpiresAt.After(now) {
			return conflict("Human task claim changed before renewal")
		}
		task.ClaimExpiresAt = &expiresAt
		task.Revision++
		task.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.HumanTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"claim_expires_at": expiresAt, "revision": task.Revision, "updated_at": task.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		persisted, mapErr := taskDomain(task)
		if mapErr != nil {
			return mapErr
		}
		result = domain.ClaimResult{Task: persisted, ClaimToken: command.ClaimToken}
		return storeResultReceipt(
			ctx, transaction, task.WorkspaceID, renewOperation, command.IdempotencyKey,
			inputHash, task.ID, actorID, result, now,
		)
	})
	return result, err
}

func (store *Store) Release(
	ctx context.Context,
	actor application.Actor,
	command application.ReleaseCommand,
	now time.Time,
) (domain.HumanTask, error) {
	taskID, token, actorID, err := claimIdentities(command.TaskID, command.ClaimToken, actor.UserID)
	if err != nil {
		return domain.HumanTask{}, application.ErrNotFound
	}
	var result domain.HumanTask
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var task model.HumanTask
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id = ?", taskID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, task.WorkspaceID, actorID, actor.TokenVersion, true); authorizeErr != nil {
			return authorizeErr
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID string
			Command application.ReleaseCommand
		}{ActorID: actor.UserID, Command: command})
		if hashErr != nil {
			return hashErr
		}
		if receipt, receiptErr := platformcommandgorm.Find(ctx, transaction, task.WorkspaceID.String(), releaseOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[domain.HumanTask](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different release input")
			}
			result = replayed
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if task.Revision != command.ExpectedRevision || task.Status != "CLAIMED" || task.ClaimedBy == nil ||
			*task.ClaimedBy != actorID || task.ClaimToken == nil || *task.ClaimToken != token ||
			task.ClaimExpiresAt == nil || !task.ClaimExpiresAt.After(now) {
			return conflict("Human task claim changed before release")
		}
		task.Status, task.ClaimedBy, task.ClaimToken, task.ClaimExpiresAt = "OPEN", nil, nil, nil
		task.Revision++
		task.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.HumanTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status": task.Status, "claimed_by": nil, "claim_token": nil, "claim_expires_at": nil,
			"revision": task.Revision, "updated_at": task.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		persisted, mapErr := taskDomain(task)
		if mapErr != nil {
			return mapErr
		}
		result = persisted
		return storeResultReceipt(
			ctx, transaction, task.WorkspaceID, releaseOperation, command.IdempotencyKey,
			inputHash, task.ID, actorID, result, now,
		)
	})
	return result, err
}

func (store *Store) ExpireClaims(ctx context.Context, limit int, now time.Time) (int, error) {
	expired := 0
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var tasks []model.HumanTask
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND claim_expires_at IS NOT NULL AND claim_expires_at <= ?", "CLAIMED", now.UTC()).
			Order("claim_expires_at ASC").Order("id ASC").Limit(limit).Find(&tasks).Error; loadErr != nil {
			return loadErr
		}
		for _, task := range tasks {
			result := transaction.Model(&model.HumanTask{}).
				Where("id = ? AND revision = ?", task.ID, task.Revision).
				Updates(map[string]any{
					"status": "OPEN", "claimed_by": nil, "claim_token": nil, "claim_expires_at": nil,
					"revision": task.Revision + 1, "updated_at": now.UTC(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return conflict("Human task claim changed during expiry sweep")
			}
			expired++
		}
		return nil
	})
	return expired, err
}

func (store *Store) Decide(
	ctx context.Context,
	actor application.Actor,
	command application.DecideCommand,
	desired domain.ReviewDecision,
	now time.Time,
) (domain.DecisionResult, error) {
	taskID, token, actorID, err := claimIdentities(command.TaskID, command.ClaimToken, actor.UserID)
	if err != nil {
		return domain.DecisionResult{}, application.ErrNotFound
	}
	decisionID, err := uuid.Parse(desired.ID)
	if err != nil {
		return domain.DecisionResult{}, err
	}
	var result domain.DecisionResult
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var task model.HumanTask
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id = ?", taskID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if authorizeErr := authorizeReviewAccess(ctx, transaction, task.WorkspaceID, actorID, actor.TokenVersion, true); authorizeErr != nil {
			return authorizeErr
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID string
			Command application.DecideCommand
		}{ActorID: actor.UserID, Command: command})
		if hashErr != nil {
			return hashErr
		}
		if receipt, receiptErr := platformcommandgorm.Find(ctx, transaction, task.WorkspaceID.String(), decideOperation, command.IdempotencyKey); receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[domain.DecisionResult](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different decision input")
			}
			result = replayed
			return replayErr
		} else if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if task.Revision != command.ExpectedTaskRevision || task.SubjectRevision != command.ExpectedSubjectRevision ||
			task.SubjectHash != command.ExpectedSubjectHash ||
			task.Status != "CLAIMED" || task.ClaimedBy == nil || *task.ClaimedBy != actorID ||
			task.ClaimToken == nil || *task.ClaimToken != token || task.ClaimExpiresAt == nil || !task.ClaimExpiresAt.After(now) {
			return conflict("Human task changed before decision")
		}
		if command.Decision == "selected" && !containsCandidate(task.CandidateIDs, command.SelectedCandidateID) {
			return conflict("Selected candidate is not bound to the human task")
		}
		if !containsDecision(task.AllowedDecisions, command.Decision) {
			return conflict("Decision is not allowed by the frozen human task rubric")
		}
		decision := model.ReviewDecision{
			ID: decisionID, WorkspaceID: task.WorkspaceID, HumanTaskID: task.ID, Decision: desired.Decision,
			SubjectRevision: desired.SubjectRevision, SubjectHash: desired.SubjectHash,
			CreatedBy: actorID, CreatedAt: now.UTC(),
		}
		if command.SelectedCandidateID != "" {
			selected, parseErr := uuid.Parse(command.SelectedCandidateID)
			if parseErr != nil {
				return parseErr
			}
			decision.SelectedCandidateID = &selected
		}
		if createErr := transaction.Omit(clause.Associations).Create(&decision).Error; createErr != nil {
			return createErr
		}
		task.Status, task.ClaimToken, task.ClaimExpiresAt = "COMPLETED", nil, nil
		task.Revision++
		task.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.HumanTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status": task.Status, "claim_token": nil, "claim_expires_at": nil,
			"revision": task.Revision, "updated_at": task.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		persistedTask, mapErr := taskDomain(task)
		if mapErr != nil {
			return mapErr
		}
		result = domain.DecisionResult{Task: persistedTask, Decision: decisionDomain(decision)}
		return storeResultReceipt(ctx, transaction, task.WorkspaceID, decideOperation, command.IdempotencyKey, inputHash, task.ID, actorID, result, now)
	})
	return result, err
}

func taskRecord(value domain.HumanTask) (model.HumanTask, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.HumanTask{}, err
	}
	workspaceID, projectID, err := taskScope(value.WorkspaceID, value.ProjectID)
	if err != nil {
		return model.HumanTask{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.HumanTask{}, err
	}
	nodeID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.HumanTask{}, err
	}
	subjectID, err := uuid.Parse(value.SubjectID)
	if err != nil {
		return model.HumanTask{}, err
	}
	candidates, err := json.Marshal(value.CandidateIDs)
	if err != nil {
		return model.HumanTask{}, err
	}
	allowedDecisions, err := json.Marshal(value.AllowedDecisions)
	if err != nil {
		return model.HumanTask{}, err
	}
	return model.HumanTask{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: runID, NodeRunID: nodeID,
		SubjectType: value.SubjectType, SubjectID: subjectID, SubjectRevision: value.SubjectRevision,
		SubjectHash: value.SubjectHash, CandidateIDs: datatypes.JSON(candidates), RubricVersion: value.RubricVersion,
		AllowedDecisions: datatypes.JSON(allowedDecisions), Status: value.Status,
		Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func taskDomain(value model.HumanTask) (domain.HumanTask, error) {
	var candidates []string
	if err := json.Unmarshal(value.CandidateIDs, &candidates); err != nil {
		return domain.HumanTask{}, err
	}
	var allowedDecisions []string
	if err := json.Unmarshal(value.AllowedDecisions, &allowedDecisions); err != nil {
		return domain.HumanTask{}, err
	}
	var claimedBy, claimToken *string
	if value.ClaimedBy != nil {
		text := value.ClaimedBy.String()
		claimedBy = &text
	}
	if value.ClaimToken != nil {
		text := value.ClaimToken.String()
		claimToken = &text
	}
	return domain.HumanTask{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		WorkflowRunID: value.WorkflowRunID.String(), NodeRunID: value.NodeRunID.String(), SubjectType: value.SubjectType,
		SubjectID: value.SubjectID.String(), SubjectRevision: value.SubjectRevision, SubjectHash: value.SubjectHash,
		CandidateIDs: candidates, RubricVersion: value.RubricVersion, AllowedDecisions: allowedDecisions,
		Status: value.Status, ClaimedBy: claimedBy, ClaimToken: claimToken,
		ClaimExpiresAt: value.ClaimExpiresAt, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func decisionDomain(value model.ReviewDecision) domain.ReviewDecision {
	var selected *string
	if value.SelectedCandidateID != nil {
		text := value.SelectedCandidateID.String()
		selected = &text
	}
	return domain.ReviewDecision{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), HumanTaskID: value.HumanTaskID.String(),
		Decision: value.Decision, SubjectRevision: value.SubjectRevision, SubjectHash: value.SubjectHash,
		SelectedCandidateID: selected,
		CreatedBy:           value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}
}

func validateDecisionResult(task domain.HumanTask, decision domain.ReviewDecision) error {
	if task.Status != "COMPLETED" || decision.WorkspaceID != task.WorkspaceID ||
		decision.HumanTaskID != task.ID || decision.SubjectRevision != task.SubjectRevision ||
		decision.SubjectHash != task.SubjectHash ||
		task.ClaimedBy == nil || *task.ClaimedBy != decision.CreatedBy {
		return errors.New("review task and decision facts have drifted")
	}
	candidates := append([]string(nil), task.CandidateIDs...)
	for _, candidateID := range candidates {
		if _, err := uuid.Parse(candidateID); err != nil {
			return errors.New("review task candidate binding has drifted")
		}
	}
	slices.Sort(candidates)
	if !slices.Equal(candidates, task.CandidateIDs) || len(slices.Compact(candidates)) != len(task.CandidateIDs) {
		return errors.New("review task candidate binding has drifted")
	}
	if decision.Decision == "selected" {
		if decision.SelectedCandidateID == nil || !slices.Contains(task.CandidateIDs, *decision.SelectedCandidateID) {
			return errors.New("review selected candidate binding has drifted")
		}
		return nil
	}
	if decision.SelectedCandidateID != nil || !slices.Contains([]string{"approved", "rejected", "changes_requested"}, decision.Decision) {
		return errors.New("review decision value has drifted")
	}
	return nil
}

func taskScope(left, right string) (uuid.UUID, uuid.UUID, error) {
	leftID, err := uuid.Parse(left)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	rightID, err := uuid.Parse(right)
	return leftID, rightID, err
}

func claimIdentities(taskID, claimToken, actorID string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	task, err := uuid.Parse(taskID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	token, err := uuid.Parse(claimToken)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	actor, err := uuid.Parse(actorID)
	return task, token, actor, err
}

func authorizeReviewAccess(
	ctx context.Context,
	transaction *gorm.DB,
	workspaceID uuid.UUID,
	actorID uuid.UUID,
	tokenVersion int,
	write bool,
) error {
	var membership model.Membership
	if err := transaction.WithContext(ctx).Where(
		"workspace_id = ? AND user_id = ? AND status = ?", workspaceID, actorID, "active",
	).First(&membership).Error; err != nil {
		return normalizeNotFound(err)
	}
	var user model.UserAccount
	if err := transaction.WithContext(ctx).First(&user, "id = ?", actorID).Error; err != nil {
		return normalizeNotFound(err)
	}
	if user.Status != "active" || user.TokenVersion != tokenVersion {
		return application.ErrNotFound
	}
	if write && membership.Role != "owner" && membership.Role != "editor" {
		return &application.Error{Code: "forbidden", Message: "Review permission is required", Status: 403}
	}
	return nil
}

func containsDecision(raw []byte, decision string) bool {
	var allowed []string
	return json.Unmarshal(raw, &allowed) == nil && slices.Contains(allowed, decision)
}

func containsCandidate(raw []byte, candidate string) bool {
	var candidates []string
	if json.Unmarshal(raw, &candidates) != nil {
		return false
	}
	return slices.Contains(candidates, candidate)
}

func storeResultReceipt(
	ctx context.Context,
	transaction *gorm.DB,
	workspaceID uuid.UUID,
	operation string,
	idempotencyKey string,
	inputHash string,
	resourceID uuid.UUID,
	actorID uuid.UUID,
	result any,
	now time.Time,
) error {
	encoded, err := platformcommand.Result(result)
	if err != nil {
		return err
	}
	receiptID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(workspaceID.String()+":"+operation+":"+idempotencyKey))
	return platformcommandgorm.Create(ctx, transaction, platformcommand.Receipt{
		ID: receiptID.String(), WorkspaceID: workspaceID.String(), Operation: operation, IdempotencyKey: idempotencyKey,
		InputHash: inputHash, ResourceID: resourceID.String(), Result: encoded,
		CreatedBy: actorID.String(), CreatedAt: now.UTC(),
	})
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func conflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}

var _ application.Repository = (*Store)(nil)
