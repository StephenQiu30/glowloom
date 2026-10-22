// Package canvasfixture builds finite OT operations for integration fixtures.
package canvasfixture

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ClientID is stable across retries and deliberately carries no authority.
var ClientID = uuid.MustParse("ac8647b6-3994-4a42-8a9f-fd356ee10432")

// Service pairs lifecycle and OT services over the exact same transactional store.
type Service struct {
	*application.Service
	collaboration *application.CollaborationService
}

// NewService binds lifecycle and collaboration to the supplied store.
func NewService(store interface {
	application.Store
	application.CollaborationStore
}) *Service {
	return &Service{application.NewService(store), application.NewCollaborationService(store)}
}

// Edit requires a terminal acceptance before reading the current database graph.
// Callers testing receipt identity use CollaborationService directly.
func (s *Service) Edit(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input application.CollaborationInput) (domain.Document, error) {
	receipt, err := s.collaboration.Submit(ctx, actor, id, key, input)
	if err != nil {
		return domain.Document{}, err
	}
	if receipt.Status != "accepted" {
		return domain.Document{}, &domain.OperationError{Code: receipt.Code}
	}
	doc, err := s.Get(ctx, actor, id)

	return doc, err
}

// Concat combines one atomic fixture batch.
func Concat(parts ...[]domain.Operation) []domain.Operation {
	var result []domain.Operation
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

// Nodes creates stable entities through real OT admission.
func Nodes(nodes []domain.Node) []domain.Operation {
	var result []domain.Operation
	for _, node := range nodes {
		result = append(result, domain.Operation{Type: "create_node", Node: &node})
	}
	return result
}

// Edges creates typed visual annotations.
func Edges(edges []domain.Edge) []domain.Operation {
	var result []domain.Operation
	for _, edge := range edges {
		result = append(result, domain.Operation{Type: "connect", Edge: &edge})
	}
	return result
}

// Deletes removes nodes by stable identity.
func Deletes(ids []uuid.UUID) []domain.Operation {
	var result []domain.Operation
	for _, id := range ids {
		result = append(result, domain.Operation{Type: "delete_node", NodeID: id})
	}
	return result
}

// Field builds one finite named-field operation.
func Field(id uuid.UUID, path []string, value any) domain.Operation {
	body, _ := json.Marshal(value)
	return domain.Operation{Type: "set", NodeID: id, Path: path, Value: body}
}

// Positions builds atomic x/y field groups.
func Positions(moves []domain.Move) []domain.Operation {
	var result []domain.Operation
	for _, m := range moves {
		result = append(result, Field(m.ID, []string{"position"}, map[string]any{"x": m.X, "y": m.Y}))
	}
	return result
}

// Sizes builds atomic width/height field groups.
func Sizes(sizes []domain.Size) []domain.Operation {
	var result []domain.Operation
	for _, s := range sizes {
		result = append(result, Field(s.ID, []string{"size"}, map[string]any{"width": s.Width, "height": s.Height}))
	}
	return result
}

// Parents changes membership while retaining world coordinates.
func Parents(parents []domain.Parent) []domain.Operation {
	var result []domain.Operation
	for _, p := range parents {
		result = append(result, Field(p.ID, []string{"parent_id"}, p.ParentID))
	}
	return result
}

// TextSource is an explicit compare-and-replace, not an arbitrary config write.
func TextSource(id uuid.UUID, before, after string) []domain.Operation {
	op := Field(id, []string{"config", "text_source"}, map[string]any{"text": after})
	op.Expected, _ = json.Marshal(map[string]any{"text": before})
	yes := true
	op.ExpectedExists = &yes
	return []domain.Operation{op}
}

// Text replaces an explicit base string using a sequence edit.
func Text(id uuid.UUID, path []string, before, after string) []domain.Operation {
	var edit domain.SequenceEdit
	if count := len([]rune(before)); count > 0 {
		edit = append(edit, domain.SequenceComponent{Delete: count})
	}
	if after != "" {
		edit = append(edit, domain.SequenceComponent{Insert: []domain.Atom{{Type: "text", Text: after}}})
	}
	return []domain.Operation{{Type: "text", NodeID: id, Path: path, Text: edit}}
}
