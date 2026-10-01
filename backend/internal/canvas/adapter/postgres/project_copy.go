package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// CopyMediaReaderFactory authorizes copied assets inside their unpublished target.
// The composition root supplies a media-owned reader bound to this immutable job.
type CopyMediaReaderFactory func(*gorm.DB, application.ProjectCopyBinding) application.MediaReader

// ProjectCopyStore owns frozen canvas content and internal unpublished graph writes.
// Each method runs on the workspace coordinator's injected transaction; it never
// starts an independent transaction or inspects workspace's job/finance tables.
type ProjectCopyStore struct {
	db          *gorm.DB
	sourceMedia MediaReaderFactory
	targetMedia CopyMediaReaderFactory
}

// NewProjectCopyStore injects the admission transaction and media owner readers.
func NewProjectCopyStore(tx *gorm.DB, source MediaReaderFactory, target CopyMediaReaderFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: tx, sourceMedia: source, targetMedia: target}
}

func copyTarget(tx *gorm.DB, binding application.ProjectCopyBinding) error {
	var present int
	read := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete FOR SHARE`, binding.TargetProjectID, binding.OrgID).Scan(&present)
	if read.Error != nil {
		return fmt.Errorf("read unpublished canvas target: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ErrNotFound
	}
	return nil
}

func strictCopyJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: frozen canvas JSON", domain.ErrUnsupportedProjectCopy)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ErrUnsupportedProjectCopy
	}
	return nil
}

func loadCopyDocument(tx *gorm.DB, row documentRow) (domain.Document, error) {
	doc, err := row.document()
	if err != nil {
		return domain.Document{}, err
	}
	if err := strictCopyJSON(row.Viewport, &doc.Viewport); err != nil {
		return domain.Document{}, err
	}
	var nodes []nodeRow
	if err := tx.Raw(`SELECT id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index,last_operation_id FROM canvas.node WHERE document_id=? AND NOT is_delete ORDER BY id FOR SHARE`, row.ID).Scan(&nodes).Error; err != nil {
		return domain.Document{}, err
	}
	for _, node := range nodes {
		n := domain.Node{ID: node.ID, Title: node.Title, NodeType: node.NodeType, NodeAction: node.NodeAction, RefType: node.RefType, RefID: node.RefID, X: node.X, Y: node.Y, Width: node.Width, Height: node.Height, ParentID: node.ParentID, ZIndex: node.ZIndex, LastOperationID: node.LastOperationID}
		if err := strictCopyJSON(node.Config, &n.Config); err != nil {
			return domain.Document{}, err
		}
		doc.Nodes = append(doc.Nodes, n)
	}
	if err := tx.Raw(`SELECT id,edge_type,source_node_id,target_node_id,role,binding FROM canvas.edge WHERE document_id=? AND NOT is_delete ORDER BY id FOR SHARE`, row.ID).Scan(&doc.Edges).Error; err != nil {
		return domain.Document{}, err
	}
	return doc, nil
}

// Freeze snapshots every live canvas under the coordinator's source project lock.
// Unknown persisted config is rejected before any private target content is written.
func (s *ProjectCopyStore) Freeze(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID) (application.ProjectCopySnapshot, error) {
	if s == nil || s.db == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return application.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
	}
	tx := s.db.WithContext(ctx)
	if err := authorize(tx, actor, binding.SourceProjectID, false); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	if err := copyTarget(tx, binding); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	var rows []documentRow
	if err := tx.Raw(`SELECT id,project_id,name,scope,revision,viewport FROM canvas.document WHERE project_id=? AND NOT is_delete ORDER BY id LIMIT 257 FOR SHARE`, binding.SourceProjectID).Scan(&rows).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("freeze project canvases: %w", err)
	}
	if len(rows) > 256 {
		return application.ProjectCopySnapshot{}, domain.ErrUnsupportedProjectCopy
	}
	documents := make([]domain.Document, 0, len(rows))
	for _, row := range rows {
		doc, err := loadCopyDocument(tx, row)
		if err != nil {
			return application.ProjectCopySnapshot{}, err
		}
		if err := (&Store{db: tx, media: s.sourceMedia}).checkMediaReferences(ctx, tx, actor, binding.SourceProjectID, []domain.Command{{Type: "AddNodes", Nodes: doc.Nodes}}, doc); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
		documents = append(documents, doc)
	}
	if _, _, err := application.PrepareProjectCopy(binding, documents, assets); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	body, err := json.Marshal(documents)
	if err != nil || len(body) > 32<<20 {
		return application.ProjectCopySnapshot{}, domain.ErrUnsupportedProjectCopy
	}
	mapping, err := json.Marshal(assets)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	hash := sha256.Sum256(append(append(body, '\n'), mapping...))
	snapshot := application.ProjectCopySnapshot{ID: uuid.NewSHA1(binding.JobID, []byte("canvas-snapshot")), ManifestSHA256: hex.EncodeToString(hash[:]), Documents: len(documents)}
	if err := tx.Exec(`INSERT INTO canvas.project_copy_snapshot(id,job_id,org_id,source_project_id,target_project_id,manifest_sha256,document_count,documents,asset_mapping) VALUES(?,?,?,?,?,?,?,?::jsonb,?::jsonb)`, snapshot.ID, binding.JobID, actor.OrgID, binding.SourceProjectID, binding.TargetProjectID, snapshot.ManifestSHA256, snapshot.Documents, string(body), string(mapping)).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("persist frozen project canvases: %w", err)
	}
	return snapshot, nil
}

// Copy writes all private target graphs and their exact receipt in one transaction.
// The coordinator must first claim and lock its worker fence in this transaction.
func (s *ProjectCopyStore) Copy(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, expected application.ProjectCopySnapshot) (application.ProjectCopyReceipt, error) {
	if s == nil || s.db == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID || expected.ID == uuid.Nil {
		return application.ProjectCopyReceipt{}, domain.ErrInvalidProjectCopy
	}
	tx := s.db.WithContext(ctx)
	if err := copyTarget(tx, binding); err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	var row struct {
		ManifestSHA256 string
		DocumentCount  int
		Documents      []byte
		AssetMapping   []byte
	}
	read := tx.Raw(`SELECT manifest_sha256,document_count,documents,asset_mapping FROM canvas.project_copy_snapshot WHERE id=? AND job_id=? AND org_id=? AND source_project_id=? AND target_project_id=?`, expected.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID).Scan(&row)
	if read.Error != nil {
		return application.ProjectCopyReceipt{}, fmt.Errorf("read frozen project canvases: %w", read.Error)
	}
	if read.RowsAffected != 1 || row.ManifestSHA256 != expected.ManifestSHA256 || row.DocumentCount != expected.Documents {
		return application.ProjectCopyReceipt{}, domain.ErrInvalidProjectCopy
	}
	var documents []domain.Document
	var assets map[uuid.UUID]uuid.UUID
	if strictCopyJSON(row.Documents, &documents) != nil || strictCopyJSON(row.AssetMapping, &assets) != nil || len(documents) != expected.Documents {
		return application.ProjectCopyReceipt{}, domain.ErrInvalidProjectCopy
	}
	body, err := json.Marshal(documents)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	mapping, err := json.Marshal(assets)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	hash := sha256.Sum256(append(append(body, '\n'), mapping...))
	if hex.EncodeToString(hash[:]) != expected.ManifestSHA256 {
		return application.ProjectCopyReceipt{}, domain.ErrInvalidProjectCopy
	}
	copied, contentHash, err := application.PrepareProjectCopy(binding, documents, assets)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	result := application.ProjectCopyReceipt{ManifestSHA256: expected.ManifestSHA256, ContentSHA256: contentHash, Documents: len(copied)}
	for _, item := range copied {
		result.ClearedOperationBindings += item.ClearedOperationBindings
	}
	var previous struct {
		ContentSHA256            string
		DocumentCount            int
		ClearedOperationBindings int
	}
	replayed := tx.Raw(`SELECT content_sha256,document_count,cleared_operation_bindings FROM canvas.project_copy_receipt WHERE snapshot_id=?`, expected.ID).Scan(&previous)
	if replayed.Error != nil {
		return application.ProjectCopyReceipt{}, replayed.Error
	}
	if replayed.RowsAffected == 1 {
		if previous.ContentSHA256 != contentHash || previous.DocumentCount != result.Documents || previous.ClearedOperationBindings != result.ClearedOperationBindings {
			return application.ProjectCopyReceipt{}, domain.ErrInvalidProjectCopy
		}
		if err := verifyCopiedDocuments(tx, copied, binding.TargetProjectID); err != nil {
			return application.ProjectCopyReceipt{}, err
		}
		return result, nil
	}
	if s.targetMedia == nil && len(copied) > 0 {
		return application.ProjectCopyReceipt{}, fmt.Errorf("copy media authorization unavailable")
	}
	for _, item := range copied {
		doc := item.Document
		reader := &Store{db: tx, media: func(database *gorm.DB) application.MediaReader { return s.targetMedia(database, binding) }}
		if err := reader.checkMediaReferences(ctx, tx, actor, binding.TargetProjectID, []domain.Command{{Type: "AddNodes", Nodes: doc.Nodes}}, doc); err != nil {
			return application.ProjectCopyReceipt{}, err
		}
		viewport, err := json.Marshal(doc.Viewport)
		if err != nil {
			return application.ProjectCopyReceipt{}, err
		}
		if err := tx.Exec(`INSERT INTO canvas.document(id,project_id,name,scope,revision,viewport) VALUES(?,?,?,?::jsonb,1,?::jsonb)`, doc.ID, doc.ProjectID, doc.Name, string(doc.Scope), string(viewport)).Error; err != nil {
			return application.ProjectCopyReceipt{}, fmt.Errorf("insert unpublished canvas: %w", err)
		}
		if err := saveGraph(tx, domain.Document{}, doc); err != nil {
			return application.ProjectCopyReceipt{}, fmt.Errorf("insert unpublished graph: %w", err)
		}
	}
	if err := tx.Exec(`INSERT INTO canvas.project_copy_receipt(snapshot_id,content_sha256,document_count,cleared_operation_bindings) VALUES(?,?,?,?)`, expected.ID, result.ContentSHA256, result.Documents, result.ClearedOperationBindings).Error; err != nil {
		return application.ProjectCopyReceipt{}, fmt.Errorf("persist copied canvas receipt: %w", err)
	}
	return result, nil
}

// Cleanup hides only graphs already bound to this immutable private snapshot.
// It does not remove media or alter source documents.
func (s *ProjectCopyStore) Cleanup(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) error {
	if s == nil || s.db == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return domain.ErrInvalidProjectCopy
	}
	tx := s.db.WithContext(ctx)
	if err := copyTarget(tx, binding); err != nil {
		return err
	}
	var frozen struct{ Documents, AssetMapping []byte }
	read := tx.Raw(`SELECT documents,asset_mapping FROM canvas.project_copy_snapshot WHERE id=? AND job_id=? AND org_id=? AND source_project_id=? AND target_project_id=? AND manifest_sha256=? AND document_count=?`, snapshot.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID, snapshot.ManifestSHA256, snapshot.Documents).Scan(&frozen)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return domain.ErrInvalidProjectCopy
	}
	var documents []domain.Document
	var assets map[uuid.UUID]uuid.UUID
	if strictCopyJSON(frozen.Documents, &documents) != nil || strictCopyJSON(frozen.AssetMapping, &assets) != nil || len(documents) != snapshot.Documents {
		return domain.ErrInvalidProjectCopy
	}
	body, err := json.Marshal(documents)
	if err != nil {
		return err
	}
	mapping, err := json.Marshal(assets)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(append(append(body, '\n'), mapping...))
	if hex.EncodeToString(hash[:]) != snapshot.ManifestSHA256 {
		return domain.ErrInvalidProjectCopy
	}
	copied, _, err := application.PrepareProjectCopy(binding, documents, assets)
	if err != nil {
		return err
	}
	for _, document := range copied {
		if err := tx.Exec(`UPDATE canvas.document SET is_delete=true,update_time=statement_timestamp() WHERE id=? AND project_id=? AND NOT is_delete`, document.Document.ID, binding.TargetProjectID).Error; err != nil {
			return err
		}
	}
	return nil
}
