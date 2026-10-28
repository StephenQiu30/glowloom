package postgres

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func recordYjsVersion(tx *gorm.DB, doc domain.Document, epoch, actor uuid.UUID, reason string) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO canvas.collab_version(id,document_id,epoch,revision,document_json,actor_id,reason,writer_term,commit_seq,state,graph_hash)
SELECT ?,document_id,epoch,?,?::jsonb,?,?,writer_term,commit_seq,state,graph_hash FROM canvas.collab_document WHERE document_id=? AND epoch=? AND content_revision=?
ON CONFLICT(document_id,revision) DO NOTHING`, uuid.New(), doc.Revision, string(body), actor, reason, doc.ID, epoch, doc.Revision).Error
}

// YjsVersions lists metadata without reading every historical body.
func (s *Store) YjsVersions(ctx context.Context, a identity.Principal, id uuid.UUID, before application.Revision) ([]application.YjsVersion, error) {
	result := []application.YjsVersion{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, false); err != nil {
			return err
		}
		limit := int64(before)
		if limit == 0 {
			limit = math.MaxInt64
		}
		return tx.Raw(`SELECT id,epoch,revision,created_at,reason FROM canvas.collab_version WHERE document_id=? AND revision<? ORDER BY revision DESC LIMIT 30`, id, limit).Scan(&result).Error
	})
	return result, err
}
func loadYjsVersion(tx *gorm.DB, id, version uuid.UUID) (domain.Document, error) {
	var row struct{ DocumentJSON []byte }
	result := tx.Raw(`SELECT document_json FROM canvas.collab_version WHERE document_id=? AND id=?`, id, version).Scan(&row)
	if result.Error != nil {
		return domain.Document{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Document{}, &domain.OperationError{Code: "version_unavailable"}
	}
	var doc domain.Document
	if err := json.Unmarshal(row.DocumentJSON, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

// YjsVersionState prepares a read-only source with current label tombstones retained.
func (s *Store) YjsVersionState(ctx context.Context, a identity.Principal, id, version uuid.UUID, restore bool) (application.YjsState, error) {
	var result application.YjsState
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, false); err != nil {
			return err
		}
		doc, err := loadYjsVersion(tx, id, version)
		if err != nil {
			return err
		}
		if restore {
			current, err := load(tx, id, true)
			if err != nil {
				return err
			}
			if err = prepareYjsHistory(tx, current, &doc); err != nil {
				return err
			}
		}
		result = application.YjsState{YjsBinding: application.YjsBinding{DocumentID: id}, Document: application.Snapshot(doc)}
		return nil
	})
	return result, err
}
func restoreHash(input application.YjsRestore) string {
	body, _ := json.Marshal(input)
	return requestHash("yjs-restore/v1", body)
}
func resolveYjsRestore(tx *gorm.DB, actor, id uuid.UUID, input application.YjsRestore) (*application.YjsRestoreResult, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := tx.Raw(`SELECT request_hash,response_body FROM canvas.collab_control_receipt WHERE document_id=? AND actor_id=? AND session_id=? AND command_id=?`, id, actor, input.SessionID, input.ID).Scan(&row)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	if row.RequestHash != restoreHash(input) {
		return nil, application.ErrIdempotencyConflict
	}
	var result application.YjsRestoreResult
	if err := json.Unmarshal(row.ResponseBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ResolveYjsRestore checks current read access before original-outcome lookup.
func (s *Store) ResolveYjsRestore(ctx context.Context, a identity.Principal, id uuid.UUID, input application.YjsRestore) (*application.YjsRestoreResult, error) {
	var result *application.YjsRestoreResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, false); err != nil {
			return err
		}
		result, err = resolveYjsRestore(tx, a.ID, id, input)
		return err
	})
	return result, err
}

// CommitYjsRestore atomically validates historical sources and advances the epoch.
func (s *Store) CommitYjsRestore(ctx context.Context, a identity.Principal, base application.YjsState, input application.YjsRestore, candidate application.YjsCandidate) (application.YjsRestoreResult, error) {
	var result application.YjsRestoreResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id := base.DocumentID
		p, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, p, true); err != nil {
			return err
		}
		current, err := load(tx, id, true)
		if err != nil {
			return err
		}
		replay, err := resolveYjsRestore(tx, a.ID, id, input)
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
		if row.Epoch != input.Epoch || row.CommitSeq != int64(input.CommitSeq) || row.WriterTerm != int64(base.WriterTerm) || current.Revision != int64(base.Document.Revision) {
			return application.ErrYjsContention
		}
		if current.ReadOnlyReason != "" {
			return domain.ErrGraphReadOnly
		}
		if current.Revision == math.MaxInt64 {
			return domain.ErrInvalidCommand
		}
		target, err := loadYjsVersion(tx, id, input.VersionID)
		if err != nil {
			return err
		}
		if err = prepareYjsHistory(tx, current, &target); err != nil {
			return err
		}
		if !yjsEditableProjectionEqual(application.Snapshot(target), candidate.Projection) {
			return domain.ErrInvalidCommand
		}
		if err = domain.ValidateGraph(target); err != nil {
			return err
		}
		commands := []domain.Command{}
		for _, node := range target.Nodes {
			commands = append(commands, domain.Command{Type: "AddNodes", Nodes: []domain.Node{node}})
			if node.Config.Composer != nil {
				if err = pgprompt.ValidateDraftSources(tx, a, p, *node.Config.Composer, false, s.subjectSources(tx)); err != nil {
					return err
				}
			}
		}
		if err = s.checkMediaReferences(ctx, tx, a, p, commands, target); err != nil {
			return err
		}
		// The accepted history is the only source of restored identities. Versions
		// advance from the most recent lifetime, including currently deleted rows.
		for i := range target.Nodes {
			var v int64
			if err = tx.Raw(`SELECT content_version FROM canvas.node WHERE document_id=? AND id=?`, id, target.Nodes[i].ID).Scan(&v).Error; err != nil {
				return err
			}
			target.Nodes[i].ContentVersion = max(v, target.Nodes[i].ContentVersion)
		}
		if err = advanceContentVersions(current, &target); err != nil {
			return err
		}
		target.Revision = current.Revision + 1
		target.Name = current.Name
		target.Viewport = current.Viewport
		if err = recordYjsVersion(tx, current, row.Epoch, a.ID, "before_restore"); err != nil {
			return err
		}
		if err = tx.Exec(`SELECT set_config('glowloom.yjs_document_id',?,true)`, id.String()).Error; err != nil {
			return err
		}
		if err = saveGraphWithRestoration(tx, current, target, true); err != nil {
			return err
		}
		presentation, err := s.presentation(tx, a, p)
		if err != nil {
			return err
		}
		if id == presentation.MainCanvasID && domain.HasContentChanges(current, target) {
			if err = s.project(tx).MarkProjectContentChanged(ctx, a, p); err != nil {
				return err
			}
		}
		if err = changed(tx, a, id, p, target.Revision); err != nil {
			return err
		}
		effects := domain.DocumentEffects(current, target)
		refs, err := projectionRetainedMedia(current, target)
		if err != nil {
			return err
		}
		log := projectionLog{System: "yjs_history_restore", OriginalBase: current.Revision, Operations: []domain.Operation{}, Effects: effects, MediaReferences: refs}
		body, err := json.Marshal(log)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,create_by,op_id,base_revision,operation_version,client_id) VALUES(?,?,?,?::jsonb,?,?,?,1,?)`, uuid.New(), id, target.Revision, string(body), a.ID, input.ID, current.Revision, input.SessionID).Error; err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE canvas.collab_document SET epoch=?,writer_term=?,commit_seq=0,content_revision=?,state=?,graph_hash=?,updated_at=clock_timestamp() WHERE document_id=?`, candidate.Epoch, candidate.WriterTerm, target.Revision, candidate.State, candidate.GraphHash, id).Error; err != nil {
			return err
		}
		if err = recordYjsVersion(tx, target, candidate.Epoch, a.ID, "history_restore"); err != nil {
			return err
		}
		result = application.YjsRestoreResult{Epoch: candidate.Epoch, Revision: application.Revision(target.Revision)}
		body, _ = json.Marshal(result)
		if err = tx.Exec(`INSERT INTO canvas.collab_control_receipt(document_id,actor_id,session_id,command_id,request_hash,response_body) VALUES(?,?,?,?,?,?::jsonb)`, id, a.ID, input.SessionID, input.ID, restoreHash(input), string(body)).Error; err != nil {
			return err
		}
		return collaborationCommitted(tx, a, id, p, candidate.Epoch, 0, target.Revision)
	})
	return result, err
}

// Historical content never reclaims a display allocation used after that version.
func prepareYjsHistory(tx *gorm.DB, current domain.Document, target *domain.Document) error {
	target.Name = current.Name
	for i := range target.Nodes {
		n := &target.Nodes[i]
		var row struct{ Config []byte }
		if err := tx.Raw(`SELECT config FROM canvas.node WHERE document_id=? AND id=?`, current.ID, n.ID).Scan(&row).Error; err != nil {
			return err
		}
		var config domain.NodeConfig
		if len(row.Config) == 0 {
			continue
		}
		if err := json.Unmarshal(row.Config, &config); err != nil {
			return err
		}
		if config.Editor != nil && len(config.Editor.Metadata["referenceLabels"]) > 0 {
			if n.Config.Editor == nil {
				n.Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
			}
			if n.Config.Editor.Metadata == nil {
				n.Config.Editor.Metadata = map[string]json.RawMessage{}
			}
			n.Config.Editor.Metadata["referenceLabels"] = config.Editor.Metadata["referenceLabels"]
		}
		if config.Composer == nil {
			continue
		}
		if n.Config.Composer == nil && len(config.Composer.LabelState.Allocations) == 0 {
			continue
		}
		if n.Config.Composer == nil {
			// Restoring a plain-text version retains later allocation tombstones.
			text := ""
			if n.Config.Text != nil {
				text = *n.Config.Text
			}
			paragraphs := []promptdomain.Paragraph{}
			for _, line := range strings.Split(text, "\n") {
				p := promptdomain.Paragraph{Type: "paragraph"}
				if line != "" {
					p.Content = []promptdomain.Inline{{Type: "text", Text: line}}
				}
				paragraphs = append(paragraphs, p)
			}
			n.Config.Composer = &promptdomain.Draft{SchemaVersion: 1, Document: promptdomain.Document{Type: "doc", Content: paragraphs}, References: []promptdomain.Reference{}, Params: map[string]any{}, LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]promptdomain.LabelAllocation{}}}
			n.Config.Text = nil
			n.Config.Format = "tiptap"
			n.Config.FormatVersion = 1
		}
		labels := n.Config.Composer.LabelState
		for id, allocation := range config.Composer.LabelState.Allocations {
			if old, ok := labels.Allocations[id]; ok && old != allocation {
				return domain.ErrInvalidCommand
			}
			labels.Allocations[id] = allocation
		}
		for kind, n := range config.Composer.LabelState.HighWater {
			labels.HighWater[kind] = max(labels.HighWater[kind], n)
		}
		if err := n.Config.Composer.Validate(); err != nil {
			return err
		}
	}
	return nil
}
