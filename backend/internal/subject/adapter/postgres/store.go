// Package postgres persists immutable subjects under current creation-scope authority.
package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

// Factories bind consuming ports to the exact subject transaction.
type Factories struct {
	Voice func(*gorm.DB) application.VoiceJobs
	Scope func(*gorm.DB) application.ScopeAccess
	Media func(*gorm.DB) application.MediaAccess
}

// Store contains no request identity or ambient scope.
type Store struct {
	db        *gorm.DB
	factories Factories
}

// NewStore injects transaction-bound current authority and media proof factories.
func NewStore(db *gorm.DB, factories Factories) *Store { return &Store{db: db, factories: factories} }

type bound struct {
	tx    *gorm.DB
	actor identity.Principal
	scope application.ScopeFacts
	media application.MediaAccess
}

func (b *bound) Scope() application.ScopeFacts  { return b.scope }
func (b *bound) Media() application.MediaAccess { return b.media }

func (s *Store) authorize(ctx context.Context, tx *gorm.DB, a identity.Principal, write bool) (*bound, error) {
	if s == nil || s.factories.Scope == nil || s.factories.Scope(tx) == nil {
		return nil, application.ErrUnavailable
	}
	facts, err := s.factories.Scope(tx).Resolve(ctx, a, write)
	if err != nil {
		return nil, err
	}
	if facts.ScopeID != a.ScopeID || facts.PhysicalScopeID != a.OrgID || facts.ScopeID == uuid.Nil || facts.PhysicalScopeID == uuid.Nil || a.ID == uuid.Nil {
		return nil, application.ErrNotFound
	}
	result := &bound{tx: tx, actor: a, scope: facts}
	if s.factories.Media != nil {
		result.media = s.factories.Media(tx)
	}
	return result, nil
}

// Write retains current authority through permanent receipt replay and commit.
func (s *Store) Write(ctx context.Context, a identity.Principal, run func(application.Transaction) (application.CommandResult, error)) (application.CommandResult, error) {
	if s == nil || s.db == nil {
		return application.CommandResult{}, application.ErrUnavailable
	}
	var result application.CommandResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		b, err := s.authorize(ctx, tx, a, true)
		if err != nil {
			return err
		}
		result, err = run(b)
		return err
	})
	return result, err
}

func (s *Store) read(ctx context.Context, a identity.Principal, run func(*bound) error) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return platformdb.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		b, err := s.authorize(ctx, tx, a, false)
		if err != nil {
			return err
		}
		return run(b)
	})
}

func (b *bound) Replay(ctx context.Context, key uuid.UUID, hash string) (*application.CommandResult, error) {
	if err := b.tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "subject-command/"+b.actor.ID.String()+"/"+key.String()).Error; err != nil {
		return nil, err
	}
	var row struct {
		OwnerScopeID, PhysicalScopeID uuid.UUID
		RequestSHA256                 string
		ResponseBody                  []byte
	}
	r := b.tx.WithContext(ctx).Raw(`SELECT owner_scope_id,physical_scope_id,request_sha256,response_body FROM subject.command WHERE actor_id=? AND idem_key=?`, b.actor.ID, key).Scan(&row)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	if row.OwnerScopeID != b.scope.ScopeID || row.PhysicalScopeID != b.scope.PhysicalScopeID || row.RequestSHA256 != hash {
		return nil, application.ErrKeyConflict
	}
	var result application.CommandResult
	if len(row.ResponseBody) > 1<<20 || json.Unmarshal(row.ResponseBody, &result) != nil {
		return nil, application.ErrUnavailable
	}
	return &result, nil
}

func (b *bound) Record(ctx context.Context, key uuid.UUID, hash string, result application.CommandResult) error {
	body, err := json.Marshal(result)
	if err != nil || len(body) > 1<<20 {
		return application.ErrUnavailable
	}
	return changed(b.tx.WithContext(ctx).Exec(`INSERT INTO subject.command(actor_id,idem_key,owner_scope_id,physical_scope_id,request_sha256,response_body) VALUES(?,?,?,?,?,?::jsonb)`, b.actor.ID, key, b.scope.ScopeID, b.scope.PhysicalScopeID, hash, string(body)))
}

func changed(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}

func (b *bound) Find(ctx context.Context, id uuid.UUID) (domain.Head, domain.Version, bool, error) {
	row, err := b.detailRow(ctx, id, true)
	if err != nil {
		return domain.Head{}, domain.Version{}, false, err
	}
	return decode(row)
}

func (b *bound) Publish(ctx context.Context, h domain.Head, v domain.Version, expected int64) error {
	if h.OwnerScopeID != b.scope.ScopeID || h.PhysicalScopeID != b.scope.PhysicalScopeID || v.OwnerScopeID != h.OwnerScopeID || v.PhysicalScopeID != h.PhysicalScopeID || v.SubjectID != h.ID || v.ID != h.VersionID || v.Content.Validate() != nil {
		return domain.ErrInvalid
	}
	if expected == 0 {
		if err := changed(b.tx.WithContext(ctx).Exec(`INSERT INTO subject.subject_head(id,owner_scope_id,physical_scope_id,revision,current_version_id,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, h.ID, h.OwnerScopeID, h.PhysicalScopeID, h.Revision, h.VersionID, h.CreatedBy, h.CreatedAt, h.UpdatedAt)); err != nil {
			return err
		}
	}
	content, err := json.Marshal(v.Content)
	if err != nil || len(content) > 1<<20 {
		return domain.ErrInvalid
	}
	if err := changed(b.tx.WithContext(ctx).Exec(`INSERT INTO subject.subject_version(id,subject_id,owner_scope_id,physical_scope_id,version_no,previous_id,created_by,created_at,content,content_sha256) VALUES(?,?,?,?,?,?,?,?,?::jsonb,?)`, v.ID, v.SubjectID, v.OwnerScopeID, v.PhysicalScopeID, v.Number, v.PreviousID, v.CreatedBy, v.CreatedAt, string(content), v.SHA256)); err != nil {
		return err
	}
	for _, entry := range bindings(v.Content) {
		fact, err := json.Marshal(entry.binding.Media)
		if err != nil {
			return err
		}
		hash, err := domain.Digest(entry.binding)
		if err != nil {
			return err
		}
		media := entry.binding.Media
		if err := changed(b.tx.WithContext(ctx).Exec(`INSERT INTO subject.media_binding(binding_id,subject_id,version_id,owner_scope_id,physical_scope_id,position,role,asset_id,asset_revision,kind,sha256,binding_sha256,fact) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb)`, entry.binding.ID, h.ID, v.ID, h.OwnerScopeID, h.PhysicalScopeID, entry.position, entry.role, media.AssetID, media.Revision, media.Kind, media.SHA256, hash, string(fact))); err != nil {
			return err
		}
	}
	if expected != 0 {
		return changed(b.tx.WithContext(ctx).Exec(`UPDATE subject.subject_head SET revision=?,current_version_id=?,updated_at=? WHERE id=? AND owner_scope_id=? AND physical_scope_id=? AND revision=? AND deleted_at IS NULL`, h.Revision, h.VersionID, h.UpdatedAt, h.ID, h.OwnerScopeID, h.PhysicalScopeID, expected))
	}
	return nil
}

func (b *bound) Delete(ctx context.Context, h domain.Head, expected int64) error {
	return changed(b.tx.WithContext(ctx).Exec(`UPDATE subject.subject_head SET revision=?,updated_at=?,deleted_at=? WHERE id=? AND owner_scope_id=? AND physical_scope_id=? AND revision=? AND deleted_at IS NULL`, h.Revision, h.UpdatedAt, h.DeletedAt, h.ID, b.scope.ScopeID, b.scope.PhysicalScopeID, expected))
}
func (b *bound) SetFavorite(ctx context.Context, id uuid.UUID, value bool) error {
	return changed(b.tx.WithContext(ctx).Exec(`INSERT INTO subject.favorite(actor_id,subject_id,owner_scope_id,physical_scope_id,favorite) VALUES(?,?,?,?,?) ON CONFLICT(actor_id,subject_id) DO UPDATE SET favorite=excluded.favorite,updated_at=clock_timestamp()`, b.actor.ID, id, b.scope.ScopeID, b.scope.PhysicalScopeID, value))
}

type bindingEntry struct {
	binding  domain.Binding
	role     string
	position int
}

func bindings(c domain.Content) []bindingEntry {
	entries := []bindingEntry{{binding: c.Primary, role: "primary", position: 0}}
	for i, ref := range c.References {
		entries = append(entries, bindingEntry{binding: ref, role: "reference", position: i})
	}
	if c.Voice != nil {
		entries = append(entries, bindingEntry{binding: c.Voice.Binding, role: "voice", position: 0})
	}
	return entries
}

var _ application.CommandStore = (*Store)(nil)
