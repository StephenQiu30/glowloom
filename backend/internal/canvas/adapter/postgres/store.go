// Package postgres persists project-authorized canvas documents and replay receipts.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// Store owns database transactions and current project access checks.
type Store struct {
	db    *gorm.DB
	media MediaReaderFactory
}

// MediaReaderFactory binds the media application query to the canvas transaction.
type MediaReaderFactory func(*gorm.DB) application.MediaReader

// NewStore injects an existing migration-managed database connection.
func NewStore(database *gorm.DB, media MediaReaderFactory) *Store {
	return &Store{db: database, media: media}
}

func authorize(tx *gorm.DB, a identityapp.Principal, projectID uuid.UUID, write bool) error {
	if a.ID == uuid.Nil || a.OrgID == uuid.Nil || a.MustChangePassword {
		return identityapp.ErrForbidden
	}
	var current int
	r := tx.Raw(`SELECT 1 FROM identity."user" u JOIN workspace.organization o ON o.id=u.org_id
	 WHERE u.id=? AND u.org_id=? AND u.status='active' AND NOT u.is_delete AND NOT u.must_change_password
	 AND u.role IN ('admin','producer') AND o.status='active' AND NOT o.is_delete FOR SHARE OF u,o`, a.ID, a.OrgID).Scan(&current)
	if r.Error != nil {
		return fmt.Errorf("check canvas actor: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	query := `SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete`
	if write {
		query += ` AND status='active'`
	}
	query += ` FOR SHARE`
	r = tx.Raw(query, projectID, a.OrgID).Scan(&current)
	if r.Error != nil {
		return fmt.Errorf("check canvas project: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrNotFound
	}
	return nil
}
func documentProject(tx *gorm.DB, id uuid.UUID) (uuid.UUID, error) {
	var row struct{ ProjectID uuid.UUID }
	r := tx.Raw(`SELECT project_id FROM canvas.document WHERE id=? AND NOT is_delete`, id).Scan(&row)
	if r.Error != nil {
		return uuid.Nil, r.Error
	}
	if r.RowsAffected != 1 {
		return uuid.Nil, application.ErrNotFound
	}
	return row.ProjectID, nil
}

// List reads document summaries after rechecking organization and project access.
func (s *Store) List(ctx context.Context, a identityapp.Principal, p uuid.UUID) ([]domain.Document, error) {
	items := make([]domain.Document, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorize(tx, a, p, false); err != nil {
			return err
		}
		var rows []documentRow
		if err := tx.Raw(`SELECT id,project_id,name,scope,revision,viewport FROM canvas.document WHERE project_id=? AND NOT is_delete ORDER BY update_time DESC,id`, p).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			d, err := row.document()
			if err != nil {
				return err
			}
			items = append(items, d)
		}
		return nil
	})
	return items, err
}

// Get reads one complete live graph under a document lock.
func (s *Store) Get(ctx context.Context, a identityapp.Principal, id uuid.UUID) (domain.Document, error) {
	var doc domain.Document
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = authorize(tx, a, p, false); err != nil {
			return err
		}
		doc, err = load(tx, id, false)
		return err
	})
	return doc, err
}

// Create commits a document and a durable actor/key response in the same transaction.
func (s *Store) Create(ctx context.Context, a identityapp.Principal, p uuid.UUID, key string, input application.CreateInput) (domain.Document, error) {
	var doc domain.Document
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorize(tx, a, p, true); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		hash := requestHash("create/"+p.String(), body)
		replayed, err := replay(tx, a.ID, key, hash, &doc)
		if err != nil || replayed {
			return err
		}
		doc = domain.Document{ID: uuid.New(), ProjectID: p, Name: input.Name, Scope: input.Scope, Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{}, Edges: []domain.Edge{}}
		viewport, _ := json.Marshal(doc.Viewport)
		if err := tx.Exec(`INSERT INTO canvas.document(id,project_id,name,scope,viewport) VALUES (?,?,?,?::jsonb,?::jsonb)`, doc.ID, p, doc.Name, string(doc.Scope), string(viewport)).Error; err != nil {
			return err
		}
		if err := changed(tx, a, doc.ID, p, 1); err != nil {
			return err
		}
		return receipt(tx, a.ID, key, hash, 201, doc)
	})
	return doc, err
}

// Execute locks the document, applies the complete batch, and stores one revision and replay response.
func (s *Store) Execute(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input application.CommandsInput) (application.Result, error) {
	var result application.Result
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = authorize(tx, a, p, true); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return domain.ErrInvalidCommand
		}
		hash := requestHash("commands/"+id.String(), body)
		replayed, err := replay(tx, a.ID, key, hash, &result)
		if err != nil || replayed {
			return err
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		if doc.Revision != input.ExpectedRevision {
			return &application.RevisionConflict{CurrentRevision: doc.Revision}
		}
		updated, err := domain.Apply(doc, input.Commands)
		if err != nil {
			return err
		}
		if doc.Revision >= math.MaxInt32 {
			return domain.ErrInvalidCommand
		}
		if err := s.checkMediaReferences(ctx, tx, a, p, input.Commands, updated); err != nil {
			return err
		}
		updated.Revision++
		if err := saveGraph(tx, doc, updated); err != nil {
			var collision *identityConflict
			if errors.As(err, &collision) {
				for index, command := range input.Commands {
					for _, node := range command.Nodes {
						if !collision.Edge && node.ID == collision.ID {
							return &domain.CommandError{Index: index, Cause: application.ErrNotFound}
						}
					}
					for _, edge := range command.Edges {
						if collision.Edge && edge.ID == collision.ID {
							return &domain.CommandError{Index: index, Cause: application.ErrNotFound}
						}
					}
				}
			}
			return err
		}
		commands, err := json.Marshal(input.Commands)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,create_by) VALUES(?,?,?,?::jsonb,?)`, uuid.New(), id, updated.Revision, string(commands), a.ID).Error; err != nil {
			return err
		}
		if err := changed(tx, a, id, p, updated.Revision); err != nil {
			return err
		}
		result = application.Result{Document: updated, Results: make([]application.CommandResult, len(input.Commands))}
		for i := range result.Results {
			result.Results[i].OK = true
		}
		return receipt(tx, a.ID, key, hash, 200, result)
	})
	return result, err
}

type documentRow struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Scope     []byte
	Revision  int64
	Viewport  []byte
}

func (r documentRow) document() (domain.Document, error) {
	d := domain.Document{ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Scope: json.RawMessage(r.Scope), Revision: r.Revision, Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	if err := json.Unmarshal(r.Viewport, &d.Viewport); err != nil {
		return domain.Document{}, err
	}
	if d.Viewport.Zoom == 0 {
		d.Viewport.Zoom = 1
	}
	return d, nil
}
func load(tx *gorm.DB, id uuid.UUID, write bool) (domain.Document, error) {
	var row documentRow
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	r := tx.Raw(`SELECT id,project_id,name,scope,revision,viewport FROM canvas.document WHERE id=? AND NOT is_delete`+lock, id).Scan(&row)
	if r.Error != nil {
		return domain.Document{}, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Document{}, application.ErrNotFound
	}
	doc, err := row.document()
	if err != nil {
		return doc, err
	}
	var nodes []nodeRow
	if err := tx.Raw(`SELECT id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index,last_operation_id FROM canvas.node WHERE document_id=? AND NOT is_delete ORDER BY id`, id).Scan(&nodes).Error; err != nil {
		return doc, err
	}
	for _, r := range nodes {
		n := domain.Node{ID: r.ID, Title: r.Title, NodeType: r.NodeType, NodeAction: r.NodeAction, X: r.X, Y: r.Y, Width: r.Width, Height: r.Height, RefType: r.RefType, RefID: r.RefID, ParentID: r.ParentID, LastOperationID: r.LastOperationID, ZIndex: r.ZIndex}
		if err := json.Unmarshal(r.Config, &n.Config); err != nil {
			return doc, err
		}
		doc.Nodes = append(doc.Nodes, n)
	}
	if err := tx.Raw(`SELECT id,edge_type,source_node_id,target_node_id,role,binding FROM canvas.edge WHERE document_id=? AND NOT is_delete ORDER BY id`, id).Scan(&doc.Edges).Error; err != nil {
		return doc, err
	}
	return doc, nil
}

type nodeRow struct {
	ID              uuid.UUID
	Title           string
	NodeType        string
	NodeAction      string
	RefType         string
	RefID           *uuid.UUID
	Config          []byte
	X               float64
	Y               float64
	Width           *float64
	Height          *float64
	ParentID        *uuid.UUID
	ZIndex          int32
	LastOperationID *uuid.UUID
}

func saveGraph(tx *gorm.DB, before, after domain.Document) error {
	previousNodes := make(map[uuid.UUID]domain.Node, len(before.Nodes))
	for _, n := range before.Nodes {
		previousNodes[n.ID] = n
	}
	previousEdges := make(map[uuid.UUID]domain.Edge, len(before.Edges))
	for _, e := range before.Edges {
		previousEdges[e.ID] = e
	}
	for _, n := range before.Nodes {
		found := false
		for _, kept := range after.Nodes {
			if kept.ID == n.ID {
				found = true
				break
			}
		}
		if !found {
			if err := tx.Exec(`UPDATE canvas.node SET is_delete=true,update_time=now() WHERE id=? AND document_id=?`, n.ID, after.ID).Error; err != nil {
				return err
			}
		}
	}
	for _, n := range after.Nodes {
		if previous, exists := previousNodes[n.ID]; exists && reflect.DeepEqual(previous, n) {
			continue
		}
		if (n.NodeAction != "resource" && n.NodeAction != "tool") || n.LastOperationID != nil {
			continue
		}
		config, err := json.Marshal(n.Config)
		if err != nil {
			return err
		}
		r := tx.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index)
 VALUES(?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,config=excluded.config,x=excluded.x,y=excluded.y,width=excluded.width,height=excluded.height,parent_id=excluded.parent_id,z_index=excluded.z_index,is_delete=false,update_time=now()
 WHERE canvas.node.document_id=excluded.document_id AND canvas.node.node_type=excluded.node_type AND canvas.node.node_action=excluded.node_action AND canvas.node.node_action IN ('resource','tool') AND canvas.node.ref_type IS NOT DISTINCT FROM excluded.ref_type AND canvas.node.ref_id IS NOT DISTINCT FROM excluded.ref_id AND canvas.node.last_operation_id IS NULL`, n.ID, after.ID, n.Title, n.NodeType, n.NodeAction, nullableText(n.RefType), n.RefID, string(config), n.X, n.Y, n.Width, n.Height, n.ParentID, n.ZIndex)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return &identityConflict{ID: n.ID}
		}
	}
	for _, e := range before.Edges {
		found := false
		for _, kept := range after.Edges {
			if e.ID == kept.ID {
				found = true
				break
			}
		}
		if !found {
			if err := tx.Exec(`UPDATE canvas.edge SET is_delete=true,update_time=now() WHERE id=? AND document_id=?`, e.ID, after.ID).Error; err != nil {
				return err
			}
		}
	}
	for _, e := range after.Edges {
		if previous, exists := previousEdges[e.ID]; exists && reflect.DeepEqual(previous, e) {
			continue
		}
		if e.EdgeType != "annotation" {
			continue
		}
		r := tx.Exec(`INSERT INTO canvas.edge(id,document_id,edge_type,source_node_id,target_node_id) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET source_node_id=excluded.source_node_id,target_node_id=excluded.target_node_id,is_delete=false,update_time=now() WHERE canvas.edge.document_id=excluded.document_id AND canvas.edge.edge_type='annotation' AND canvas.edge.role IS NULL AND canvas.edge.binding IS NULL`, e.ID, after.ID, e.EdgeType, e.SourceNodeID, e.TargetNodeID)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return &identityConflict{ID: e.ID, Edge: true}
		}
	}
	viewport, err := json.Marshal(after.Viewport)
	if err != nil {
		return err
	}
	return tx.Exec(`UPDATE canvas.document SET revision=?,viewport=?::jsonb,update_time=now() WHERE id=?`, after.Revision, string(viewport), after.ID).Error
}
func requestHash(route string, body []byte) string {
	hash := sha256.Sum256(append([]byte(route+"\x00"), body...))
	return hex.EncodeToString(hash[:])
}
func replay(tx *gorm.DB, actor uuid.UUID, key, hash string, out any) (bool, error) {
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.String()+":"+key).Scan(&locked).Error; err != nil {
		return false, err
	}
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := tx.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>now()`, actor, key).Scan(&row)
	if r.Error != nil {
		return false, r.Error
	}
	if r.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != hash {
		return false, application.ErrIdempotencyConflict
	}
	return true, json.Unmarshal(row.ResponseBody, out)
}
func receipt(tx *gorm.DB, actor uuid.UUID, key, hash string, status int, response any) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,?,?,?::jsonb,now()+interval '24 hours')
	 ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=excluded.status_code,response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=now()`, uuid.New(), actor, key, hash, status, string(body)).Error
}
func changed(tx *gorm.DB, a identityapp.Principal, id, p uuid.UUID, revision int64) error {
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": "lanverse.canvas.document_changed.v1", "occurred_at": time.Now().UTC(), "org_id": a.OrgID, "project_id": p, "actor": map[string]any{"kind": "user", "id": a.ID}, "aggregate": map[string]any{"type": "canvas_document", "id": id, "revision": revision}, "data": map[string]any{"document_id": id, "revision": revision}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?, 'lanverse.canvas.document_changed.v1',?,?::jsonb)`, eventID, p.String(), string(payload)).Error
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type identityConflict struct {
	ID   uuid.UUID
	Edge bool
}

func (e *identityConflict) Error() string { return "canvas identity unavailable" }
func (e *identityConflict) Unwrap() error { return application.ErrNotFound }
func (s *Store) checkMediaReferences(ctx context.Context, tx *gorm.DB, a identityapp.Principal, p uuid.UUID, commands []domain.Command, document domain.Document) error {
	checked := make(map[uuid.UUID]string)
	check := func(id uuid.UUID, kind string, index int) error {
		if previous, ok := checked[id]; ok {
			if previous != kind {
				return &domain.CommandError{Index: index, Cause: domain.ErrInvalidCommand}
			}
			return nil
		}
		if s.media == nil {
			return fmt.Errorf("media query not configured")
		}
		asset, err := s.media(tx).Reference(ctx, a, p, id)
		if err != nil {
			return &domain.CommandError{Index: index, Cause: err}
		}
		if asset.Kind != kind {
			return &domain.CommandError{Index: index, Cause: domain.ErrInvalidCommand}
		}
		checked[id] = kind
		return nil
	}
	checkTimeline := func(config *domain.TimelineConfig, index int) error {
		if config == nil {
			return nil
		}
		for _, clip := range config.Clips {
			assetID := clip.AssetID
			if assetID == nil && clip.NodeID != nil {
				for _, node := range document.Nodes {
					if node.ID == *clip.NodeID {
						assetID = node.RefID
						break
					}
				}
			}
			if assetID != nil {
				if err := check(*assetID, clip.Kind, index); err != nil {
					return err
				}
			}
		}
		return nil
	}
	checkDirector := func(config *domain.DirectorConfig, index int) error {
		if config == nil {
			return nil
		}
		if config.Panorama != nil {
			if err := check(config.Panorama.AssetID, "image", index); err != nil {
				return err
			}
		}
		for _, object := range config.Objects {
			assetID := object.AssetID
			if assetID == nil && object.SourceNodeID != nil {
				for _, node := range document.Nodes {
					if node.ID == *object.SourceNodeID {
						assetID = node.RefID
						break
					}
				}
			}
			if assetID == nil {
				continue
			}
			kind := "model"
			if object.Kind == "billboard" {
				kind = "image"
			}
			if err := check(*assetID, kind, index); err != nil {
				return err
			}
		}
		return nil
	}
	checkGeneration := func(config *domain.GenerationConfig, index int) error {
		if config == nil {
			return nil
		}
		for _, input := range config.Inputs {
			if s.media == nil {
				return fmt.Errorf("media query not configured")
			}
			asset, err := s.media(tx).Reference(ctx, a, p, input.MediaAssetID)
			if err != nil {
				return &domain.CommandError{Index: index, Cause: err}
			}
			if asset.Kind != "image" && asset.Kind != "video" && asset.Kind != "audio" {
				return &domain.CommandError{Index: index, Cause: domain.ErrInvalidCommand}
			}
		}
		return nil
	}
	for index, c := range commands {
		if c.Config != nil {
			if err := checkGeneration(c.Config.Generation, index); err != nil {
				return err
			}
			if err := checkDirector(c.Config.Director, index); err != nil {
				return err
			}
			if err := checkTimeline(c.Config.Timeline, index); err != nil {
				return err
			}
		}
		for _, n := range c.Nodes {
			if err := checkGeneration(n.Config.Generation, index); err != nil {
				return err
			}
			if err := checkDirector(n.Config.Director, index); err != nil {
				return err
			}
			if err := checkTimeline(n.Config.Timeline, index); err != nil {
				return err
			}
			if n.RefType != "media_asset" || n.RefID == nil {
				continue
			}
			if err := check(*n.RefID, n.NodeType, index); err != nil {
				return err
			}
		}
	}
	return nil
}

// Rename keeps document metadata in the same revision, log, event and receipt transaction.
func (s *Store) Rename(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input application.RenameInput) (domain.Document, error) {
	var result domain.Document
	err := s.mutateDocument(ctx, a, id, key, input.ExpectedRevision, "rename", input, input.Name, false, &result)
	return result, err
}

// Delete retains rows for recovery while making the entire graph inaccessible.
func (s *Store) Delete(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input application.DeleteInput) (application.DeleteResult, error) {
	var result application.DeleteResult
	err := s.mutateDocument(ctx, a, id, key, input.ExpectedRevision, "delete", input, "", true, &result)
	return result, err
}
func (s *Store) mutateDocument(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, expected int64, kind string, input any, name string, remove bool, result any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ProjectID uuid.UUID }
		r := tx.Raw(`SELECT project_id FROM canvas.document WHERE id=?`, id).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrNotFound
		}
		if err := authorize(tx, a, row.ProjectID, true); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		hash := requestHash(kind+"/"+id.String(), body)
		replayed, err := replay(tx, a.ID, key, hash, result)
		if err != nil || replayed {
			return err
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		if doc.Revision != expected {
			return &application.RevisionConflict{CurrentRevision: doc.Revision}
		}
		if doc.Revision >= math.MaxInt32 {
			return domain.ErrInvalidCommand
		}
		doc.Revision++
		if remove {
			if err := tx.Exec(`UPDATE canvas.document SET is_delete=true,revision=?,update_time=now() WHERE id=?`, doc.Revision, id).Error; err != nil {
				return err
			}
			*result.(*application.DeleteResult) = application.DeleteResult{ID: id, Revision: doc.Revision, Deleted: true}
		} else {
			doc.Name = name
			if err := tx.Exec(`UPDATE canvas.document SET name=?,revision=?,update_time=now() WHERE id=?`, name, doc.Revision, id).Error; err != nil {
				return err
			}
			*result.(*domain.Document) = doc
		}
		log, err := json.Marshal([]map[string]any{{"type": kind, "name": name}})
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,create_by) VALUES(?,?,?,?::jsonb,?)`, uuid.New(), id, doc.Revision, string(log), a.ID).Error; err != nil {
			return err
		}
		if err := changed(tx, a, id, doc.ProjectID, doc.Revision); err != nil {
			return err
		}
		return receipt(tx, a.ID, key, hash, 200, result)
	})
}
