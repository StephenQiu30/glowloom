package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

const (
	maxMediaReferenceFacts        = 50000
	maxMediaReferencePayloadBytes = 1 << 20
	maxMediaReferenceHistoryBytes = 64 << 20
)

// MediaReferenceGuard protects current and retained canvas facts in the caller's transaction.
type MediaReferenceGuard struct{ tx *gorm.DB }

// NewMediaReferenceGuard never acquires a media reader or an independent transaction.
func NewMediaReferenceGuard(tx *gorm.DB) *MediaReferenceGuard { return &MediaReferenceGuard{tx: tx} }

// HasMediaReferences includes soft removed graphs, nodes and all historical typed commands.
func (g *MediaReferenceGuard) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if g == nil || g.tx == nil || g.tx.Statement == nil || asset == uuid.Nil || project == uuid.Nil {
		return false, application.ErrNotFound
	}
	if _, ok := g.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, fmt.Errorf("canvas reference guard requires caller transaction")
	}
	tx := g.tx.WithContext(ctx)
	if err := authorize(tx, actor, project, false); err != nil {
		return false, err
	}
	// Current canvas writers take a shared project lock. Upgrade only after
	// current actor and project authorization, retaining the same caller
	// transaction and actor-before-project order throughout admission.
	var locked int
	lock := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? FOR UPDATE`, project, actor.OrgID).Scan(&locked)
	if lock.Error != nil {
		return false, fmt.Errorf("retain canvas history admission lock: %w", lock.Error)
	}
	if lock.RowsAffected != 1 {
		return false, application.ErrNotFound
	}
	// Account for nodes and commands together before either scan materializes
	// JSON. The exclusive project lock prevents owning writers from committing
	// new history between this preflight and the subsequent bounded scans.
	var history struct{ Facts, Bytes, MaxBytes int64 }
	budget := tx.Raw(`SELECT count(*) AS facts,coalesce(sum(payload_bytes),0) AS bytes,coalesce(max(payload_bytes),0) AS max_bytes FROM (
 SELECT octet_length(n.config::text)::bigint AS payload_bytes FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id WHERE d.project_id=?
 UNION ALL SELECT octet_length(c.commands::text)::bigint FROM canvas.command_log c JOIN canvas.document d ON d.id=c.document_id WHERE d.project_id=?
 LIMIT ?
) retained`, project, project, maxMediaReferenceFacts+1).Scan(&history)
	if budget.Error != nil {
		return false, fmt.Errorf("read retained canvas history budget: %w", budget.Error)
	}
	if budget.RowsAffected != 1 || history.Facts > maxMediaReferenceFacts || history.Bytes > maxMediaReferenceHistoryBytes || history.MaxBytes > maxMediaReferencePayloadBytes {
		return false, fmt.Errorf("retained canvas history exceeds reference read budget")
	}
	var nodes []struct {
		RefType string
		RefID   *uuid.UUID
		Config  []byte
	}
	if err := tx.Raw(`SELECT n.ref_type,n.ref_id,n.config FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id WHERE d.project_id=? ORDER BY d.id,n.id LIMIT ? FOR SHARE OF n,d`, project, maxMediaReferenceFacts+1).Scan(&nodes).Error; err != nil {
		return false, fmt.Errorf("read retained canvas nodes: %w", err)
	}
	if len(nodes) > maxMediaReferenceFacts {
		return false, fmt.Errorf("retained canvas nodes exceed reference read budget")
	}
	for _, node := range nodes {
		var config domain.NodeConfig
		if err := decodeReferenceJSON(node.Config, &config); err != nil {
			return false, err
		}
		found, err := nodeMediaReference(node.RefType, node.RefID, config, asset)
		if err != nil || found {
			return found, err
		}
	}
	var logs []struct{ Commands []byte }
	if err := tx.Raw(`SELECT c.commands FROM canvas.command_log c JOIN canvas.document d ON d.id=c.document_id WHERE d.project_id=? ORDER BY d.id,c.revision LIMIT ? FOR SHARE OF c,d`, project, maxMediaReferenceFacts+1).Scan(&logs).Error; err != nil {
		return false, fmt.Errorf("read retained canvas commands: %w", err)
	}
	if len(nodes)+len(logs) > maxMediaReferenceFacts {
		return false, fmt.Errorf("retained canvas facts exceed reference read budget")
	}
	for _, log := range logs {
		var commands []json.RawMessage
		if err := decodeReferenceJSON(log.Commands, &commands); err != nil || commands == nil {
			return false, domain.ErrInvalidCommand
		}
		for _, raw := range commands {
			var header struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &header); err != nil {
				return false, domain.ErrInvalidCommand
			}
			if header.Type == "rename" || header.Type == "delete" {
				var metadata struct {
					Type string `json:"type"`
					Name string `json:"name"`
				}
				if err := decodeReferenceJSON(raw, &metadata); err != nil {
					return false, err
				}
				continue
			}
			var command domain.Command
			if err := decodeReferenceJSON(raw, &command); err != nil {
				return false, err
			}
			switch command.Type {
			case "AddNodes", "MoveNodes", "ResizeNodes", "RenameNodes", "SetNodeParents", "SetNodeZIndex", "UpdateNodeConfig", "DeleteNodes", "Connect", "Disconnect", "SetViewport":
			default:
				return false, domain.ErrUnsupportedCommand
			}
			if command.Type == "UpdateNodeConfig" && command.Config == nil {
				return false, domain.ErrInvalidCommand
			}
			if command.Config != nil {
				found, err := configMediaReference(*command.Config, asset)
				if err != nil || found {
					return found, err
				}
			}
			for _, node := range command.Nodes {
				found, err := nodeMediaReference(node.RefType, node.RefID, node.Config, asset)
				if err != nil || found {
					return found, err
				}
			}
		}
	}
	return false, nil
}
func decodeReferenceJSON(raw []byte, target any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return domain.ErrInvalidCommand
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return domain.ErrInvalidCommand
	}
	return nil
}
func nodeMediaReference(kind string, id *uuid.UUID, config domain.NodeConfig, asset uuid.UUID) (bool, error) {
	if kind != "" && kind != "media_asset" {
		return false, domain.ErrUnsupportedCommand
	}
	if kind == "media_asset" {
		if id == nil || *id == uuid.Nil {
			return false, domain.ErrInvalidCommand
		}
		if *id == asset {
			return true, nil
		}
	}
	return configMediaReference(config, asset)
}
func configMediaReference(config domain.NodeConfig, asset uuid.UUID) (bool, error) {
	var ids []uuid.UUID
	if c := config.Generation; c != nil {
		for _, input := range c.Inputs {
			ids = append(ids, input.MediaAssetID)
		}
	}
	if c := config.Timeline; c != nil {
		for _, clip := range c.Clips {
			if clip.AssetID != nil {
				ids = append(ids, *clip.AssetID)
			}
		}
	}
	if c := config.Director; c != nil {
		if c.Cover != nil {
			ids = append(ids, c.Cover.AssetID)
		}
		if c.Panorama != nil {
			ids = append(ids, c.Panorama.AssetID)
		}
		for _, object := range c.Objects {
			if object.AssetID != nil {
				ids = append(ids, *object.AssetID)
			}
		}
		for _, shot := range c.Shots {
			for _, screenshot := range shot.Screenshots {
				ids = append(ids, screenshot.AssetID)
			}
		}
	}
	for _, id := range ids {
		if id == uuid.Nil {
			return false, domain.ErrInvalidCommand
		}
		if id == asset {
			return true, nil
		}
	}
	return false, nil
}
