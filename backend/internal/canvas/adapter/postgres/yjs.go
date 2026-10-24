package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type yjsRow struct {
	DocumentID      uuid.UUID
	Epoch           uuid.UUID
	WriterTerm      int64
	CommitSeq       int64
	ContentRevision int64
	State           []byte
	GraphHash       string
}

func loadYjsRow(tx *gorm.DB, id uuid.UUID) (yjsRow, error) {
	var row yjsRow
	err := tx.Raw(`SELECT document_id,epoch,writer_term,commit_seq,content_revision,state,graph_hash FROM canvas.collab_document WHERE document_id=?`, id).Scan(&row).Error
	return row, err
}
func yjsHash(input application.YjsUpdate) string {
	body, _ := json.Marshal(input)
	return requestHash("yjs/v1", body)
}
func yjsReceipt(tx *gorm.DB, a identity.Principal, id uuid.UUID, input application.YjsUpdate) (*application.YjsReceipt, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	result := tx.Raw(`SELECT request_hash,response_body FROM canvas.collab_receipt WHERE document_id=? AND epoch=? AND actor_id=? AND session_id=? AND update_id=?`, id, input.Epoch, a.ID, input.SessionID, input.UpdateID).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	if row.RequestHash != yjsHash(input) {
		return nil, application.ErrIdempotencyConflict
	}
	var receipt application.YjsReceipt
	if err := json.Unmarshal(row.ResponseBody, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}
func storeYjsReceipt(tx *gorm.DB, a identity.Principal, id uuid.UUID, input application.YjsUpdate, result application.YjsReceipt) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO canvas.collab_receipt(document_id,epoch,actor_id,session_id,update_id,request_hash,response_body) VALUES(?,?,?,?,?,?,?::jsonb)`, id, input.Epoch, a.ID, input.SessionID, input.UpdateID, yjsHash(input), string(body)).Error
}

// LoadYjs serializes the graph read with accepted binary state; permissions are current.
func (s *Store) LoadYjs(ctx context.Context, a identity.Principal, id uuid.UUID) (application.YjsState, error) {
	var result application.YjsState
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		write, err := s.readAuthority(tx, a, p)
		if err != nil {
			return err
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		doc.Writable = write && doc.ReadOnlyReason == ""
		row, err := loadYjsRow(tx, id)
		if err != nil {
			return err
		}
		if len(row.State) > 0 && row.ContentRevision != doc.Revision {
			return fmt.Errorf("collaboration projection drift")
		}
		var deleted []nodeRow
		if err := tx.Raw(`SELECT id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index,last_operation_id,COALESCE(input_revision,0) AS input_revision,content_version FROM canvas.node WHERE document_id=? AND is_delete ORDER BY update_time DESC LIMIT 128`, id).Scan(&deleted).Error; err != nil {
			return err
		}
		tombstones := domain.Document{}
		for _, r := range deleted {
			n := domain.Node{ID: r.ID, ContentVersion: r.ContentVersion, InputRevision: r.InputRevision, Title: r.Title, NodeType: r.NodeType, NodeAction: r.NodeAction, X: r.X, Y: r.Y, Width: r.Width, Height: r.Height, RefType: r.RefType, RefID: r.RefID, ParentID: r.ParentID, LastOperationID: r.LastOperationID, ZIndex: r.ZIndex}
			if err := json.Unmarshal(r.Config, &n.Config); err != nil {
				return err
			}
			tombstones.Nodes = append(tombstones.Nodes, n)
		}
		result = application.YjsState{RestorationNodes: application.Snapshot(tombstones).Nodes, YjsBinding: application.YjsBinding{DocumentID: id, Epoch: row.Epoch, WriterTerm: application.Revision(row.WriterTerm), CommitSeq: application.Revision(row.CommitSeq)}, State: row.State, GraphHash: row.GraphHash, Document: application.Snapshot(doc)}
		return nil
	})
	return result, err
}

// InitializeYjs stores the lossless conversion under the document write lock.
func (s *Store) InitializeYjs(ctx context.Context, a identity.Principal, base application.YjsState, candidate application.YjsCandidate) (application.YjsState, error) {
	var result application.YjsState
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id := base.DocumentID
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, true); err != nil {
			return err
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		row, err := loadYjsRow(tx, id)
		if err != nil {
			return err
		}
		if row.DocumentID != uuid.Nil || application.Revision(doc.Revision) != base.Document.Revision {
			return application.ErrYjsContention
		}
		ops, err := application.YjsProjectionOperations(doc, candidate.Projection)
		if err != nil || len(ops) > 0 || candidate.Projection.Name != doc.Name {
			return fmt.Errorf("initial collaboration projection differs from source")
		}
		if err := tx.Exec(`INSERT INTO canvas.collab_document(document_id,epoch,writer_term,commit_seq,content_revision,state,graph_hash) VALUES(?,?,?,0,?,?,?)`, id, base.Epoch, base.WriterTerm, doc.Revision, candidate.State, candidate.GraphHash).Error; err != nil {
			return err
		}
		if err := recordYjsVersion(tx, doc, base.Epoch, a.ID, "migration"); err != nil {
			return err
		}
		result = base
		result.State = candidate.State
		result.GraphHash = candidate.GraphHash
		return nil
	})
	return result, err
}

// ResolveYjs checks current access before returning a durable original outcome.
func (s *Store) ResolveYjs(ctx context.Context, a identity.Principal, id uuid.UUID, input application.YjsUpdate) (*application.YjsReceipt, error) {
	var result *application.YjsReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, false); err != nil {
			return err
		}
		result, err = yjsReceipt(tx, a, id, input)
		return err
	})
	return result, err
}

// CommitYjs reuses the domain editing validator inside the same outer transaction.
// No accepted state, receipt, operation log or Outbox is visible before this commits.
func (s *Store) CommitYjs(ctx context.Context, a identity.Principal, base application.YjsState, input application.YjsUpdate, candidate *application.YjsCandidate, rejection string) (application.YjsReceipt, error) {
	var result application.YjsReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id := base.DocumentID
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, true); err != nil {
			return err
		}
		doc, err := load(tx, id, true)
		if err != nil {
			return err
		}
		replay, err := yjsReceipt(tx, a, id, input)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		row, err := loadYjsRow(tx, id)
		if err != nil {
			return err
		}
		if row.Epoch != base.Epoch || row.WriterTerm != int64(base.WriterTerm) || row.CommitSeq != int64(base.CommitSeq) || doc.Revision != int64(base.Document.Revision) || row.ContentRevision != doc.Revision {
			return application.ErrYjsContention
		}
		reject := func(code string) error {
			result = application.YjsReceipt{Epoch: input.Epoch, UpdateID: input.UpdateID, PayloadHash: input.PayloadHash, Status: "rejected", Code: code, CommitSeq: base.CommitSeq, ContentRevision: application.Revision(doc.Revision), GraphHash: row.GraphHash}
			return storeYjsReceipt(tx, a, id, input, result)
		}
		if rejection != "" {
			return reject(rejection)
		}
		if candidate == nil {
			return fmt.Errorf("missing collaboration candidate")
		}
		if doc.ReadOnlyReason != "" {
			return domain.ErrGraphReadOnly
		}
		if candidate.Projection.Name != doc.Name {
			presentation, err := s.presentation(tx, a, doc.ProjectID)
			if err != nil {
				return err
			}
			if presentation.MainCanvasID == id {
				return reject("MAIN_CANVAS_MANAGED_BY_PROJECT")
			}
		}
		operations, err := application.YjsProjectionOperations(doc, candidate.Projection)
		restore := candidate.Transition != nil
		if restore {
			operations, err = yjsRestoration(tx, a.ID, doc, row, candidate)
		}
		if err != nil {
			code := projectionBusinessCode(err)
			if code == "" {
				code = "INVALID_PROJECTION"
			}
			return reject(code)
		}
		if row.CommitSeq == math.MaxInt64 {
			return reject("COMMIT_SEQUENCE_LIMIT")
		}
		if candidate.Effect != "NO_CHANGE" && candidate.Effect != "CAUSAL_CHANGE" && candidate.Effect != "VISIBLE_CHANGE" {
			return fmt.Errorf("invalid candidate effect")
		}
		if candidate.Effect == "NO_CHANGE" && (!reflect.DeepEqual(candidate.State, row.State) || len(operations) > 0 || candidate.Projection.Name != doc.Name) {
			return fmt.Errorf("inconsistent no-change candidate")
		}
		if err := recordYjsVersion(tx, doc, row.Epoch, a.ID, "accepted"); err != nil {
			return err
		}
		revision := doc.Revision
		if len(operations) > 0 {
			if err := tx.SavePoint("yjs_projection").Error; err != nil {
				return err
			}
			if err := tx.Exec(`SELECT set_config('glowloom.yjs_document_id',?,true)`, id.String()).Error; err != nil {
				return err
			}
			revision, err = s.persistYjsProjection(ctx, tx, a, doc, input, operations, restore)
			if err != nil {
				if code := projectionBusinessCode(err); code != "" {
					if rollback := tx.RollbackTo("yjs_projection").Error; rollback != nil {
						return rollback
					}
					return reject(code)
				}
				return err
			}
		}
		if candidate.Projection.Name != doc.Name {
			if err := tx.Exec(`SELECT set_config('glowloom.yjs_document_id',?,true)`, id.String()).Error; err != nil {
				return err
			}
			nested := *s
			nested.db = tx
			key := uuid.NewSHA1(input.UpdateID, []byte("rename/"+a.ID.String()+input.SessionID.String()+input.Epoch.String())).String()
			renamed, err := application.NewService(&nested).Rename(ctx, a, id, key, application.RenameInput{Name: candidate.Projection.Name, ExpectedRevision: revision})
			if err != nil {
				return err
			}
			revision = renamed.Revision
		}
		seq := row.CommitSeq
		if candidate.Effect != "NO_CHANGE" {
			seq++
			if err := tx.Exec(`UPDATE canvas.collab_document SET commit_seq=?,content_revision=?,state=?,graph_hash=?,updated_at=clock_timestamp() WHERE document_id=?`, seq, revision, candidate.State, candidate.GraphHash, id).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO canvas.collab_update(document_id,epoch,commit_seq,content_revision,actor_id,session_id,update_id,payload_hash,update_bytes) VALUES(?,?,?,?,?,?,?,?,?)`, id, input.Epoch, seq, revision, a.ID, input.SessionID, input.UpdateID, input.PayloadHash, candidate.AppliedUpdate).Error; err != nil {
				return err
			}
			if err := collaborationCommitted(tx, a, id, doc.ProjectID, input.Epoch, seq, revision); err != nil {
				return err
			}
		}
		updated, err := load(tx, id, false)
		if err != nil {
			return err
		}
		if err = recordYjsVersion(tx, updated, row.Epoch, a.ID, "accepted"); err != nil {
			return err
		}
		result = application.YjsReceipt{Epoch: input.Epoch, CommitSeq: application.Revision(seq), ContentRevision: application.Revision(revision), GraphHash: candidate.GraphHash, UpdateID: input.UpdateID, PayloadHash: input.PayloadHash, Effect: candidate.Effect, Status: "accepted", Update: candidate.AppliedUpdate}
		return storeYjsReceipt(tx, a, id, input, result)
	})
	return result, err
}
