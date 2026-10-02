// Package postgres persists Bible content in exact transaction-bound owning scopes.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Factories bind each actual owner to the current Bible SQL transaction.
type Factories struct {
	Access  func(*gorm.DB) application.ProjectAccess
	Media   func(*gorm.DB) application.MediaReferences
	Voices  func(*gorm.DB) application.CatalogVoices
	Scopes  func(*gorm.DB) application.ScriptScopes
	Impacts func(*gorm.DB) application.Impacts
	Results func(*gorm.DB) application.GeneratedResults
}

// Store owns only Bible tables and delegates foreign scope facts through explicit factories.
type Store struct {
	db        *gorm.DB
	factories Factories
}

// NewStore injects the database and transaction-bound dependencies.
func NewStore(db *gorm.DB, factories Factories) *Store { return &Store{db: db, factories: factories} }

type bound struct {
	tx      *gorm.DB
	actor   identityapp.Principal
	project workspaceapp.ProjectContentAccess
	access  application.ProjectAccess
	media   application.MediaReferences
	voices  application.CatalogVoices
	scopes  application.ScriptScopes
	impacts application.Impacts
	results application.GeneratedResults
}

func (s *Store) transaction(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool, f func(*bound) error) error {
	if s == nil || s.db == nil || s.factories.Access == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := s.factories.Access(tx)
		if access == nil {
			return application.ErrUnavailable
		}
		facts, err := access.Authorize(ctx, actor, project, write)
		if errors.Is(err, workspaceapp.ErrProjectNotFound) {
			return errors.Join(application.ErrNotFound, err)
		}
		if err != nil {
			return err
		}
		if facts.ProjectID != project || facts.OrgID != actor.OrgID || facts.Revision < 1 {
			return application.ErrUnavailable
		}
		b := &bound{tx: tx, actor: actor, project: facts, access: access}
		if s.factories.Media != nil {
			b.media = s.factories.Media(tx)
		}
		if s.factories.Voices != nil {
			b.voices = s.factories.Voices(tx)
		}
		if s.factories.Scopes != nil {
			b.scopes = s.factories.Scopes(tx)
		}
		if s.factories.Impacts != nil {
			b.impacts = s.factories.Impacts(tx)
		}
		if s.factories.Results != nil {
			b.results = s.factories.Results(tx)
		}
		return f(b)
	})
}

// Write holds current workspace authorization throughout content, invalidation and receipt persistence.
func (s *Store) Write(ctx context.Context, actor identityapp.Principal, project uuid.UUID, f func(application.Transaction) (application.Receipt, error)) (application.Receipt, error) {
	var result application.Receipt
	err := s.transaction(ctx, actor, project, true, func(tx *bound) error { var err error; result, err = f(tx); return err })
	return result, err
}

func (b *bound) Project() workspaceapp.ProjectContentAccess { return b.project }
func (b *bound) Access() application.ProjectAccess          { return b.access }
func (b *bound) Media() application.MediaReferences         { return b.media }
func (b *bound) Voices() application.CatalogVoices          { return b.voices }
func (b *bound) Scopes() application.ScriptScopes           { return b.scopes }
func (b *bound) Impacts() application.Impacts               { return b.impacts }
func (b *bound) Results() application.GeneratedResults      { return b.results }

func exactlyOne(write *gorm.DB) error {
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}

func tables(kind domain.Kind) (string, string, string, error) {
	switch kind {
	case domain.KindCharacter:
		return "bible.character", "bible.character_version", "bible.character_confirmation", nil
	case domain.KindLocation:
		return "bible.location", "bible.location_version", "bible.location_confirmation", nil
	case domain.KindProp:
		return "bible.prop", "bible.prop_version", "bible.prop_confirmation", nil
	default:
		return "", "", "", domain.ErrInvalidContent
	}
}

// Replay verifies a permanent key's entire scope after live authorization.
func (b *bound) Replay(ctx context.Context, actor identityapp.Principal, c application.Command, hash string) (*application.Receipt, error) {
	if actor.ID != b.actor.ID || actor.OrgID != b.actor.OrgID || c.ProjectID != b.project.ProjectID {
		return nil, application.ErrUnavailable
	}
	var locked int
	if err := b.tx.WithContext(ctx).Raw(`SELECT 1 FROM pg_advisory_xact_lock(69421,hashtext(?))`, actor.ID.String()+":"+c.Key.String()).Scan(&locked).Error; err != nil {
		return nil, err
	}
	var row struct {
		OrgID, ProjectID              uuid.UUID
		Kind                          domain.Kind
		Action, RequestHash, Response string
	}
	read := b.tx.Raw(`SELECT org_id,project_id,kind,action,request_hash,response::text AS response FROM bible.command WHERE actor_id=? AND request_id=?`, actor.ID, c.Key).Scan(&row)
	if read.Error != nil {
		return nil, fmt.Errorf("read Bible command: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	if row.OrgID != actor.OrgID || row.ProjectID != c.ProjectID || row.Kind != c.Kind || row.Action != c.Action || row.RequestHash != hash {
		return nil, application.ErrIdempotencyConflict
	}
	var result application.Receipt
	if domain.DecodeClosedJSON([]byte(row.Response), &result) != nil || result.EntryID == uuid.Nil || result.VersionID == uuid.Nil || result.Kind != c.Kind || result.Revision < 1 {
		return nil, domain.ErrCorruptHistory
	}
	return &result, nil
}

// InsertHead inserts one stable identity; its version foreign key is checked at commit.
func (b *bound) InsertHead(ctx context.Context, h domain.Head) error {
	table, _, _, err := tables(h.Kind)
	if err != nil {
		return err
	}
	if h.OrgID != b.actor.OrgID || h.ProjectID != b.project.ProjectID {
		return application.ErrUnavailable
	}
	write := b.tx.WithContext(ctx).Table(table).Create(map[string]any{"id": h.ID, "org_id": h.OrgID, "project_id": h.ProjectID, "revision": h.Revision, "current_version_id": h.CurrentVersionID, "confirmed_version_id": h.ConfirmedVersionID, "is_delete": h.Deleted, "created_at": h.CreatedAt, "updated_at": h.UpdatedAt})
	return exactlyOne(write)
}

// UpdateHead changes only permitted head fields under exact compare-and-swap.
func (b *bound) UpdateHead(ctx context.Context, h domain.Head, expected int64) error {
	table, _, _, err := tables(h.Kind)
	if err != nil {
		return err
	}
	fields := map[string]any{"revision": h.Revision, "current_version_id": h.CurrentVersionID, "confirmed_version_id": h.ConfirmedVersionID, "is_delete": h.Deleted, "updated_at": h.UpdatedAt}
	if h.Kind == domain.KindCharacter {
		fields["redirect_id"] = h.RedirectID
	}
	return exactlyOne(b.tx.WithContext(ctx).Table(table).Where("id=? AND org_id=? AND project_id=? AND revision=?", h.ID, b.actor.OrgID, b.project.ProjectID, expected).Updates(fields))
}

// Confirm appends the explicit accepted version and identity revision.
func (b *bound) Confirm(ctx context.Context, kind domain.Kind, c domain.Confirmation) error {
	_, _, table, err := tables(kind)
	if err != nil {
		return err
	}
	return exactlyOne(b.tx.WithContext(ctx).Table(table).Create(map[string]any{"id": c.ID, "org_id": b.actor.OrgID, "project_id": b.project.ProjectID, "entry_id": c.EntryID, "version_id": c.VersionID, "revision": c.Revision, "actor_id": c.ActorID, "created_at": c.CreatedAt}))
}

// Redirect preserves an explicit merge's exact historical endpoints.
func (b *bound) Redirect(ctx context.Context, r domain.Redirect) error {
	return exactlyOne(b.tx.WithContext(ctx).Table("bible.character_redirect").Create(map[string]any{"id": r.ID, "org_id": b.actor.OrgID, "project_id": b.project.ProjectID, "source_id": r.SourceID, "target_id": r.TargetID, "source_version_id": r.SourceVersionID, "target_version_id": r.TargetVersionID, "actor_id": r.ActorID, "created_at": r.CreatedAt}))
}

// Split records an explicit independent child without modifying any historical parent version.
func (b *bound) Split(ctx context.Context, r domain.Split) error {
	return exactlyOne(b.tx.WithContext(ctx).Table("bible.character_split").Create(map[string]any{"id": r.ID, "org_id": b.actor.OrgID, "project_id": b.project.ProjectID, "source_id": r.SourceID, "source_version_id": r.SourceVersionID, "target_id": r.TargetID, "target_version_id": r.TargetVersionID, "actor_id": r.ActorID, "created_at": r.CreatedAt}))
}

// Record persists a small original response and a safe audit event in the same transaction.
func (b *bound) Record(ctx context.Context, actor identityapp.Principal, c application.Command, hash string, r application.Receipt, now time.Time) error {
	response, err := json.Marshal(r)
	if err != nil || len(response) > 16384 {
		return application.ErrUnavailable
	}
	if err := exactlyOne(b.tx.WithContext(ctx).Exec(`INSERT INTO bible.command(actor_id,request_id,org_id,project_id,kind,action,request_hash,response,created_at) VALUES(?,?,?,?,?,?,?,?::jsonb,?)`, actor.ID, c.Key, actor.OrgID, c.ProjectID, c.Kind, c.Action, hash, string(response), now)); err != nil {
		return err
	}
	const topic = "lanverse.audit.recorded.v1"
	id := uuid.NewSHA1(c.Key, []byte("bible-audit/"+actor.ID.String()))
	after := struct {
		Revision        int64     `json:"revision"`
		ProjectRevision int64     `json:"project_revision"`
		VersionID       uuid.UUID `json:"version_id"`
		ContentSHA256   string    `json:"content_sha256"`
	}{r.Revision, r.ProjectRevision, r.VersionID, r.ContentSHA256}
	data, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": c.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": "bible." + string(c.Kind) + "_" + c.Action, "object": map[string]any{"type": string(c.Kind), "id": r.EntryID.String()}, "before": nil, "after": after, "request_id": c.RequestID.String()}})
	if err != nil {
		return err
	}
	return exactlyOne(b.tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, c.ProjectID.String(), string(data)))
}
