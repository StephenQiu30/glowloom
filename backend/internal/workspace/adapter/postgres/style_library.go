package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// StyleImageReader proves the caller's usable personal images inside the command transaction.
type StyleImageReader interface {
	Image(context.Context, identity.Principal, uuid.UUID) (app.StyleLibraryReference, error)
}

// StyleImageFactory binds personal image checks to the library transaction.
type StyleImageFactory func(*gorm.DB) StyleImageReader

// StyleLibraryStore keeps library ownership separate from legacy project presets.
type StyleLibraryStore struct {
	db     *gorm.DB
	scopes RequestScopeFactory
	images StyleImageFactory
}

// NewStyleLibraryStore injects scope and image authority explicitly.
func NewStyleLibraryStore(db *gorm.DB, scopes RequestScopeFactory, images StyleImageFactory) *StyleLibraryStore {
	return &StyleLibraryStore{db: db, scopes: scopes, images: images}
}
func (s *StyleLibraryStore) authorize(ctx context.Context, tx *gorm.DB, a identity.Principal, write bool) error {
	return (&Store{scopes: s.scopes}).requireScope(ctx, tx, a, write)
}

const styleLibraryColumns = `id,name,COALESCE(library_category,CASE WHEN style_type='realistic' THEN 'realistic' WHEN style_subtype='cartoon_3d' THEN '3d' ELSE '2d' END) AS category,revision,owner_user_id,prompt_fragment,negative_prompt,array_to_json(reference_asset_ids)::text AS images,is_delete`

type styleLibraryRow struct {
	ID                                     uuid.UUID
	Name, Category                         string
	Revision                               int64
	OwnerUserID                            *uuid.UUID
	PromptFragment, NegativePrompt, Images string
	IsDelete                               bool
}

func (row styleLibraryRow) detail(a identity.Principal) (app.StyleLibraryDetail, error) {
	refs := []uuid.UUID{}
	if json.Unmarshal([]byte(row.Images), &refs) != nil || refs == nil || len(refs) > 4 || row.ID == uuid.Nil || row.Revision < 1 {
		return app.StyleLibraryDetail{}, app.ErrProjectDependencyUnavailable
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range refs {
		if id == uuid.Nil || seen[id] {
			return app.StyleLibraryDetail{}, app.ErrProjectDependencyUnavailable
		}
		seen[id] = true
	}
	owned := row.OwnerUserID != nil && *row.OwnerUserID == a.ID
	out := app.StyleLibraryDetail{StyleLibraryEntry: app.StyleLibraryEntry{ID: row.ID, Name: row.Name, Category: row.Category, Revision: row.Revision, ReferenceAssetIDs: refs, Owned: owned, CanEdit: owned && !row.IsDelete, ReferenceItems: []app.StyleLibraryReference{}, ReferencesAvailable: len(refs) == 0}, PromptFragment: row.PromptFragment, NegativePrompt: row.NegativePrompt}
	if len(refs) > 0 {
		id := refs[0]
		out.CoverAssetID = &id
	}
	return out, nil
}

// ListStyleLibrary reads prompt-free cards under current scope authority.
func (s *StyleLibraryStore) ListStyleLibrary(ctx context.Context, a identity.Principal, in app.StyleLibraryFilter) (app.StyleLibraryPage, error) {
	out := app.StyleLibraryPage{Items: []app.StyleLibraryEntry{}}
	if s == nil || s.db == nil {
		return out, app.ErrProjectDependencyUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, false); err != nil {
			return err
		}
		// Summary reads deliberately exclude prompt columns from SQL.
		query := `SELECT id,name,COALESCE(library_category,CASE WHEN style_type='realistic' THEN 'realistic' WHEN style_subtype='cartoon_3d' THEN '3d' ELSE '2d' END) AS category,revision,owner_user_id,array_to_json(reference_asset_ids)::text AS images FROM workspace.style_preset WHERE org_id=? AND owner_scope_id=? AND project_id IS NULL AND NOT is_delete AND (owner_user_id IS NULL OR owner_user_id=?)`
		args := []any{a.OrgID, a.ScopeID, a.ID}
		if in.Mine != nil {
			if *in.Mine {
				query += ` AND owner_user_id=?`
				args = append(args, a.ID)
			} else {
				query += ` AND owner_user_id IS NULL`
			}
		}
		if in.Category != "" {
			query += ` AND COALESCE(library_category,CASE WHEN style_type='realistic' THEN 'realistic' WHEN style_subtype='cartoon_3d' THEN '3d' ELSE '2d' END)=?`
			args = append(args, in.Category)
		}
		if in.Query != "" {
			query += ` AND strpos(lower(name),lower(?))>0`
			args = append(args, in.Query)
		}
		if in.After != nil {
			query += ` AND (name,id)>(?,?::uuid)`
			args = append(args, in.After.Name, in.After.ID)
		}
		query += ` ORDER BY name,id LIMIT ?`
		args = append(args, in.Limit+1)
		var rows []styleLibraryRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > in.Limit {
			rows = rows[:in.Limit]
			last := rows[len(rows)-1]
			out.Next = &app.StylePresetCursor{Name: last.Name, ID: last.ID}
		}
		canWrite := s.authorize(ctx, tx, a, true) == nil
		for _, row := range rows {
			d, err := row.detail(a)
			if err != nil {
				return err
			}
			items, err := s.imageProofs(ctx, tx, a, d.ReferenceAssetIDs)
			if err == nil {
				setStyleReferences(&d, items)
			}
			d.CanEdit = d.Owned && canWrite
			out.Items = append(out.Items, d.StyleLibraryEntry)
		}
		return nil
	})
	return out, err
}
func readLibraryStyle(tx *gorm.DB, a identity.Principal, id uuid.UUID, deleted, write bool) (styleLibraryRow, error) {
	var row styleLibraryRow
	query := `SELECT ` + styleLibraryColumns + ` FROM workspace.style_preset WHERE id=? AND org_id=? AND owner_scope_id=? AND project_id IS NULL AND (owner_user_id IS NULL OR owner_user_id=?)`
	if !deleted {
		query += ` AND NOT is_delete`
	}
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	r := tx.Raw(query, id, a.OrgID, a.ScopeID, a.ID).Scan(&row)
	if r.Error != nil {
		return row, r.Error
	}
	if r.RowsAffected != 1 {
		return row, app.ErrStylePresetNotFound
	}
	return row, nil
}
func setStyleReferences(out *app.StyleLibraryDetail, items []app.StyleLibraryReference) {
	out.ReferenceItems = items
	out.ReferencesAvailable = true
	if len(items) > 0 {
		id := items[0].ID
		out.CoverItemID = &id
	}
}
func (s *StyleLibraryStore) imageProofs(ctx context.Context, tx *gorm.DB, a identity.Principal, ids []uuid.UUID) ([]app.StyleLibraryReference, error) {
	items := []app.StyleLibraryReference{}
	if len(ids) == 0 {
		return items, nil
	}
	if s.images == nil {
		return nil, app.ErrStyleLibraryReference
	}
	reader := s.images(tx)
	if reader == nil {
		return nil, app.ErrStyleLibraryReference
	}
	for _, id := range ids {
		item, err := reader.Image(ctx, a, id)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", app.ErrStyleLibraryReference, err)
		}
		if item.ID == uuid.Nil || item.AssetID != id || item.Revision < 1 || item.AssetRevision < 1 || item.Title == "" {
			return nil, app.ErrStyleLibraryReference
		}
		items = append(items, item)
	}
	return items, nil
}

// ReadStyleLibrary returns authorized content while marking stale image references.
func (s *StyleLibraryStore) ReadStyleLibrary(ctx context.Context, a identity.Principal, id uuid.UUID) (app.StyleLibraryDetail, error) {
	var out app.StyleLibraryDetail
	if s == nil || s.db == nil {
		return out, app.ErrProjectDependencyUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, false); err != nil {
			return err
		}
		row, err := readLibraryStyle(tx, a, id, false, false)
		if err != nil {
			return err
		}
		out, err = row.detail(a)
		if err != nil {
			return err
		}
		items, err := s.imageProofs(ctx, tx, a, out.ReferenceAssetIDs)
		if err == nil {
			setStyleReferences(&out, items)
		}
		out.CanEdit = out.Owned && s.authorize(ctx, tx, a, true) == nil
		// A current owner can remove stale images; explicit availability prevents adoption.
		return nil
	})
	return out, err
}

// ChangeStyleLibrary commits or replays one guarded mutation atomically.
func (s *StyleLibraryStore) ChangeStyleLibrary(ctx context.Context, a identity.Principal, in app.StyleLibraryMutation) (app.StyleLibraryChangeResult, error) {
	var out app.StyleLibraryChangeResult
	if s == nil || s.db == nil {
		return out, app.ErrProjectDependencyUnavailable
	}
	canonical := in
	canonical.RequestID = ""
	canonical.IdempotencyKey = uuid.Nil
	body, err := json.Marshal(struct {
		Contract   string
		Org, Scope uuid.UUID
		Input      app.StyleLibraryMutation
	}{"workspace.style-library.v1", a.OrgID, a.ScopeID, canonical})
	if err != nil {
		return out, err
	}
	digest := sha256.Sum256(body)
	hash := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, true); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69362,hashtext(?))`, a.ID.String()+":"+in.IdempotencyKey.String()).Error; err != nil {
			return err
		}
		var receipt struct {
			OrgID, OwnerScopeID, PresetID uuid.UUID
			RequestSHA256                 string
			ResponseBody                  []byte
		}
		r := tx.Raw(`SELECT org_id,owner_scope_id,preset_id,request_sha256,response_body FROM workspace.style_preset_command WHERE actor_id=? AND idem_key=?`, a.ID, in.IdempotencyKey).Scan(&receipt)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if receipt.OrgID != a.OrgID || receipt.OwnerScopeID != a.ScopeID || receipt.RequestSHA256 != hash {
				return app.ErrIdempotencyConflict
			}
			row, err := readLibraryStyle(tx, a, receipt.PresetID, true, false)
			if err != nil {
				return err
			}
			if row.OwnerUserID == nil || *row.OwnerUserID != a.ID {
				return app.ErrStylePresetNotFound
			}
			if json.Unmarshal(receipt.ResponseBody, &out) != nil || out.RequestID != in.IdempotencyKey || out.Action != in.Action || out.Preset.ID != receipt.PresetID || !out.Preset.Owned {
				return app.ErrProjectDependencyUnavailable
			}
			return nil
		}
		id := in.ID
		if in.Action == "create" {
			if _, err := s.imageProofs(ctx, tx, a, in.ReferenceAssetIDs); err != nil {
				return err
			}
			id = uuid.New()
			refs, err := json.Marshal(in.ReferenceAssetIDs)
			if err != nil {
				return err
			}
			styleType := "stylized"
			if in.Category == "realistic" {
				styleType = "realistic"
			}
			r = tx.Exec(`INSERT INTO workspace.style_preset(id,org_id,owner_scope_id,owner_user_id,library_category,name,style_type,prompt_fragment,reference_asset_ids) VALUES(?,?,?,?,?,?,?,?,ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid))`, id, a.OrgID, a.ScopeID, a.ID, in.Category, in.Name, styleType, in.PromptFragment, string(refs))
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return app.ErrProjectDependencyUnavailable
			}
		} else {
			row, err := readLibraryStyle(tx, a, id, false, true)
			if err != nil {
				return err
			}
			if row.OwnerUserID == nil || *row.OwnerUserID != a.ID {
				return app.ErrStylePresetNotFound
			}
			if row.Revision != in.ExpectedRevision {
				return app.ErrStyleLibraryRevision
			}
			if in.Action == "delete" {
				var used bool
				if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project WHERE style_preset_id=?)`, id).Scan(&used).Error; err != nil {
					return err
				}
				if used {
					return app.ErrStyleLibraryInUse
				}
				r = tx.Exec(`UPDATE workspace.style_preset SET is_delete=true,revision=revision+1 WHERE id=? AND revision=? AND owner_user_id=? AND NOT is_delete`, id, in.ExpectedRevision, a.ID)
			} else {
				if _, err := s.imageProofs(ctx, tx, a, in.ReferenceAssetIDs); err != nil {
					return err
				}
				refs, err := json.Marshal(in.ReferenceAssetIDs)
				if err != nil {
					return err
				}
				r = tx.Exec(`UPDATE workspace.style_preset SET name=?,library_category=?,prompt_fragment=?,reference_asset_ids=ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid),revision=revision+1 WHERE id=? AND revision=? AND owner_user_id=? AND NOT is_delete`, in.Name, in.Category, in.PromptFragment, string(refs), id, in.ExpectedRevision, a.ID)
			}
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return app.ErrStyleLibraryRevision
			}
		}
		row, err := readLibraryStyle(tx, a, id, true, false)
		if err != nil {
			return err
		}
		detail, err := row.detail(a)
		if err != nil {
			return err
		}
		for _, asset := range detail.ReferenceAssetIDs {
			if err := tx.Exec(`INSERT INTO workspace.style_preset_reference(preset_id,asset_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, asset).Error; err != nil {
				return err
			}
		}
		if in.Action != "delete" {
			items, err := s.imageProofs(ctx, tx, a, detail.ReferenceAssetIDs)
			if err != nil {
				return err
			}
			setStyleReferences(&detail, items)
		}
		out = app.StyleLibraryChangeResult{RequestID: in.IdempotencyKey, Action: in.Action, Preset: detail, Deleted: row.IsDelete}
		if err := styleLibraryAudit(tx, a, in, out); err != nil {
			return err
		}
		response, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.style_preset_command(actor_id,idem_key,org_id,owner_scope_id,preset_id,action,request_sha256,response_body) VALUES(?,?,?,?,?,?,?,?::jsonb)`, a.ID, in.IdempotencyKey, a.OrgID, a.ScopeID, id, in.Action, hash, string(response)).Error
	})
	if err != nil {
		return app.StyleLibraryChangeResult{}, fmt.Errorf("style library command: %w", err)
	}
	return out, nil
}
func styleLibraryAudit(tx *gorm.DB, a identity.Principal, in app.StyleLibraryMutation, out app.StyleLibraryChangeResult) error {
	action := map[string]string{"create": "created", "patch": "updated", "delete": "deleted"}[in.Action]
	event := uuid.New()
	const topic = "glowloom.audit.recorded.v1"
	payload, err := json.Marshal(map[string]any{"event_id": event, "event_type": topic, "occurred_at": time.Now().UTC(), "org_id": a.OrgID, "actor": map[string]any{"kind": "user", "id": a.ID}, "aggregate": map[string]any{"type": "audit", "id": event}, "data": map[string]any{"action": "style_preset." + action, "object": map[string]any{"type": "style_preset", "id": out.Preset.ID}, "request_id": in.RequestID, "before": nil, "after": map[string]any{"revision": out.Preset.Revision, "category": out.Preset.Category, "reference_count": len(out.Preset.ReferenceAssetIDs), "is_delete": out.Deleted}}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event, topic, a.OrgID.String(), string(payload)).Error
}

// StyleMediaGuard retains all adopted personal image references, including old revisions.
type StyleMediaGuard struct{ db *gorm.DB }

// NewStyleMediaGuard binds permanent image retention to the purge transaction.
func NewStyleMediaGuard(tx *gorm.DB) *StyleMediaGuard { return &StyleMediaGuard{db: tx} }

// HasRetainedMediaReference protects current and historical style image pins.
func (g *StyleMediaGuard) HasRetainedMediaReference(ctx context.Context, id uuid.UUID) (bool, error) {
	if g == nil || g.db == nil || g.db.Statement == nil || id == uuid.Nil {
		return false, app.ErrProjectDependencyUnavailable
	}
	if _, ok := g.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, app.ErrProjectDependencyUnavailable
	}
	var found bool
	err := g.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM workspace.style_preset_reference WHERE asset_id=?) OR EXISTS(SELECT 1 FROM workspace.style_preset WHERE reference_asset_ids @> ARRAY[?::uuid])`, id, id).Scan(&found).Error
	return found, err
}

var _ app.StyleLibraryRepository = (*StyleLibraryStore)(nil)
