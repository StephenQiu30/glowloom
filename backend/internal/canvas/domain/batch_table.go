package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// BatchTableConfig stores editable inputs, never generation or receipt state.
// Runtime model validation and price freezing happen at the quote boundary.
type BatchTableConfig struct {
	Version          int                    `json:"version"`
	Operation        string                 `json:"operation"`
	GlobalPrompt     string                 `json:"global_prompt"`
	Concurrency      int                    `json:"concurrency"`
	ModelProfileID   *uuid.UUID             `json:"model_profile_id" extensions:"x-nullable"`
	Mode             string                 `json:"mode"`
	OutputCount      int                    `json:"output_count"`
	Params           json.RawMessage        `json:"params" swaggertype:"object"`
	ReferenceColumns []BatchReferenceColumn `json:"reference_columns"`
	Rows             []BatchTableRow        `json:"rows"`
}

// BatchReferenceColumn retains column identity while users reorder references.
type BatchReferenceColumn struct {
	ID    uuid.UUID `json:"id"`
	Label string    `json:"label"`
}

// BatchTableRow preserves null slots so reference numbering never shifts.
type BatchTableRow struct {
	ID           uuid.UUID    `json:"id"`
	Enabled      bool         `json:"enabled"`
	Prompt       string       `json:"prompt"`
	InputNodeIDs []*uuid.UUID `json:"input_node_ids" swaggertype:"array,string" extensions:"x-nullable-items"`
}

func validBatchTable(c BatchTableConfig) bool {
	if c.Version != 1 || (c.Operation != "try_on" && c.Operation != "creative") || !validPrompt(c.GlobalPrompt) ||
		c.Concurrency < 1 || c.Concurrency > 32 || !utf8.ValidString(c.Mode) || utf8.RuneCountInString(c.Mode) > 128 || c.OutputCount < 1 || c.OutputCount > 8 || (c.ModelProfileID != nil && *c.ModelProfileID == uuid.Nil) ||
		len(c.ReferenceColumns) < 1 || len(c.ReferenceColumns) > 6 || len(c.Rows) > 500 || !validToolParams(c.Params) {
		return false
	}
	columns := make(map[uuid.UUID]bool, len(c.ReferenceColumns))
	for _, column := range c.ReferenceColumns {
		if column.ID == uuid.Nil || columns[column.ID] || !validTitle(column.Label) {
			return false
		}
		columns[column.ID] = true
	}
	rows := make(map[uuid.UUID]bool, len(c.Rows))
	for _, row := range c.Rows {
		if row.ID == uuid.Nil || rows[row.ID] || !validPrompt(row.Prompt) || len(row.InputNodeIDs) != len(c.ReferenceColumns) {
			return false
		}
		rows[row.ID] = true
		for _, id := range row.InputNodeIDs {
			if id != nil && *id == uuid.Nil {
				return false
			}
		}
	}
	encoded, err := json.Marshal(c)
	return err == nil && len(encoded) <= 512<<10
}

func validToolParams(raw json.RawMessage) bool {
	if len(raw) > 64<<10 {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for name := range fields {
		switch strings.ToLower(name) {
		case "status", "results", "result", "task_id", "operation_id", "batch_id", "provider_request_id", "quote_micros", "receipt", "error":
			return false
		}
	}
	return true
}

func validPrompt(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 10000
}

func validToolReferences(nodes []Node) bool {
	byID := make(map[uuid.UUID]Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, node := range nodes {
		if node.Config.Director != nil && !validDirectorReferences(*node.Config.Director, byID) {
			return false
		}
		if node.Config.Timeline != nil && !validTimelineReferences(*node.Config.Timeline, byID) {
			return false
		}
		if node.Config.BatchTable == nil {
			continue
		}
		for _, row := range node.Config.BatchTable.Rows {
			for _, id := range row.InputNodeIDs {
				if id == nil {
					continue
				}
				reference, ok := byID[*id]
				if !ok || reference.NodeType != "image" || reference.RefType != "media_asset" || reference.RefID == nil {
					return false
				}
			}
		}
	}
	return true
}

func clearBatchReferences(nodes []Node, removed map[uuid.UUID]bool) {
	for i, node := range nodes {
		if node.Config.BatchTable == nil {
			continue
		}
		config := *node.Config.BatchTable
		config.Rows = slices.Clone(config.Rows)
		for j, row := range config.Rows {
			config.Rows[j].InputNodeIDs = slices.Clone(row.InputNodeIDs)
			for slot, id := range row.InputNodeIDs {
				if id != nil && removed[*id] {
					config.Rows[j].InputNodeIDs[slot] = nil
				}
			}
		}
		nodes[i].Config.BatchTable = &config
	}
}
