package pgsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

var _ source.ProjectClosureStore = (*Store)(nil)

// The caller retains the original project SHARE lock. Gate mutation requires
// the same project's UPDATE lock, so no new registration crosses the decision.
func checkNewProjectAdmission(tx *gorm.DB, request source.AdmissionRequest) error {
	var blocked bool
	read := tx.Raw(`SELECT blocked FROM workspace.project_admission_gate WHERE project_id=?`, request.ProjectID).Scan(&blocked)
	if read.Error != nil {
		return read.Error
	}
	if !blocked {
		return nil
	}
	if err := exactCreationAdmission(tx, request.Command, request.Target); err != nil {
		if errors.Is(err, writefence.ErrNotFound) {
			return source.ErrProjectClosureBusy
		}
		return err
	}
	return nil
}

type closureRow struct {
	ID, ActorID, SessionID, ScopeID, OrgID, ProjectID, IdempotencyKey uuid.UUID
	SourceCommand, Spec, Result                                       json.RawMessage
	SourceSHA256                                                      string
	Cycle                                                             int64
	Phase, LedgerSHA256                                               string
}
type closureGate struct {
	CloseID uuid.UUID
	Cycle   int64
	Blocked bool
}
type closureReference struct {
	CloseID uuid.UUID `json:"close_id"`
	Cycle   int64     `json:"cycle"`
}
type closureAdmission struct {
	ID                                        uuid.UUID
	RequestSHA256, Action, AuthoritySetSHA256 string
	Authorities                               json.RawMessage
	TargetWriter                              writefence.Writer
}

func readClosure(tx *gorm.DB, id uuid.UUID, update bool) (source.ProjectClosureState, error) {
	var row closureRow
	lock := ""
	if update {
		lock = " FOR UPDATE"
	}
	read := tx.Raw(`SELECT id,actor_id,session_id,scope_id,org_id,project_id,idempotency_key,source_command,source_sha256,spec,cycle,phase,ledger_sha256,result FROM workspace.project_closure WHERE id=?`+lock, id).Scan(&row)
	if read.Error != nil {
		return source.ProjectClosureState{}, read.Error
	}
	if read.RowsAffected != 1 {
		return source.ProjectClosureState{}, writefence.ErrNotFound
	}
	var spec source.ProjectClosureSpec
	var command writefence.Command
	if projectJSON(row.Spec, &spec) != nil || projectJSON(row.SourceCommand, &command) != nil || spec.Validate() != nil || spec.ID != row.ID || spec.ActorID != row.ActorID || spec.SessionID != row.SessionID || spec.ScopeID != row.ScopeID || spec.OrganizationID != row.OrgID || spec.ProjectID != row.ProjectID || spec.IdempotencyKey != row.IdempotencyKey || spec.Command.SHA256 != row.SourceSHA256 {
		return source.ProjectClosureState{}, writefence.ErrConflict
	}
	original, _ := json.Marshal(spec.Command)
	encoded, _ := json.Marshal(command)
	if !bytes.Equal(original, encoded) {
		return source.ProjectClosureState{}, writefence.ErrConflict
	}
	state := source.ProjectClosureState{Spec: spec, Cycle: row.Cycle, LedgerSHA256: row.LedgerSHA256, AuthorityStarted: row.Phase == "closing" || row.Phase == "closed", Aborting: row.Phase == "aborting", Aborted: row.Phase == "aborted"}
	var gate closureGate
	if err := tx.Raw(`SELECT close_id,cycle,blocked FROM workspace.project_admission_gate WHERE project_id=?`, spec.ProjectID).Scan(&gate).Error; err != nil {
		return state, err
	}
	state.GateClosed = gate.CloseID == spec.ID && gate.Cycle == state.Cycle && gate.Blocked
	if len(row.Result) > 0 {
		if row.Phase != "closed" {
			return state, writefence.ErrConflict
		}
		if err := decodeClosedDetails(row.Result, &state); err != nil {
			return state, err
		}
	}
	return state, nil
}
func closureBusinessID(id uuid.UUID, cycle int64) string { return fmt.Sprintf("%s/%d", id, cycle) }
func fenceClosureJob(ctx context.Context, tx *gorm.DB, state source.ProjectClosureState, job *taskruntime.Job) error {
	if job == nil {
		return nil
	}
	var reference closureReference
	if job.Kind != source.ProjectClosureTask || job.BusinessID != closureBusinessID(state.Spec.ID, state.Cycle) || projectJSON(job.Reference, &reference) != nil || reference.CloseID != state.Spec.ID || reference.Cycle != state.Cycle {
		return taskruntime.ErrLeaseLost
	}
	return taskruntime.Fence(ctx, tx, *job)
}
func currentClosureJob(ctx context.Context) *taskruntime.Job {
	job, ok := source.ProjectClosureJob(ctx)
	if !ok {
		return nil
	}
	return &job
}
func lockClosureProject(ctx context.Context, tx *gorm.DB, spec source.ProjectClosureSpec) (domain.Project, error) {
	physical, err := lockScope(ctx, tx, spec.ActorID, spec.ScopeID)
	if err != nil {
		return domain.Project{}, err
	}
	if physical != spec.OrganizationID {
		return domain.Project{}, source.ErrProjectNotFound
	}
	var scope struct{ OwnerScopeID uuid.UUID }
	read := tx.Raw(`SELECT owner_scope_id FROM workspace.project WHERE id=? AND org_id=? FOR UPDATE`, spec.ProjectID, spec.OrganizationID).Scan(&scope)
	if read.Error != nil {
		return domain.Project{}, read.Error
	}
	if read.RowsAffected != 1 || scope.OwnerScopeID != spec.ScopeID {
		return domain.Project{}, source.ErrProjectNotFound
	}
	var project domain.Project
	read = tx.Raw(`SELECT id,org_id,owner_id,initial_canvas_id AS canvas_id,style_key,name,description,aspect_ratio,style_type,COALESCE(style_subtype,'') AS style_subtype,style_preset_id,resolution,allow_overseas_models,status,archived_at,is_delete,delete_time,purge_after,revision,create_time,update_time,cover_asset_id,independent_canvas,independent_media FROM workspace.project WHERE id=?`, spec.ProjectID).Scan(&project)
	if read.Error != nil {
		return project, read.Error
	}
	if project.Validate() != nil {
		return project, writefence.ErrConflict
	}
	if (project.CoverAssetID == nil) != (spec.ExpectedCoverAssetID == nil) || (project.CoverAssetID != nil && *project.CoverAssetID != *spec.ExpectedCoverAssetID) {
		return project, writefence.ErrConflict
	}
	if spec.ExpectedProjectStatus!=""&&project.Status!=spec.ExpectedProjectStatus{return project,domain.ErrProjectStateConflict}
	if project.Revision != spec.ExpectedRevision {
		return project, domain.ErrProjectRevisionConflict
	}
	if project.IsDelete || project.Status == domain.ProjectStatusCopying {
		return project, domain.ErrProjectStateConflict
	}
	return project, nil
}
func checkClosureGate(tx *gorm.DB, state source.ProjectClosureState) error {
	var gate closureGate
	read := tx.Raw(`SELECT close_id,cycle,blocked FROM workspace.project_admission_gate WHERE project_id=?`, state.Spec.ProjectID).Scan(&gate)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !gate.Blocked || gate.CloseID != state.Spec.ID || gate.Cycle != state.Cycle {
		return taskruntime.ErrLeaseLost
	}
	return nil
}
func exactCreationAdmission(tx *gorm.DB, command writefence.Command, target writefence.Writer) error {
	var row closureAdmission
	read := tx.Raw(`SELECT id,request_sha256,action,authority_set_sha256,authorities,target_writer FROM infra.authority_admission WHERE service_owner='creation' AND id=?`, command.ID).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return writefence.ErrNotFound
	}
	binding, err := command.AuthorityDigest()
	if err != nil || row.TargetWriter != target || row.RequestSHA256 != command.SHA256 || row.Action != command.Action || row.AuthoritySetSHA256 != binding {
		return writefence.ErrConflict
	}
	return nil
}
func projectLedgerSelector(project uuid.UUID) string {
	encoded, _ := json.Marshal([]map[string]any{{"issuer": writefence.Creation, "kind": writefence.Project, "id": project}})
	return string(encoded)
}
func decodeClosureAdmission(row closureAdmission) (writefence.Command, error) {
	command := writefence.Command{ID: row.ID, SHA256: row.RequestSHA256, Action: row.Action}
	if projectJSON(row.Authorities, &command.Authorities) != nil {
		return command, writefence.ErrConflict
	}
	binding, err := command.AuthorityDigest()
	if err != nil || binding != row.AuthoritySetSHA256 || (row.TargetWriter != writefence.WriterCreation && row.TargetWriter != writefence.WriterExecution && row.TargetWriter != writefence.WriterMedia) {
		return command, writefence.ErrConflict
	}
	return command, nil
}
func closureLedgerSHA(tx *gorm.DB, project uuid.UUID) (string, error) {
	rows, err := tx.Raw(`SELECT id,request_sha256,action,authority_set_sha256,authorities,target_writer FROM infra.authority_admission WHERE service_owner='creation' AND authorities @> ?::jsonb ORDER BY id`, projectLedgerSelector(project)).Rows()
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	hash := sha256.New()
	_, _ = hash.Write([]byte("glowloom.creation.project_ledger\x00"))
	for rows.Next() {
		var row closureAdmission
		if err := tx.ScanRows(rows, &row); err != nil {
			return "", err
		}
		command, err := decodeClosureAdmission(row)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(struct {
			Target  writefence.Writer
			Command writefence.Command
		}{row.TargetWriter, command})
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(encoded)
		_, _ = hash.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// BeginProjectClosure closes only source admission, never a writer generation.
// Existing registered commands remain deliverable under their original authority.
func (s *Store) BeginProjectClosure(ctx context.Context, spec source.ProjectClosureSpec) (source.ProjectClosureState, error) {
	if spec.Validate() != nil {
		return source.ProjectClosureState{}, writefence.ErrInvalid
	}
	var result source.ProjectClosureState
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69360,hashtext(?))`, spec.ActorID.String()+":"+spec.IdempotencyKey.String()).Error; err != nil {
			return err
		}
		state, err := readClosure(tx, spec.ID, true)
		found := err == nil
		if err != nil && !errors.Is(err, writefence.ErrNotFound) {
			return err
		}
		if found {
			left, _ := json.Marshal(state.Spec)
			right, _ := json.Marshal(spec)
			if !bytes.Equal(left, right) {
				return writefence.ErrConflict
			}
			if state.Result != nil {
				result = state
				return nil
			}
		}
		project,err:=lockClosureProject(ctx,tx,spec);if err!=nil{return err}
		if err := exactCreationAdmission(tx, spec.Command, writefence.WriterCreation); err != nil {
			return err
		}
		if err := pgfence.New(tx).Guard(ctx, tx, spec.Command); err != nil {
			return err
		}
		var gate closureGate
		if err := tx.Raw(`SELECT close_id,cycle,blocked FROM workspace.project_admission_gate WHERE project_id=?`, spec.ProjectID).Scan(&gate).Error; err != nil {
			return err
		}
		if gate.Blocked && (gate.CloseID != spec.ID || gate.Cycle != state.Cycle) {
			return source.ErrProjectClosureUnknown
		}
		if found && state.GateClosed {
			result = state
			return nil
		}
		var head struct {
			Epoch             int64
			Blocked, Complete bool
		}
		read := tx.Raw(`SELECT epoch,blocked,complete FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=?`, spec.ProjectID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch != spec.ProjectAuthority.Epoch || (head.Blocked&&!(spec.Action=="delete"&&project.Status==domain.ProjectStatusArchived)) || !head.Complete {
			return writefence.ErrFenced
		}
		ledger, err := closureLedgerSHA(tx, spec.ProjectID)
		if err != nil {
			return err
		}
		cycle := int64(1)
		if found {
			cycle = state.Cycle + 1
			updated := tx.Exec(`UPDATE workspace.project_closure SET cycle=?,phase='draining',ledger_sha256=? WHERE id=? AND cycle=? AND phase='busy'`, cycle, ledger, spec.ID, state.Cycle)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return writefence.ErrConflict
			}
		} else {
			command, _ := json.Marshal(spec.Command)
			encoded, _ := json.Marshal(spec)
			if err := tx.Exec(`INSERT INTO workspace.project_closure(id,actor_id,session_id,scope_id,org_id,project_id,idempotency_key,source_command,source_sha256,spec,cycle,phase,ledger_sha256) VALUES(?,?,?,?,?,?,?,?::jsonb,?,?::jsonb,1,'draining',?)`, spec.ID, spec.ActorID, spec.SessionID, spec.ScopeID, spec.OrganizationID, spec.ProjectID, spec.IdempotencyKey, string(command), spec.Command.SHA256, string(encoded), ledger).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`INSERT INTO workspace.project_admission_gate(project_id,close_id,cycle,blocked) VALUES(?,?,?,true) ON CONFLICT(project_id) DO UPDATE SET close_id=excluded.close_id,cycle=excluded.cycle,blocked=true,updated_at=statement_timestamp()`, spec.ProjectID, spec.ID, cycle).Error; err != nil {
			return err
		}
		if err := taskruntime.EnqueueReference(ctx, tx, source.ProjectClosureTask, closureBusinessID(spec.ID, cycle), time.Now(), closureReference{CloseID: spec.ID, Cycle: cycle}); err != nil {
			return err
		}
		result = source.ProjectClosureState{Spec: spec, Cycle: cycle, LedgerSHA256: ledger, GateClosed: true}
		return nil
	})
	return result, err
}

// ReadProjectClosure returns only the original durable decision for recovery.
func (s *Store) ReadProjectClosure(ctx context.Context, id uuid.UUID) (source.ProjectClosureState, error) {
	var state source.ProjectClosureState
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error { var err error; state, err = readClosure(tx, id, false); return err })
	return state, err
}

// ReadProjectClosureForTask rejects both a replaced cycle and an old lease.
func (s *Store) ReadProjectClosureForTask(ctx context.Context, job taskruntime.Job) (source.ProjectClosureState, error) {
	var reference closureReference
	if projectJSON(job.Reference, &reference) != nil {
		return source.ProjectClosureState{}, taskruntime.ErrLeaseLost
	}
	var state source.ProjectClosureState
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		var err error
		state, err = readClosure(tx, reference.CloseID, false)
		if err != nil {
			return err
		}
		return fenceClosureJob(ctx, tx, state, &job)
	})
	return state, err
}

// ReadProjectClosureCommands pages the complete append-only source ledger.
func (s *Store) ReadProjectClosureCommands(ctx context.Context, state source.ProjectClosureState, target writefence.Writer, after uuid.UUID, limit int) ([]writefence.Command, error) {
	if limit < 1 || limit > source.ProjectWorkPageSize {
		return nil, writefence.ErrInvalid
	}
	var commands []writefence.Command
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, false)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle || current.LedgerSHA256 != state.LedgerSHA256 {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		var rows []closureAdmission
		if err := tx.Raw(`SELECT id,request_sha256,action,authority_set_sha256,authorities,target_writer FROM infra.authority_admission WHERE service_owner='creation' AND authorities @> ?::jsonb AND target_writer=? AND id>? ORDER BY id LIMIT ?`, projectLedgerSelector(state.Spec.ProjectID), target, after, limit).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			command, err := decodeClosureAdmission(row)
			if err != nil {
				return err
			}
			commands = append(commands, command)
		}
		return nil
	})
	return commands, err
}

// ReopenBusyProjectGate leaves all project writer epochs and old jobs intact.
func (s *Store) ReopenBusyProjectGate(ctx context.Context, state source.ProjectClosureState) error {
	return s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, true)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		if _, err := lockClosureProject(ctx, tx, current.Spec); err != nil {
			return err
		}
		updated := tx.Exec(`UPDATE workspace.project_closure SET phase='busy' WHERE id=? AND cycle=? AND phase='draining'`, state.Spec.ID, state.Cycle)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return writefence.ErrConflict
		}
		return tx.Exec(`UPDATE workspace.project_admission_gate SET blocked=false,updated_at=statement_timestamp() WHERE project_id=? AND close_id=? AND cycle=?`, state.Spec.ProjectID, state.Spec.ID, state.Cycle).Error
	})
}

// BeginClosedProjectAuthority runs only after all exact participant responsibility
// reads are idle, rechecking the gate and complete ledger under the project lock.
func (s *Store) BeginClosedProjectAuthority(ctx context.Context, state source.ProjectClosureState) (writefence.Control, error) {
	var control writefence.Control
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, true)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		if _, err := lockClosureProject(ctx, tx, current.Spec); err != nil {
			return err
		}
		ledger, err := closureLedgerSHA(tx, current.Spec.ProjectID)
		if err != nil {
			return err
		}
		if ledger != current.LedgerSHA256 {
			return writefence.ErrConflict
		}
		if err := pgfence.New(tx).Guard(ctx, tx, current.Spec.Command); err != nil {
			return err
		}
		coordination, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, current.Spec.ProjectAuthority, current.Spec.CoordinationID, true)
		if err != nil {
			return err
		}
		control = coordination.Control
		if control != current.Spec.Control() {
			return writefence.ErrConflict
		}
		return tx.Exec(`UPDATE workspace.project_closure SET phase='closing' WHERE id=? AND cycle=? AND phase='draining'`, current.Spec.ID, current.Cycle).Error
	})
	return control, err
}

// CommitClosedProject couples the unique project change, exact original receipt,
// Outbox and final actor/scope Guard in the same local Creation transaction.
func (s *Store) CommitClosedProject(ctx context.Context, state source.ProjectClosureState, control writefence.Control) (domain.Project, error) {
	var result domain.Project
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, true)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if current.Result != nil {
			result = *current.Result
			return nil
		}
		if control != current.Spec.Control() || !current.AuthorityStarted {
			return writefence.ErrConflict
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		project, err := lockClosureProject(ctx, tx, current.Spec)
		if err != nil {
			return err
		}
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		read := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=? FOR UPDATE`, current.Spec.ProjectID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch != control.Epoch || head.CoordinationID != control.CoordinationID || !head.Blocked || !head.Complete {
			return writefence.ErrFenced
		}
		if err := pgfence.New(tx).Guard(ctx, tx, current.Spec.Command); err != nil {
			return err
		}
		before := project
		var now time.Time
		if err := tx.Raw(`SELECT clock_timestamp()`).Scan(&now).Error; err != nil {
			return err
		}
		switch current.Spec.Action {
		case "archive":
			err = project.Archive(now, false)
		case "delete":
			err = project.Delete(now, false)
		default:
			return writefence.ErrInvalid
		}
		if err != nil {
			return err
		}
		updated := tx.Exec(`UPDATE workspace.project SET status=?,is_delete=?,archived_at=?,delete_time=?,purge_after=?,revision=? WHERE id=? AND org_id=? AND revision=?`, project.Status, project.IsDelete, project.ArchivedAt, project.DeleteTime, project.PurgeAfter, project.Revision, project.ID, project.OrgID, before.Revision)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return domain.ErrProjectRevisionConflict
		}
		if err := tx.Raw(`SELECT update_time FROM workspace.project WHERE id=?`, project.ID).Scan(&project.UpdateTime).Error; err != nil {
			return err
		}
		events, err := source.ProjectLifecycleEvents(current.Spec.ActorID, current.Spec.OrganizationID, current.Spec.Action, before, project, current.Spec.RequestID.String(), now, uuid.NewSHA1(current.Spec.ID, []byte("project.changed")), uuid.NewSHA1(current.Spec.ID, []byte("project.lifecycle.audit")))
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload)).Error; err != nil {
				return err
			}
		}
		details, err := freezeClosedDetails(ctx, tx, current, project)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(details)
		if err != nil {
			return err
		}
		receipt, err := writefence.CanonicalReceipt(raw)
		if err != nil {
			return err
		}
		if err := pgfence.New(tx).Complete(ctx, tx, current.Spec.Command, receipt); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workspace.project_closure SET phase='closed',result=?::jsonb,completed_at=statement_timestamp() WHERE id=? AND cycle=? AND phase='closing'`, string(receipt), current.Spec.ID, current.Cycle).Error; err != nil {
			return err
		}
		if err:=tx.Exec(`UPDATE workspace.project_admission_gate SET blocked=false,updated_at=statement_timestamp() WHERE project_id=? AND close_id=? AND cycle=?`,current.Spec.ProjectID,current.Spec.ID,current.Cycle).Error;err!=nil{return err}
		result = project
		return nil
	})
	return result, err
}
