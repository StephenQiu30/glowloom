package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// readQuoteCanvasSource shares the document lock used by its command writer.
// It freezes provenance and checks references against the saved batch row.
func readQuoteCanvasSource(tx *gorm.DB, projectID uuid.UUID, input application.FreeQuoteItemInput) (int, error) {
	if input.Source == nil {
		return 0, nil
	}
	if !input.Source.Valid() {
		return 0, application.ErrInvalidFreeQuote
	}
	var document struct{ Revision int64 }
	read := tx.Raw(`SELECT revision FROM canvas.document WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete FOR SHARE`, input.Source.CanvasID, projectID).Scan(&document)
	if read.Error != nil {
		return 0, fmt.Errorf("lock quote canvas source: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return 0, application.ErrPublicNotFound
	}
	if document.Revision != input.Source.Revision {
		return 0, application.ErrQuoteSourceStale
	}
	var node struct{ NodeType, Config string }
	read = tx.Raw(`SELECT node_type,config::text AS config FROM canvas.node WHERE id=?::uuid AND document_id=?::uuid AND NOT is_delete`, input.Source.NodeID, input.Source.CanvasID).Scan(&node)
	if read.Error != nil {
		return 0, fmt.Errorf("read quote canvas node: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return 0, application.ErrPublicNotFound
	}
	if node.NodeType != "batch_table" {
		if node.NodeType == "generation" {
			return 0, checkGenerationSource(tx, node.Config, input)
		}
		if input.Source.RowID != nil {
			return 0, application.ErrInvalidFreeQuote
		}
		return 0, nil
	}
	if input.Source.RowID == nil {
		return 0, application.ErrInvalidFreeQuote
	}
	var config canvasdomain.NodeConfig
	if json.Unmarshal([]byte(node.Config), &config) != nil || config.BatchTable == nil || config.BatchTable.Concurrency < 1 || config.BatchTable.Concurrency > 32 {
		return 0, application.ErrInvalidFreeQuote
	}
	if config.BatchTable.Mode != input.Mode || config.BatchTable.OutputCount != int(input.OutputCount) {
		return 0, application.ErrInvalidFreeQuote
	}
	var selected *canvasdomain.BatchTableRow
	for _, row := range config.BatchTable.Rows {
		if row.ID == *input.Source.RowID {
			snapshot := row
			selected = &snapshot
			break
		}
	}
	if selected == nil || !selected.Enabled {
		return 0, application.ErrInvalidFreeQuote
	}
	prompt := strings.TrimSpace(config.BatchTable.GlobalPrompt)
	if prompt == "" {
		prompt = selected.Prompt
	}
	if input.Prompt != prompt || !sameCanvasQuoteParams(config.BatchTable.Params, input.Params) {
		return 0, application.ErrInvalidFreeQuote
	}
	if config.BatchTable.ModelProfileID != nil {
		var model struct{ ID uuid.UUID }
		read = tx.Raw(`SELECT id FROM catalog.model_profile WHERE model_key=? AND NOT is_delete`, input.ModelKey).Scan(&model)
		if read.Error != nil {
			return 0, fmt.Errorf("check source model: %w", read.Error)
		}
		if model.ID != *config.BatchTable.ModelProfileID {
			return 0, application.ErrInvalidFreeQuote
		}
	}
	references := []uuid.UUID{}
	for _, nodeID := range selected.InputNodeIDs {
		if nodeID == nil {
			continue
		}
		var ref struct{ RefID *uuid.UUID }
		read = tx.Raw(`SELECT ref_id FROM canvas.node WHERE id=?::uuid AND document_id=?::uuid AND node_type='image' AND ref_type='media_asset' AND NOT is_delete`, *nodeID, input.Source.CanvasID).Scan(&ref)
		if read.Error != nil {
			return 0, fmt.Errorf("read source reference node: %w", read.Error)
		}
		if read.RowsAffected != 1 || ref.RefID == nil {
			return 0, application.ErrInvalidFreeQuote
		}
		references = append(references, *ref.RefID)
	}
	if len(references) != len(input.MediaInputs) {
		return 0, application.ErrInvalidFreeQuote
	}
	for index, id := range references {
		if id != input.MediaInputs[index].MediaAssetID {
			return 0, application.ErrInvalidFreeQuote
		}
	}
	return config.BatchTable.Concurrency, nil
}

func checkGenerationSource(tx *gorm.DB, raw string, input application.FreeQuoteItemInput) error {
	var config canvasdomain.NodeConfig
	if input.Source.RowID != nil || json.Unmarshal([]byte(raw), &config) != nil || config.Generation == nil {
		return application.ErrInvalidFreeQuote
	}
	saved := config.Generation
	if saved.Capability != input.Capability || saved.Mode != input.Mode || strings.TrimSpace(saved.Prompt) != input.Prompt || saved.OutputCount != int(input.OutputCount) || !sameCanvasQuoteParams(saved.Params, input.Params) || saved.ModelProfileID == nil || len(saved.Inputs) != len(input.MediaInputs) {
		return application.ErrInvalidFreeQuote
	}
	var model struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM catalog.model_profile WHERE model_key=? AND NOT is_delete`, input.ModelKey).Scan(&model).Error; err != nil {
		return fmt.Errorf("check generating model: %w", err)
	}
	if model.ID != *saved.ModelProfileID {
		return application.ErrInvalidFreeQuote
	}
	for i, reference := range saved.Inputs {
		if reference.Role != input.MediaInputs[i].Role || reference.MediaAssetID != input.MediaInputs[i].MediaAssetID {
			return application.ErrInvalidFreeQuote
		}
	}
	return nil
}

func sameCanvasQuoteParams(saved, requested json.RawMessage) bool {
	canonical := func(raw json.RawMessage) ([]byte, error) {
		var object map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&object); err != nil {
			return nil, err
		}
		if object == nil {
			return nil, application.ErrInvalidFreeQuote
		}
		return json.Marshal(object)
	}
	a, err := canonical(saved)
	if err != nil {
		return false
	}
	b, err := canonical(requested)
	return err == nil && bytes.Equal(a, b)
}

func readBatchQuoteSourceScope(tx *gorm.DB, input application.CreateBatchFreeQuoteInput, project freeQuoteProject) (json.RawMessage, error) {
	var first *application.FreeQuoteItemInput
	seen := map[uuid.UUID]bool{}
	for _, item := range input.Items {
		if item.Source == nil {
			continue
		}
		if first == nil {
			snapshot := item
			first = &snapshot
		}
		if item.Source.RowID == nil || item.Source.CanvasID != first.Source.CanvasID || item.Source.NodeID != first.Source.NodeID || item.Source.Revision != first.Source.Revision || seen[*item.Source.RowID] {
			return nil, application.ErrInvalidBatchFreeQuote
		}
		seen[*item.Source.RowID] = true
	}
	if first == nil {
		return json.RawMessage(`{"origin":"canvas"}`), nil
	}
	if len(seen) != len(input.Items) {
		return nil, application.ErrInvalidBatchFreeQuote
	}
	modelKey, err := resolveFreeQuoteModelKey(*first, project)
	if err != nil {
		return nil, err
	}
	first.ModelKey = modelKey
	concurrency, err := readQuoteCanvasSource(tx, input.ProjectID, *first)
	if err != nil {
		return nil, err
	}
	if concurrency < 1 {
		return nil, application.ErrInvalidBatchFreeQuote
	}
	return json.Marshal(struct {
		Origin      string    `json:"origin"`
		CanvasID    uuid.UUID `json:"canvas_id"`
		NodeID      uuid.UUID `json:"node_id"`
		Revision    int64     `json:"revision"`
		Concurrency int       `json:"concurrency"`
	}{"canvas", first.Source.CanvasID, first.Source.NodeID, first.Source.Revision, concurrency})
}
