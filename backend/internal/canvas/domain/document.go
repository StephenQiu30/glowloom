package domain

import (
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalid  = errors.New("invalid canvas operation")
	ErrConflict = errors.New("canvas operation conflicts with current revision")
)

const (
	SchemaVersion  = 1
	MaxNodes       = 1000
	MaxConnections = 2000
)

// Document stores visual instances and references, never media bytes or execution state.
type Document struct {
	SchemaVersion int          `json:"schema_version"`
	ProjectID     string       `json:"project_id"`
	Revision      int          `json:"revision"`
	Nodes         []Node       `json:"nodes"`
	Connections   []Connection `json:"connections"`
	Tombstones    []string     `json:"tombstones"`
}

type Node struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Prompt         string  `json:"prompt"`
	MediaVersionID string  `json:"media_version_id,omitempty"`
	GroupID        string  `json:"group_id,omitempty"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	Revision       int     `json:"revision"`
}

type Connection struct {
	ID         string `json:"id"`
	FromNodeID string `json:"from_node_id"`
	ToNodeID   string `json:"to_node_id"`
	Purpose    string `json:"purpose"`
	Revision   int    `json:"revision"`
}

type Operation struct {
	Kind             string     `json:"kind"`
	Node             Node       `json:"node"`
	NodeID           string     `json:"node_id"`
	Connection       Connection `json:"connection"`
	ConnectionID     string     `json:"connection_id"`
	ExpectedRevision int        `json:"expected_revision"`
}

func ApplyOperations(current Document, operations []Operation) (Document, error) {
	if current.SchemaVersion != SchemaVersion || len(operations) == 0 || len(operations) > 100 {
		return Document{}, ErrInvalid
	}
	next := Document{
		SchemaVersion: SchemaVersion, ProjectID: current.ProjectID, Revision: current.Revision + 1,
		Nodes:       append([]Node(nil), current.Nodes...),
		Connections: append([]Connection(nil), current.Connections...),
		Tombstones:  append([]string(nil), current.Tombstones...),
	}
	for _, operation := range operations {
		switch operation.Kind {
		case "create_node":
			if operation.ExpectedRevision != 0 || operation.Node.Revision != 0 || hasNode(next.Nodes, operation.Node.ID) || contains(next.Tombstones, operation.Node.ID) {
				return Document{}, ErrConflict
			}
			if len(next.Nodes) >= MaxNodes || !validNode(operation.Node) {
				return Document{}, ErrInvalid
			}
			operation.Node.Revision = 1
			next.Nodes = append(next.Nodes, operation.Node)
		case "update_node":
			index := nodeIndex(next.Nodes, operation.Node.ID)
			if index < 0 || next.Nodes[index].Revision != operation.ExpectedRevision {
				return Document{}, ErrConflict
			}
			if operation.Node.Kind != next.Nodes[index].Kind || operation.Node.Revision != operation.ExpectedRevision || !validNode(operation.Node) {
				return Document{}, ErrInvalid
			}
			operation.Node.Revision++
			next.Nodes[index] = operation.Node
		case "delete_node":
			index := nodeIndex(next.Nodes, operation.NodeID)
			if index < 0 || next.Nodes[index].Revision != operation.ExpectedRevision {
				return Document{}, ErrConflict
			}
			for _, node := range next.Nodes {
				if node.GroupID == operation.NodeID {
					return Document{}, ErrInvalid
				}
			}
			next.Nodes = append(next.Nodes[:index], next.Nodes[index+1:]...)
			next.Tombstones = append(next.Tombstones, operation.NodeID)
			kept := next.Connections[:0]
			for _, edge := range next.Connections {
				if edge.FromNodeID != operation.NodeID && edge.ToNodeID != operation.NodeID {
					kept = append(kept, edge)
				}
			}
			next.Connections = kept
		case "create_edge":
			if operation.ExpectedRevision != 0 || operation.Connection.Revision != 0 || hasConnection(next.Connections, operation.Connection.ID) {
				return Document{}, ErrConflict
			}
			for _, existing := range next.Connections {
				if existing.FromNodeID == operation.Connection.FromNodeID && existing.ToNodeID == operation.Connection.ToNodeID {
					return Document{}, ErrConflict
				}
			}
			if len(next.Connections) >= MaxConnections || !validConnection(operation.Connection, next.Nodes) {
				return Document{}, ErrInvalid
			}
			operation.Connection.Revision = 1
			next.Connections = append(next.Connections, operation.Connection)
		case "delete_edge":
			index := connectionIndex(next.Connections, operation.ConnectionID)
			if index < 0 || next.Connections[index].Revision != operation.ExpectedRevision {
				return Document{}, ErrConflict
			}
			next.Connections = append(next.Connections[:index], next.Connections[index+1:]...)
		default:
			return Document{}, ErrInvalid
		}
	}
	if !validGroups(next.Nodes) {
		return Document{}, ErrInvalid
	}
	return next, nil
}

func validNode(node Node) bool {
	parsed, err := uuid.Parse(node.ID)
	if err != nil || parsed.String() != node.ID || len(strings.TrimSpace(node.Title)) == 0 || len(node.Title) > 120 || len(node.Content) > 16000 || len(node.Prompt) > 32000 {
		return false
	}
	switch node.Kind {
	case "text", "image", "video", "audio", "group", "note":
	default:
		return false
	}
	if node.MediaVersionID != "" {
		mediaID, err := uuid.Parse(node.MediaVersionID)
		if err != nil || mediaID.String() != node.MediaVersionID || (node.Kind != "image" && node.Kind != "video" && node.Kind != "audio") {
			return false
		}
	}
	if node.GroupID != "" {
		groupID, err := uuid.Parse(node.GroupID)
		if err != nil || groupID.String() != node.GroupID || node.GroupID == node.ID {
			return false
		}
	}
	for _, value := range []float64{node.X, node.Y, node.Width, node.Height} {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 1_000_000 {
			return false
		}
	}
	return node.Width >= 120 && node.Width <= 3000 && node.Height >= 80 && node.Height <= 3000
}

func validConnection(edge Connection, nodes []Node) bool {
	parsed, err := uuid.Parse(edge.ID)
	if err != nil || parsed.String() != edge.ID {
		return false
	}
	return edge.Purpose == "visual" && edge.FromNodeID != edge.ToNodeID &&
		hasNode(nodes, edge.FromNodeID) && hasNode(nodes, edge.ToNodeID)
}

func validGroups(nodes []Node) bool {
	byID := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, node := range nodes {
		seen := map[string]bool{node.ID: true}
		for parentID := node.GroupID; parentID != ""; {
			parent, ok := byID[parentID]
			if !ok || parent.Kind != "group" || seen[parentID] {
				return false
			}
			seen[parentID] = true
			parentID = parent.GroupID
		}
	}
	return true
}

func hasNode(nodes []Node, id string) bool { return nodeIndex(nodes, id) >= 0 }
func nodeIndex(nodes []Node, id string) int {
	for index, node := range nodes {
		if node.ID == id {
			return index
		}
	}
	return -1
}
func hasConnection(edges []Connection, id string) bool { return connectionIndex(edges, id) >= 0 }
func connectionIndex(edges []Connection, id string) int {
	for index, edge := range edges {
		if edge.ID == id {
			return index
		}
	}
	return -1
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
