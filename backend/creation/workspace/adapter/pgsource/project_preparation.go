package pgsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

var _ workspacesource.ProjectPreparationRepository = (*Store)(nil)

type projectPreparationRow struct {
	PreparationID, SourceCommandID, ActorID, SessionID, IdempotencyKey, RequestID, ScopeID, OrganizationID, ProjectID, CanvasID, InitialCoordinationID uuid.UUID
	SourceSHA256                                                                                                                                       string `gorm:"column:source_sha256"`
	InputSHA256                                                                                                                                        string `gorm:"column:input_sha256"`
	ScopeRevision                                                                                                                                      int64
	Command, ProjectSnapshot, WalletReceipt, ActivationReceipt                                                                                         json.RawMessage
	ActivatedAt                                                                                                                                        *time.Time
}

func projectJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return workspacesource.ErrProjectPreparationConflict
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return workspacesource.ErrProjectPreparationConflict
	}
	return nil
}

func (s *Store) projectTransaction(ctx context.Context, run func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return writefence.ErrUnavailable
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrInvalid
		}
		return run(tx)
	})
}

func readProjectPreparation(tx *gorm.DB, id uuid.UUID, update bool) (workspacesource.ProjectPreparation, error) {
	var row projectPreparationRow
	lock := " FOR SHARE"
	if update {
		lock = " FOR UPDATE"
	}
	read := tx.Raw(`SELECT preparation_id,source_command_id,source_sha256,input_sha256,command,actor_id,session_id,idempotency_key,request_id,scope_id,organization_id,scope_revision,project_id,canvas_id,project_snapshot,initial_coordination_id,wallet_receipt,activation_receipt,activated_at FROM workspace.project_preparation WHERE preparation_id=?`+lock, id).Scan(&row)
	if read.Error != nil {
		return workspacesource.ProjectPreparation{}, read.Error
	}
	if read.RowsAffected != 1 {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationUnknown
	}
	spec := workspacesource.ProjectPreparationSpec{PreparationID: row.PreparationID, ActorID: row.ActorID, SessionID: row.SessionID, IdempotencyKey: row.IdempotencyKey, RequestID: row.RequestID, ScopeID: row.ScopeID, ScopeRevision: row.ScopeRevision, InputSHA256: row.InputSHA256}
	if projectJSON(row.Command, &spec.Command) != nil || projectJSON(row.ProjectSnapshot, &spec.Project) != nil || spec.Validate() != nil || spec.Command.ID != row.SourceCommandID || spec.Command.SHA256 != row.SourceSHA256 || spec.Project.ID != row.ProjectID || spec.Project.CanvasID != row.CanvasID || spec.Project.OrgID != row.OrganizationID || workspacesource.InitialProjectControl(spec).CoordinationID != row.InitialCoordinationID {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	hash, err := spec.CommandSHA256()
	if err != nil || hash != spec.Command.SHA256 {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	state := workspacesource.ProjectPreparation{Spec: spec}
	if len(row.WalletReceipt) > 0 {
		state.Budget = new(workspacesource.PreparedProjectBudget)
		if projectJSON(row.WalletReceipt, state.Budget) != nil || state.Budget.Validate() != nil || state.Budget.Request != spec.BudgetRequest() {
			return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
		}
	}
	if len(row.ActivationReceipt) > 0 {
		state.Activation = new(workspacesource.CreatedProject)
		if projectJSON(row.ActivationReceipt, state.Activation) != nil || row.ActivatedAt == nil || state.Budget == nil || !validActivatedProject(spec, *state.Activation) {
			return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
		}
	} else if row.ActivatedAt != nil {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	return state, nil
}

func samePreparationSpec(a, b workspacesource.ProjectPreparationSpec) bool {
	left, le := json.Marshal(a)
	right, re := json.Marshal(b)
	return le == nil && re == nil && bytes.Equal(left, right)
}

func preparationKeyMatches(state workspacesource.ProjectPreparation, key workspacesource.ProjectBudgetPreparation) error {
	if key.Validate() != nil || state.Spec.BudgetRequest() != key {
		return workspacesource.ErrProjectPreparationConflict
	}
	return nil
}

func lockPreparationPolicy(ctx context.Context, tx *gorm.DB, spec workspacesource.ProjectPreparationSpec, revision bool) error {
	physical, err := lockScope(ctx, tx, spec.ActorID, spec.ScopeID)
	if err != nil {
		return err
	}
	if physical != spec.Project.OrgID {
		return workspacesource.ErrProjectPreparationConflict
	}
	if revision {
		var row struct{ Revision int64 }
		read := tx.Raw(`SELECT revision FROM workspace.creation_scope WHERE id=? FOR SHARE`, spec.ScopeID).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || row.Revision != spec.ScopeRevision {
			return workspacesource.ErrProjectPreparationConflict
		}
	}
	return nil
}

func requireProjectCreationAdmission(ctx context.Context, tx *gorm.DB, spec workspacesource.ProjectPreparationSpec) error {
	binding, err := spec.Command.AuthorityDigest()
	if err != nil {
		return err
	}
	var row struct {
		RequestSHA256      string `gorm:"column:request_sha256"`
		Action             string
		AuthoritySetSHA256 string `gorm:"column:authority_set_sha256"`
		TargetWriter       string
	}
	read := tx.Raw(`SELECT request_sha256,action,authority_set_sha256,target_writer FROM infra.authority_admission WHERE service_owner=infra.runtime_owner() AND id=?`, spec.Command.ID).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return writefence.ErrFenced
	}
	if row.TargetWriter != "creation" || row.RequestSHA256 != spec.Command.SHA256 || row.Action != spec.Command.Action || row.AuthoritySetSHA256 != binding {
		return workspacesource.ErrProjectPreparationConflict
	}
	return pgfence.New(tx).Guard(ctx, tx, spec.Command)
}

// PrepareProject atomically retains the true hidden project/canvas, original
// responsibility and existing task work. It never accesses an Execution table.
func (s *Store) PrepareProject(ctx context.Context, spec workspacesource.ProjectPreparationSpec) (workspacesource.ProjectPreparation, error) {
	hash, err := spec.CommandSHA256()
	if spec.Validate() != nil || err != nil || hash != spec.Command.SHA256 {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	var result workspacesource.ProjectPreparation
	err = s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69360,hashtext(?))`, spec.ActorID.String()+":"+spec.IdempotencyKey.String()).Error; err != nil {
			return err
		}
		existing, err := readProjectPreparation(tx, spec.PreparationID, true)
		if err == nil {
			if !samePreparationSpec(existing.Spec, spec) {
				return workspacesource.ErrProjectPreparationConflict
			}
			if err := lockPreparationPolicy(ctx, tx, spec, false); err != nil {
				return err
			}
			result = existing
			return nil
		}
		if !errors.Is(err, workspacesource.ErrProjectPreparationUnknown) {
			return err
		}
		if err := lockPreparationPolicy(ctx, tx, spec, true); err != nil {
			return err
		}
		if err := requireProjectCreationAdmission(ctx, tx, spec); err != nil {
			return err
		}
		if err := requirePreparedPreset(tx, spec); err != nil {
			return err
		}
		project := spec.Project
		var subtype, preset any
		if project.StyleSubtype != "" {
			subtype = project.StyleSubtype
		}
		if project.StylePresetID != uuid.Nil {
			preset = project.StylePresetID
		}
		if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,initial_canvas_id,independent_canvas,name,description,aspect_ratio,style_type,style_subtype,style_preset_id,style_key,resolution,status,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'copying',1)`, project.ID, project.OrgID, project.OwnerID, spec.ScopeID, project.CanvasID, project.IndependentCanvas, project.Name, project.Description, project.AspectRatio, project.StyleType, subtype, preset, project.StyleKey, project.Resolution).Error; err != nil {
			return fmt.Errorf("prepare hidden project: %w", err)
		}
		if err := tx.Exec(`INSERT INTO canvas.document(id,project_id,name,viewport) VALUES(?,?,?,'{"x":0,"y":0,"zoom":1}'::jsonb)`, project.CanvasID, project.ID, project.Name).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) SELECT id,create_time,'created' FROM workspace.project WHERE id=?`, project.ID).Error; err != nil {
			return err
		}
		command, _ := json.Marshal(spec.Command)
		snapshot, _ := json.Marshal(project)
		if err := tx.Exec(`INSERT INTO workspace.project_preparation(preparation_id,source_command_id,source_sha256,input_sha256,command,actor_id,session_id,idempotency_key,request_id,scope_id,organization_id,scope_revision,project_id,canvas_id,project_snapshot,initial_coordination_id) VALUES(?,?,?,?,?::jsonb,?,?,?,?,?,?,?,?,?,?::jsonb,?)`, spec.PreparationID, spec.Command.ID, spec.Command.SHA256, spec.InputSHA256, string(command), spec.ActorID, spec.SessionID, spec.IdempotencyKey, spec.RequestID, spec.ScopeID, project.OrgID, spec.ScopeRevision, project.ID, project.CanvasID, string(snapshot), workspacesource.InitialProjectControl(spec).CoordinationID).Error; err != nil {
			return err
		}
		if err := taskruntime.EnqueueReference(ctx, tx, workspacesource.ProjectPreparationTask, spec.PreparationID.String(), time.Now(), map[string]any{"preparation_id": spec.PreparationID}); err != nil {
			return err
		}
		eventID := uuid.NewSHA1(spec.PreparationID, []byte("creation.project.prepared"))
		key, _ := json.Marshal(map[string]any{"event_id": eventID, "event_type": "glowloom.creation.project_prepared", "occurred_at": time.Now().UTC(), "org_id": project.OrgID, "project_id": project.ID, "data": spec.BudgetRequest()})
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, eventID, "glowloom.creation.project_prepared", project.ID.String(), string(key)).Error; err != nil {
			return err
		}
		result = workspacesource.ProjectPreparation{Spec: spec}
		return nil
	})
	return result, err
}

func requirePreparedPreset(tx *gorm.DB, spec workspacesource.ProjectPreparationSpec) error {
	project := spec.Project
	if project.StylePresetID == uuid.Nil {
		return nil
	}
	var row struct {
		StyleType    string
		StyleSubtype *string
	}
	read := tx.Raw(`SELECT style_type,style_subtype FROM workspace.style_preset WHERE owner_user_id IS NULL AND id=? AND org_id=? AND owner_scope_id=? AND NOT is_delete AND (project_id IS NULL OR project_id=?) FOR SHARE`, project.StylePresetID, project.OrgID, spec.ScopeID, project.ID).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return workspacesource.ErrStylePresetNotFound
	}
	subtype := ""
	if row.StyleSubtype != nil {
		subtype = *row.StyleSubtype
	}
	if row.StyleType != project.StyleType || subtype != project.StyleSubtype {
		return workspacesource.ErrStylePresetMismatch
	}
	return nil
}

// ReadProjectPreparation proves only exact persisted source facts, including the
// original hidden project/canvas binding. It cannot authorize spending or writes.
func (s *Store) ReadProjectPreparation(ctx context.Context, key workspacesource.ProjectBudgetPreparation) (workspacesource.ProjectPreparation, error) {
	if key.Validate() != nil {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	var state workspacesource.ProjectPreparation
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		var err error
		state, err = readProjectPreparation(tx, key.PreparationID, false)
		if err != nil {
			return err
		}
		if err := preparationKeyMatches(state, key); err != nil {
			return err
		}
		return checkPreparedResources(tx, state)
	})
	return state, err
}

func checkPreparedResources(tx *gorm.DB, state workspacesource.ProjectPreparation) error {
	var row struct {
		Status   string
		IsDelete bool
	}
	project := state.Spec.Project
	query := `SELECT p.status,p.is_delete FROM workspace.project p JOIN canvas.document c ON c.id=p.initial_canvas_id AND c.project_id=p.id WHERE p.id=? AND p.org_id=? AND p.owner_scope_id=? AND p.owner_id=? AND c.id=?`
	args := []any{project.ID, project.OrgID, state.Spec.ScopeID, state.Spec.ActorID, project.CanvasID}
	if state.Activation == nil {
		var subtype, preset any
		if project.StyleSubtype != "" {
			subtype = project.StyleSubtype
		}
		if project.StylePresetID != uuid.Nil {
			preset = project.StylePresetID
		}
		query += ` AND NOT p.is_delete AND p.status='copying' AND p.revision=1
		 AND p.name=? AND p.description=? AND p.aspect_ratio=? AND p.style_type=?
		 AND p.style_subtype IS NOT DISTINCT FROM ?::text AND p.style_preset_id IS NOT DISTINCT FROM ?::uuid
		 AND p.style_key IS NOT DISTINCT FROM ?::text AND p.resolution='1080p'
		 AND p.independent_canvas=? AND NOT p.independent_media AND NOT p.allow_overseas_models
		 AND p.cover_asset_id IS NULL AND p.archived_at IS NULL AND p.delete_time IS NULL AND p.purge_after IS NULL
		 AND NOT c.is_delete AND c.name=? AND c.kind='canvas' AND c.schema_version=1 AND c.operation_version=1 AND c.revision=1
		 AND c.scope='{}'::jsonb AND c.viewport='{"x":0,"y":0,"zoom":1}'::jsonb`
		args = append(args, project.Name, project.Description, project.AspectRatio, project.StyleType, subtype, preset, project.StyleKey, project.IndependentCanvas, project.Name)
	}
	read := tx.Raw(query+` FOR SHARE OF p,c`, args...).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || (state.Activation == nil && (row.Status != "copying" || row.IsDelete)) {
		return workspacesource.ErrProjectPreparationConflict
	}
	if state.Activation == nil {
		return requirePreparedPreset(tx, state.Spec)
	}
	return nil
}

func fenceProjectTask(ctx context.Context, tx *gorm.DB, id uuid.UUID, job *taskruntime.Job) error {
	if job == nil {
		return nil
	}
	var reference struct {
		PreparationID uuid.UUID `json:"preparation_id"`
	}
	if job.Kind != workspacesource.ProjectPreparationTask || job.BusinessID != id.String() || projectJSON(job.Reference, &reference) != nil || reference.PreparationID != id {
		return taskruntime.ErrLeaseLost
	}
	return taskruntime.Fence(ctx, tx, *job)
}

// ReadProjectPreparationForTask rejects an expired/replaced task lease before IO.
func (s *Store) ReadProjectPreparationForTask(ctx context.Context, job taskruntime.Job) (workspacesource.ProjectPreparation, error) {
	id, err := uuid.Parse(job.BusinessID)
	if err != nil || id == uuid.Nil {
		return workspacesource.ProjectPreparation{}, taskruntime.ErrLeaseLost
	}
	var state workspacesource.ProjectPreparation
	err = s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if err := fenceProjectTask(ctx, tx, id, &job); err != nil {
			return err
		}
		var err error
		state, err = readProjectPreparation(tx, id, false)
		return err
	})
	return state, err
}

// SaveProjectBudget stores only the exact participant ACK, never a guessed ready.
func (s *Store) SaveProjectBudget(ctx context.Context, key workspacesource.ProjectBudgetPreparation, receipt workspacesource.PreparedProjectBudget, job *taskruntime.Job) (workspacesource.ProjectPreparation, error) {
	if key.Validate() != nil || receipt.Validate() != nil || receipt.Request != key {
		return workspacesource.ProjectPreparation{}, workspacesource.ErrProjectPreparationConflict
	}
	var state workspacesource.ProjectPreparation
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if err := fenceProjectTask(ctx, tx, key.PreparationID, job); err != nil {
			return err
		}
		var err error
		state, err = readProjectPreparation(tx, key.PreparationID, true)
		if err != nil {
			return err
		}
		if err := preparationKeyMatches(state, key); err != nil {
			return err
		}
		if state.Budget != nil {
			if *state.Budget != receipt {
				return workspacesource.ErrProjectPreparationConflict
			}
			return nil
		}
		if err := checkPreparedResources(tx, state); err != nil {
			return err
		}
		raw, _ := json.Marshal(receipt)
		if err := tx.Exec(`UPDATE workspace.project_preparation SET wallet_receipt=?::jsonb WHERE preparation_id=? AND wallet_receipt IS NULL`, string(raw), key.PreparationID).Error; err != nil {
			return err
		}
		state.Budget = &receipt
		return nil
	})
	return state, err
}

// BeginProjectAuthority can initialize only the exact accepted new project after
// wallet readiness. BeginTx(expected epoch 0) refuses every old/higher head.
func (s *Store) BeginProjectAuthority(ctx context.Context, key workspacesource.ProjectBudgetPreparation, job *taskruntime.Job) (writefence.Control, error) {
	var control writefence.Control
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if key.Validate() != nil {
			return workspacesource.ErrProjectPreparationConflict
		}
		if err := fenceProjectTask(ctx, tx, key.PreparationID, job); err != nil {
			return err
		}
		state, err := readProjectPreparation(tx, key.PreparationID, true)
		if err != nil {
			return err
		}
		if err := preparationKeyMatches(state, key); err != nil {
			return err
		}
		if state.Budget == nil {
			return workspacesource.ErrProjectPreparationUnknown
		}
		if err := lockPreparationPolicy(ctx, tx, state.Spec, false); err != nil {
			return err
		}
		if err := requireProjectCreationAdmission(ctx, tx, state.Spec); err != nil {
			return err
		}
		if err := checkPreparedResources(tx, state); err != nil {
			return err
		}
		control = workspacesource.InitialProjectControl(state.Spec)
		expected := control.Authority
		expected.Epoch = 0
		coordination, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, expected, control.CoordinationID, false)
		if err != nil {
			return err
		}
		if coordination.Control != control {
			return workspacesource.ErrProjectPreparationConflict
		}
		return nil
	})
	return control, err
}

// ActivatePreparedProject makes the sole visible decision with original public
// receipt, events and writer Complete in the same local business transaction.
func (s *Store) ActivatePreparedProject(ctx context.Context, key workspacesource.ProjectBudgetPreparation, control writefence.Control, job *taskruntime.Job) (workspacesource.CreatedProject, error) {
	var created workspacesource.CreatedProject
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		if key.Validate() != nil {
			return workspacesource.ErrProjectPreparationConflict
		}
		if err := fenceProjectTask(ctx, tx, key.PreparationID, job); err != nil {
			return err
		}
		state, err := readProjectPreparation(tx, key.PreparationID, true)
		if err != nil {
			return err
		}
		if err := preparationKeyMatches(state, key); err != nil {
			return err
		}
		if control != workspacesource.InitialProjectControl(state.Spec) {
			return workspacesource.ErrProjectPreparationConflict
		}
		if state.Activation != nil {
			created = *state.Activation
			return nil
		}
		if state.Budget == nil {
			return workspacesource.ErrProjectPreparationUnknown
		}
		if err := lockPreparationPolicy(ctx, tx, state.Spec, false); err != nil {
			return err
		}
		if err := requireProjectCreationAdmission(ctx, tx, state.Spec); err != nil {
			return err
		}
		if err := checkPreparedResources(tx, state); err != nil {
			return err
		}
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		read := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='project' AND authority_id=? FOR SHARE`, key.ProjectID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch != 1 || head.CoordinationID != control.CoordinationID || head.Blocked || !head.Complete {
			return writefence.ErrFenced
		}
		coordination, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, control.CoordinationID)
		if err != nil {
			return err
		}
		if coordination.Control != control || !coordination.Complete {
			return writefence.ErrFenced
		}
		var times struct{ CreateTime, UpdateTime time.Time }
		changed := tx.Raw(`UPDATE workspace.project SET status='active',update_time=statement_timestamp() WHERE id=? AND status='copying' AND NOT is_delete RETURNING create_time,update_time`, key.ProjectID).Scan(&times)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return workspacesource.ErrProjectPreparationConflict
		}
		project := state.Spec.Project
		project.Status = domain.ProjectStatusActive
		project.CreateTime = times.CreateTime.UTC()
		project.UpdateTime = times.UpdateTime.UTC()
		created = createdProject(project)
		if !validActivatedProject(state.Spec, created) {
			return workspacesource.ErrProjectPreparationConflict
		}
		events, err := workspacesource.ProjectCreatedEvents(state.Spec.ActorID, project.OrgID, project, state.Spec.RequestID.String(), times.UpdateTime.UTC(), uuid.NewSHA1(state.Spec.PreparationID, []byte("project.changed")), uuid.NewSHA1(state.Spec.PreparationID, []byte("project.created.audit")))
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload)).Error; err != nil {
				return err
			}
		}
		snapshot, _ := json.Marshal(project)
		if err := tx.Exec(`INSERT INTO workspace.project_creation_receipt(actor_id,idem_key,request_hash,project_id,response_body) VALUES(?,?,?,?,?::jsonb)`, state.Spec.ActorID, state.Spec.IdempotencyKey, state.Spec.InputSHA256, project.ID, string(snapshot)).Error; err != nil {
			return err
		}
		receipt, err := json.Marshal(created)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workspace.project_preparation SET activation_receipt=?::jsonb,activated_at=statement_timestamp() WHERE preparation_id=? AND activated_at IS NULL`, string(receipt), state.Spec.PreparationID).Error; err != nil {
			return err
		}
		return pgfence.New(tx).Complete(ctx, tx, state.Spec.Command, receipt)
	})
	return created, err
}

func createdProject(project domain.Project) workspacesource.CreatedProject {
	var preset *uuid.UUID
	if project.StylePresetID != uuid.Nil {
		id := project.StylePresetID
		preset = &id
	}
	return workspacesource.CreatedProject{ID: project.ID, OrgID: project.OrgID, CanvasID: project.CanvasID, StyleKey: project.StyleKey, Name: project.Name, Description: project.Description, AspectRatio: project.AspectRatio, StyleType: project.StyleType, StyleSubtype: project.StyleSubtype, StylePresetID: preset, Resolution: project.Resolution, AllowOverseasModels: project.AllowOverseasModels, Status: project.Status, Revision: project.Revision, CreateTime: project.CreateTime, UpdateTime: project.UpdateTime}
}

func validActivatedProject(spec workspacesource.ProjectPreparationSpec, created workspacesource.CreatedProject) bool {
	expected := spec.Project
	expected.Status = domain.ProjectStatusActive
	expected.CreateTime = created.CreateTime
	expected.UpdateTime = created.UpdateTime
	if created.CreateTime.IsZero() || created.UpdateTime.Before(created.CreateTime) {
		return false
	}
	left, le := json.Marshal(createdProject(expected))
	right, re := json.Marshal(created)
	return le == nil && re == nil && bytes.Equal(left, right)
}
