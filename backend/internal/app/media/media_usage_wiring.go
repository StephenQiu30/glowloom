package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgmediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type mediaStorageIntents struct {
	tools    *pgmediatool.RetainedMediaReader
	packages mediaapp.StorageUsageIntentSource
}

func (s mediaStorageIntents) StorageUsageKeys(ctx context.Context, actor identityapp.Principal, scope mediadomain.LibraryScope) ([]string, error) {
	if scope.Validate() != nil || s.tools == nil || s.packages == nil {
		return nil, mediaapp.ErrUnavailable
	}
	packageKeys, err := s.packages.StorageUsageKeys(ctx, actor, scope)
	if err != nil {
		return nil, err
	}
	var toolKeys []string
	if scope.Kind == mediadomain.LibraryProject {
		// Installed media tools only accept project sources. Personal package
		// archives and output intents still require their own authorized reader.
		toolKeys, err = s.tools.StorageUsageKeys(ctx, actor, *scope.ProjectID)
		if err != nil {
			return nil, err
		}
	}
	unique := make(map[string]bool)
	for _, keys := range [][]string{packageKeys, toolKeys} {
		for _, key := range keys {
			unique[key] = true
			if len(unique) > mediaapp.MaxStorageUsageObjects {
				return nil, mediaapp.ErrUnavailable
			}
		}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// ProvideMediaStorageUsage assembles the storage usage reader.
func ProvideMediaStorageUsage(database *gorm.DB, storage *objectstorage.Client) *mediaapp.StorageUsageReader {
	source := pgmedia.NewScopedStorageUsageStore(database, NewMediaLibraryProjectAccess, func(tx *gorm.DB) mediaapp.StorageUsageIntentSource {
		return mediaStorageIntents{
			tools:    pgmediatool.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, NewMediaScopeAccess))),
			packages: provideMediaPackageStore(tx),
		}
	}, NewMediaScopeAccess)
	return mediaapp.NewStorageUsageReader(source, mediaobjects.NewStorageUsageObjects(storage), time.Now)
}

// ProvideRoutedMediaStorageUsage accounts for each configured readable resource
// while the media owner retains current scope authority throughout inventory I/O.
func ProvideRoutedMediaStorageUsage(database *gorm.DB, registry *objectstorage.Registry, cfg *config.ObjectStorageConfig) *mediaapp.StorageUsageReader {
	if cfg == nil || registry == nil {
		return mediaapp.NewRoutedStorageUsageReader(nil, nil, time.Now)
	}
	validated, err := cfg.Validated()
	if err != nil {
		return mediaapp.NewRoutedStorageUsageReader(nil, nil, time.Now)
	}
	buckets := make([]mediaapp.StorageUsageBucket, 0, len(validated.Profiles))
	ids := make([]string, 0, len(validated.Profiles))
	for id := range validated.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := validated.Profiles[id]
		if p.Mode == "disabled" {
			continue
		}
		sum := sha256.Sum256([]byte(p.ResourceIdentity()))
		buckets = append(buckets, mediaapp.StorageUsageBucket{ProfileID: id, Bucket: p.Bucket, ResourceIdentity: hex.EncodeToString(sum[:])})
	}
	library := pgmedia.NewScopedLibraryStore(database, NewMediaLibraryProjectAccess, time.Now, NewMediaScopeAccess)
	source := pgmedia.NewRoutedStorageUsageStore(library, buckets, func(tx *gorm.DB) mediaapp.StorageUsageLocationSource {
		return pgmedia.NewScopedLibraryStore(tx, NewMediaLibraryProjectAccess, time.Now, NewMediaScopeAccess)
	})
	return mediaapp.NewRoutedStorageUsageReader(source, mediaobjects.NewRoutedStorageUsageObjects(registry), time.Now)
}
