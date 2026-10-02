package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// Find reads a stable identity without mutating its current or confirmed pointers.
func (s *Store) Find(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, id uuid.UUID) (application.Detail, error) {
	var result application.Detail
	err := s.transaction(ctx, actor, project, false, func(b *bound) error {
		h, v, err := b.Load(ctx, kind, id)
		if err != nil {
			return err
		}
		result = application.Detail{Head: h, Current: v, ResolvedID: h.ID}
		seen := map[uuid.UUID]bool{h.ID: true}
		for depth := 0; h.RedirectID != nil; depth++ {
			if depth >= 32 || seen[*h.RedirectID] {
				return domain.ErrCorruptHistory
			}
			seen[*h.RedirectID] = true
			h, err = b.loadHead(ctx, kind, *h.RedirectID)
			if err != nil {
				return domain.ErrCorruptHistory
			}
			if h.Deleted || h.ConfirmedVersionID == nil {
				return domain.ErrCorruptHistory
			}
			result.ResolvedID = h.ID
		}
		return nil
	})
	return result, err
}

// Version keeps pinned historical identity and media facts unchanged across current redirects.
func (s *Store) Version(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, entry, version uuid.UUID) (domain.Version, error) {
	var result domain.Version
	err := s.transaction(ctx, actor, project, false, func(b *bound) error {
		if _, err := b.loadHead(ctx, kind, entry); err != nil {
			return err
		}
		v, err := b.LoadVersion(ctx, kind, version)
		if err != nil {
			return err
		}
		if v.EntryID != entry {
			return application.ErrNotFound
		}
		result = v
		return nil
	})
	return result, err
}

// List returns bounded stable heads under actual current project authorization.
func (s *Store) List(ctx context.Context, actor identityapp.Principal, input application.ListInput) (application.Page, error) {
	result := application.Page{Entries: []application.Summary{}, CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID}
	err := s.transaction(ctx, actor, input.ProjectID, false, func(b *bound) error {
		table, _, _, err := tables(input.Kind)
		if err != nil {
			return err
		}
		query := b.tx.Table(table).Select("id,created_at").Where("org_id=? AND project_id=? AND is_delete=false", actor.OrgID, input.ProjectID)
		if input.Kind == domain.KindCharacter {
			query = query.Where("redirect_id IS NULL")
		}
		if input.After != nil {
			if input.After.ID == uuid.Nil || input.After.CreatedAt.IsZero() {
				return domain.ErrInvalidContent
			}
			query = query.Where("(created_at,id)>(?,?)", input.After.CreatedAt, input.After.ID)
		}
		var ids []application.ListCursor
		if err := query.Order("created_at,id").Limit(input.Limit + 1).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) > input.Limit {
			last := ids[input.Limit-1]
			result.Next = &last
			ids = ids[:input.Limit]
		}
		for _, row := range ids {
			h, v, err := b.Load(ctx, input.Kind, row.ID)
			if err != nil {
				return err
			}
			result.Entries = append(result.Entries, application.Summary{Head: h, Name: v.Name(), ContentSHA256: v.ContentSHA256})
		}
		return nil
	})
	return result, err
}

// History pages full immutable versions in reverse creation order.
func (s *Store) History(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, entry uuid.UUID, limit int, after *application.HistoryCursor) (application.HistoryPage, error) {
	result := application.HistoryPage{Versions: []domain.Version{}}
	err := s.transaction(ctx, actor, project, false, func(b *bound) error {
		if _, err := b.loadHead(ctx, kind, entry); err != nil {
			return err
		}
		_, table, _, err := tables(kind)
		if err != nil {
			return err
		}
		query := b.tx.Table(table).Select("id,version_no AS number").Where("org_id=? AND project_id=? AND entry_id=?", actor.OrgID, project, entry)
		if after != nil {
			if after.ID == uuid.Nil || after.Number < 1 {
				return domain.ErrInvalidContent
			}
			query = query.Where("(version_no,id)<(?,?)", after.Number, after.ID)
		}
		var ids []application.HistoryCursor
		if err := query.Order("version_no DESC,id DESC").Limit(limit + 1).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) > limit {
			last := ids[limit-1]
			result.Next = &last
			ids = ids[:limit]
		}
		for _, row := range ids {
			v, err := b.LoadVersion(ctx, kind, row.ID)
			if err != nil {
				return err
			}
			result.Versions = append(result.Versions, v)
		}
		return nil
	})
	return result, err
}

// Voices lists only configured catalog facts and never writes built-in candidate seeds.
func (s *Store) Voices(ctx context.Context, actor identityapp.Principal, project uuid.UUID, limit int, after *application.VoiceCursor) (application.VoicePage, error) {
	var result application.VoicePage
	err := s.transaction(ctx, actor, project, false, func(b *bound) error {
		if b.voices == nil {
			return application.ErrUnavailable
		}
		page, err := b.voices.List(ctx, actor, project, limit, after)
		if err != nil {
			return err
		}
		if len(page.Voices) > limit {
			return application.ErrUnavailable
		}
		for _, v := range page.Voices {
			if v.ModelKey == "" || v.VoiceKey == "" || v.ModelVersion < 1 {
				return application.ErrUnavailable
			}
		}
		if page.Voices == nil {
			page.Voices = []application.VoiceChoice{}
		}
		result = page
		return nil
	})
	return result, err
}
