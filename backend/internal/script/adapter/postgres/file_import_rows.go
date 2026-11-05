package postgres

import (
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/task"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

func readImport(tx *gorm.DB, actor identityapp.Principal, project, id uuid.UUID, lock bool) (scriptapp.ImportRecord, error) {
	var job scriptImportJobRow
	if err := tx.Where("org_id = ? AND project_id = ? AND id = ?", actor.OrgID, project, id).Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return scriptapp.ImportRecord{}, scriptapp.ErrNotFound
		}
		return scriptapp.ImportRecord{}, err
	}
	stateQuery := tx.Model(&scriptImportStateRow{}).Where("job_id = ?", id)
	if lock {
		stateQuery = stateQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var state scriptImportStateRow
	if err := stateQuery.Take(&state).Error; err != nil {
		return scriptapp.ImportRecord{}, err
	}
	var attempt scriptImportAttemptRow
	if err := tx.Where("job_id = ? AND attempt = ?", id, state.Attempt).Take(&attempt).Error; err != nil {
		return scriptapp.ImportRecord{}, err
	}
	record := scriptapp.ImportRecord{
		Actor:          identityapp.Principal{ID: job.ActorID, OrgID: job.OrgID},
		BaseVersionID:  attempt.BaseVersionID,
		PublicationKey: attempt.PublicationKey,
		OwnerID:        state.IOOwnerID,
		IOState:        state.IOState,
	}
	result := scriptapp.ImportJob{
		ID:                      job.ID,
		ProjectID:               job.ProjectID,
		Revision:                state.Revision,
		Attempt:                 state.Attempt,
		Status:                  state.Status,
		Stage:                   state.Stage,
		ExpectedScriptRevision:  job.ExpectedScriptRevision,
		LatestScriptRevision:    state.LatestScriptRevision,
		LatestVersionID:         state.LatestVersionID,
		CancellationRequested:   state.CancellationRequested,
		ReconciliationRequested: state.ReconciliationRequested,
		NeedsReconciliation:     state.NeedsReconciliation,
		ActiveIO:                state.IOOwnerID != nil,
		FailureCode:             state.FailureCode,
		Files:                   []scriptapp.ImportFile{},
		CreatedAt:               job.CreatedAt.UTC(),
		UpdatedAt:               state.UpdatedAt.UTC(),
	}
	var files []scriptImportFileRow
	if err := tx.Where("job_id = ?", id).Order("position ASC").Find(&files).Error; err != nil {
		return record, err
	}
	if len(files) < 1 || len(files) > 200 {
		return record, scriptapp.ErrUnavailable
	}
	var results []scriptImportFileResultRow
	if err := tx.Where("job_id = ?", id).Order("position ASC, attempt DESC").Find(&results).Error; err != nil {
		return record, err
	}
	latest := make(map[int]scriptapp.ImportFileResult, len(files))
	latestAttempts := make(map[int]int, len(files))
	for _, row := range results {
		var outcome scriptapp.ImportFileResult
		if err := json.Unmarshal(row.Result, &outcome); err != nil || outcome.Position != row.Position {
			return record, scriptapp.ErrUnavailable
		}
		if _, exists := latest[row.Position]; !exists {
			latest[row.Position] = outcome
			latestAttempts[row.Position] = row.Attempt
		}
		if row.Attempt == state.Attempt {
			record.Results = append(record.Results, outcome)
		}
	}
	var publications []scriptImportPublicationRow
	if err := tx.Where("job_id = ?", id).Order("attempt ASC").Find(&publications).Error; err != nil {
		return record, err
	}
	published := make(map[uuid.UUID]bool)
	for _, publication := range publications {
		var receipt scriptapp.SourceReceipt
		if err := json.Unmarshal(publication.Response, &receipt); err != nil {
			return record, scriptapp.ErrUnavailable
		}
		for _, mapping := range receipt.Mappings {
			if mapping.NewSourceID != nil && *mapping.NewSourceID == mapping.LineageID {
				published[mapping.LineageID] = true
			}
		}
	}
	failures := 0
	for position, file := range files {
		if file.Position != position || file.ProjectID != project {
			return record, scriptapp.ErrUnavailable
		}
		record.Files = append(record.Files, scriptapp.FrozenImportFile{
			Position: position,
			SourceID: file.SourceID,
			Source: mediaapp.DocumentSource{
				AssetID: file.AssetID, ProjectID: file.ProjectID, Revision: file.MediaRevision,
				SHA256: file.SHA256, ByteSize: file.ByteSize, MIME: file.MIME, FileName: file.FileName,
			},
		})
		fileView := scriptapp.ImportFile{
			Position: position, AssetID: file.AssetID, FileName: file.FileName,
			Status: domain.ImportFileStatusQueued, Warnings: []scriptapp.ExtractionWarning{}, Attempt: state.Attempt,
		}
		if outcome, ok := latest[position]; ok {
			fileView.Status = outcome.Status
			fileView.FailureCode = outcome.FailureCode
			fileView.Warnings = outcome.Warnings
			fileView.Attempt = latestAttempts[position]
		}
		if published[file.SourceID] {
			fileView.Status = domain.ImportFileStatusSucceeded
			sourceID := file.SourceID
			fileView.SourceID = &sourceID
			fileView.LineageID = &sourceID
		} else if slices.Contains(attempt.Positions, position) && latestAttempts[position] != state.Attempt && (result.Status == domain.ImportStatusQueued || result.Status == domain.ImportStatusRunning) {
			fileView.Status = domain.ImportFileStatusQueued
			fileView.FailureCode = ""
			fileView.Attempt = state.Attempt
		}
		if fileView.Status == domain.ImportFileStatusFailed {
			failures++
		}
		result.Files = append(result.Files, fileView)
	}
	result.Retryable = actor.ID == job.ActorID && domain.CanRetryImport(result.Status, result.ActiveIO, state.IOState == domain.ImportIOStateUnknown, result.NeedsReconciliation, failures)
	result.CanControl = result.Status != domain.ImportStatusSucceeded && result.Status != domain.ImportStatusCancelled
	record.Job = result
	return record, nil
}

func updateImportState(tx *gorm.DB, job scriptapp.ImportJob, owner *uuid.UUID, ioState domain.ImportIOState) error {
	result := tx.Model(&scriptImportStateRow{}).
		Where("job_id = ? AND revision = ?", job.ID, job.Revision-1).
		Updates(map[string]any{
			"revision": job.Revision, "attempt": job.Attempt, "status": job.Status, "stage": job.Stage,
			"latest_script_revision": job.LatestScriptRevision, "latest_version_id": job.LatestVersionID,
			"cancellation_requested": job.CancellationRequested, "reconciliation_requested": job.ReconciliationRequested,
			"needs_reconciliation": job.NeedsReconciliation, "failure_code": job.FailureCode,
			"io_owner_id": owner, "io_state": ioState, "updated_at": job.UpdatedAt,
		})
	return exactlyOne(result)
}

// ImportCommandTopic is the sole new command topic produced by document imports.
const ImportCommandTopic = "glowloom.script.import_command.v1"

func recordImportCommand(tx *gorm.DB, actor identityapp.Principal, key, request uuid.UUID, action, hash string, revision int64, job scriptapp.ImportJob, emit string, now time.Time) error {
	id := uuid.NewSHA1(key, []byte("script-file-import/"+actor.ID.String()+"/"+emit))
	delivery := scriptapp.ImportDelivery{ImportWork: scriptapp.ImportWork{JobID: job.ID, Attempt: job.Attempt}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: job.ProjectID, Action: domain.ImportAction(emit)}
	payload, err := json.Marshal(struct {
		EventID    uuid.UUID                `json:"event_id"`
		EventType  string                   `json:"event_type"`
		OccurredAt time.Time                `json:"occurred_at"`
		OrgID      uuid.UUID                `json:"org_id"`
		Data       scriptapp.ImportDelivery `json:"data"`
	}{id, ImportCommandTopic, now, actor.OrgID, delivery})
	if err != nil {
		return err
	}
	response, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := taskruntime.EnqueueReference(tx.Statement.Context, tx, task.Import, id.String(), now, delivery); err != nil {
		return err
	}
	if err := exactlyOne(tx.Create(&scriptOutboxRow{ID: id, Topic: ImportCommandTopic, PartitionKey: job.ProjectID.String(), Payload: scriptJSON(payload)})); err != nil {
		return err
	}
	command := scriptImportCommandRow{
		ActorID: actor.ID, RequestID: key, OrgID: actor.OrgID, ProjectID: job.ProjectID,
		JobID: job.ID, Action: action, RequestHash: hash, ExpectedRevision: revision,
		EventID: id, EventAction: emit, EventAttempt: job.Attempt, Response: scriptJSON(response), CreatedAt: now,
	}
	if err := exactlyOne(tx.Create(&command)); err != nil {
		return err
	}
	return importAudit(tx, actor, key, request, action, job, now)
}

func importAudit(tx *gorm.DB, actor identityapp.Principal, key, request uuid.UUID, action string, job scriptapp.ImportJob, now time.Time) error {
	const topic = "glowloom.audit.recorded.v1"
	id := uuid.NewSHA1(key, []byte("script-file-import-audit/"+actor.ID.String()+"/"+action))
	summary := map[string]any{"id": job.ID, "revision": job.Revision, "attempt": job.Attempt, "status": job.Status, "stage": job.Stage, "file_count": len(job.Files), "script_revision": job.LatestScriptRevision}
	payload, err := json.Marshal(map[string]any{
		"event_id": id, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": job.ProjectID,
		"actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id},
		"data": map[string]any{"action": "script.file_import_" + action, "object": map[string]any{"type": "script_import", "id": job.ID.String()}, "before": nil, "after": summary, "request_id": request.String()},
	})
	if err != nil {
		return err
	}
	return exactlyOne(tx.Create(&scriptOutboxRow{ID: id, Topic: topic, PartitionKey: job.ProjectID.String(), Payload: scriptJSON(payload)}))
}

func rejectPendingSourceExcept(tx *gorm.DB, org, project, key uuid.UUID) error {
	var count int64
	err := tx.Model(&scriptCommandRow{}).
		Joins("JOIN script.command_state ON script.command_state.actor_id = script.command.actor_id AND script.command_state.request_id = script.command.request_id").
		Where("script.command.org_id = ? AND script.command.project_id = ? AND script.command.request_id <> ? AND script.command_state.status = ?", org, project, key, domain.CommandStatusPending).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return scriptapp.ErrNeedsReconciliation
	}
	return nil
}

func importPositions(tx *gorm.DB, work scriptapp.ImportWork) ([]int, error) {
	var attempt scriptImportAttemptRow
	if err := tx.Where("job_id = ? AND attempt = ?", work.JobID, work.Attempt).Take(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, scriptapp.ErrNotFound
		}
		return nil, err
	}
	return append([]int(nil), attempt.Positions...), nil
}
