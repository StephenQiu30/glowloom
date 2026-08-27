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

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (store *Store) FindSignalPreparation(
	ctx context.Context,
	workspaceID string,
	idempotencyKey string,
) (domain.SignalPreparation, bool, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.SignalPreparation{}, false, application.ErrNotFound
	}
	var prepared domain.SignalPreparation
	found := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var intent model.WorkflowSignalIntent
		loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", workspace, idempotencyKey).First(&intent).Error
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		var apply model.WorkflowHumanGateApplyReceipt
		if loadErr = transaction.First(&apply, "id = ?", intent.ApplyReceiptID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		prepared = signalPreparationDomain(apply, intent)
		found = true
		return nil
	})
	return prepared, found, err
}

func (store *Store) ResolveHumanGateOwnerApplication(
	ctx context.Context,
	request domain.HumanGateDecisionRequest,
) (domain.HumanGateOwnerApplication, error) {
	workspaceID, err := uuid.Parse(request.WorkspaceID)
	if err != nil {
		return domain.HumanGateOwnerApplication{}, application.ErrNotFound
	}
	runID, err := uuid.Parse(request.WorkflowRunID)
	if err != nil {
		return domain.HumanGateOwnerApplication{}, application.ErrNotFound
	}
	nodeID, err := uuid.Parse(request.NodeRunID)
	if err != nil {
		return domain.HumanGateOwnerApplication{}, application.ErrNotFound
	}
	taskID, err := uuid.Parse(request.HumanTaskID)
	if err != nil {
		return domain.HumanGateOwnerApplication{}, application.ErrNotFound
	}
	decisionID, err := uuid.Parse(request.ReviewDecisionID)
	if err != nil {
		return domain.HumanGateOwnerApplication{}, application.ErrNotFound
	}
	var applicationResult domain.HumanGateOwnerApplication
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.WorkspaceID != workspaceID || node.WorkflowRunID != run.ID || run.Status != "WAITING_HUMAN" ||
			node.Status != "WAITING_HUMAN" || node.Revision != request.SubjectRevision {
			return errors.New("workflow human gate changed before owner application")
		}
		task, decision, bindingErr := loadHumanGateDecision(
			transaction, run, node, taskID, decisionID, request.SubjectRevision, request.Decision,
		)
		if bindingErr != nil {
			return bindingErr
		}
		resolved, resolveErr := resolveNodeExecution(transaction, run, node)
		if resolveErr != nil {
			return resolveErr
		}
		_, persistedHash, inputErr := persistedNodeInput(node)
		if inputErr != nil || persistedHash != resolved.InputHash || resolved.Execution.RiskLevel != "human_gate" ||
			resolved.Execution.CachePolicy != "never" || resolved.CacheKey != "" || len(resolved.Execution.OutputPorts) != 1 ||
			!resolved.Execution.OutputPorts[0].Required {
			return errors.New("workflow human gate owner contract has drifted")
		}
		candidateID, candidateErr := selectedHumanGateCandidate(task, decision)
		if candidateErr != nil {
			return candidateErr
		}
		var candidate domain.NodeInputBinding
		candidateFound := false
		for _, binding := range resolved.Input.Bindings {
			if node.Executor == "gate.generation_image_review" && binding.SourceKind == domain.NodeInputSourceNodeOutput &&
				binding.Port == "candidates" && binding.ValueType == "generation_candidate_set" {
				candidate, candidateFound = binding, true
				break
			}
			if node.Executor != "gate.generation_image_review" &&
				binding.SourceKind == domain.NodeInputSourceNodeOutput && binding.ReferenceID == candidateID {
				candidate, candidateFound = binding, true
				break
			}
		}
		if !candidateFound {
			return errors.New("workflow human gate owner candidate is not in frozen input")
		}
		output := resolved.Execution.OutputPorts[0]
		applicationResult = domain.HumanGateOwnerApplication{
			WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(), WorkflowRunID: run.ID.String(),
			NodeRunID: node.ID.String(), HumanTaskID: task.ID.String(), ReviewDecisionID: decision.ID.String(),
			SubjectRevision: node.Revision, Decision: decision.Decision, Executor: node.Executor,
			Candidate: candidate, OutputPort: output.Key, OutputValueType: output.ValueType,
		}
		return nil
	})
	return applicationResult, err
}

func (store *Store) GetHumanGateCoordination(
	ctx context.Context,
	workspaceID string,
	decisionID string,
) (domain.HumanGateCoordination, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.HumanGateCoordination{}, application.ErrNotFound
	}
	decision, err := uuid.Parse(decisionID)
	if err != nil {
		return domain.HumanGateCoordination{}, application.ErrNotFound
	}
	status := domain.HumanGateCoordination{
		ReviewDecisionID: decisionID, DecisionStatus: "recorded",
		OwnerApplyStatus: "pending", WorkflowResumeStatus: "pending",
	}
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var reviewDecision model.ReviewDecision
		if loadErr := transaction.First(&reviewDecision, "id = ?", decision).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if reviewDecision.WorkspaceID != workspace {
			return application.ErrNotFound
		}
		var apply model.WorkflowHumanGateApplyReceipt
		applyErr := transaction.Where("review_decision_id = ?", decision).First(&apply).Error
		if errors.Is(applyErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if applyErr != nil {
			return applyErr
		}
		status.OwnerApplyStatus = apply.Status
		if apply.OwnerReceiptID != nil {
			status.OwnerReceiptID = apply.OwnerReceiptID.String()
		}
		if apply.ConflictCode != nil {
			status.ConflictCode = *apply.ConflictCode
		}
		var intent model.WorkflowSignalIntent
		intentErr := transaction.Where("review_decision_id = ?", decision).First(&intent).Error
		if errors.Is(intentErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if intentErr != nil {
			return intentErr
		}
		if intent.WorkspaceID != workspace || intent.ApplyReceiptID != apply.ID {
			return errors.New("workflow human gate coordination has drifted")
		}
		status.WorkflowResumeStatus = intent.Status
		var receipt model.WorkflowSignalReceipt
		receiptErr := transaction.Where("signal_intent_id = ?", intent.ID).
			Order("attempt_no DESC").First(&receipt).Error
		if receiptErr == nil {
			status.SignalReceiptID = receipt.ID.String()
		} else if !errors.Is(receiptErr, gorm.ErrRecordNotFound) {
			return receiptErr
		}
		return nil
	})
	return status, err
}

func (store *Store) RecordHumanGateOwnerConflict(
	ctx context.Context,
	desired domain.HumanGateApplyReceipt,
) error {
	record, err := signalApplyRecord(desired)
	if err != nil {
		return err
	}
	if record.Status != "conflict" {
		return errors.New("workflow human gate owner conflict status is invalid")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", record.WorkflowRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", record.NodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.WorkspaceID != record.WorkspaceID || node.WorkflowRunID != run.ID || run.Status != "WAITING_HUMAN" ||
			node.Status != "WAITING_HUMAN" || node.Revision != record.SubjectRevision {
			return errors.New("workflow human gate changed before owner conflict recording")
		}
		if _, _, loadErr := loadHumanGateDecision(
			transaction, run, node, record.HumanTaskID, record.ReviewDecisionID, record.SubjectRevision, record.Decision,
		); loadErr != nil {
			return loadErr
		}
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "review_decision_id"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&record).Error; createErr != nil {
			return createErr
		}
		var persisted model.WorkflowHumanGateApplyReceipt
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("review_decision_id = ?", record.ReviewDecisionID).First(&persisted).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if !sameSignalApply(persisted, desired) {
			return errors.New("persisted human gate owner conflict has drifted")
		}
		return nil
	})
}

func (store *Store) PrepareSignal(ctx context.Context, desired domain.SignalPreparation) (domain.SignalPreparation, error) {
	applyRecord, err := signalApplyRecord(desired.ApplyReceipt)
	if err != nil {
		return domain.SignalPreparation{}, err
	}
	intentRecord, err := signalIntentRecord(desired.Intent, applyRecord.ID)
	if err != nil {
		return domain.SignalPreparation{}, err
	}
	var prepared domain.SignalPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var existingIntent model.WorkflowSignalIntent
		existingErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", intentRecord.WorkspaceID, intentRecord.IdempotencyKey).
			First(&existingIntent).Error
		if existingErr == nil {
			var existingApply model.WorkflowHumanGateApplyReceipt
			if loadErr := transaction.First(&existingApply, "id = ?", existingIntent.ApplyReceiptID).Error; loadErr != nil {
				return normalizeNotFound(loadErr)
			}
			prepared = signalPreparationDomain(existingApply, existingIntent)
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", intentRecord.WorkflowRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", intentRecord.NodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.WorkspaceID != intentRecord.WorkspaceID || node.WorkflowRunID != run.ID || node.Status != "WAITING_HUMAN" ||
			run.Status != "WAITING_HUMAN" || node.Revision != intentRecord.SubjectRevision {
			return errors.New("workflow human gate changed before signal preparation")
		}
		if bindingErr := validateHumanGateDecisionBinding(transaction, run, node, applyRecord, intentRecord); bindingErr != nil {
			return bindingErr
		}
		intentRecord.TemporalWorkflowID = run.TemporalWorkflowID
		temporary := domain.SignalPreparation{ApplyReceipt: signalApplyDomain(applyRecord), Intent: signalIntentDomain(intentRecord)}
		request, requestErr := application.NewSignalRequest(temporary)
		if requestErr != nil {
			return requestErr
		}
		intentRecord.InputHash = request.InputHash
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "review_decision_id"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&applyRecord).Error; createErr != nil {
			return createErr
		}
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("review_decision_id = ?", applyRecord.ReviewDecisionID).First(&applyRecord).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if !sameSignalApply(applyRecord, desired.ApplyReceipt) {
			return errors.New("persisted human gate apply receipt has drifted")
		}
		intentRecord.ApplyReceiptID = applyRecord.ID
		if createErr := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}, {Name: "idempotency_key"}}, DoNothing: true,
		}).Omit(clause.Associations).Create(&intentRecord).Error; createErr != nil {
			return createErr
		}
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND idempotency_key = ?", intentRecord.WorkspaceID, intentRecord.IdempotencyKey).
			First(&intentRecord).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		prepared = signalPreparationDomain(applyRecord, intentRecord)
		return nil
	})
	return prepared, err
}

func validateHumanGateDecisionBinding(
	transaction *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	apply model.WorkflowHumanGateApplyReceipt,
	intent model.WorkflowSignalIntent,
) error {
	if apply.WorkspaceID != run.WorkspaceID || apply.WorkflowRunID != run.ID || apply.NodeRunID != node.ID ||
		intent.WorkspaceID != apply.WorkspaceID || intent.WorkflowRunID != apply.WorkflowRunID ||
		intent.NodeRunID != apply.NodeRunID || intent.HumanTaskID != apply.HumanTaskID ||
		intent.ReviewDecisionID != apply.ReviewDecisionID || intent.SubjectRevision != apply.SubjectRevision ||
		intent.Decision != apply.Decision {
		return errors.New("workflow human gate signal binding has drifted")
	}
	task, decision, err := loadHumanGateDecision(
		transaction, run, node, apply.HumanTaskID, apply.ReviewDecisionID, apply.SubjectRevision, apply.Decision,
	)
	if err != nil {
		return err
	}
	return validateHumanGateOwnerEvidence(transaction, run, node, task, decision, apply)
}

func loadHumanGateDecision(
	transaction *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	taskID uuid.UUID,
	decisionID uuid.UUID,
	subjectRevision int,
	decisionValue string,
) (model.HumanTask, model.ReviewDecision, error) {
	var task model.HumanTask
	if err := transaction.First(&task, "id = ?", taskID).Error; err != nil {
		return model.HumanTask{}, model.ReviewDecision{}, normalizeNotFound(err)
	}
	var decision model.ReviewDecision
	if err := transaction.First(&decision, "id = ?", decisionID).Error; err != nil {
		return model.HumanTask{}, model.ReviewDecision{}, normalizeNotFound(err)
	}
	if task.WorkspaceID != run.WorkspaceID || task.WorkflowRunID != run.ID || task.NodeRunID != node.ID ||
		task.SubjectType != humanGateSubjectType(node.Executor) || task.SubjectID != node.ID || task.SubjectRevision != node.Revision || task.SubjectRevision != subjectRevision ||
		task.Status != "COMPLETED" || decision.WorkspaceID != run.WorkspaceID || decision.HumanTaskID != task.ID ||
		decision.SubjectRevision != task.SubjectRevision || decision.SubjectHash != task.SubjectHash ||
		decision.Decision != decisionValue || !humanTaskContainsDecision(task.AllowedDecisions, decision.Decision) {
		return model.HumanTask{}, model.ReviewDecision{}, errors.New("workflow human gate review decision has drifted")
	}
	if decision.Decision == "selected" {
		if decision.SelectedCandidateID == nil || !humanTaskContainsCandidate(task.CandidateIDs, *decision.SelectedCandidateID) {
			return model.HumanTask{}, model.ReviewDecision{}, errors.New("workflow human gate selected candidate has drifted")
		}
	}
	return task, decision, nil
}

func humanTaskContainsDecision(raw []byte, decision string) bool {
	var allowed []string
	return json.Unmarshal(raw, &allowed) == nil && slices.Contains(allowed, decision)
}

func humanTaskContainsCandidate(raw []byte, candidateID uuid.UUID) bool {
	candidates, err := humanTaskCandidateIDs(raw)
	if err != nil {
		return false
	}
	for _, candidate := range candidates {
		if candidate == candidateID.String() {
			return true
		}
	}
	return false
}

func humanTaskCandidateIDs(raw []byte) ([]string, error) {
	var candidates []string
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func selectedHumanGateCandidate(task model.HumanTask, decision model.ReviewDecision) (string, error) {
	candidates, err := humanTaskCandidateIDs(task.CandidateIDs)
	if err != nil || len(candidates) == 0 {
		return "", errors.New("workflow human gate candidate binding has drifted")
	}
	if decision.Decision == "selected" {
		if decision.SelectedCandidateID == nil || !humanTaskContainsCandidate(task.CandidateIDs, *decision.SelectedCandidateID) {
			return "", errors.New("workflow selected human gate has no valid selected candidate")
		}
		return decision.SelectedCandidateID.String(), nil
	}
	if decision.Decision == "approved" && len(candidates) != 1 {
		return "", errors.New("workflow approved human gate requires exactly one candidate")
	}
	return candidates[0], nil
}

func validateHumanGateOwnerEvidence(
	transaction *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	task model.HumanTask,
	decision model.ReviewDecision,
	apply model.WorkflowHumanGateApplyReceipt,
) error {
	if decision.Decision != "approved" && decision.Decision != "selected" {
		if apply.Status != "not_required" || apply.ConflictCode != nil || apply.OwnerReceiptID != nil ||
			apply.OwnerOperation != nil || len(apply.Output) != 0 || apply.OutputHash != nil {
			return errors.New("workflow rejected human gate has owner evidence")
		}
		return nil
	}
	if apply.Status != "completed" || apply.ConflictCode != nil || apply.OwnerReceiptID == nil ||
		apply.OwnerOperation == nil || *apply.OwnerOperation == "" || apply.OutputHash == nil {
		return errors.New("workflow approved human gate has no owner evidence")
	}
	output, canonical, outputHash, outputErr := domain.ParseNodeOutput(json.RawMessage(apply.Output))
	if outputErr != nil || outputHash != *apply.OutputHash || string(canonical) != string(apply.Output) {
		return errors.New("workflow human gate owner output has drifted")
	}
	resolved, resolveErr := resolveNodeExecution(transaction, run, node)
	if resolveErr != nil {
		return resolveErr
	}
	_, persistedHash, inputErr := persistedNodeInput(node)
	if inputErr != nil || persistedHash != resolved.InputHash || len(resolved.Execution.OutputPorts) != 1 ||
		domain.ValidateNodeOutputPorts(output, resolved.Execution.OutputPorts) != nil || len(output.Bindings) != 1 {
		return errors.New("workflow human gate owner output contract has drifted")
	}
	candidateID, candidateErr := selectedHumanGateCandidate(task, decision)
	if candidateErr != nil {
		return candidateErr
	}
	var candidate domain.NodeInputBinding
	candidateFound := false
	for _, binding := range resolved.Input.Bindings {
		if node.Executor == "gate.generation_image_review" && binding.SourceKind == domain.NodeInputSourceNodeOutput &&
			binding.Port == "candidates" && binding.ValueType == "generation_candidate_set" {
			candidate, candidateFound = binding, true
			break
		}
		if node.Executor != "gate.generation_image_review" &&
			binding.SourceKind == domain.NodeInputSourceNodeOutput && binding.ReferenceID == candidateID {
			candidate, candidateFound = binding, true
			break
		}
	}
	binding := output.Bindings[0]
	if !candidateFound || !domain.HumanGateOutputMatchesCandidate(node.Executor, candidate, binding) {
		return errors.New("workflow human gate owner output does not match the frozen candidate")
	}
	var receipt model.CommandReceipt
	if err := transaction.First(&receipt, "id = ?", *apply.OwnerReceiptID).Error; err != nil {
		return normalizeNotFound(err)
	}
	expectedOperation, supported := humanGateOwnerOperation(node.Executor)
	if !supported || *apply.OwnerOperation != expectedOperation || receipt.WorkspaceID != run.WorkspaceID || receipt.Operation != *apply.OwnerOperation ||
		receipt.ResourceID.String() != binding.ReferenceID || receipt.CreatedBy != apply.CreatedBy {
		return errors.New("workflow human gate owner receipt has drifted")
	}
	return nil
}

func humanGateOwnerOperation(executor string) (string, bool) {
	switch executor {
	case "gate.production_bible_review":
		return "production_bible.confirm", true
	case "gate.episode_plan_review":
		return "episode_plan.confirm", true
	case "gate.episode_structure_review":
		return "episode_structure.confirm_batch", true
	case "gate.storyboard_review":
		return "storyboard.apply_set", true
	case "gate.generation_image_review":
		return "generation.candidate.select", true
	default:
		return "", false
	}
}

func humanGateSubjectType(executor string) string {
	if executor == "gate.generation_image_review" {
		return "generation_candidate_selection"
	}
	return "workflow_node_output"
}

func (store *Store) BeginSignalAttempt(ctx context.Context, intentID string, now time.Time) (domain.SignalPreparation, error) {
	id, err := uuid.Parse(intentID)
	if err != nil {
		return domain.SignalPreparation{}, application.ErrNotFound
	}
	var prepared domain.SignalPreparation
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var intent model.WorkflowSignalIntent
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", id).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var apply model.WorkflowHumanGateApplyReceipt
		if loadErr := transaction.First(&apply, "id = ?", intent.ApplyReceiptID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if intent.Status != "completed" && intent.Status != "conflict" {
			intent.Status = "pending"
			intent.AttemptNo++
			intent.Revision++
			intent.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.WorkflowSignalIntent{}).Where("id = ?", intent.ID).Updates(map[string]any{
				"status": intent.Status, "attempt_no": intent.AttemptNo,
				"revision": intent.Revision, "updated_at": intent.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		}
		prepared = signalPreparationDomain(apply, intent)
		return nil
	})
	return prepared, err
}

func (store *Store) FinalizeSignalAttempt(
	ctx context.Context,
	intent domain.SignalIntent,
	receipt domain.SignalReceipt,
	expectedRevision int,
) error {
	intentRecord, err := signalIntentRecord(intent, uuid.Nil)
	if err != nil {
		return err
	}
	receiptRecord, err := signalReceiptRecord(receipt)
	if err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		result := transaction.Model(&model.WorkflowSignalIntent{}).
			Where("id = ? AND revision = ?", intentRecord.ID, expectedRevision).
			Updates(map[string]any{
				"status": intentRecord.Status, "attempt_no": intentRecord.AttemptNo,
				"revision": intentRecord.Revision, "updated_at": intentRecord.UpdatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return &application.Error{Code: "resource_conflict", Message: "Workflow signal intent changed before receipt", Status: 409}
		}
		if createErr := transaction.Omit(clause.Associations).Create(&receiptRecord).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return &application.Error{Code: "resource_conflict", Message: "Workflow signal attempt already has a receipt", Status: 409}
			}
			return createErr
		}
		return nil
	})
}

func signalApplyRecord(value domain.HumanGateApplyReceipt) (model.WorkflowHumanGateApplyReceipt, error) {
	id, workspaceID, runID, nodeID, taskID, decisionID, createdBy, err := signalIDs(
		value.ID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID, value.HumanTaskID, value.ReviewDecisionID, value.CreatedBy,
	)
	if err != nil {
		return model.WorkflowHumanGateApplyReceipt{}, err
	}
	var ownerReceiptID *uuid.UUID
	var ownerOperation *string
	var output datatypes.JSON
	var outputHash *string
	var conflictCode *string
	if value.Status == "completed" && (value.Decision == "approved" || value.Decision == "selected") {
		parsedReceiptID, parseErr := uuid.Parse(value.OwnerReceiptID)
		normalized, encoded, canonicalHash, outputErr := domain.BuildNodeOutput(value.Output)
		if parseErr != nil || outputErr != nil || value.OwnerOperation == "" || value.OutputHash != canonicalHash || len(normalized.Bindings) != 1 {
			return model.WorkflowHumanGateApplyReceipt{}, errors.New("invalid workflow human gate owner evidence")
		}
		operation := value.OwnerOperation
		hash := value.OutputHash
		ownerReceiptID, ownerOperation, output, outputHash = &parsedReceiptID, &operation, datatypes.JSON(encoded), &hash
	} else if value.Status == "conflict" && (value.Decision == "approved" || value.Decision == "selected") {
		if value.ConflictCode == "" || value.OwnerReceiptID != "" || value.OwnerOperation != "" ||
			value.OutputHash != "" || value.Output.SchemaVersion != "" || len(value.Output.Bindings) != 0 {
			return model.WorkflowHumanGateApplyReceipt{}, errors.New("invalid workflow human gate owner conflict")
		}
		code := value.ConflictCode
		conflictCode = &code
	} else if value.Status != "not_required" || (value.Decision != "rejected" && value.Decision != "changes_requested") ||
		value.ConflictCode != "" || value.OwnerReceiptID != "" || value.OwnerOperation != "" || value.OutputHash != "" ||
		value.Output.SchemaVersion != "" || len(value.Output.Bindings) != 0 {
		return model.WorkflowHumanGateApplyReceipt{}, errors.New("invalid rejected workflow human gate owner evidence")
	}
	return model.WorkflowHumanGateApplyReceipt{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, NodeRunID: nodeID,
		HumanTaskID: taskID, ReviewDecisionID: decisionID, SubjectRevision: value.SubjectRevision,
		Decision: value.Decision, Status: value.Status, ConflictCode: conflictCode,
		OwnerReceiptID: ownerReceiptID, OwnerOperation: ownerOperation,
		Output: output, OutputHash: outputHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func signalIntentRecord(value domain.SignalIntent, applyReceiptID uuid.UUID) (model.WorkflowSignalIntent, error) {
	id, workspaceID, runID, nodeID, taskID, decisionID, createdBy, err := signalIDs(
		value.ID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID, value.HumanTaskID, value.ReviewDecisionID, value.CreatedBy,
	)
	if err != nil {
		return model.WorkflowSignalIntent{}, err
	}
	return model.WorkflowSignalIntent{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: runID, NodeRunID: nodeID,
		HumanTaskID: taskID, ReviewDecisionID: decisionID, ApplyReceiptID: applyReceiptID,
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, SignalID: value.SignalID, InputHash: value.InputHash,
		Decision: value.Decision, SubjectRevision: value.SubjectRevision, Status: value.Status,
		AttemptNo: value.AttemptNo, Revision: value.Revision, CreatedBy: createdBy,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func signalReceiptRecord(value domain.SignalReceipt) (model.WorkflowSignalReceipt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	intentID, err := uuid.Parse(value.SignalIntentID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.WorkflowSignalReceipt{}, err
	}
	return model.WorkflowSignalReceipt{
		ID: id, WorkspaceID: workspaceID, SignalIntentID: intentID, WorkflowRunID: runID,
		AttemptNo: value.AttemptNo, Outcome: value.Outcome, SignalID: value.SignalID,
		ExpectedInputHash: value.ExpectedInputHash, ObservedInputHash: value.ObservedInputHash, CreatedAt: value.CreatedAt,
	}, nil
}

func signalApplyDomain(value model.WorkflowHumanGateApplyReceipt) domain.HumanGateApplyReceipt {
	result := domain.HumanGateApplyReceipt{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		NodeRunID: value.NodeRunID.String(), HumanTaskID: value.HumanTaskID.String(), ReviewDecisionID: value.ReviewDecisionID.String(),
		SubjectRevision: value.SubjectRevision, Decision: value.Decision, Status: value.Status,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}
	if value.ConflictCode != nil {
		result.ConflictCode = *value.ConflictCode
	}
	if value.OwnerReceiptID != nil && value.OwnerOperation != nil && value.OutputHash != nil {
		output, _, outputHash, err := domain.ParseNodeOutput(json.RawMessage(value.Output))
		if err == nil && outputHash == *value.OutputHash {
			result.OwnerReceiptID, result.OwnerOperation = value.OwnerReceiptID.String(), *value.OwnerOperation
			result.Output, result.OutputHash = output, outputHash
		}
	}
	return result
}

func signalIntentDomain(value model.WorkflowSignalIntent) domain.SignalIntent {
	return domain.SignalIntent{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), WorkflowRunID: value.WorkflowRunID.String(),
		NodeRunID: value.NodeRunID.String(), HumanTaskID: value.HumanTaskID.String(), ReviewDecisionID: value.ReviewDecisionID.String(),
		IdempotencyKey: value.IdempotencyKey, CommandInputHash: value.CommandInputHash,
		TemporalWorkflowID: value.TemporalWorkflowID, SignalID: value.SignalID, InputHash: value.InputHash,
		Decision: value.Decision, SubjectRevision: value.SubjectRevision, Status: value.Status,
		AttemptNo: value.AttemptNo, Revision: value.Revision, CreatedBy: value.CreatedBy.String(),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func sameSignalApply(record model.WorkflowHumanGateApplyReceipt, desired domain.HumanGateApplyReceipt) bool {
	if record.ID.String() != desired.ID || record.WorkspaceID.String() != desired.WorkspaceID ||
		record.WorkflowRunID.String() != desired.WorkflowRunID || record.NodeRunID.String() != desired.NodeRunID ||
		record.HumanTaskID.String() != desired.HumanTaskID || record.ReviewDecisionID.String() != desired.ReviewDecisionID ||
		record.SubjectRevision != desired.SubjectRevision || record.Decision != desired.Decision ||
		record.Status != desired.Status ||
		record.CreatedBy.String() != desired.CreatedBy {
		return false
	}
	persisted := signalApplyDomain(record)
	return persisted.ConflictCode == desired.ConflictCode && persisted.OwnerReceiptID == desired.OwnerReceiptID &&
		persisted.OwnerOperation == desired.OwnerOperation &&
		persisted.OutputHash == desired.OutputHash && persisted.Output.SchemaVersion == desired.Output.SchemaVersion &&
		len(persisted.Output.Bindings) == len(desired.Output.Bindings) && slices.Equal(persisted.Output.Bindings, desired.Output.Bindings)
}

func signalPreparationDomain(
	apply model.WorkflowHumanGateApplyReceipt,
	intent model.WorkflowSignalIntent,
) domain.SignalPreparation {
	return domain.SignalPreparation{ApplyReceipt: signalApplyDomain(apply), Intent: signalIntentDomain(intent)}
}

func signalIDs(values ...string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		ids[index] = parsed
	}
	return ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6], nil
}

var _ application.SignalRepository = (*Store)(nil)
var _ application.HumanGateOwnerConflictRepository = (*Store)(nil)
var _ application.HumanGateCoordinationRepository = (*Store)(nil)
