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
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func (s *SourceStore) sourceWriteRecord(tx *gorm.DB, actor identityapp.Principal, project, id uuid.UUID, lock bool) (application.SourceIntentRecord, error) {
	var row struct {
		ID, ActorID                                uuid.UUID
		Revision                                   int64
		Action, Status, Plan, IOState              string
		IOOwnerID                                  *uuid.UUID
		CancellationRequested, NeedsReconciliation bool
		CreatedAt, UpdatedAt                       time.Time
	}
	query := `SELECT s.id,c.actor_id,s.revision,c.action,s.status,c.plan::text AS plan,s.io_state,s.io_owner_id,s.cancellation_requested,s.needs_reconciliation,c.created_at,s.updated_at FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND s.id=?`
	if lock {
		query += ` FOR UPDATE OF s`
	}
	read := tx.Raw(query, actor.OrgID, project, id).Scan(&row)
	if read.Error != nil {
		return application.SourceIntentRecord{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.SourceIntentRecord{}, application.ErrNotFound
	}
	r := application.SourceIntentRecord{IOState: row.IOState, OwnerID: row.IOOwnerID}
	if err := json.Unmarshal([]byte(row.Plan), &r.Plan); err != nil {
		return r, application.ErrUnavailable
	}
	if r.Plan.ActorID != row.ActorID || r.Plan.OrgID != actor.OrgID || r.Plan.Command.ProjectID != project || sourceIntentID(r.Plan) != id {
		return r, application.ErrUnavailable
	}
	var internal bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_attempt a ON a.job_id=j.id WHERE j.actor_id=? AND j.org_id=? AND j.project_id=? AND a.publication_key=?)`, r.Plan.ActorID, r.Plan.OrgID, project, r.Plan.Command.Key).Scan(&internal).Error; err != nil {
		return r, err
	}
	if internal != s.importRecovery {
		return r, application.ErrNotFound
	}
	r.View = application.SourceWriteIntent{ID: id, Revision: row.Revision, Action: row.Action, Status: row.Status, ExpectedScriptRevision: r.Plan.Command.ExpectedRevision, BaseVersionID: r.Plan.Command.BaseVersionID, ObjectCount: len(r.Plan.Objects), CancellationRequested: row.CancellationRequested, NeedsReconciliation: row.NeedsReconciliation, ActiveIO: row.IOOwnerID != nil, CanControl: row.Status == "pending" && (actor.ID == row.ActorID || actor.Role == identitydomain.RoleAdmin), CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	var objects []struct {
		ObjectKey, SHA256, MIME        string
		ByteSize                       int64
		PutStarted, Confirmed, Removed bool
	}
	if err := tx.Raw(`SELECT i.object_key,i.sha256,i.byte_size,i.mime,s.put_started,s.confirmed,s.removed FROM script.object_intent i JOIN script.object_state s USING(object_key) WHERE i.actor_id=? AND i.request_id=? ORDER BY i.object_key`, row.ActorID, r.Plan.Command.Key).Scan(&objects).Error; err != nil {
		return r, err
	}
	if len(objects) != len(r.Plan.Objects) {
		return r, application.ErrUnavailable
	}
	for _, object := range objects {
		fact := domain.ObjectFact{Key: object.ObjectKey, SHA256: object.SHA256, ByteSize: object.ByteSize, MIME: object.MIME}
		if !slices.Contains(r.Plan.Objects, fact) {
			return r, application.ErrObjectMismatch
		}
		r.Objects = append(r.Objects, application.SourceObjectState{Fact: fact, PutStarted: object.PutStarted, Confirmed: object.Confirmed, Removed: object.Removed})
		if object.Confirmed {
			r.View.ConfirmedObjectCount++
		}
	}
	return r, nil
}

// ListSourceWrites lists current scoped intents so an unknown response can be recovered.
func (s *SourceStore) ListSourceWrites(ctx context.Context, actor identityapp.Principal, project uuid.UUID, after int64, limit int) (application.SourceWritePage, error) {
	result := application.SourceWritePage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Items: []application.SourceWriteIntent{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var ids []struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT s.id FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_attempt a ON a.job_id=j.id WHERE j.actor_id=c.actor_id AND j.org_id=c.org_id AND j.project_id=c.project_id AND a.publication_key=c.request_id)=? ORDER BY c.created_at DESC,s.id DESC OFFSET ? LIMIT ?`, actor.OrgID, project, s.importRecovery, after, limit+1).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) > limit {
			ids = ids[:limit]
			next := after + int64(limit)
			result.NextAfter = &next
		}
		for _, id := range ids {
			r, err := s.sourceWriteRecord(tx, actor, project, id.ID, false)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, r.View)
		}
		return nil
	})
	return result, err
}

// SourceWrite never exposes the private plan through the public handler.
func (s *SourceStore) SourceWrite(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (application.SourceIntentRecord, error) {
	var result application.SourceIntentRecord
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var err error
		result, err = s.sourceWriteRecord(tx, actor, project, id, false)
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
	if err := tx.Raw(`SELECT count(*) FROM script.source_control WHERE actor_id=? AND request_id=? AND org_id=? AND project_id=? AND intent_id=? AND action=? AND expected_revision=? AND request_hash=?`, actor.ID, input.Key, actor.OrgID, input.ProjectID, input.IntentID, input.Action, input.ExpectedRevision, hash).Scan(&count).Error; err != nil {
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
		if err := commandLock(tx, actor.ID, input.Key); err != nil {
			return err
		}
		if err := checkRequestScope(tx, actor, input.ProjectID, input.Key, "source_control_"+input.Action, hash); err != nil {
			return err
		}
		r, err := s.sourceWriteRecord(tx, actor, input.ProjectID, input.IntentID, true)
		if err != nil {
			return err
		}
		if err := s.requireImportRecoveryControl(tx, r); err != nil {
			return err
		}
		var prior struct{ Response string }
		read := tx.Raw(`SELECT response::text FROM script.source_control WHERE actor_id=? AND request_id=?`, actor.ID, input.Key).Scan(&prior)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			return json.Unmarshal([]byte(prior.Response), &result)
		}
		if actor.ID != r.Plan.ActorID && actor.Role != identitydomain.RoleAdmin {
			return identityapp.ErrForbidden
		}
		if r.View.Status != "pending" || r.View.Revision != input.ExpectedRevision {
			return application.ErrConflict
		}
		if err := registerRequest(tx, actor, input.ProjectID, input.Key, "source_control_"+input.Action, hash, now); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`UPDATE script.command_state SET cancellation_requested=cancellation_requested OR ?,needs_reconciliation=needs_reconciliation OR ?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND status='pending'`, input.Action == "cancel", input.Action == "reconcile", now, input.IntentID, input.ExpectedRevision)); err != nil {
			return err
		}
		result = application.SourceControlReceipt{IntentID: input.IntentID, Revision: r.View.Revision + 1, Action: input.Action, Accepted: true}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.source_control(actor_id,request_id,org_id,project_id,intent_id,action,expected_revision,request_hash,response,created_at) VALUES(?,?,?,?,?,?,?,?,?::jsonb,?)`, actor.ID, input.Key, actor.OrgID, input.ProjectID, input.IntentID, input.Action, input.ExpectedRevision, hash, string(response), now)); err != nil {
			return err
		}
		return reviewAudit(tx, actor, input.ProjectID, input.Key, input.RequestID, "script.source_write_"+input.Action, r.Plan.Command.ExpectedRevision, r.Plan.Version.ID, nil, now)
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
		if r.OwnerID == nil && r.IOState == "ended" {
			return nil
		}
		if r.OwnerID == nil || *r.OwnerID != owner {
			return application.ErrNeedsReconciliation
		}
		return exactlyOne(tx.Exec(`UPDATE script.command_state SET io_owner_id=NULL,io_state='ended',revision=revision+1,updated_at=? WHERE id=? AND io_owner_id=?`, now, input.IntentID, owner))
	})
}

func (s *SourceStore) authorizeRemoval(tx *gorm.DB, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact) error {
	r, err := s.controlProof(tx, actor, input)
	if err != nil {
		return err
	}
	if r.View.Status != "pending" || !r.View.CancellationRequested || r.OwnerID != nil || (r.IOState != "idle" && r.IOState != "ended") {
		return application.ErrNeedsReconciliation
	}
	if err := exactSourceObject(tx, r.Plan, f); err != nil {
		return err
	}
	var referenced bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.script_source WHERE original_key=? OR rich_key=?) OR EXISTS(SELECT 1 FROM script.script_version WHERE text_key=? OR rich_key=?)`, f.Key, f.Key, f.Key, f.Key).Scan(&referenced).Error; err != nil {
		return err
	}
	if referenced {
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
		return exactlyOne(tx.Exec(`UPDATE script.object_state SET removed=true,updated_at=? WHERE object_key=?`, now, f.Key))
	})
}

// FinishSourceControl cannot publish old content or remove a fence lacking physical proof.
func (s *SourceStore) FinishSourceControl(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, uncertain bool, now time.Time) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		r, err := s.controlProof(tx, actor, input)
		if err != nil {
			return err
		}
		if r.View.Status == "completed" || r.View.Status == "cancelled" {
			return nil
		}
		status := "pending"
		if !uncertain && r.View.CancellationRequested {
			if r.OwnerID != nil || (r.IOState != "idle" && r.IOState != "ended") {
				return application.ErrNeedsReconciliation
			}
			for _, object := range r.Objects {
				if !object.Removed {
					return application.ErrNeedsReconciliation
				}
			}
			status = "cancelled"
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
		if err := exactlyOne(tx.Exec(`UPDATE script.command_state SET status=?,needs_reconciliation=?,failure_code=?,revision=revision+1,updated_at=? WHERE id=? AND status='pending'`, status, uncertain, code, now, input.IntentID)); err != nil {
			return fmt.Errorf("finish source control: %w", err)
		}
		return nil
	})
}

// BeginSourceRemoval freezes the verified digest before any physical delete.
func (s *SourceStore) BeginSourceRemoval(ctx context.Context, actor identityapp.Principal, input application.SourceControlCommand, f domain.ObjectFact, now time.Time) error {
	return s.transaction(ctx, actor, input.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := s.authorizeRemoval(tx, actor, input, f); err != nil {
			return err
		}
		return exactlyOne(tx.Exec(`UPDATE script.object_state SET confirmed=true,updated_at=? WHERE object_key=? AND NOT removed`, now, f.Key))
	})
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
	var allowed bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_state st ON st.job_id=j.id JOIN script.import_attempt a ON a.job_id=j.id AND a.attempt=st.attempt WHERE j.actor_id=? AND j.org_id=? AND j.project_id=? AND a.publication_key=? AND st.cancellation_requested)`, r.Plan.ActorID, r.Plan.OrgID, r.Plan.Command.ProjectID, r.Plan.Command.Key).Scan(&allowed).Error; err != nil {
		return err
	}
	if !allowed {
		return application.ErrConflict
	}
	return nil
}
