package postgres

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// StorageUsageIntentFactory binds installed owning intent readers to the caller transaction.
type StorageUsageIntentFactory func(*gorm.DB) application.StorageUsageIntentSource

// StorageUsageStore reads media-owned keys without querying foreign execution tables.
type StorageUsageStore struct {
	library *LibraryStore
	intents StorageUsageIntentFactory
}

// NewStorageUsageStore requires the installed intent aggregate. Missing wiring
// fails closed, including a legitimately empty library with unresolved work.
func NewStorageUsageStore(db *gorm.DB, project LibraryProjectAccessFactory, intents StorageUsageIntentFactory) *StorageUsageStore {
	return &StorageUsageStore{library: NewLibraryStore(db, project, nil), intents: intents}
}

// InspectStorageUsage holds actor/project/library read locks during private reads.
func (s *StorageUsageStore) InspectStorageUsage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, consume func(application.StorageUsageInventory) error) error {
	if s == nil || s.library == nil || s.intents == nil || consume == nil {
		return application.ErrUnavailable
	}
	return s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		keys, err := storageUsageMediaKeys(tx, actor, scope)
		if err != nil {
			return err
		}
		intents := s.intents(tx)
		if intents == nil {
			return application.ErrUnavailable
		}
		extra, err := intents.StorageUsageKeys(ctx, actor, scope)
		if err != nil {
			return err
		}
		if len(extra) > application.MaxStorageUsageObjects || len(keys)+len(extra) > 2*application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
		unique := make(map[string]bool, len(keys)+len(extra))
		for _, key := range append(keys, extra...) {
			unique[key] = true
		}
		if len(unique) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
		keys = make([]string, 0, len(unique))
		for key := range unique {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
		if scope.Kind == domain.LibraryProject {
			base = "projects/" + scope.ProjectID.String() + "/"
		}
		prefixes := make([]string, 0, 5)
		for _, kind := range []string{"image", "video", "audio", "model", "document"} {
			prefixes = append(prefixes, base+kind+"/")
		}
		return consume(application.StorageUsageInventory{LibraryID: library.ID, Prefixes: prefixes, ObjectKeys: keys})
	})
}

func storageUsageMediaKeys(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope) ([]string, error) {
	var keys []string
	condition := "a.project_id=? AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL"
	var args []any
	if scope.Kind == domain.LibraryProject {
		args = []any{*scope.ProjectID, *scope.ProjectID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID}
	} else {
		condition = "a.project_id IS NULL AND a.personal_org_id=? AND a.personal_actor_id=?"
		args = []any{actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID}
	}
	query := `SELECT object_key FROM (
	 SELECT a.object_key FROM media.media_asset a WHERE ` + condition + `
	 UNION SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE ` + condition
	if scope.Kind == domain.LibraryProject {
		query += ` UNION SELECT o.source_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.source_project_id=? AND j.org_id=?
		 UNION SELECT o.target_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.target_project_id=? AND j.org_id=?
		 UNION SELECT o.source_object_key FROM media.project_copy_object o JOIN media.project_copy_snapshot s ON s.id=o.snapshot_id WHERE s.source_project_id=? AND s.org_id=?
		 UNION SELECT o.target_object_key FROM media.project_copy_object o JOIN media.project_copy_snapshot s ON s.id=o.snapshot_id WHERE s.target_project_id=? AND s.org_id=?`
	} else {
		query += ` UNION SELECT o.source_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.source_kind='personal' AND j.org_id=? AND j.actor_id=?
		 UNION SELECT o.target_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.target_kind='personal' AND j.org_id=? AND j.actor_id=?`
	}
	query += `) held ORDER BY object_key LIMIT 50001`
	if err := tx.Raw(query, args...).Scan(&keys).Error; err != nil {
		return nil, fmt.Errorf("read media-owned usage keys: %w", err)
	}
	if len(keys) > application.MaxStorageUsageObjects {
		return nil, application.ErrUnavailable
	}
	return keys, nil
}
