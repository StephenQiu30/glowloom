package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// Versions are independent: protocol 3 introduces string revisions and client identity.
const (
	ProtocolVersion  = 3
	OperationVersion = 1
	SchemaVersion    = 1
)

// Revision is a canonical nonnegative signed-64-bit decimal string on the wire.
type Revision int64

// MarshalJSON encodes the exact decimal value as a JSON string.
func (r Revision) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatInt(int64(r), 10)) }

// UnmarshalJSON accepts canonical nonnegative signed-64-bit decimal strings.
func (r *Revision) UnmarshalJSON(body []byte) error {
	var value string
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("revision must be a decimal string")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || strconv.FormatInt(parsed, 10) != value {
		return fmt.Errorf("invalid revision")
	}
	*r = Revision(parsed)
	return nil
}

// Permissions contains current authorized access, never a cached client claim.
type Permissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// NodeDTO separates immutable media identity from protected historical bindings.
type NodeDTO struct {
	ID              uuid.UUID         `json:"id"`
	Title           string            `json:"title"`
	NodeType        string            `json:"node_type"`
	NodeAction      string            `json:"node_action"`
	Config          domain.NodeConfig `json:"config"`
	X               float64           `json:"x"`
	Y               float64           `json:"y"`
	Width           *float64          `json:"width,omitempty"`
	Height          *float64          `json:"height,omitempty"`
	ParentID        *uuid.UUID        `json:"parent_id" extensions:"x-nullable"`
	ZIndex          int32             `json:"z_index"`
	ContentVersion  Revision          `json:"content_version" swaggertype:"string"`
	InputRevision   int64             `json:"input_revision,omitempty"`
	MediaRef        *domain.MediaRef  `json:"media_ref,omitempty"`
	RefType         string            `json:"ref_type,omitempty"`
	RefID           *uuid.UUID        `json:"ref_id,omitempty"`
	LastOperationID *uuid.UUID        `json:"last_operation_id,omitempty"`
}

// DocumentDTO is a complete shared graph; the camera belongs to the local session.
type DocumentDTO struct {
	ID               uuid.UUID       `json:"id"`
	ProjectID        uuid.UUID       `json:"project_id"`
	Name             string          `json:"name"`
	Scope            json.RawMessage `json:"scope" swaggertype:"object"`
	SchemaVersion    int             `json:"schema_version"`
	OperationVersion int             `json:"operation_version"`
	Revision         Revision        `json:"revision" swaggertype:"string"`
	Permissions      Permissions     `json:"permissions"`
	ReadOnlyReason   *string         `json:"read_only_reason" extensions:"x-nullable"`
	Nodes            []NodeDTO       `json:"nodes"`
	Edges            []domain.Edge   `json:"edges"`
}

func nodeDTO(n domain.Node) NodeDTO {
	config := n.Config
	if n.NodeType == "text" && !config.IsOpaque() && config.Format == "" && config.FormatVersion == 0 {
		config.Format = "plain"
		config.FormatVersion = 1
	}
	dto := NodeDTO{ID: n.ID, Title: n.Title, NodeType: n.NodeType, NodeAction: n.NodeAction, Config: config, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height, ParentID: n.ParentID, ZIndex: n.ZIndex, ContentVersion: Revision(n.ContentVersion), InputRevision: n.InputRevision, RefType: n.RefType, RefID: n.RefID, LastOperationID: n.LastOperationID}
	if n.RefType == "media_asset" && n.RefID != nil {
		dto.MediaRef = &domain.MediaRef{AssetID: *n.RefID}
		dto.RefType = ""
		dto.RefID = nil
	}
	return dto
}

// Snapshot maps authorized internal state without changing persisted content.
func Snapshot(doc domain.Document) DocumentDTO {
	// Pre-version lifecycle receipts contain the same known graph without schema
	// metadata. Translate the boundary only; never rewrite their durable payload.
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
		if doc.ReadOnlyReason == "" && domain.ValidateGraph(doc) != nil {
			doc.ReadOnlyReason = "graph_read_only"
		}
	}
	nodes := make([]NodeDTO, 0, len(doc.Nodes))
	for _, node := range doc.Nodes {
		nodes = append(nodes, nodeDTO(node))
	}
	var reason *string
	if doc.ReadOnlyReason != "" {
		reason = &doc.ReadOnlyReason
	}
	return DocumentDTO{ID: doc.ID, ProjectID: doc.ProjectID, Name: doc.Name, Scope: doc.Scope, SchemaVersion: doc.SchemaVersion, OperationVersion: OperationVersion, Revision: Revision(doc.Revision), Permissions: Permissions{Read: true, Write: doc.Writable && reason == nil}, ReadOnlyReason: reason, Nodes: nodes, Edges: doc.Edges}
}

// SubmissionDTO is the protocol-3 request. Raw operations retain rejected fields
// in request identity; node conversion is the only transport/storage mapping.
type SubmissionDTO struct {
	StrictBase       bool              `json:"strict_base,omitempty"`
	ClientID         uuid.UUID         `json:"client_id"`
	OperationVersion int               `json:"operation_version"`
	BaseRevision     Revision          `json:"base_revision" swaggertype:"string"`
	Operations       []json.RawMessage `json:"operations,omitempty"`
	UndoOf           string            `json:"undo_of,omitempty"`
}

// DecodeSubmission rejects unknown envelope fields and legacy media aliases.
func DecodeSubmission(body []byte) (CollaborationInput, error) {
	var dto SubmissionDTO
	if !json.Valid(body) {
		return CollaborationInput{}, domain.ErrInvalidCommand
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dto); err != nil {
		return CollaborationInput{}, domain.ErrInvalidCommand
	}
	input := CollaborationInput{StrictBase: dto.StrictBase, ClientID: dto.ClientID, OperationVersion: dto.OperationVersion, BaseRevision: int64(dto.BaseRevision), UndoOf: dto.UndoOf}
	for _, raw := range dto.Operations {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return CollaborationInput{}, domain.ErrInvalidCommand
		}
		var node *domain.Node
		if value, ok := fields["node"]; ok {
			var nodeFields map[string]json.RawMessage
			if json.Unmarshal(value, &nodeFields) != nil || nodeFields == nil {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}
			if _, present := nodeFields["ref_type"]; present {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}
			if _, present := nodeFields["ref_id"]; present {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}

			var n NodeDTO
			decoder := json.NewDecoder(bytes.NewReader(value))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&n); err != nil {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}
			if n.MediaRef != nil && (n.MediaRef.AssetID == uuid.Nil) {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}
			node = &domain.Node{ID: n.ID, Title: n.Title, NodeType: n.NodeType, NodeAction: n.NodeAction, Config: n.Config, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height, ParentID: n.ParentID, ZIndex: n.ZIndex, ContentVersion: int64(n.ContentVersion), InputRevision: n.InputRevision, LastOperationID: n.LastOperationID}
			if n.MediaRef != nil {
				node.RefType = "media_asset"
				node.RefID = &n.MediaRef.AssetID
			}
			// The existing operation decoder retains unknown fields for durable rejection.
			delete(fields, "node")
			raw, _ = json.Marshal(fields)
		}
		if edge, ok := fields["edge"]; ok {
			var decoded domain.Edge
			decoder := json.NewDecoder(bytes.NewReader(edge))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&decoded); err != nil {
				return CollaborationInput{}, domain.ErrInvalidCommand
			}
		}
		var operation domain.Operation
		if err := json.Unmarshal(raw, &operation); err != nil {
			return CollaborationInput{}, domain.ErrInvalidCommand
		}
		operation.Node = node
		input.Operations = append(input.Operations, operation)
	}
	if dto.ClientID == uuid.Nil || dto.BaseRevision < 1 || dto.OperationVersion == 0 {
		return CollaborationInput{}, domain.ErrInvalidCommand
	}
	return input, nil
}

// Submission serializes a request using the same DTO as the public protocol.
func Submission(input CollaborationInput) (SubmissionDTO, error) {
	dto := SubmissionDTO{StrictBase: input.StrictBase, ClientID: input.ClientID, OperationVersion: input.OperationVersion, BaseRevision: Revision(input.BaseRevision), UndoOf: input.UndoOf}
	for _, op := range input.Operations {
		body, err := publicOperation(op)
		if err != nil {
			return SubmissionDTO{}, err
		}
		dto.Operations = append(dto.Operations, body)
	}
	return dto, nil
}
func publicOperation(op domain.Operation) (json.RawMessage, error) {
	body, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}
	if op.Node == nil {
		return body, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	fields["node"], err = json.Marshal(nodeDTO(*op.Node))
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// ReceiptDTO never serializes a revision as a JavaScript number.
type ReceiptDTO struct {
	Status   string   `json:"status"`
	OpID     string   `json:"op_id"`
	Revision Revision `json:"revision,omitempty" swaggertype:"string"`
	Code     string   `json:"code,omitempty"`
}

// Receipt maps a durable outcome to the public protocol.
func Receipt(receipt CollaborationReceipt) ReceiptDTO {
	return ReceiptDTO{Status: receipt.Status, OpID: receipt.OpID, Revision: Revision(receipt.Revision), Code: receipt.Code}
}

// ChangeDTO carries public effects only; private before-images stay in storage.
type ChangeDTO struct {
	Revision   Revision          `json:"revision" swaggertype:"string"`
	ActorID    uuid.UUID         `json:"actor_id"`
	OpID       string            `json:"op_id"`
	Operations []json.RawMessage `json:"operations"`
	Effects    []domain.Effect   `json:"effects"`
}

// Changes maps ordered effects without private preimages.
func Changes(changes []CollaborationChange) ([]ChangeDTO, error) {
	result := make([]ChangeDTO, 0, len(changes))
	for _, change := range changes {
		dto := ChangeDTO{Revision: Revision(change.Revision), ActorID: change.ActorID, OpID: change.OpID, Effects: make([]domain.Effect, 0, len(change.Effects)), Operations: make([]json.RawMessage, 0, len(change.Operations))}
		for _, op := range change.Operations {
			body, err := publicOperation(op)
			if err != nil {
				return nil, err
			}
			dto.Operations = append(dto.Operations, body)
		}
		for _, effect := range change.Effects {
			effect.Before = nil
			effect.BeforeExists = false
			if effect.Entity == "node" {
				switch {
				case len(effect.Path) == 0 && effect.AfterExists:
					var node domain.Node
					if err := json.Unmarshal(effect.After, &node); err != nil {
						return nil, err
					}
					body, err := json.Marshal(nodeDTO(node))
					if err != nil {
						return nil, err
					}
					effect.After = body
				case len(effect.Path) == 1 && effect.Path[0] == "ref_type":
					continue // Internal media discriminator; public content identity is media_ref.
				case len(effect.Path) == 1 && effect.Path[0] == "ref_id":
					effect.Path = []string{"media_ref", "asset_id"}
				}
			}
			dto.Effects = append(dto.Effects, effect)
		}
		result = append(result, dto)
	}
	return result, nil
}

// RenameDTO and DeleteDTO keep lifecycle wire revisions separate from durable
// internal request hashes written before protocol 3.
type RenameDTO struct {
	ExpectedRevision Revision `json:"expected_revision" swaggertype:"string"`
	Name             string   `json:"name"`
}

// DeleteDTO carries a string revision for lifecycle deletion.
type DeleteDTO struct {
	ExpectedRevision Revision `json:"expected_revision" swaggertype:"string"`
}

// DeleteResultDTO reports the committed deletion version.
type DeleteResultDTO struct {
	ID       uuid.UUID `json:"id"`
	Revision Revision  `json:"revision" swaggertype:"string"`
	Deleted  bool      `json:"deleted"`
}

// ValidateSnapshotCapacity measures the exact public representation, including
// normalized text metadata and media references, before a graph is accepted.
func ValidateSnapshotCapacity(document domain.Document) error {
	body, err := json.Marshal(Snapshot(document))
	if err != nil {
		return err
	}
	if len(body) > domain.MaxDocumentBytes {
		return &domain.OperationError{Code: "canvas_capacity_exceeded"}
	}
	return nil
}
