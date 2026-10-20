package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

const maxCollaborationHistory = 128
const maxCollaborationBytes = 4 << 20

type collaborationLog struct {
	System             string                         `json:"system,omitempty"`
	Name               string                         `json:"name,omitempty"`
	OriginalBase       int64                          `json:"original_base"`
	OriginalOperations []domain.Operation             `json:"original_operations"`
	MediaReferences    []uuid.UUID                    `json:"media_references"`
	Operations         []domain.Operation             `json:"operations"`
	Original           application.CollaborationInput `json:"original"`
	Effects            []domain.Effect                `json:"effects"`
	Touches            []domain.FieldChange           `json:"touches"`
}
type collaborationRow struct {
	Revision         int64
	OperationVersion int
	OpID             string
	CreateBy         uuid.UUID
	Commands         []byte
	Log              collaborationLog `gorm:"-"`
}

func collaborationHash(id uuid.UUID, input application.CollaborationInput) (string, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return requestHash("collaboration/"+id.String(), body), nil
}
func collaborationReceipt(tx *gorm.DB, actor, document uuid.UUID, key, hash string) (*application.CollaborationReceipt, error) {
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.String()+":"+key).Scan(&locked).Error; err != nil {
		return nil, err
	}
	var row struct {
		DocumentID   uuid.UUID
		RequestHash  string
		ResponseBody []byte
	}
	r := tx.Raw(`SELECT document_id,request_hash,response_body FROM canvas.save_receipt WHERE actor_id=? AND idem_key=?`, actor, key).Scan(&row)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	if row.DocumentID != document || row.RequestHash != hash {
		return nil, application.ErrIdempotencyConflict
	}
	var receipt application.CollaborationReceipt
	if err := json.Unmarshal(row.ResponseBody, &receipt); err != nil {
		return nil, err
	}
	if receipt.OpID != key || (receipt.Status != "accepted" && receipt.Status != "rejected") {
		return nil, domain.ErrInvalidCommand
	}
	return &receipt, nil
}
func recordCollaborationReceipt(tx *gorm.DB, actor, document uuid.UUID, key, hash string, receipt application.CollaborationReceipt) error {
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	var revision any
	if receipt.Status == "accepted" {
		revision = receipt.Revision
	}
	return tx.Exec(`INSERT INTO canvas.save_receipt(actor_id,idem_key,document_id,request_hash,status,result_revision,response_body) VALUES(?,?,?,?,?,?,?::jsonb)`, actor, key, document, hash, receipt.Status, revision, string(body)).Error
}

func collaborationHistory(tx *gorm.DB, id uuid.UUID, base, head int64) ([]collaborationRow, error) {
	if base < 1 || base > head || head-base > maxCollaborationHistory {
		return nil, &domain.OperationError{Code: "history_unavailable"}
	}
	if base == head {
		return []collaborationRow{}, nil
	}
	var size int64
	if err := tx.Raw(`SELECT COALESCE(sum(octet_length(commands::text)),0) FROM canvas.command_log WHERE document_id=? AND revision>? AND revision<=? AND NOT is_delete`, id, base, head).Scan(&size).Error; err != nil {
		return nil, err
	}
	if size > maxCollaborationBytes {
		return nil, &domain.OperationError{Code: "history_unavailable"}
	}
	var rows []collaborationRow
	if err := tx.Raw(`SELECT revision,operation_version,COALESCE(op_id::text,'') AS op_id,create_by,commands FROM canvas.command_log WHERE document_id=? AND revision>? AND revision<=? AND NOT is_delete ORDER BY revision LIMIT 129`, id, base, head).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if int64(len(rows)) != head-base {
		return nil, &domain.OperationError{Code: "history_unavailable"}
	}
	for i := range rows {
		r := &rows[i]
		if r.Revision != base+int64(i)+1 || r.OperationVersion != 1 || r.OpID == "" || json.Unmarshal(r.Commands, &r.Log) != nil || r.Log.Operations == nil || r.Log.Effects == nil {
			return nil, &domain.OperationError{Code: "history_unavailable"}
		}
	}
	return rows, nil
}

func collaborationBusinessCode(err error) string {
	var rejection *domain.OperationError
	switch {
	case errors.As(err, &rejection):
		return rejection.Code
	case errors.Is(err, domain.ErrGraphReadOnly):
		return "canvas_graph_read_only"
	case errors.Is(err, domain.ErrInvalidSequence):
		return "invalid_text_operation"
	case errors.Is(err, domain.ErrInvalidCommand):
		return "invalid_operation"
	case errors.Is(err, domain.ErrUnsupportedCommand):
		return "protected_target"
	case errors.Is(err, promptdomain.ErrReferenceUnavailable):
		return "source_unavailable"
	case errors.Is(err, promptdomain.ErrInvalidDraft):
		return "invalid_operation"
	case errors.Is(err, media.ErrNotFound):
		return "source_unavailable"
	default:
		return ""
	}
}

// SubmitCollaboration serializes accepted order under the document row lock.
// Business rejections commit a receipt; transient failures roll back and remain
// unknown, so resolving the exact original request is always safe.
func (s *Store) SubmitCollaboration(ctx context.Context, a identity.Principal, id uuid.UUID, key string, input application.CollaborationInput) (application.CollaborationReceipt, error) {
	var result application.CollaborationReceipt
	hash, err := collaborationHash(id, input)
	if err != nil {
		return result, domain.ErrInvalidCommand
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(tx, a, p, true); err != nil {
			return err
		}
		replay, err := collaborationReceipt(tx, a.ID, id, key, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		reject := func(code string) error {
			result = application.CollaborationReceipt{Status: "rejected", OpID: key, Code: code}
			return recordCollaborationReceipt(tx, a.ID, id, key, hash, result)
		}
		rejectError := func(err error) error {
			if code := collaborationBusinessCode(err); code != "" {
				return reject(code)
			}
			return err
		}
		if input.OperationVersion != 1 {
			return reject("unsupported_operation_version")
		}
		if input.UndoOf != "" {
			undoID, err := uuid.Parse(input.UndoOf)
			if err != nil || undoID == uuid.Nil || len(input.Operations) != 0 {
				return reject("invalid_undo_target")
			}
		} else if err := domain.ValidateWireOperations(input.Operations); err != nil {
			return rejectError(err)
		}
		if doc.Revision >= math.MaxInt32 {
			return reject("revision_limit")
		}
		originalBase := input.BaseRevision
		originalOperations := input.Operations
		if input.UndoOf != "" {
			if input.BaseRevision < 1 || input.BaseRevision > doc.Revision {
				return reject("history_unavailable")
			}
			floor, batches, err := collaborationUndoHistory(tx, doc, a.ID, input.UndoOf)
			if err != nil {
				return rejectError(err)
			}
			originalOperations, err = domain.UndoContribution(floor, batches, input.UndoOf, a.ID)
			if err != nil {
				return rejectError(err)
			}
			originalBase = doc.Revision
		}
		history, err := collaborationHistory(tx, id, originalBase, doc.Revision)
		if err != nil {
			return rejectError(err)
		}
		base := doc
		for i := len(history) - 1; i >= 0; i-- {
			base, err = domain.RewindEffects(base, history[i].Log.Effects)
			if err != nil {
				return rejectError(err)
			}
			base.Revision--
		}
		normalized, err := normalizeKnownRemovals(tx, base, originalOperations)
		if err != nil {
			return err
		}
		if _, err := domain.ApplyOperations(base, normalized); err != nil {
			return rejectError(err)
		}
		work := 0
		for _, row := range history {
			work += len(normalized)*len(row.Log.Operations) + len(normalized) + len(row.Log.Operations)
		}
		if err := domain.ValidateTransformWork(doc, work); err != nil {
			return rejectError(err)
		}
		transformed := normalized
		for _, row := range history {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, transformed, err = domain.TransformOperationPair(base, row.Log.Operations, transformed)
			if err != nil {
				return rejectError(err)
			}
			step, err := domain.ApplyEffects(base, row.Log.Effects)
			if err != nil {
				return reject("history_unavailable")
			}
			base = step
			base.Revision = row.Revision
		}
		candidate, err := domain.ApplyOperations(doc, transformed)
		if err != nil {
			return rejectError(err)
		}
		if err := s.collaborationIdentities(tx, doc, transformed); err != nil {
			return rejectError(err)
		}
		sourceNodes := append(slices.Clone(candidate.Document.Nodes), candidate.IntermediateNodes...)
		commands := make([]domain.Command, 0, len(sourceNodes))
		for _, n := range sourceNodes {
			commands = append(commands, domain.Command{Type: "AddNodes", Nodes: []domain.Node{n}})
		}
		if err := s.checkMediaReferences(ctx, tx, a, p, commands, candidate.Document); err != nil {
			return rejectError(err)
		}
		for _, n := range sourceNodes {
			if domain.IsDraft(n) && n.Config.Draft.Composer != nil {
				if err := pgprompt.ValidateDraftSources(tx, a, p, *n.Config.Draft.Composer, false); err != nil {
					return rejectError(err)
				}
			}
		}
		// A savepoint keeps deterministic identity collisions durable while rolling
		// back every graph/input/activity effect from this attempted batch.
		if err := tx.SavePoint("collaboration_save").Error; err != nil {
			return err
		}
		updated := candidate.Document
		updated.Revision++
		updated.OperationVersion = 1
		if err := saveGraph(tx, doc, updated); err != nil {
			var collision *identityConflict
			if errors.As(err, &collision) {
				if err := tx.RollbackTo("collaboration_save").Error; err != nil {
					return err
				}
				return reject("id_conflict")
			}
			return err
		}
		if err := tx.Exec(`UPDATE canvas.document SET operation_version=1 WHERE id=?`, id).Error; err != nil {
			return err
		}
		if err := saveInputs(tx, a.ID, candidate.Inputs); err != nil {
			return err
		}
		if len(candidate.Effects) > 0 {
			presentation, err := s.presentation(tx, a, p)
			if err != nil {
				return err
			}
			if id == presentation.MainCanvasID && domain.HasContentChanges(doc, updated) {
				if err := s.project(tx).MarkProjectContentChanged(ctx, a, p); err != nil {
					return err
				}
			}
			if err := changed(tx, a, id, p, updated.Revision); err != nil {
				return err
			}
			if len(candidate.Inputs) > 0 {
				if err := savedAudit(tx, a, updated, key); err != nil {
					return err
				}
			}
		}
		effects := candidate.Effects
		if effects == nil {
			effects = []domain.Effect{}
		}
		refs, err := collaborationRetainedMedia(doc, updated, domain.Document{Nodes: candidate.IntermediateNodes})
		if err != nil {
			return err
		}
		log := collaborationLog{OriginalBase: originalBase, OriginalOperations: normalized, MediaReferences: refs, Operations: transformed, Original: input, Effects: effects, Touches: candidate.Changes}
		body, err := json.Marshal(log)
		if err != nil {
			return err
		}
		if len(body) > maxMediaReferencePayloadBytes {
			if err := tx.RollbackTo("collaboration_save").Error; err != nil {
				return err
			}
			return reject("operation_too_large")
		}
		if err := tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,create_by,op_id,base_revision,operation_version) VALUES(?,?,?,?::jsonb,?,?,?,1)`, uuid.New(), id, updated.Revision, string(body), a.ID, key, input.BaseRevision).Error; err != nil {
			return err
		}
		result = application.CollaborationReceipt{Status: "accepted", OpID: key, Revision: updated.Revision}
		return recordCollaborationReceipt(tx, a.ID, id, key, hash, result)
	})
	return result, err
}

func (s *Store) collaborationIdentities(tx *gorm.DB, doc domain.Document, ops []domain.Operation) error {
	for _, op := range ops {
		if err := collectionIdentityUsed(tx, doc, op); err != nil {
			return err
		}
		table := ""
		var id uuid.UUID
		if op.Type == "create_node" && op.Node != nil {
			table = "canvas.node"
			id = op.Node.ID
		}
		if op.Type == "connect" && op.Edge != nil {
			table = "canvas.edge"
			id = op.Edge.ID
		}
		if table != "" {
			var needle []byte
			// Only the immutable identity belongs in the containment query.
			if op.Node != nil {
				needle, _ = json.Marshal(map[string]any{"operations": []any{map[string]any{"type": "create_node", "node": map[string]any{"id": id}}}})
			} else {
				needle, _ = json.Marshal(map[string]any{"operations": []any{map[string]any{"type": "connect", "edge": map[string]any{"id": id}}}})
			}
			var used int
			if err := tx.Raw(`SELECT 1 FROM canvas.command_log WHERE document_id=? AND operation_version=1 AND commands @> ?::jsonb LIMIT 1`, doc.ID, string(needle)).Scan(&used).Error; err != nil {
				return err
			}
			if used == 1 {
				return &domain.OperationError{Code: "id_conflict"}
			}
			var count int64
			// table is a closed constant selected above, never client SQL.
			if err := tx.Raw("SELECT count(*) FROM "+table+" WHERE id=?", id).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return &domain.OperationError{Code: "id_conflict"}
			}
		}
	}
	return nil
}

// ResolveCollaboration queries the exact original receipt without writing the graph.
func (s *Store) ResolveCollaboration(ctx context.Context, a identity.Principal, id uuid.UUID, key string, input application.CollaborationInput) (*application.CollaborationReceipt, error) {
	hash, err := collaborationHash(id, input)
	if err != nil {
		return nil, domain.ErrInvalidCommand
	}
	var result *application.CollaborationReceipt
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(tx, a, p, false); err != nil {
			return err
		}
		result, err = collaborationReceipt(tx, a.ID, id, key, hash)
		return err
	})
	return result, err
}

// CollaborationChanges returns ordered authorized canonical graph effects.
func (s *Store) CollaborationChanges(ctx context.Context, a identity.Principal, id uuid.UUID, after int64) ([]application.CollaborationChange, error) {
	result := []application.CollaborationChange{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(tx, a, p, false); err != nil {
			return err
		}
		doc, err := load(tx, id, false)
		if err != nil {
			return err
		}
		rows, err := collaborationHistory(tx, id, after, doc.Revision)
		if err != nil {
			return err
		}
		for _, r := range rows {
			effects := make([]domain.Effect, len(r.Log.Effects))
			copy(effects, r.Log.Effects)
			for i := range effects {
				effects[i].Before = nil
				effects[i].BeforeExists = false
			}
			result = append(result, application.CollaborationChange{Revision: r.Revision, ActorID: r.CreateBy, OpID: r.OpID, Operations: r.Log.Operations, Effects: effects})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read collaboration changes: %w", err)
	}
	return result, nil
}

// History keeps immutable asset identities needed for owning retention admission.
// This compact index contains no URL, source body or full-node snapshot.
func collaborationRetainedMedia(documents ...domain.Document) ([]uuid.UUID, error) {
	refs := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, document := range documents {
		for _, node := range document.Nodes {
			ids, err := configMediaReferences(node.Config)
			if err != nil {
				return nil, err
			}
			if node.RefType == "media_asset" && node.RefID != nil {
				ids = append(ids, *node.RefID)
			}
			for _, id := range ids {
				if id == uuid.Nil {
					return nil, domain.ErrInvalidCommand
				}
				if !seen[id] {
					seen[id] = true
					refs = append(refs, id)
				}
			}
		}
	}
	return refs, nil
}
