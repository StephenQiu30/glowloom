package postgres

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// SubjectSourceActorFactory resolves target and personal authority in this snapshot.
type SubjectSourceActorFactory func(*gorm.DB) application.SubjectSourceActor

// SubjectCatalogueStore selects personal originals and proven initiating outputs.
// It never broadens the ordinary project media or personal history routes.
type SubjectCatalogueStore struct {
	db      *gorm.DB
	library *LibraryStore
	actors  SubjectSourceActorFactory
}

// NewSubjectCatalogueStore composes current authority with the owning media loader.
func NewSubjectCatalogueStore(db *gorm.DB, library *LibraryStore, actors SubjectSourceActorFactory) *SubjectCatalogueStore {
	return &SubjectCatalogueStore{db: db, library: library, actors: actors}
}

type subjectCatalogueRow struct {
	ID         uuid.UUID
	Source     string
	ProjectID  *uuid.UUID
	Revision   int64
	Favorite   bool
	Upscaled   bool
	Title      string
	Kind       string
	CreateTime time.Time
	UpdateTime time.Time
	AssetFacts []byte
}

func (r subjectCatalogueRow) item() (application.SubjectSourceItem, error) {
	a, err := decodeLibraryAsset(r.AssetFacts)
	if err != nil || a.ContainsRealPerson || a.ConsentRecordID != nil {
		return application.SubjectSourceItem{}, application.ErrUnavailable
	}
	if r.Source == "personal_asset" && (a.Personal == nil || a.ProjectID != uuid.Nil || r.ID != a.ID) || r.Source == "generated_output" && (a.Personal != nil || r.ProjectID == nil || a.ProjectID != *r.ProjectID) {
		return application.SubjectSourceItem{}, application.ErrUnavailable
	}
	item := application.SubjectSourceItem{ID: r.ID, Source: r.Source, ProjectID: r.ProjectID, Revision: r.Revision, Favorite: r.Favorite, Upscaled: r.Upscaled, Title: r.Title, Kind: r.Kind, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime, Media: application.SubjectSourceMedia{LibraryAssetSummary: application.PersonalUploadSummary(a), Origin: string(a.Origin)}}
	if err := item.Validate(); err != nil {
		return application.SubjectSourceItem{}, err
	}
	return item, nil
}

// Three immutable receipt paths prove who initiated a generation. JSON remains
// text to safely ignore malformed legacy values; confirmed_by is never authority.
func subjectCatalogueCTE(actor, personal identity.Principal, library uuid.UUID) (string, []any) {
	query, args := libraryEntries(personal, domain.LibraryScope{Kind: domain.LibraryPersonal}, library)
	query += `, source_limits AS (SELECT ?::bigint AS image_limit,?::bigint AS video_limit,?::bigint AS audio_limit),
 source_operation_ids AS (
 SELECT ps.operation_id::text AS operation_id FROM operation.prompt_submission ps
 JOIN operation.operation submitted ON submitted.id=ps.operation_id AND submitted.project_id=ps.project_id
 JOIN workspace.project submitted_project ON submitted_project.id=ps.project_id
 WHERE ps.actor_id=? AND submitted_project.org_id=?
 UNION SELECT qr.result->>'operation_id' FROM operation.quote_request qr
 WHERE qr.actor_id=? AND qr.org_id=? AND jsonb_typeof(qr.result->'operation_id')='string'
 UNION SELECT item->>'operation_id' FROM operation.quote_request qr
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(qr.result->'items')='array' THEN qr.result->'items' ELSE '[]'::jsonb END) item
 WHERE qr.actor_id=? AND qr.org_id=? AND jsonb_typeof(item->'operation_id')='string'
 ), catalogue AS (
 SELECT e.asset_id AS id,'personal_asset'::text AS source,NULL::uuid AS project_id,e.revision,e.favorite,false AS upscaled,
 e.title,e.kind,e.create_time,e.update_time,e.asset_facts,
 concat_ws(' ',e.title,e.note,e.source_label,e.tags::text) AS search_text
 FROM entries e CROSS JOIN source_limits lim
 WHERE e.asset_id IS NOT NULL AND e.catalog_state='active' AND e.kind IN ('image','video','audio')
 AND e.asset_facts->>'contains_real_person'='false' AND e.asset_facts->>'consent_record_id' IS NULL
 AND e.asset_facts->>'sha256' ~ '^[a-f0-9]{64}$'
 AND (e.asset_facts->>'byte_size')::bigint BETWEEN 1 AND CASE e.kind WHEN 'image' THEN lim.image_limit WHEN 'video' THEN lim.video_limit ELSE lim.audio_limit END
 UNION ALL
 SELECT output.id,'generated_output'::text,p.id,COALESCE(meta.revision,0),COALESCE(meta.favorite,false),
 (o.capability IN ('image.upscale','video.upscale','image.super_resolution','video.super_resolution') OR o.mode='super_resolution') AS upscaled,
 LEFT(COALESCE(NULLIF(asset.file_name,''),asset.kind),240) AS title,asset.kind,output.create_time,GREATEST(output.update_time,asset.update_time),to_jsonb(asset),
 concat_ws(' ',asset.file_name,(SELECT string_agg(input.text_value,' ' ORDER BY input.seq_no) FROM operation.operation_input input WHERE input.operation_id=o.id AND NOT input.is_delete)) AS search_text
 FROM operation.operation o
 JOIN workspace.project p ON p.id=o.project_id
 JOIN source_operation_ids initiated ON initiated.operation_id=o.id::text
 JOIN operation.operation_output output ON output.operation_id=o.id AND output.project_id=o.project_id
 JOIN media.media_asset asset ON asset.id=output.media_asset_id AND asset.project_id=o.project_id
 LEFT JOIN operation.personal_history_record history ON history.operation_id=o.id AND history.actor_id=? AND history.org_id=? AND history.scope_id=?
 LEFT JOIN operation.personal_history_output meta ON meta.operation_id=o.id AND meta.output_id=output.id AND meta.actor_id=? AND meta.org_id=? AND meta.scope_id=?
 LEFT JOIN media.library project_library ON project_library.kind='project' AND project_library.project_id=p.id AND project_library.org_id=p.org_id
 LEFT JOIN media.library_item catalog_item ON catalog_item.library_id=project_library.id AND catalog_item.asset_id=asset.id
 CROSS JOIN source_limits lim
 WHERE p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete AND p.status IN ('active','archived')
 AND NOT o.is_delete AND o.status='completed' AND o.origin IN ('pipeline','batch','canvas','agent')
 AND output.kind='media' AND NOT output.is_delete AND output.moderation_status='passed'
 AND history.deleted_at IS NULL AND meta.deleted_at IS NULL
 AND asset.kind IN ('image','video','audio') AND asset.personal_org_id IS NULL AND asset.personal_actor_id IS NULL
 AND NOT asset.is_delete AND asset.status='ready' AND asset.moderation_status='passed' AND NOT asset.contains_real_person AND asset.consent_record_id IS NULL
 AND asset.sha256 ~ '^[a-f0-9]{64}$' AND asset.byte_size BETWEEN 1 AND CASE asset.kind WHEN 'image' THEN lim.image_limit WHEN 'video' THEN lim.video_limit ELSE lim.audio_limit END
 AND COALESCE(catalog_item.catalog_state,'active')='active' AND catalog_item.purged_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM media.purge_item purge WHERE purge.item_id=asset.id AND purge.status IN ('queued','running','needs_reconciliation','succeeded'))
 ) `
	args = append(args, application.MaxUploadImageBytes, application.MaxUploadVideoBytes, application.MaxUploadAudioBytes, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ScopeID, actor.ID, actor.OrgID, actor.ScopeID, actor.OrgID, actor.ScopeID)
	return query, args
}

func subjectCatalogueFilter(query application.SubjectCatalogueQuery) (string, []any) {
	filter := "kind=?"
	args := []any{query.Kind}
	if query.FavoriteOnly {
		filter += " AND favorite"
	}
	if query.Quality == "upscaled" {
		filter += " AND upscaled"
	}
	if query.Search != "" {
		filter += " AND strpos(lower(search_text),lower(?))>0"
		args = append(args, query.Search)
	}
	return filter, args
}

func (s *SubjectCatalogueStore) read(ctx context.Context, actor identity.Principal, consume func(*gorm.DB, string, []any) error) error {
	if s == nil || s.db == nil || s.library == nil || s.actors == nil {
		return application.ErrUnavailable
	}
	return platformdb.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		access := s.actors(tx)
		if access == nil {
			return application.ErrUnavailable
		}
		personal, err := access.PersonalSourceActor(ctx, actor, false)
		if err != nil {
			return err
		}
		if personal.ID != actor.ID || personal.SessionID != actor.SessionID || personal.ID == uuid.Nil || personal.ScopeID == uuid.Nil || personal.OrgID == uuid.Nil {
			return application.ErrNotFound
		}
		if err := s.library.requireCurrentActor(tx, personal); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, personal, domain.LibraryScope{Kind: domain.LibraryPersonal}, false); err != nil {
			return err
		}
		library, err := readLibrary(tx, personal, domain.LibraryScope{Kind: domain.LibraryPersonal}, false)
		if err != nil {
			return err
		}
		query, args := subjectCatalogueCTE(actor, personal, library.ID)
		return consume(tx, query, args)
	})
}

func (s *SubjectCatalogueStore) projects(ctx context.Context, tx *gorm.DB, actor identity.Principal, cte, filter string, args []any) error {
	var projects []struct{ ProjectID uuid.UUID }
	if err := tx.Raw(cte+`SELECT DISTINCT project_id FROM catalogue WHERE project_id IS NOT NULL AND `+filter+` ORDER BY project_id`, args...).Scan(&projects).Error; err != nil {
		return fmt.Errorf("read subject source projects: %w", err)
	}
	for _, project := range projects {
		if _, err := s.library.authorize(ctx, tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project.ProjectID}, false); err != nil {
			return err
		}
	}
	return nil
}

// List authorizes and filters both owning sources before count and stable paging.
func (s *SubjectCatalogueStore) List(ctx context.Context, actor identity.Principal, input application.SubjectCatalogueQuery) (application.SubjectSourcePage, error) {
	if err := input.Validate(); err != nil {
		return application.SubjectSourcePage{}, err
	}
	var page application.SubjectSourcePage
	err := s.read(ctx, actor, func(tx *gorm.DB, cte string, base []any) error {
		page = application.SubjectSourcePage{CurrentActorID: actor.ID, CurrentScopeID: actor.ScopeID, Items: []application.SubjectSourceItem{}, Page: input.Page, PageSize: input.PageSize, Capabilities: application.SubjectSourceCapabilities{UpscaledFilter: true}}
		filter, filters := subjectCatalogueFilter(input)
		args := append(slices.Clone(base), filters...)
		if err := s.projects(ctx, tx, actor, cte, filter, args); err != nil {
			return err
		}
		var collisions []struct{ ID uuid.UUID }
		if err := tx.Raw(cte+`SELECT id FROM catalogue GROUP BY id HAVING count(*)>1 LIMIT 1`, base...).Scan(&collisions).Error; err != nil {
			return fmt.Errorf("check subject source identities: %w", err)
		}
		if len(collisions) != 0 {
			return application.ErrUnavailable
		}
		if err := tx.Raw(cte+`SELECT count(*) FROM catalogue WHERE `+filter, args...).Scan(&page.Total).Error; err != nil {
			return fmt.Errorf("count subject source catalogue: %w", err)
		}
		var rows []subjectCatalogueRow
		paging := append(slices.Clone(args), input.PageSize, (input.Page-1)*input.PageSize)
		if err := tx.Raw(cte+`SELECT * FROM catalogue WHERE `+filter+` ORDER BY update_time DESC,id ASC LIMIT ? OFFSET ?`, paging...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read subject source catalogue: %w", err)
		}
		for _, row := range rows {
			item, err := row.item()
			if err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		return nil
	})
	return page, err
}

// Detail resolves only one source ID under fresh initiator and owner authority.
func (s *SubjectCatalogueStore) Detail(ctx context.Context, actor identity.Principal, id uuid.UUID) (application.SubjectSourceItem, error) {
	if id == uuid.Nil {
		return application.SubjectSourceItem{}, application.ErrNotFound
	}
	var item application.SubjectSourceItem
	err := s.read(ctx, actor, func(tx *gorm.DB, cte string, base []any) error {
		args := append(slices.Clone(base), id)
		var rows []subjectCatalogueRow
		if err := tx.Raw(cte+`SELECT * FROM catalogue WHERE id=? LIMIT 2`, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read subject source: %w", err)
		}
		if len(rows) == 0 {
			return application.ErrNotFound
		}
		if len(rows) != 1 {
			return application.ErrUnavailable
		}
		if rows[0].ProjectID != nil {
			if _, err := s.library.authorize(ctx, tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: rows[0].ProjectID}, false); err != nil {
				return err
			}
		}
		var err error
		item, err = rows[0].item()
		return err
	})
	return item, err
}

var _ application.SubjectCatalogue = (*SubjectCatalogueStore)(nil)

// FindLibraryMedia uses the closed server-resolved owner and an asset identity.
// Catalogue metadata IDs from older libraries are not media identities. This
// reader is composed only behind SubjectLibrary's source proof and does not
// alter ordinary media routes or infer generation ownership from project access.
func (s *SubjectCatalogueStore) FindLibraryMedia(ctx context.Context, actor identity.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryMediaFile, error) {
	if s == nil || s.library == nil || id == uuid.Nil {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var file application.LibraryMediaFile
	err := s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		query, args := libraryEntries(actor, scope, library.ID)
		args = append(args, id)
		var rows []libraryEntryRow
		if err := tx.Raw(query+`SELECT * FROM entries WHERE asset_id=? AND catalog_state='active' LIMIT 2`, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read subject source original: %w", err)
		}
		if len(rows) != 1 {
			return application.ErrNotFound
		}
		expected, err := decodeLibraryAsset(rows[0].AssetFacts)
		if err != nil || expected.ID != id {
			return application.ErrUnavailable
		}
		file, err = NewSubjectSourceStore(tx, nil).ReadBoundSubjectSource(ctx, id)
		if err != nil {
			return err
		}
		actual := file.Asset
		samePersonal := expected.Personal == nil && actual.Personal == nil || expected.Personal != nil && actual.Personal != nil && *expected.Personal == *actual.Personal
		if expected.ProjectID != actual.ProjectID || !samePersonal || expected.Revision != actual.Revision || expected.Kind != actual.Kind {
			return application.ErrNotFound
		}
		return nil
	})
	return file, err
}

var _ application.LibraryMediaReader = (*SubjectCatalogueStore)(nil)
