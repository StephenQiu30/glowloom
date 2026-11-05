package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func (s *SourceStore) sourceWriteRecord(tx *gorm.DB, actor identityapp.Principal, project, id uuid.UUID, lock bool) (application.SourceIntentRecord, error) {
	var state scriptCommandStateRow
	query := tx.Model(&scriptCommandStateRow{}).Where("id = ?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Take(&state).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.SourceIntentRecord{}, application.ErrNotFound
	} else if err != nil {
		return application.SourceIntentRecord{}, err
	}
	var command scriptCommandRow
	if err := tx.Where("actor_id = ? AND request_id = ? AND org_id = ? AND project_id = ?", state.ActorID, state.RequestID, actor.OrgID, project).Take(&command).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.SourceIntentRecord{}, application.ErrNotFound
	} else if err != nil {
		return application.SourceIntentRecord{}, err
	}
	r := application.SourceIntentRecord{IOState: state.IOState, OwnerID: state.IOOwnerID}
	if err := json.Unmarshal(command.Plan, &r.Plan); err != nil {
		return r, application.ErrUnavailable
	}
	if err := loadPlanLocations(tx, &r.Plan); err != nil {
		return r, err
	}
	if r.Plan.ActorID != command.ActorID || r.Plan.OrgID != actor.OrgID || r.Plan.Command.ProjectID != project || sourceIntentID(r.Plan) != id {
		return r, application.ErrUnavailable
	}
	internal, err := importPublicationExists(tx, r.Plan.ActorID, r.Plan.OrgID, project, r.Plan.Command.Key)
	if err != nil {
		return r, err
	}
	if internal != s.importRecovery {
		return r, application.ErrNotFound
	}
	r.View = application.SourceWriteIntent{ID: id, Revision: state.Revision, Action: domain.SourceAction(command.Action), Status: state.Status, ExpectedScriptRevision: r.Plan.Command.ExpectedRevision, BaseVersionID: r.Plan.Command.BaseVersionID, ObjectCount: len(r.Plan.Objects), CancellationRequested: state.CancellationRequested, NeedsReconciliation: state.NeedsReconciliation, ActiveIO: state.IOOwnerID != nil, CanControl: state.Status == scriptCommandPending, CreatedAt: command.CreatedAt.UTC(), UpdatedAt: state.UpdatedAt.UTC()}
	var intents []scriptObjectIntentRow
	if err := tx.Where("actor_id = ? AND request_id = ?", command.ActorID, r.Plan.Command.Key).Order("object_key").Find(&intents).Error; err != nil {
		return r, err
	}
	if len(intents) != len(r.Plan.Objects) {
		return r, application.ErrUnavailable
	}
	keys := make([]string, len(intents))
	for i := range intents {
		keys[i] = intents[i].ObjectKey
	}
	var states []scriptObjectStateRow
	if len(keys) > 0 {
		if err := tx.Where("object_key IN ?", keys).Find(&states).Error; err != nil {
			return r, err
		}
	}
	stateByKey := make(map[string]scriptObjectStateRow, len(states))
	for _, objectState := range states {
		stateByKey[objectState.ObjectKey] = objectState
	}
	for _, object := range intents {
		storedState, ok := stateByKey[object.ObjectKey]
		if !ok || storedState.ObjectLocationID != object.ObjectLocationID || object.ObjectLocationID == uuid.Nil {
			return r, application.ErrUnavailable
		}
		fact := domain.ObjectFact{LocationID: object.ObjectLocationID, Key: object.ObjectKey, SHA256: object.SHA256, ByteSize: object.ByteSize, MIME: object.MIME}
		if !slices.Contains(r.Plan.Objects, fact) {
			return r, application.ErrObjectMismatch
		}
		r.Objects = append(r.Objects, application.SourceObjectState{Fact: fact, PutStarted: storedState.PutStarted, Confirmed: storedState.Confirmed, Removed: storedState.Removed})
		if storedState.Confirmed {
			r.View.ConfirmedObjectCount++
		}
	}
	return r, nil
}

func importPublicationExists(tx *gorm.DB, actor, org, project, publicationKey uuid.UUID) (bool, error) {
	var jobs []uuid.UUID
	if err := tx.Model(&scriptImportJobRow{}).Select("id").Where("actor_id = ? AND org_id = ? AND project_id = ?", actor, org, project).Find(&jobs).Error; err != nil {
		return false, err
	}
	if len(jobs) == 0 {
		return false, nil
	}
	var count int64
	err := tx.Model(&scriptImportAttemptRow{}).Where("job_id IN ? AND publication_key = ?", jobs, publicationKey).Count(&count).Error
	return count > 0, err
}

// ListSourceWrites lists current scoped intents so an unknown response can be recovered.
func (s *SourceStore) ListSourceWrites(ctx context.Context, actor identityapp.Principal, project uuid.UUID, after int64, limit int) (application.SourceWritePage, error) {
	result := application.SourceWritePage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Items: []application.SourceWriteIntent{}}
	err := readWithControls(ctx, s.db, s.access, actor, project, func(tx *gorm.DB, canWrite bool) error {
		importAttempts := tx.Model(&scriptImportAttemptRow{}).
			Select("script.import_attempt.publication_key").
			Joins("JOIN script.import_job ON script.import_job.id = script.import_attempt.job_id").
			Where("script.import_job.org_id = ? AND script.import_job.project_id = ? AND script.import_job.actor_id = script.command.actor_id", actor.OrgID, project)
		commands := tx.Model(&scriptCommandRow{}).
			Select("actor_id, request_id, created_at").
			Where("org_id = ? AND project_id = ?", actor.OrgID, project)
		if s.importRecovery {
			commands = commands.Where("request_id IN (?)", importAttempts)
		} else {
			commands = commands.Where("request_id NOT IN (?)", importAttempts)
		}
		var commandsPage []scriptCommandRow
		if err := commands.Order("created_at DESC, request_id DESC").Offset(int(after)).Limit(limit + 1).Find(&commandsPage).Error; err != nil {
			return err
		}
		if len(commandsPage) > limit {
			commandsPage = commandsPage[:limit]
			next := after + int64(limit)
			result.NextAfter = &next
		}
		for _, command := range commandsPage {
			var state scriptCommandStateRow
			if err := tx.Where("actor_id = ? AND request_id = ?", command.ActorID, command.RequestID).Take(&state).Error; err != nil {
				return err
			}
			r, err := s.sourceWriteRecord(tx, actor, project, state.ID, false)
			if err != nil {
				return err
			}
			r.View.CanControl = r.View.CanControl && canWrite
			result.Items = append(result.Items, r.View)
		}
		return nil
	})
	return result, err
}

// SourceWrite never exposes the private plan through the public handler.
func (s *SourceStore) SourceWrite(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (application.SourceIntentRecord, error) {
	var result application.SourceIntentRecord
	err := readWithControls(ctx, s.db, s.access, actor, project, func(tx *gorm.DB, canWrite bool) error {
		var err error
		result, err = s.sourceWriteRecord(tx, actor, project, id, false)
		result.View.CanControl = result.View.CanControl && canWrite
		return err
	})
	return result, err
}

func controlHash(input application.SourceControlCommand) (string, error) {
	input.RequestID = uuid.Nil
	data, err := json.Marshal(input)
	return domain.ContentSHA(data), err
}
func (s *SourceStore) controlProof(tx *gorm.DB, actor identityapp.Principal, input application.SourceControlCommand) (application.SourceIntentRecord, error) {
	hash, err := controlHash(input)
	if err != nil {
		return application.SourceIntentRecord{}, err
	}
	var count int64
	if err := tx.Model(&scriptSourceControlRow{}).Where("actor_id = ? AND request_id = ? AND org_id = ? AND project_id = ? AND intent_id = ? AND action = ? AND expected_revision = ? AND request_hash = ?", actor.ID, input.Key, actor.OrgID, input.ProjectID, input.IntentID, input.Action, input.ExpectedRevision, hash).Count(&count).Error; err != nil {
		return application.SourceIntentRecord{}, err
	}
	if count != 1 {
		return application.SourceIntentRecord{}, application.ErrIdempotencyConflict
	}
	r, err := s.sourceWriteRecord(tx, actor, input.ProjectID, input.IntentID, true)
	if err != nil {
		return r, err
	}
	if err := s.requireImportRecoveryControl(tx, r); err != nil {
		return r, err
	}
	if !r.View.CanControl {
		return r, identityapp.ErrForbidden
	}
	return r, nil
}

// ControlSourceWrite freezes current authorization and the original acceptance in one transaction.
func (s *SourceStore) ControlSourceWrite(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, now time.Time) (application.SourceControlReceipt, error) {
	var result application.SourceControlReceipt
	hash, err := controlHash(input)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := registerCommand(tx, actor, input.ProjectID, input.Key, "source_control_"+string(input.Action), hash, now); err != nil {
			return err
		}
		if err := checkRequestScope(tx, actor, input.ProjectID, input.Key, "source_control_"+string(input.Action), hash); err != nil {
			return err
		}
		r, err := s.sourceWriteRecord(tx, actor, input.ProjectID, input.IntentID, true)
		if err != nil {
			return err
		}
		if err := s.requireImportRecoveryControl(tx, r); err != nil {
			return err
		}
		var prior scriptSourceControlRow
		read := tx.Where("actor_id = ? AND request_id = ?", actor.ID, input.Key).Take(&prior)
		if read.Error == nil {
			return json.Unmarshal(prior.Response, &result)
		}
		if !errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return read.Error
		}
		if r.View.Status != scriptCommandPending || r.View.Revision != input.ExpectedRevision {
			return application.ErrConflict
		}
		if err := registerRequest(tx, actor, input.ProjectID, input.Key, "source_control_"+string(input.Action), hash, now); err != nil {
			return err
		}
		updated := tx.Model(&scriptCommandStateRow{}).
			Where("id = ? AND revision = ? AND status = ?", input.IntentID, input.ExpectedRevision, scriptCommandPending).
			Updates(map[string]any{
				"cancellation_requested": r.View.CancellationRequested || input.Action == domain.SourceControlActionCancel,
				"needs_reconciliation":   r.View.NeedsReconciliation || input.Action == domain.SourceControlActionReconcile,
				"revision":               r.View.Revision + 1,
				"updated_at":             now,
			})
		if err := exactlyOne(updated); err != nil {
			return err
		}
		if (input.Action == domain.SourceControlActionCancel || r.View.CancellationRequested) && len(r.Plan.Objects) > 0 {
			if r.Plan.StorageIntentID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
				return application.ErrUnavailable
			}
			if err := s.locations(tx).Cancel(ctx, r.Plan.StorageIntentID); err != nil {
				return err
			}
		}
		result = application.SourceControlReceipt{IntentID: input.IntentID, Revision: r.View.Revision + 1, Action: input.Action, Accepted: true}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		receipt := scriptSourceControlRow{ActorID: actor.ID, RequestID: input.Key, OrgID: actor.OrgID, ProjectID: input.ProjectID, IntentID: input.IntentID, Action: string(input.Action), ExpectedRevision: input.ExpectedRevision, RequestHash: hash, Response: scriptJSON(response), CreatedAt: now}
		if err := exactlyOne(tx.Create(&receipt)); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.source_write_"+string(input.Action), r.Plan.Command.ExpectedRevision, r.Plan.Version.ID, nil, now)
	})
	return result, err
}

// ProveSourceStop requires the exact current controller and the joined I/O nonce.
func (s *SourceStore) ProveSourceStop(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, owner uuid.UUID, now time.Time) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		r, err := s.controlProof(tx, actor, input)
		if err != nil {
			return err
		}
		if r.OwnerID == nil && r.IOState == scriptIOEnded {
			return nil
		}
		if r.OwnerID == nil || *r.OwnerID != owner {
			return application.ErrNeedsReconciliation
		}
		updated := tx.Model(&scriptCommandStateRow{}).Where("id = ? AND io_owner_id = ?", input.IntentID, owner).
			Updates(map[string]any{"io_owner_id": nil, "io_state": scriptIOEnded, "revision": r.View.Revision + 1, "updated_at": now})
		return exactlyOne(updated)
	})
}

func (s *SourceStore) authorizeRemoval(tx *gorm.DB, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact) error {
	r, err := s.controlProof(tx, actor, input)
	if err != nil {
		return err
	}
	if r.View.Status != scriptCommandPending || !r.View.CancellationRequested || r.OwnerID != nil || (r.IOState != scriptIOIdle && r.IOState != scriptIOEnded) {
		return application.ErrNeedsReconciliation
	}
	if err := exactSourceObject(tx, r.Plan, f); err != nil {
		return err
	}
	if f.LocationID == uuid.Nil {
		return application.ErrObjectMismatch
	}
	var sourceReferences, versionReferences int64
	if err := tx.Table("script.script_source").Where("original_location_id = ? OR rich_location_id = ?", f.LocationID, f.LocationID).Count(&sourceReferences).Error; err != nil {
		return err
	}
	if err := tx.Table("script.script_version").Where("text_location_id = ? OR rich_location_id = ?", f.LocationID, f.LocationID).Count(&versionReferences).Error; err != nil {
		return err
	}
	if sourceReferences > 0 || versionReferences > 0 {
		return application.ErrConflict
	}
	return nil
}

// AuthorizeSourceRemoval fences cancellation against publication and all formal history references.
func (s *SourceStore) AuthorizeSourceRemoval(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		return s.authorizeRemoval(tx, actor, input, f)
	})
}

// ConfirmSourceRemoval records only a subsequent actual absence proof.
func (s *SourceStore) ConfirmSourceRemoval(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact, now time.Time) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := s.authorizeRemoval(tx, actor, input, f); err != nil {
			return err
		}
		r, err := s.controlProof(tx, actor, input)
		if err != nil {
			return err
		}
		if r.Plan.StorageIntentID == uuid.Nil || f.LocationID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
			return application.ErrUnavailable
		}
		if err := s.locations(tx).ConfirmRemoval(ctx, r.Plan.StorageIntentID, f); err != nil {
			return err
		}
		updated := tx.Model(&scriptObjectStateRow{}).Where("object_key = ? AND object_location_id = ?", f.Key, f.LocationID).
			Updates(map[string]any{"removed": true, "updated_at": now})
		return exactlyOne(updated)
	})
}

// FinishSourceControl cannot publish old content or remove a fence lacking physical proof.
func (s *SourceStore) FinishSourceControl(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, uncertain bool, now time.Time) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		r, err := s.controlProof(tx, actor, input)
		if err != nil {
			return err
		}
		if r.View.Status == scriptCommandCompleted || r.View.Status == scriptCommandCancelled {
			return nil
		}
		status := scriptCommandPending
		if !uncertain && r.View.CancellationRequested {
			if r.OwnerID != nil || (r.IOState != scriptIOIdle && r.IOState != scriptIOEnded) {
				return application.ErrNeedsReconciliation
			}
			for _, object := range r.Objects {
				if !object.Removed {
					return application.ErrNeedsReconciliation
				}
			}
			status = scriptCommandCancelled
		}
		if !uncertain && !r.View.CancellationRequested {
			for _, object := range r.Objects {
				if !object.Confirmed {
					return application.ErrNeedsReconciliation
				}
			}
		}
		code := ""
		if uncertain {
			code = "source_write_needs_reconciliation"
		}
		updated := tx.Model(&scriptCommandStateRow{}).Where("id = ? AND status = ?", input.IntentID, scriptCommandPending).
			Updates(map[string]any{"status": status, "needs_reconciliation": uncertain, "failure_code": code, "revision": r.View.Revision + 1, "updated_at": now})
		if err := exactlyOne(updated); err != nil {
			return fmt.Errorf("finish source control: %w", err)
		}
		return nil
	})
}

// BeginSourceRemoval freezes the verified digest before any physical delete.
func (s *SourceStore) BeginSourceRemoval(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact, now time.Time) error {
	ready := true
	err := s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := s.authorizeRemoval(tx, actor, input, f); err != nil {
			return err
		}
		r, err := s.controlProof(tx, actor, input)
		if err != nil {
			return err
		}
		if r.Plan.StorageIntentID == uuid.Nil || f.LocationID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
			return application.ErrUnavailable
		}
		ready, err = s.locations(tx).BeginRemoval(ctx, r.Plan.StorageIntentID, f)
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
		updated := tx.Model(&scriptObjectStateRow{}).Where("object_key = ? AND object_location_id = ? AND removed = ?", f.Key, f.LocationID, false).
			Updates(map[string]any{"confirmed": true, "updated_at": now})
		return exactlyOne(updated)
	})
	if err != nil {
		return err
	}
	if !ready {
		return application.ErrNeedsReconciliation
	}
	return nil
}

// NewImportRecoveryStore exposes internal publication only to the durable import worker.
// Ordinary Source8 queries and controls exclude this private command namespace.
func NewImportRecoveryStore(db *gorm.DB, access ProjectAccessFactory) *SourceStore {
	return &SourceStore{db: db, access: access, importRecovery: true}
}

func (s *SourceStore) requireImportRecoveryControl(tx *gorm.DB, r application.SourceIntentRecord) error {
	if !s.importRecovery {
		return nil
	}
	var jobs []scriptImportJobRow
	if err := tx.Where("actor_id = ? AND org_id = ? AND project_id = ?", r.Plan.ActorID, r.Plan.OrgID, r.Plan.Command.ProjectID).Find(&jobs).Error; err != nil {
		return err
	}
	jobIDs := make([]uuid.UUID, len(jobs))
	for i := range jobs {
		jobIDs[i] = jobs[i].ID
	}
	if len(jobIDs) == 0 {
		return application.ErrConflict
	}
	var attempts []scriptImportAttemptRow
	if err := tx.Where("job_id IN ? AND publication_key = ?", jobIDs, r.Plan.Command.Key).Find(&attempts).Error; err != nil {
		return err
	}
	var states []scriptImportStateRow
	if err := tx.Where("job_id IN ? AND cancellation_requested = ?", jobIDs, true).Find(&states).Error; err != nil {
		return err
	}
	allowed := false
	for _, state := range states {
		for _, attempt := range attempts {
			if state.JobID == attempt.JobID && state.Attempt == attempt.Attempt {
				allowed = true
				break
			}
		}
		if allowed {
			break
		}
	}
	if !allowed {
		return application.ErrConflict
	}
	return nil
}
