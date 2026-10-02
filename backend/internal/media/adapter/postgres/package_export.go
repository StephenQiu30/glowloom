package postgres

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// This internal complete-export query includes retained trash only. It does not
// change the ordinary list, preview, document or reference authorization paths.
func packageExportEntries(actor identityapp.Principal, scope domain.LibraryScope, library uuid.UUID) (string, []any) {
	ownership := `a.project_id=? AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL`
	args := []any{library, scope.ProjectID, library}
	if scope.Kind == domain.LibraryPersonal {
		ownership = `a.project_id IS NULL AND a.personal_org_id=? AND a.personal_actor_id=?`
		args = []any{library, actor.OrgID, actor.ID, library}
	}
	return `WITH entries AS (
 SELECT COALESCE(i.id,a.id) AS id,a.id AS asset_id,i.folder_id,NULL::text AS plain_text,a.kind,
 LEFT(COALESCE(i.title,NULLIF(a.file_name,''),a.kind),240) AS title,COALESCE(i.category,'material') AS category,
 COALESCE(i.tags,'[]'::jsonb) AS tags,COALESCE(i.source_label,'') AS source_label,COALESCE(i.note,'') AS note,
 COALESCE(i.favorite,false) AS favorite,COALESCE(i.catalog_state,'active') AS catalog_state,i.trashed_at,
 COALESCE(i.position,0) AS position,COALESCE(i.revision,0) AS revision,
 COALESCE(i.create_time,a.create_time) AS create_time,COALESCE(i.update_time,a.update_time) AS update_time,
 to_jsonb(a) AS asset_facts
 FROM media.media_asset a LEFT JOIN media.library_item i ON i.library_id=? AND i.asset_id=a.id
 WHERE ` + ownership + ` AND i.purged_at IS NULL AND
 ((NOT a.is_delete AND a.status='ready' AND a.moderation_status='passed') OR i.id IS NOT NULL)
 UNION ALL
 SELECT i.id,NULL::uuid,i.folder_id,i.plain_text,'text',i.title,i.category,i.tags,i.source_label,i.note,
 i.favorite,i.catalog_state,i.trashed_at,i.position,i.revision,i.create_time,i.update_time,NULL::jsonb
 FROM media.library_item i WHERE i.library_id=? AND i.asset_id IS NULL AND i.purged_at IS NULL) `, args
}

func decodePackageExportAsset(raw []byte, state string) (domain.MediaAsset, error) {
	var wire struct {
		assetRow
		ModerationDetail json.RawMessage `json:"moderation_detail"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF {
		return domain.MediaAsset{}, application.ErrUnavailable
	}
	wire.assetRow.ModerationDetail = wire.ModerationDetail
	a := wire.domain()
	if a.Validate() != nil || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.IsDelete && state != "trashed" {
		return domain.MediaAsset{}, application.ErrPackageIncomplete
	}
	return a, nil
}

func packageExportDetail(r libraryEntryRow, library uuid.UUID) (application.LibraryItemDetail, error) {
	if r.AssetID == nil {
		return r.detail(library)
	}
	a, err := decodePackageExportAsset(r.AssetFacts, r.CatalogState)
	if err != nil || a.ID != *r.AssetID || r.ID != a.ID || string(a.Kind) != r.Kind {
		return application.LibraryItemDetail{}, application.ErrPackageIncomplete
	}
	var tags []string
	if json.Unmarshal(r.Tags, &tags) != nil {
		return application.LibraryItemDetail{}, application.ErrUnavailable
	}
	item := domain.LibraryItem{ID: r.ID, LibraryID: library, AssetID: r.AssetID, FolderID: r.FolderID, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}
	if item.Revision == 0 {
		item.Revision = 1
	}
	if item.Validate() != nil {
		return application.LibraryItemDetail{}, application.ErrPackageIncomplete
	}
	return application.LibraryItemDetail{LibraryItemSummary: application.LibraryItemSummary{ID: r.ID, AssetID: r.AssetID, FolderID: r.FolderID, Kind: r.Kind, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}}, nil
}

func packageExportFileName(a domain.MediaAsset) (string, error) {
	if a.FileName != "" {
		name, err := application.SafeUploadFileName(a.FileName)
		if err != nil || name != a.FileName {
			return "", application.ErrPackageIncomplete
		}
		return name, nil
	}
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "video/mp4": ".mp4", "video/quicktime": ".mov", "audio/mpeg": ".mp3", "audio/wave": ".wav", "audio/mp4": ".m4a"}
	ext, ok := extensions[a.MimeType]
	if !ok {
		return "", application.ErrPackageIncomplete
	}
	return string(a.Kind) + ext, nil
}
