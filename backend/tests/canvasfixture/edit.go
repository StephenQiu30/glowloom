// Package canvasfixture builds semantic fixture edits and submits real Yjs updates.
package canvasfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ClientID is a fixed synthetic fixture session.
var ClientID = uuid.MustParse("ac8647b6-3994-4a42-8a9f-fd356ee10432")

// EditInput is a test builder, never a wire protocol or production editing API.
type EditInput struct {
	ClientID         uuid.UUID          `json:"client_id"`
	OperationVersion int                `json:"operation_version"`
	BaseRevision     int64              `json:"base_revision"`
	StrictBase       bool               `json:"strict_base"`
	Operations       []domain.Operation `json:"operations"`
	Name             string             `json:"name,omitempty"`
	UndoOf           string             `json:"undo_of"`
}

// EditReceipt exposes the durable outcome to semantic business tests.
type EditReceipt struct {
	Status   string
	Code     string
	OpID     string
	Revision int64
}
type fixtureRecord struct {
	Input  []byte
	Update application.YjsUpdate
	Before []byte
}

// FixtureStore requires both read and authoritative Yjs acceptance.
type FixtureStore interface {
	application.Store
	application.YjsStore
}

// Service builds real Yjs edits for business integration fixtures.
type Service struct {
	*application.Service
	shared  *application.YjsService
	mu      *sync.Mutex
	records map[string]fixtureRecord
}

var fixtureMu sync.Mutex
var fixtureRecords = map[string]fixtureRecord{}

// NewService constructs a test-only semantic client.
func NewService(store FixtureStore) *Service {
	return &Service{Service: application.NewService(store), shared: application.NewYjsService(store, nodeRuntime{}), mu: &fixtureMu, records: fixtureRecords}
}

// NewEditService names the same client for write-only test usage.
func NewEditService(store FixtureStore) *Service { return NewService(store) }

// Edit submits and reloads the accepted public document.
func (s *Service) Edit(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input EditInput) (domain.Document, error) {
	receipt, err := s.Submit(ctx, actor, id, key, input)
	if err != nil {
		return domain.Document{}, err
	}
	if receipt.Status != "accepted" {
		return domain.Document{}, &domain.OperationError{Code: receipt.Code}
	}
	return s.Get(ctx, actor, id)
}

// Submit converts a semantic fixture into an immutable binary update.
func (s *Service) Submit(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input EditInput) (EditReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := EditReceipt{OpID: key}
	if _, err := s.shared.Load(ctx, actor, id); err != nil {
		return result, err
	}
	body := canonicalInput(input)
	record, exists := s.records[key]
	if exists && !bytes.Equal(record.Input, body) {
		return result, application.ErrIdempotencyConflict
	}
	if !exists {
		state, err := s.shared.Load(ctx, actor, id)
		if err != nil {
			return result, err
		}
		if len(state.State) == 0 {
			state, err = s.shared.Initialize(ctx, actor, id, state.Document.Revision)
			if err != nil {
				return result, err
			}
		}
		if input.StrictBase && input.BaseRevision != int64(state.Document.Revision) {
			result.Status = "rejected"
			result.Code = "review_stale"
			return result, nil
		}
		var projection application.DocumentDTO
		var undo any
		if input.UndoOf != "" {
			original, ok := s.records[input.UndoOf]
			if !ok {
				return result, fmt.Errorf("fixture undo requires original update")
			}
			undo = map[string]any{"state": original.Before, "update": original.Update.Update}
		} else {
			doc, err := s.Get(ctx, actor, id)
			if err != nil {
				return result, err
			}
			candidate, err := domain.ApplyOperations(doc, input.Operations)
			if err != nil {
				result.Status = "rejected"
				result.Code = fixtureCode(err)
				return result, nil
			}
			projection = application.Snapshot(candidate.Document)
			projection.Name = state.Document.Name
			if input.Name != "" {
				projection.Name = input.Name
			}
		}
		update := application.YjsUpdate{Epoch: state.Epoch, SessionID: input.ClientID, UpdateID: uuid.MustParse(key)}
		if update.SessionID == uuid.Nil {
			update.SessionID = ClientID
		}
		if err := fixtureCall(ctx, "update", map[string]any{"state": state, "projection": projection, "undo": undo}, &update); err != nil {
			return result, err
		}
		record = fixtureRecord{Input: body, Update: update, Before: state.State}
		s.records[key] = record
	}
	receipt, err := s.shared.Submit(ctx, actor, id, record.Update)
	if err != nil {
		return result, err
	}
	result.Status, result.Code, result.Revision = receipt.Status, receipt.Code, int64(receipt.ContentRevision)
	return result, nil
}

// Resolve queries PostgreSQL with the original cached binary envelope.
func (s *Service) Resolve(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input EditInput) (*EditReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return nil, fmt.Errorf("fixture resolve requires original update; test production resolve with YjsService")
	}
	body := canonicalInput(input)
	if !bytes.Equal(body, record.Input) {
		return nil, application.ErrIdempotencyConflict
	}
	receipt, err := s.shared.Resolve(ctx, actor, id, record.Update)
	if err != nil || receipt == nil {
		return nil, err
	}
	return &EditReceipt{Status: receipt.Status, Code: receipt.Code, OpID: key, Revision: int64(receipt.ContentRevision)}, nil
}

type nodeRuntime struct{}

func (nodeRuntime) Initialize(ctx context.Context, state application.YjsState) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	err := fixtureCall(ctx, "initialize", state, &result)
	return result, err
}
func (nodeRuntime) Prepare(ctx context.Context, state application.YjsState, update application.YjsUpdate) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	input := struct {
		application.YjsState
		Update           []byte                `json:"update"`
		UpdateID         uuid.UUID             `json:"updateId"`
		PayloadHash      string                `json:"payloadHash"`
		RestorationNodes []application.NodeDTO `json:"restorationNodes"`
	}{state, update.Update, update.UpdateID, update.PayloadHash, state.RestorationNodes}
	err := fixtureCall(ctx, "prepare", input, &result)
	return result, err
}
func fixtureCall(ctx context.Context, action string, input, output any) error {
	_, file, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(file), "../../../canvas-collab/tests/projection-fixture.mjs")
	body, err := json.Marshal(map[string]any{"action": action, "input": input})
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "node", script)
	command.Stdin = bytes.NewReader(body)
	data, err := command.Output()
	if err != nil {
		var failure struct {
			FixtureError string `json:"fixtureError"`
			Definitive   bool   `json:"definitive"`
		}
		if json.Unmarshal(data, &failure) == nil && failure.Definitive {
			return &application.YjsRejection{Code: failure.FixtureError}
		}
		return fmt.Errorf("yjs fixture %s: %w: %s", action, err, data)
	}
	return json.Unmarshal(data, output)
}

// Concat combines one atomic fixture batch.
func Concat(parts ...[]domain.Operation) []domain.Operation {
	var result []domain.Operation
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

// Nodes creates stable entities through real Yjs projection admission.
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

func canonicalInput(input EditInput) []byte {
	body, _ := json.Marshal(input)
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	body, _ = json.Marshal(value)
	return body
}
func fixtureCode(err error) string {
	var rejection *domain.OperationError
	if errors.As(err, &rejection) {
		return rejection.Code
	}
	if errors.Is(err, domain.ErrInvalidCommand) {
		return "invalid_operation"
	}
	if errors.Is(err, domain.ErrUnsupportedCommand) {
		return "protected_target"
	}
	return err.Error()
}

// Rename changes the shared title through Yjs.
func (s *Service) Rename(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input application.RenameInput) (domain.Document, error) {
	return s.Edit(ctx, actor, id, key, EditInput{ClientID: ClientID, BaseRevision: input.ExpectedRevision, StrictBase: true, Name: input.Name})
}
