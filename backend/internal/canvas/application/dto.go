package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// Versions are independent: protocol 4 exclusively carries Yjs updates.
const (
	ProtocolVersion  = 4
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

// decodeProjectionOperations maps trusted runtime projection differences to the
// domain admission vocabulary. This is not exposed as an editing protocol.
func decodeProjectionOperations(rawOperations []json.RawMessage) ([]domain.Operation, error) {
	var operations []domain.Operation
	for _, raw := range rawOperations {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, domain.ErrInvalidCommand
		}
		var node *domain.Node
		if value, ok := fields["node"]; ok {
			var nodeFields map[string]json.RawMessage
			if json.Unmarshal(value, &nodeFields) != nil || nodeFields == nil {
				return nil, domain.ErrInvalidCommand
			}
			if _, present := nodeFields["ref_type"]; present {
				return nil, domain.ErrInvalidCommand
			}
			if _, present := nodeFields["ref_id"]; present {
				return nil, domain.ErrInvalidCommand
			}

			var n NodeDTO
			decoder := json.NewDecoder(bytes.NewReader(value))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&n); err != nil {
				return nil, domain.ErrInvalidCommand
			}
			if n.MediaRef != nil && (n.MediaRef.AssetID == uuid.Nil) {
				return nil, domain.ErrInvalidCommand
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
				return nil, domain.ErrInvalidCommand
			}
		}
		var operation domain.Operation
		if err := json.Unmarshal(raw, &operation); err != nil {
			return nil, domain.ErrInvalidCommand
		}
		operation.Node = node
		operations = append(operations, operation)
	}
	return operations, nil
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
