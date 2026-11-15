package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// FrozenInputStore owns only Creation data and independent local transactions.
// It never constructs a Principal, checks Identity SQL or reads Media tables.
type FrozenInputStore struct {
	db   *gorm.DB
	pins application.FrozenInputPins
}

// NewFrozenInputStore injects the actual restricted Creation connection and pin verifier.
func NewFrozenInputStore(db *gorm.DB, pins application.FrozenInputPins) *FrozenInputStore {
	return &FrozenInputStore{db, pins}
}

var _ application.FrozenInputStore = (*FrozenInputStore)(nil)

type frozenInputRow struct {
	HandoffID       uuid.UUID
	RequestSHA256   string          `gorm:"column:request_sha256"`
	Snapshot        json.RawMessage `gorm:"type:jsonb"`
	SnapshotSHA256  string          `gorm:"column:snapshot_sha256"`
	AdoptionBlocked bool
	CancelCommand   json.RawMessage `gorm:"type:jsonb"`
	CancelReceipt   json.RawMessage `gorm:"type:jsonb"`
}

func (s *FrozenInputStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return writefence.ErrUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterCreation {
		return domain.ErrFrozenInputOwner
	}
	return nil
}
func frozenRequestHash(req application.FreezeInputRequest) (string, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return "", domain.ErrFrozenInputInvalid
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
func frozenEnvelope(row frozenInputRow) (application.FrozenInputEnvelope, error) {
	var out application.FrozenInputEnvelope
	decoder := json.NewDecoder(bytes.NewReader(row.Snapshot))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out.Input) != nil {
		return out, domain.ErrFrozenInputConflict
	}
	_, hash, err := out.Input.Snapshot()
	if err != nil || hash != row.SnapshotSHA256 || out.Input.HandoffID != row.HandoffID {
		return out, domain.ErrFrozenInputConflict
	}
	out.SnapshotSHA256 = hash
	out.AdoptionBlocked = row.AdoptionBlocked
	if len(row.CancelCommand) > 0 && !bytes.Equal(row.CancelCommand, []byte("null")) {
		out.CancelCommand = new(writefence.Command)
		if json.Unmarshal(row.CancelCommand, out.CancelCommand) != nil {
			return out, domain.ErrFrozenInputConflict
		}
	}
	if len(row.CancelReceipt) > 0 && !bytes.Equal(row.CancelReceipt, []byte("null")) {
		out.CancelProof = new(application.FrozenInputCancelProof)
		if json.Unmarshal(row.CancelReceipt, out.CancelProof) != nil {
			return out, domain.ErrFrozenInputConflict
		}
		if bytes.Equal(out.CancelProof.Original.Receipt, []byte("null")) {
			out.CancelProof.Original.Receipt = nil
		} else if len(out.CancelProof.Original.Receipt) > 0 {
			var compact bytes.Buffer
			if json.Compact(&compact, out.CancelProof.Original.Receipt) != nil {
				return out, domain.ErrFrozenInputConflict
			}
			var err error
			out.CancelProof.Original.Receipt, err = writefence.CanonicalReceipt(compact.Bytes())
			if err != nil {
				return out, domain.ErrFrozenInputConflict
			}
		}
		if application.ValidateFrozenCancelProof(out, *out.CancelProof) != nil {
			return out, domain.ErrFrozenInputConflict
		}
	}
	if out.AdoptionBlocked != (out.CancelCommand != nil) {
		return out, domain.ErrFrozenInputConflict
	}
	return out, nil
}
func readFrozenRow(tx *gorm.DB, id uuid.UUID, lock bool) (frozenInputRow, error) {
	var row frozenInputRow
	query := `SELECT handoff_id,request_sha256,snapshot,snapshot_sha256,adoption_blocked,cancel_command,cancel_receipt FROM workspace.frozen_input_handoff WHERE handoff_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	r := tx.Raw(query, id).Scan(&row)
	if r.Error != nil {
		return row, r.Error
	}
	if r.RowsAffected != 1 {
		return row, domain.ErrFrozenInputNotFound
	}
	return row, nil
}
func readFrozenKey(tx *gorm.DB, key application.FrozenInputKey, lock bool) (application.FrozenInputEnvelope, error) {
	row, err := readFrozenRow(tx, key.HandoffID, lock)
	if err != nil {
		return application.FrozenInputEnvelope{}, err
	}
	out, err := frozenEnvelope(row)
	if err == nil && out.Key() != key {
		err = domain.ErrFrozenInputConflict
	}
	return out, err
}

// FreezeInput requires an existing admitted command. It never opens a policy
// head or registers a new writer command merely because supplied epochs look valid.
func (s *FrozenInputStore) FreezeInput(ctx context.Context, req application.FreezeInputRequest) (application.FrozenInputEnvelope, error) {
	var out application.FrozenInputEnvelope
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	if req.HandoffID == uuid.Nil || req.RequestID == uuid.Nil || req.SourceGeneration < 1 || req.SourceGeneration == math.MaxInt64 || req.ExpectedScopeRevision < 1 || req.ExpectedProjectRevision < 1 || !req.Source.Valid() || req.Command.Action != "execution.frozen_input.accept" || req.SourceCommand.Action != "creation.frozen_input.freeze" || req.SourceCommand.ID != uuid.NewSHA1(req.HandoffID, []byte("creation.frozen_input.freeze")) || req.SourceCommand.ID == req.Command.ID || domain.ValidateFrozenAuthorities(req.SourceCommand, req.ActorID, req.SessionID, req.ScopeID, req.ProjectID) != nil || domain.ValidateFrozenAuthorities(req.Command, req.ActorID, req.SessionID, req.ScopeID, req.ProjectID) != nil || req.Pins == nil {
		return out, domain.ErrFrozenInputInvalid
	}
	sourceAuthorities, _ := req.SourceCommand.AuthorityDigest()
	targetAuthorities, _ := req.Command.AuthorityDigest()
	if sourceAuthorities != targetAuthorities {
		return out, domain.ErrFrozenInputInvalid
	}
	commandHash, err := req.SourceCommandSHA256()
	if err != nil || commandHash != req.SourceCommand.SHA256 {
		return out, domain.ErrFrozenInputConflict
	}
	digest, err := frozenRequestHash(req)
	if err != nil {
		return out, err
	}
	original, err := pgfence.New(s.db).Load(ctx, req.SourceCommand.ID)
	if err != nil {
		return out, err
	}
	if !application.SameFrozenCommand(original.Command, req.SourceCommand) {
		return out, domain.ErrFrozenInputConflict
	}
	if original.State == writefence.Frozen {
		return out, writefence.ErrFrozen
	}
	// Committed replays must not depend on today's mutable sources or a live Media
	// connection. The exact original request is compared inside the local Tx below.
	pins := req.Pins
	if original.State == writefence.Admitted && len(pins) > 0 {
		if s.pins == nil {
			return out, domain.ErrReferenceUnavailable
		}
		pins, err = s.pins.VerifyFrozenInputPins(ctx, req)
		if err != nil {
			return out, err
		}
		a, _ := json.Marshal(pins)
		b, _ := json.Marshal(req.Pins)
		if !bytes.Equal(a, b) {
			return out, domain.ErrFrozenInputConflict
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fences := pgfence.New(s.db)
		err := fences.Guard(ctx, tx, req.SourceCommand)
		if errors.Is(err, writefence.ErrCommitted) {
			row, e := readFrozenRow(tx, req.HandoffID, false)
			if e != nil {
				return e
			}
			if row.RequestSHA256 != digest {
				return domain.ErrFrozenInputConflict
			}
			out, e = frozenEnvelope(row)
			return e
		}
		if err != nil {
			return err
		}
		// A source-wide lock gives generation a durable monotonic order independent
		// of draft revision. It also serializes different IDs racing the same source.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69367,hashtext(?))`, req.ActorID.String()+":"+req.ProjectID.String()+":"+req.Source.OwnerKey()).Error; err != nil {
			return err
		}
		var generation int64
		if err := tx.Raw(`SELECT COALESCE(max(source_generation),0) FROM workspace.frozen_input_handoff WHERE actor_id=? AND project_id=? AND source_key=?`, req.ActorID, req.ProjectID, req.Source.OwnerKey()).Scan(&generation).Error; err != nil {
			return err
		}
		if generation == math.MaxInt64 || req.SourceGeneration != generation+1 {
			return domain.ErrFrozenInputConflict
		}
		input := domain.FrozenInput{HandoffID: req.HandoffID, RequestID: req.RequestID, Command: req.Command, SourceCommand: req.SourceCommand, ActorID: req.ActorID, SessionID: req.SessionID, ScopeID: req.ScopeID, ProjectID: req.ProjectID, Source: req.Source, SourceGeneration: req.SourceGeneration, Pins: pins}
		if err := lockFrozenPolicy(tx, req, &input); err != nil {
			return err
		}
		if err := readFrozenSource(tx, req, &input); err != nil {
			return err
		}
		raw, hash, err := input.Snapshot()
		if err != nil {
			return err
		}
		r := tx.Exec(`INSERT INTO workspace.frozen_input_handoff(handoff_id,request_id,command_id,command_sha256,request_sha256,actor_id,session_id,scope_id,project_id,source_key,source_generation,draft_revision,snapshot,snapshot_sha256) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?) ON CONFLICT DO NOTHING`, req.HandoffID, req.RequestID, req.Command.ID, req.Command.SHA256, digest, req.ActorID, req.SessionID, req.ScopeID, req.ProjectID, req.Source.OwnerKey(), input.SourceGeneration, input.DraftRevision, string(raw), hash)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrFrozenInputConflict
		}
		// Only an authenticated Identity ResolveAdmission -> Creation source-policy
		// pipeline may create this original target admission. A source-local writer
		// command or a targetCreation proof cannot substitute for targetExecution.
		binding, err := req.Command.AuthorityDigest()
		if err != nil {
			return err
		}
		var admission struct {
			RequestSHA256      string `gorm:"column:request_sha256"`
			Action             string
			AuthoritySetSHA256 string `gorm:"column:authority_set_sha256"`
			TargetWriter       string
		}
		admitted := tx.Raw(`SELECT request_sha256,action,authority_set_sha256,target_writer FROM infra.authority_admission WHERE service_owner=infra.runtime_owner() AND id=?`, req.Command.ID).Scan(&admission)
		if admitted.Error != nil {
			return admitted.Error
		}
		if admitted.RowsAffected != 1 {
			return writefence.ErrFenced
		}
		if admission.TargetWriter != "execution" || admission.RequestSHA256 != req.Command.SHA256 || admission.Action != req.Command.Action || admission.AuthoritySetSHA256 != binding {
			return domain.ErrFrozenInputConflict
		}
		// Existing heads must already be complete/open. Register is an idempotent
		// recheck here, never a new target admission or a policy-head bootstrap.
		if err := pgfence.NewAuthorityStore(s.db).Register(ctx, tx, req.Command, writefence.WriterExecution); err != nil {
			return err
		}
		if err := taskruntime.EnqueueReference(ctx, tx, "creation.frozen_input.deliver", req.HandoffID.String(), time.Now(), map[string]any{"handoff_id": req.HandoffID}); err != nil {
			return err
		}
		receipt, _ := json.Marshal(map[string]any{"handoff_id": req.HandoffID, "request_id": req.RequestID, "snapshot_sha256": hash})
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, uuid.NewSHA1(req.HandoffID, []byte("creation.frozen_input.frozen")), "glowloom.creation.frozen_input.frozen", req.ProjectID.String(), string(receipt)).Error; err != nil {
			return err
		}
		if err := fences.Complete(ctx, tx, req.SourceCommand, receipt); err != nil {
			return err
		}
		out = application.FrozenInputEnvelope{Input: input, SnapshotSHA256: hash}
		return nil
	})
	return out, err
}

func lockFrozenPolicy(tx *gorm.DB, req application.FreezeInputRequest, input *domain.FrozenInput) error {
	var scope struct {
		Kind                string
		OwnerUserID, TeamID *uuid.UUID
		PhysicalScopeID     uuid.UUID
		Revision            int64
	}
	r := tx.Raw(`SELECT kind,owner_user_id,team_id,physical_scope_id,revision FROM workspace.creation_scope WHERE id=? FOR SHARE`, req.ScopeID).Scan(&scope)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || (req.ExpectedScopeRevision > 0 && scope.Revision != req.ExpectedScopeRevision) {
		return domain.ErrInputStale
	}
	switch scope.Kind {
	case "personal":
		if scope.OwnerUserID == nil || *scope.OwnerUserID != req.ActorID {
			return domain.ErrReferenceUnavailable
		}
	case "team":
		if scope.TeamID == nil {
			return domain.ErrReferenceUnavailable
		}
		var team struct {
			Status            string
			GovernanceBlocked bool
			AuthEpoch         int64
		}
		r = tx.Raw(`SELECT status,governance_blocked,auth_epoch FROM workspace.team WHERE id=? FOR SHARE`, *scope.TeamID).Scan(&team)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || team.Status != "active" || team.GovernanceBlocked {
			return domain.ErrReferenceUnavailable
		}
		var member struct {
			Status, Role string
			AuthEpoch    int64
		}
		r = tx.Raw(`SELECT status,role,auth_epoch FROM workspace.team_member WHERE team_id=? AND user_id=? FOR SHARE`, *scope.TeamID, req.ActorID).Scan(&member)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || member.Status != "active" || (member.Role != "team_admin" && member.Role != "member") {
			return domain.ErrReferenceUnavailable
		}
		input.TeamEpoch = team.AuthEpoch
		input.MemberEpoch = member.AuthEpoch
	default:
		return domain.ErrReferenceUnavailable
	}
	var organization struct {
		Status   string
		IsDelete bool
	}
	r = tx.Raw(`SELECT status,is_delete FROM workspace.organization WHERE id=? FOR SHARE`, scope.PhysicalScopeID).Scan(&organization)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || organization.Status != "active" || organization.IsDelete {
		return domain.ErrReferenceUnavailable
	}
	var project struct {
		OrgID        uuid.UUID
		OwnerScopeID *uuid.UUID
		Status       string
		AspectRatio  string
		Name         string
		StyleType    string
		IsDelete     bool
		Revision     int64
	}
	r = tx.Raw(`SELECT org_id,owner_scope_id,status,aspect_ratio,name,style_type,is_delete,revision FROM workspace.project WHERE id=? FOR SHARE`, req.ProjectID).Scan(&project)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || project.OwnerScopeID == nil || *project.OwnerScopeID != req.ScopeID || project.OrgID != scope.PhysicalScopeID || project.Status != "active" || project.IsDelete {
		return domain.ErrReferenceUnavailable
	}
	if req.ExpectedProjectRevision > 0 && project.Revision != req.ExpectedProjectRevision {
		return domain.ErrInputStale
	}
	input.OrgID = project.OrgID
	input.AspectRatio = project.AspectRatio
	input.ScopeRevision = scope.Revision
	input.ProjectRevision = project.Revision
	if req.ExpectedScopeRevision > 0 && req.ExpectedProjectRevision > 0 {
		input.CompilationInputs = &domain.FrozenCompilationInputs{CompilerVersion: domain.CompilerVersion, ProjectName: project.Name, StyleType: project.StyleType}
		if req.TemplateSelection != nil {
			if err := captureFrozenTemplate(tx, req, input); err != nil {
				return err
			}
		}
	}
	return nil
}

func readFrozenSource(tx *gorm.DB, req application.FreezeInputRequest, input *domain.FrozenInput) error {
	if req.Source.Kind == "prompt_draft" {
		var row struct {
			Revision             int64
			Draft                []byte
			DraftHash, InputHash string
		}
		r := tx.Raw(`SELECT revision,draft,draft_hash,input_hash FROM workspace.prompt_draft WHERE id=? AND project_id=? AND actor_id=? FOR SHARE`, req.Source.DraftID, req.ProjectID, req.ActorID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.Revision != req.Source.Revision {
			return domain.ErrInputStale
		}
		draft, err := domain.DecodeDraft(row.Draft)
		if err != nil {
			return err
		}
		dh, ih := draft.Hashes()
		if row.DraftHash != dh || row.InputHash != ih {
			return domain.ErrInputStale
		}
		input.Draft = draft
		input.DraftHash = dh
		input.InputHash = ih
		input.DraftRevision = row.Revision
		return nil
	}
	var document struct {
		Revision      int64
		SchemaVersion int
		Scope         json.RawMessage
	}
	r := tx.Raw(`SELECT revision,schema_version,scope FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, req.Source.CanvasID, req.ProjectID).Scan(&document)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || document.Revision != req.Source.CanvasRevision || document.SchemaVersion != 1 || string(document.Scope) != "{}" {
		return domain.ErrInputStale
	}
	graph, err := readCanvasInputGraph(tx, req.Source.CanvasID)
	if err != nil {
		return err
	}
	var row struct {
		InputRevision int64
		Draft         []byte
	}
	r = tx.Raw(`SELECT n.input_revision,i.draft FROM canvas.node n JOIN canvas.node_input i ON i.node_id=n.id AND i.input_revision=n.input_revision WHERE n.id=? AND n.document_id=? AND NOT n.is_delete FOR SHARE OF n`, req.Source.NodeID, req.Source.CanvasID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || row.InputRevision != req.Source.InputRevision {
		return domain.ErrInputStale
	}
	var saved canvasdomain.Draft
	decoder := json.NewDecoder(bytes.NewReader(row.Draft))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&saved) != nil || saved.Composer == nil {
		return domain.ErrInvalidDraft
	}
	for _, node := range graph.Nodes {
		if node.ID != req.Source.NodeID {
			continue
		}
		if node.Config.Draft == nil || !canvasdomain.SameInput(saved, *node.Config.Draft) {
			return domain.ErrInputStale
		}
		effective, err := canvasdomain.GenerationInput(graph, node)
		if err != nil || effective.Composer == nil {
			return domain.ErrInvalidDraft
		}
		input.Draft = *effective.Composer
		input.DraftRevision = row.InputRevision
		input.DraftHash, input.InputHash = input.Draft.Hashes()
		return nil
	}
	return domain.ErrInputStale
}

// ReadFrozenInput returns only the exact original handoff and permanent gate.
func (s *FrozenInputStore) ReadFrozenInput(ctx context.Context, key application.FrozenInputKey) (application.FrozenInputEnvelope, error) {
	if err := s.ready(ctx); err != nil {
		return application.FrozenInputEnvelope{}, err
	}
	return readFrozenKey(s.db.WithContext(ctx), key, false)
}

// BlockFrozenInput commits the permanent prohibition and recovery task before
// contacting Execution. It never freezes the already committed source command.
func (s *FrozenInputStore) BlockFrozenInput(ctx context.Context, key application.FrozenInputKey, command writefence.Command) (application.FrozenInputEnvelope, error) {
	var out application.FrozenInputEnvelope
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	if command.Action != "creation.frozen_input.cancel" || command.ID == key.CommandID {
		return out, domain.ErrFrozenInputInvalid
	}
	binding, err := application.FrozenInputCancellationSHA256(key, command)
	if err != nil || binding != command.SHA256 {
		return out, domain.ErrFrozenInputConflict
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = readFrozenKey(tx, key, true)
		if err != nil {
			return err
		}
		if out.CancelCommand != nil {
			if !application.SameFrozenCommand(*out.CancelCommand, command) {
				return domain.ErrFrozenInputConflict
			}
			return nil
		}
		if domain.ValidateFrozenAuthorities(command, out.Input.ActorID, out.Input.SessionID, out.Input.ScopeID, out.Input.ProjectID) != nil {
			return domain.ErrFrozenInputInvalid
		}
		if err := pgfence.New(s.db).Guard(ctx, tx, command); err != nil {
			return err
		}
		raw, _ := json.Marshal(command)
		if err := tx.Exec(`UPDATE workspace.frozen_input_handoff SET adoption_blocked=true,cancel_command=?::jsonb WHERE handoff_id=?`, string(raw), key.HandoffID).Error; err != nil {
			return err
		}
		if err := taskruntime.EnqueueReference(ctx, tx, "creation.frozen_input.cancel", key.HandoffID.String(), time.Now(), map[string]any{"handoff_id": key.HandoffID}); err != nil {
			return err
		}
		out.AdoptionBlocked = true
		out.CancelCommand = &command
		return nil
	})
	return out, err
}

// CompleteFrozenInputCancellation seals the exact target proof under the local final fence.
func (s *FrozenInputStore) CompleteFrozenInputCancellation(ctx context.Context, key application.FrozenInputKey, proof application.FrozenInputCancelProof) (application.FrozenInputEnvelope, error) {
	return s.completeCancellation(ctx, key, proof, nil)
}

// CompleteFrozenInputCancellationForTask fences the source completion against a
// reclaimed worker lease in the same transaction as its final business receipt.
func (s *FrozenInputStore) CompleteFrozenInputCancellationForTask(ctx context.Context, key application.FrozenInputKey, proof application.FrozenInputCancelProof, job taskruntime.Job) (application.FrozenInputEnvelope, error) {
	if job.Kind != "creation.frozen_input.cancel" || job.BusinessID != key.HandoffID.String() {
		return application.FrozenInputEnvelope{}, taskruntime.ErrLeaseLost
	}
	return s.completeCancellation(ctx, key, proof, &job)
}
func (s *FrozenInputStore) completeCancellation(ctx context.Context, key application.FrozenInputKey, proof application.FrozenInputCancelProof, job *taskruntime.Job) (application.FrozenInputEnvelope, error) {
	var out application.FrozenInputEnvelope
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = readFrozenKey(tx, key, true)
		if err != nil {
			return err
		}
		if err := application.ValidateFrozenCancelProof(out, proof); err != nil {
			return err
		}
		raw, err := json.Marshal(proof)
		if err != nil {
			return err
		}
		canonical, err := writefence.CanonicalReceipt(raw)
		if err != nil {
			return err
		}
		if job != nil {
			if err := taskruntime.Fence(ctx, tx, *job); err != nil {
				return err
			}
		}
		if out.CancelProof != nil {
			saved, _ := json.Marshal(*out.CancelProof)
			savedCanonical, err := writefence.CanonicalReceipt(saved)
			if err != nil || !bytes.Equal(savedCanonical, canonical) {
				return domain.ErrFrozenInputConflict
			}
			return nil
		}
		if err := pgfence.New(s.db).Guard(ctx, tx, *out.CancelCommand); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workspace.frozen_input_handoff SET cancel_receipt=?::jsonb WHERE handoff_id=?`, string(canonical), key.HandoffID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, uuid.NewSHA1(key.HandoffID, []byte("creation.frozen_input.cancelled")), "glowloom.creation.frozen_input.cancelled", out.Input.ProjectID.String(), string(canonical)).Error; err != nil {
			return err
		}
		if err := pgfence.New(s.db).Complete(ctx, tx, *out.CancelCommand, canonical); err != nil {
			return err
		}
		out.CancelProof = &proof
		return nil
	})
	return out, err
}
