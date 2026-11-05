package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
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
	Source     application.SubjectSourceKind
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
	if r.Source == application.SubjectSourcePersonalAsset && (a.Personal == nil || a.ProjectID != uuid.Nil || r.ID != a.ID) || r.Source == application.SubjectSourceGeneratedOutput && (a.Personal != nil || r.ProjectID == nil || a.ProjectID != *r.ProjectID) {
		return application.SubjectSourceItem{}, application.ErrUnavailable
	}
	item := application.SubjectSourceItem{ID: r.ID, Source: r.Source, ProjectID: r.ProjectID, Revision: r.Revision, Favorite: r.Favorite, Upscaled: r.Upscaled, Title: r.Title, Kind: r.Kind, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime, Media: application.SubjectSourceMedia{LibraryAssetSummary: application.PersonalUploadSummary(a), Origin: string(a.Origin)}}
	if err := item.Validate(); err != nil {
		return application.SubjectSourceItem{}, err
	}
	return item, nil
}

// subjectSourceOperations combines immutable receipts written by prompt submission
// and quote creation. The confirming actor is deliberately not an ownership source.
func subjectSourceOperations(tx *gorm.DB, actor identity.Principal) *gorm.DB {
	promptSubmissions := tx.Table("operation.prompt_submission AS ps").
		Select("ps.operation_id::text AS operation_id").
		Joins("JOIN operation.operation AS submitted ON submitted.id = ps.operation_id AND submitted.project_id = ps.project_id").
		Joins("JOIN workspace.project AS submitted_project ON submitted_project.id = ps.project_id").
		Where("ps.actor_id = ? AND submitted_project.org_id = ?", actor.ID, actor.OrgID)
	quoteRequests := tx.Table("operation.quote_request AS qr")
	quoteOperation := quoteRequests.Select("qr.result ->> 'operation_id' AS operation_id").
		Where("qr.actor_id = ? AND qr.org_id = ?", actor.ID, actor.OrgID).
		Where("jsonb_typeof(qr.result -> 'operation_id') = ?", "string")
	quoteItems := tx.Table("operation.quote_request AS qr").
		Select("item ->> 'operation_id' AS operation_id").
		Joins("CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(qr.result -> 'items') = ? THEN qr.result -> 'items' ELSE '[]'::jsonb END) AS item", "array").
		Where("qr.actor_id = ? AND qr.org_id = ?", actor.ID, actor.OrgID).
		Where("jsonb_typeof(item -> 'operation_id') = ?", "string")
	return tx.Table("(?) AS source_operation_ids", gorm.Expr("(?) UNION (?) UNION (?)", promptSubmissions, quoteOperation, quoteItems))
}

func subjectCatalogue(tx *gorm.DB, actor, personal identity.Principal, library uuid.UUID) *gorm.DB {
	personalEntries := tx.Table("(?) AS entries", libraryEntries(tx, personal, domain.LibraryScope{Kind: domain.LibraryPersonal}, library)).
		Select("entries.asset_id AS id, ? AS source, NULL::uuid AS project_id, entries.revision, entries.favorite, false AS upscaled, entries.title, entries.kind, entries.create_time, entries.update_time, entries.asset_facts, concat_ws(' ', entries.title, entries.note, entries.source_label, entries.tags::text) AS search_text", application.SubjectSourcePersonalAsset).
		Where("entries.asset_id IS NOT NULL AND entries.catalog_state = ?", domain.LibraryItemStateActive).
		Where("entries.kind IN ?", []domain.Kind{domain.KindImage, domain.KindVideo, domain.KindAudio}).
		Where("entries.asset_facts ->> 'contains_real_person' = ? AND entries.asset_facts ->> 'consent_record_id' IS NULL", "false").
		Where("entries.asset_facts ->> 'sha256' ~ ?", "^[a-f0-9]{64}$").
		Where("(entries.asset_facts ->> 'byte_size')::bigint BETWEEN ? AND CASE entries.kind WHEN ? THEN ?::bigint WHEN ? THEN ?::bigint ELSE ?::bigint END", 1, domain.KindImage, application.MaxUploadImageBytes, domain.KindVideo, application.MaxUploadVideoBytes, application.MaxUploadAudioBytes)

	inputText := tx.Table("operation.operation_input AS input").
		Select("string_agg(input.text_value, ' ' ORDER BY input.seq_no)").
		Where("input.operation_id = o.id AND input.is_delete = ?", false)
	upscaledCapabilities := []string{
		operationdomain.CapabilityImageUpscale,
		operationdomain.CapabilityVideoUpscale,
		operationdomain.CapabilityImageSuperResolution,
		operationdomain.CapabilityVideoSuperResolution,
	}
	generatedOutputs := tx.Table("operation.operation AS o").
		Select("output.id AS id, ? AS source, p.id AS project_id, COALESCE(meta.revision, ?) AS revision, COALESCE(meta.favorite, false) AS favorite, (o.capability IN ? OR o.mode = ?) AS upscaled, LEFT(COALESCE(NULLIF(asset.file_name, ''), asset.kind), 240) AS title, asset.kind, output.create_time, GREATEST(output.update_time, asset.update_time) AS update_time, to_jsonb(asset) AS asset_facts, concat_ws(' ', asset.file_name, (?)) AS search_text", application.SubjectSourceGeneratedOutput, application.SubjectSourceRevisionInitial, upscaledCapabilities, operationdomain.ModeSuperResolution, inputText).
		Joins("JOIN workspace.project AS p ON p.id = o.project_id").
		Joins("JOIN operation.operation_output AS output ON output.operation_id = o.id AND output.project_id = o.project_id").
		Joins("JOIN media.media_asset AS asset ON asset.id = output.media_asset_id AND asset.project_id = o.project_id").
		Joins("LEFT JOIN operation.personal_history_record AS history ON history.operation_id = o.id AND history.actor_id = ? AND history.org_id = ? AND history.scope_id = ?", actor.ID, actor.OrgID, actor.ScopeID).
		Joins("LEFT JOIN operation.personal_history_output AS meta ON meta.operation_id = o.id AND meta.output_id = output.id AND meta.actor_id = ? AND meta.org_id = ? AND meta.scope_id = ?", actor.ID, actor.OrgID, actor.ScopeID).
		Joins("LEFT JOIN media.library AS project_library ON project_library.kind = ? AND project_library.project_id = p.id AND project_library.org_id = p.org_id", domain.LibraryProject).
		Joins("LEFT JOIN media.library_item AS catalog_item ON catalog_item.library_id = project_library.id AND catalog_item.asset_id = asset.id").
		Where("o.id::text IN (?)", subjectSourceOperations(tx, actor)).
		Where("p.org_id = ? AND p.owner_scope_id = ? AND p.is_delete = ? AND p.status IN ?", actor.OrgID, actor.ScopeID, false, []string{workspacedomain.ProjectStatusActive, workspacedomain.ProjectStatusArchived}).
		Where("o.is_delete = ? AND o.status = ? AND o.origin IN ?", false, operationdomain.StatusCompleted, []string{string(operationdomain.OriginPipeline), string(operationdomain.OriginBatch), string(operationdomain.OriginCanvas), string(operationdomain.OriginAgent)}).
		Where("output.kind = ? AND output.is_delete = ? AND output.moderation_status = ?", operationdomain.OutputKindMedia, false, domain.ModerationPassed).
		Where("history.deleted_at IS NULL AND meta.deleted_at IS NULL").
		Where("asset.kind IN ? AND asset.personal_org_id IS NULL AND asset.personal_actor_id IS NULL", []domain.Kind{domain.KindImage, domain.KindVideo, domain.KindAudio}).
		Where("asset.is_delete = ? AND asset.status = ? AND asset.moderation_status = ? AND asset.contains_real_person = ? AND asset.consent_record_id IS NULL", false, domain.StatusReady, domain.ModerationPassed, false).
		Where("asset.sha256 ~ ?", "^[a-f0-9]{64}$").
		Where("(asset.kind = ? AND asset.byte_size BETWEEN ? AND ?) OR (asset.kind = ? AND asset.byte_size BETWEEN ? AND ?) OR (asset.kind = ? AND asset.byte_size BETWEEN ? AND ?)", domain.KindImage, 1, application.MaxUploadImageBytes, domain.KindVideo, 1, application.MaxUploadVideoBytes, domain.KindAudio, 1, application.MaxUploadAudioBytes).
		Where("COALESCE(catalog_item.catalog_state, ?) = ? AND catalog_item.purged_at IS NULL", domain.LibraryItemStateActive, domain.LibraryItemStateActive)
	activePurgeStatuses := []domain.PurgeItemStatus{domain.PurgeItemStatusQueued, domain.PurgeItemStatusRunning, domain.PurgeItemStatusNeedsReconciliation, domain.PurgeItemStatusSucceeded}
	purges := tx.Model(&purgeItemRow{}).Select("item_id").Where("item_id = asset.id AND status IN ?", activePurgeStatuses)
	generatedOutputs = generatedOutputs.Where("NOT EXISTS (?)", purges)
	return tx.Table("(?) AS catalogue", gorm.Expr("(?) UNION ALL (?)", personalEntries, generatedOutputs))
}

func filterSubjectCatalogue(query *gorm.DB, input application.SubjectCatalogueQuery) *gorm.DB {
	query = query.Where("kind = ?", input.Kind)
	if input.FavoriteOnly {
		query = query.Where("favorite = ?", true)
	}
	if input.Quality == application.SubjectSourceQualityUpscaled {
		query = query.Where("upscaled = ?", true)
	}
	if input.Search != "" {
		query = query.Where("strpos(lower(search_text), lower(?)) > ?", input.Search, 0)
	}
	return query
}

func (s *SubjectCatalogueStore) read(ctx context.Context, actor identity.Principal, consume func(*gorm.DB, identity.Principal, uuid.UUID) error) error {
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
		return consume(tx, personal, library.ID)
	})
}

func (s *SubjectCatalogueStore) projects(ctx context.Context, tx, query *gorm.DB, actor identity.Principal) error {
	var projects []uuid.UUID
	if err := query.Where("project_id IS NOT NULL").Distinct("project_id").Order("project_id").Pluck("project_id", &projects).Error; err != nil {
		return fmt.Errorf("read subject source projects: %w", err)
	}
	for _, project := range projects {
		if _, err := s.library.authorize(ctx, tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false); err != nil {
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
	err := s.read(ctx, actor, func(tx *gorm.DB, personal identity.Principal, library uuid.UUID) error {
		page = application.SubjectSourcePage{CurrentActorID: actor.ID, CurrentScopeID: actor.ScopeID, Items: []application.SubjectSourceItem{}, Page: input.Page, PageSize: input.PageSize, Capabilities: application.SubjectSourceCapabilities{UpscaledFilter: true}}
		if err := s.projects(ctx, tx, filterSubjectCatalogue(subjectCatalogue(tx, actor, personal, library), input), actor); err != nil {
			return err
		}
		var collisions []struct{ ID uuid.UUID }
		if err := subjectCatalogue(tx, actor, personal, library).Select("id").Group("id").Having("COUNT(*) > ?", 1).Limit(1).Find(&collisions).Error; err != nil {
			return fmt.Errorf("check subject source identities: %w", err)
		}
		if len(collisions) != 0 {
			return application.ErrUnavailable
		}
		if err := filterSubjectCatalogue(subjectCatalogue(tx, actor, personal, library), input).Count(&page.Total).Error; err != nil {
			return fmt.Errorf("count subject source catalogue: %w", err)
		}
		var rows []subjectCatalogueRow
		if err := filterSubjectCatalogue(subjectCatalogue(tx, actor, personal, library), input).
			Order("update_time DESC").Order("id ASC").Limit(input.PageSize).
			Offset((input.Page - 1) * input.PageSize).Find(&rows).Error; err != nil {
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
	err := s.read(ctx, actor, func(tx *gorm.DB, personal identity.Principal, library uuid.UUID) error {
		var rows []subjectCatalogueRow
		if err := subjectCatalogue(tx, actor, personal, library).Where("id = ?", id).Limit(2).Find(&rows).Error; err != nil {
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
// Catalogue metadata IDs are not media identities. This reader is composed
// only behind SubjectLibrary's source proof and does not
// alter ordinary media routes or infer generation ownership from project access.
func (s *SubjectCatalogueStore) FindLibraryMedia(ctx context.Context, actor identity.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryMediaFile, error) {
	if s == nil || s.library == nil || id == uuid.Nil {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var file application.LibraryMediaFile
	err := s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		var rows []libraryEntryRow
		if err := libraryEntries(tx, actor, scope, library.ID).
			Where("asset_id = ? AND catalog_state = ?", id, domain.LibraryItemStateActive).
			Limit(2).Find(&rows).Error; err != nil {
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
