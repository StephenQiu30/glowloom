package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

type detailRow struct {
	ID, OwnerScopeID, PhysicalScopeID, VersionID, CreatedBy, VersionCreatedBy uuid.UUID
	Revision, VersionNo                                                       int64
	CreatedAt, UpdatedAt, VersionCreatedAt                                    time.Time
	DeletedAt                                                                 *time.Time
	PreviousID                                                                *uuid.UUID
	Content                                                                   []byte
	ContentSHA256                                                             string
	Favorite                                                                  bool
}

const detailColumns = `h.id,h.owner_scope_id,h.physical_scope_id,h.revision,h.created_by,h.created_at,h.updated_at,h.deleted_at,v.id AS version_id,v.version_no,v.previous_id,v.created_by AS version_created_by,v.created_at AS version_created_at,v.content,v.content_sha256,coalesce(f.favorite,false) AS favorite`
const detailFrom = ` FROM subject.subject_head h JOIN subject.subject_version v ON v.id=h.current_version_id AND v.subject_id=h.id AND v.owner_scope_id=h.owner_scope_id AND v.physical_scope_id=h.physical_scope_id LEFT JOIN subject.favorite f ON f.subject_id=h.id AND f.actor_id=?`

func (b *bound) detailRow(ctx context.Context, id uuid.UUID, write bool) (detailRow, error) {
	var row detailRow
	lock := " FOR SHARE OF h"
	if write {
		lock = " FOR UPDATE OF h"
	}
	r := b.tx.WithContext(ctx).Raw(`SELECT `+detailColumns+detailFrom+` WHERE h.id=? AND h.owner_scope_id=? AND h.physical_scope_id=? AND h.deleted_at IS NULL`+lock, b.actor.ID, id, b.scope.ScopeID, b.scope.PhysicalScopeID).Scan(&row)
	if r.Error != nil {
		return row, r.Error
	}
	if r.RowsAffected != 1 {
		return row, application.ErrNotFound
	}
	return row, nil
}

func decode(row detailRow) (domain.Head, domain.Version, bool, error) {
	h := domain.Head{ID: row.ID, OwnerScopeID: row.OwnerScopeID, PhysicalScopeID: row.PhysicalScopeID, Revision: row.Revision, VersionID: row.VersionID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(), DeletedAt: row.DeletedAt}
	if h.DeletedAt != nil {
		deleted := h.DeletedAt.UTC()
		h.DeletedAt = &deleted
	}
	v := domain.Version{ID: row.VersionID, SubjectID: row.ID, OwnerScopeID: row.OwnerScopeID, PhysicalScopeID: row.PhysicalScopeID, Number: row.VersionNo, PreviousID: row.PreviousID, CreatedBy: row.VersionCreatedBy, CreatedAt: row.VersionCreatedAt.UTC(), SHA256: row.ContentSHA256}
	if len(row.Content) > 1<<20 || json.Unmarshal(row.Content, &v.Content) != nil || v.Content.Validate() != nil {
		return h, v, false, application.ErrUnavailable
	}
	digest, err := domain.Digest(v.Content)
	if err != nil || digest != v.SHA256 {
		return h, v, false, application.ErrUnavailable
	}
	return h, v, row.Favorite, nil
}

// Get returns a complete safe current-space settings projection.
func (s *Store) Get(ctx context.Context, a identity.Principal, id uuid.UUID) (application.Detail, error) {
	if id == uuid.Nil {
		return application.Detail{}, domain.ErrInvalid
	}
	var result application.Detail
	err := s.read(ctx, a, func(b *bound) error {
		row, err := b.detailRow(ctx, id, false)
		if err != nil {
			return err
		}
		h, v, fav, err := decode(row)
		if err != nil {
			return err
		}
		result = application.Present(h, v, fav)
		return nil
	})
	return result, err
}

// List performs complete-space filters and ordering before applying offset bounds.
func (s *Store) List(ctx context.Context, a identity.Principal, in application.ListInput) (application.Page, error) {
	if err := in.Validate(); err != nil {
		return application.Page{}, err
	}
	var result application.Page
	err := s.read(ctx, a, func(b *bound) error {
		result = application.Page{CurrentActorID: a.ID, CurrentScopeID: a.ScopeID, Items: []application.Detail{}, Page: in.Page, PageSize: in.PageSize, Capabilities: application.Capabilities{IntelligentVoice: application.Capability{Available: false, Reason: "intelligent_voice_unavailable"}}}
		if s.factories.Voice != nil {
			if voice := s.factories.Voice(b.tx); voice != nil {
				if options, err := voice.Options(ctx, a); err == nil {
					result.Capabilities.IntelligentVoice = application.Capability{Available: options.Available, Reason: options.Reason}
				}
			}
		}
		where := ` WHERE h.owner_scope_id=? AND h.physical_scope_id=? AND h.deleted_at IS NULL`
		args := []any{a.ID, b.scope.ScopeID, b.scope.PhysicalScopeID}
		if in.Query != "" {
			where += ` AND strpos(lower(v.content->>'name'),lower(?))>0`
			args = append(args, in.Query)
		}
		if in.Favorite {
			where += ` AND coalesce(f.favorite,false)`
		}
		field := "h.updated_at"
		if in.TimeField == "created" {
			field = "h.created_at"
		}
		if in.From != nil {
			where += ` AND ` + field + `>=?`
			args = append(args, in.From.UTC())
		}
		if in.To != nil {
			where += ` AND ` + field + `<?`
			args = append(args, in.To.UTC())
		}
		if err := b.tx.WithContext(ctx).Raw(`SELECT count(*)`+detailFrom+where, args...).Scan(&result.Total).Error; err != nil {
			return err
		}
		order := map[string]string{"created_asc": "h.created_at ASC,h.id ASC", "created_desc": "h.created_at DESC,h.id DESC", "updated_asc": "h.updated_at ASC,h.id ASC", "updated_desc": "h.updated_at DESC,h.id DESC"}[in.Order]
		pageArgs := append(append([]any{}, args...), in.PageSize, (in.Page-1)*in.PageSize)
		var rows []detailRow
		if err := b.tx.WithContext(ctx).Raw(`SELECT `+detailColumns+detailFrom+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, pageArgs...).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			h, v, fav, err := decode(row)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, application.Present(h, v, fav))
		}
		return nil
	})
	return result, err
}

// ReadVersion returns a pinned immutable snapshot only while its head remains usable.
func (s *Store) ReadVersion(ctx context.Context, a identity.Principal, id, version uuid.UUID) (domain.Version, error) {
	if id == uuid.Nil || version == uuid.Nil {
		return domain.Version{}, domain.ErrInvalid
	}
	var result domain.Version
	err := s.read(ctx, a, func(b *bound) error { var err error; result, err = b.readVersion(ctx, id, version); return err })
	return result, err
}

func (b *bound) readVersion(ctx context.Context, id, version uuid.UUID) (domain.Version, error) {
	var row detailRow
	from := strings.Replace(detailFrom, "v.id=h.current_version_id", "v.id=?", 1)
	r := b.tx.WithContext(ctx).Raw(`SELECT `+detailColumns+from+` WHERE h.id=? AND h.owner_scope_id=? AND h.physical_scope_id=? AND h.deleted_at IS NULL FOR SHARE OF h`, version, b.actor.ID, id, b.scope.ScopeID, b.scope.PhysicalScopeID).Scan(&row)
	if r.Error != nil {
		return domain.Version{}, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Version{}, application.ErrNotFound
	}
	_, v, _, err := decode(row)
	return v, err
}

// BoundMedia verifies a materialized binding against its immutable subject version.
func (s *Store) BoundMedia(ctx context.Context, a identity.Principal, id uuid.UUID) (application.BoundMedia, error) {
	if id == uuid.Nil {
		return application.BoundMedia{}, domain.ErrInvalid
	}
	var result application.BoundMedia
	err := s.read(ctx, a, func(b *bound) error {
		var row struct {
			BindingID, SubjectID, VersionID, OwnerScopeID, PhysicalScopeID, AssetID uuid.UUID
			AssetRevision                                                           int64
			Kind, SHA256, BindingSHA256, Role                                       string
			Position                                                                int
			Fact                                                                    []byte
		}
		r := b.tx.WithContext(ctx).Raw(`SELECT binding_id,subject_id,version_id,owner_scope_id,physical_scope_id,asset_id,asset_revision,kind,sha256,binding_sha256,role,position,fact FROM subject.media_binding WHERE binding_id=? AND owner_scope_id=? AND physical_scope_id=?`, id, b.scope.ScopeID, b.scope.PhysicalScopeID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrNotFound
		}
		version, err := b.readVersion(ctx, row.SubjectID, row.VersionID)
		if err != nil {
			return err
		}
		for _, entry := range bindings(version.Content) {
			if entry.binding.ID != id {
				continue
			}
			hash, err := domain.Digest(entry.binding)
			if err != nil {
				return err
			}
			fact := entry.binding.Media
			var stored domain.MediaFact
			if len(row.Fact) > 64<<10 || json.Unmarshal(row.Fact, &stored) != nil {
				return application.ErrUnavailable
			}
			storedHash, err := domain.Digest(stored)
			if err != nil {
				return err
			}
			expectedHash, err := domain.Digest(fact)
			if err != nil {
				return err
			}
			if row.SubjectID != version.SubjectID || row.VersionID != version.ID || row.AssetID != fact.AssetID || row.AssetRevision != fact.Revision || row.Kind != fact.Kind || row.SHA256 != fact.SHA256 || row.BindingSHA256 != hash || row.Role != entry.role || row.Position != entry.position || storedHash != expectedHash {
				return fmt.Errorf("subject binding differs from immutable content: %w", application.ErrUnavailable)
			}
			result = application.BoundMedia{SubjectID: row.SubjectID, VersionID: row.VersionID, OwnerScopeID: row.OwnerScopeID, PhysicalScopeID: row.PhysicalScopeID, BindingID: id, BindingSHA256: hash, ContentSHA256: version.SHA256, Name: version.Content.Name, Description: version.Content.Description, Role: row.Role, Position: row.Position, Media: fact}
			return nil
		}
		return application.ErrUnavailable
	})
	return result, err
}

var _ application.Query = (*Store)(nil)

// ReadVersionIfKnown distinguishes legacy identities without exposing existence
// through a public endpoint. Existing deleted or foreign subjects never fall back.
func (s *Store) ReadVersionIfKnown(ctx context.Context, a identity.Principal, id, version uuid.UUID) (domain.Version, bool, error) {
	if id == uuid.Nil || version == uuid.Nil {
		return domain.Version{}, false, domain.ErrInvalid
	}
	var result domain.Version
	known := false
	err := s.read(ctx, a, func(b *bound) error {
		if err := b.tx.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM subject.subject_head WHERE id=?)`, id).Scan(&known).Error; err != nil {
			return err
		}
		if !known {
			return nil
		}
		var err error
		result, err = b.readVersion(ctx, id, version)
		return err
	})
	return result, known, err
}

// Recover discovers a committed result without rerunning any write or dependency.
func (s *Store) Recover(ctx context.Context, a identity.Principal, key uuid.UUID) (application.Recovery, error) {
	if key == uuid.Nil {
		return application.Recovery{}, domain.ErrInvalid
	}
	result := application.Recovery{State: "unrecorded"}
	err := s.read(ctx, a, func(b *bound) error {
		result = application.Recovery{State: "unrecorded"}
		var row struct {
			OwnerScopeID, PhysicalScopeID uuid.UUID
			ResponseBody                  []byte
		}
		r := b.tx.WithContext(ctx).Raw(`SELECT owner_scope_id,physical_scope_id,response_body FROM subject.command WHERE actor_id=? AND idem_key=?`, a.ID, key).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		if row.OwnerScopeID != b.scope.ScopeID || row.PhysicalScopeID != b.scope.PhysicalScopeID {
			return application.ErrNotFound
		}
		var response application.CommandResult
		if len(row.ResponseBody) > 1<<20 || json.Unmarshal(row.ResponseBody, &response) != nil {
			return application.ErrUnavailable
		}
		result.State = "recorded"
		result.Result = &response
		return nil
	})
	return result, err
}
