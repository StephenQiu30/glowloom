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

// projectionLog records accepted business effects for retention and deletion proof.
// Historical command_log rows remain readable, but are never transformed/replayed.
type projectionLog struct {
	// Decode historical audit fields without interpreting or executing them.
	Original           json.RawMessage      `json:"original,omitempty"`
	OriginalOperations []json.RawMessage    `json:"original_operations,omitempty"`
	System             string               `json:"system,omitempty"`
	Name               string               `json:"name,omitempty"`
	OriginalBase       int64                `json:"original_base"`
	MediaReferences    []uuid.UUID          `json:"media_references"`
	Operations         []domain.Operation   `json:"operations"`
	Effects            []domain.Effect      `json:"effects"`
	Touches            []domain.FieldChange `json:"touches"`
}

// persistYjsProjection runs under CommitYjs's row lock and transaction. Operations
// describe a diff against that exact head; no client operation or rebase is accepted.
func (s *Store) persistYjsProjection(ctx context.Context, tx *gorm.DB, a identity.Principal, doc domain.Document, input application.YjsUpdate, operations []domain.Operation, restore bool) (int64, error) {
	if doc.SchemaVersion != application.SchemaVersion {
		return 0, domain.ErrGraphReadOnly
	}
	if doc.Revision == math.MaxInt64 {
		return 0, &domain.OperationError{Code: "revision_limit"}
	}
	p, id := doc.ProjectID, doc.ID
	// A Yjs update identity is scoped to actor/session/epoch. Audit identities
	// must not accidentally collapse two sessions that reuse the same UUID.
	key := uuid.NewSHA1(input.UpdateID, []byte(a.ID.String()+input.SessionID.String()+input.Epoch.String())).String()
	candidate, err := domain.ApplyOperations(doc, operations)
	if err != nil {
		return 0, err
	}
	if err := s.projectionIdentities(tx, doc, operations); err != nil {
		return 0, err
	}
	sourceNodes := append(slices.Clone(candidate.Document.Nodes), candidate.IntermediateNodes...)
	commands := make([]domain.Command, 0, len(sourceNodes))
	for _, n := range sourceNodes {
		commands = append(commands, domain.Command{Type: "AddNodes", Nodes: []domain.Node{n}})
	}
	if err := s.checkMediaReferences(ctx, tx, a, p, commands, candidate.Document); err != nil {
		return 0, err
	}
	for _, n := range sourceNodes {
		if n.Config.Composer != nil {
			if err := pgprompt.ValidateDraftSources(tx, a, p, *n.Config.Composer, false, s.subjectSources(tx)); err != nil {
				return 0, err
			}
		}
		if domain.IsDraft(n) && n.Config.Draft.Composer != nil {
			if err := pgprompt.ValidateDraftSources(tx, a, p, *n.Config.Draft.Composer, false, s.subjectSources(tx)); err != nil {
				return 0, err
			}
		}
	}
	// A savepoint keeps deterministic identity collisions durable while rolling
	// back every graph/input/activity effect from this attempted batch.
	if err := tx.SavePoint("yjs_projection").Error; err != nil {
		return 0, err
	}
	updated := candidate.Document
	updated.Revision++
	updated.OperationVersion = 1
	if err := advanceContentVersions(doc, &updated); err != nil {
		return 0, err
	}
	if err := domain.ValidateDocumentCapacity(updated); err != nil {
		return 0, err
	}
	candidate.Effects = domain.DocumentEffects(doc, updated)
	if err := saveGraphWithRestoration(tx, doc, updated, restore); err != nil {
		var collision *identityConflict
		if errors.As(err, &collision) {
			if err := tx.RollbackTo("yjs_projection").Error; err != nil {
				return 0, err
			}
			return 0, &domain.OperationError{Code: "id_conflict"}
		}
		return 0, err
	}
	if err := tx.Exec(`UPDATE canvas.document SET operation_version=1 WHERE id=?`, id).Error; err != nil {
		return 0, err
	}
	if err := saveInputs(tx, a.ID, candidate.Inputs); err != nil {
		return 0, err
	}
	if len(candidate.Effects) > 0 {
		presentation, err := s.presentation(tx, a, p)
		if err != nil {
			return 0, err
		}
		if id == presentation.MainCanvasID && domain.HasContentChanges(doc, updated) {
			if err := s.project(tx).MarkProjectContentChanged(ctx, a, p); err != nil {
				return 0, err
			}
		}
		if err := changed(tx, a, id, p, updated.Revision); err != nil {
			return 0, err
		}
		if len(candidate.Inputs) > 0 {
			if err := savedAudit(tx, a, updated, key); err != nil {
				return 0, err
			}
		}
	}
	effects := candidate.Effects
	if effects == nil {
		effects = []domain.Effect{}
	}
	refs, err := projectionRetainedMedia(doc, updated, domain.Document{Nodes: candidate.IntermediateNodes})
	if err != nil {
		return 0, err
	}
	log := projectionLog{OriginalBase: doc.Revision, MediaReferences: refs, Operations: operations, Effects: effects, Touches: candidate.Changes}
	body, err := json.Marshal(log)
	if err != nil {
		return 0, err
	}
	tooLarge := len(body) > maxMediaReferencePayloadBytes
	if !tooLarge {
		// Reference admission measures jsonb::text, whose spaces and expanded
		// numeric literals can be larger than compact encoding/json output.
		var storedBytes int64
		if err := tx.Raw(`SELECT octet_length(?::jsonb::text)`, string(body)).Scan(&storedBytes).Error; err != nil {
			return 0, fmt.Errorf("measure projection audit payload: %w", err)
		}
		tooLarge = storedBytes > maxMediaReferencePayloadBytes
	}
	if tooLarge {
		if err := tx.RollbackTo("yjs_projection").Error; err != nil {
			return 0, err
		}
		return 0, &domain.OperationError{Code: "operation_too_large"}
	}
	if err := tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,create_by,op_id,base_revision,operation_version,client_id) VALUES(?,?,?,?::jsonb,?,?,?,1,?)`, uuid.New(), id, updated.Revision, string(body), a.ID, key, doc.Revision, input.SessionID).Error; err != nil {
		return 0, err
	}
	return updated.Revision, nil
}
func (s *Store) projectionIdentities(tx *gorm.DB, doc domain.Document, ops []domain.Operation) error {
	for _, op := range ops {
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

func projectionBusinessCode(err error) string {
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

// History keeps immutable asset identities needed for owning retention admission.
// This compact index contains no URL, source body or full-node snapshot.
func projectionRetainedMedia(documents ...domain.Document) ([]uuid.UUID, error) {
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
