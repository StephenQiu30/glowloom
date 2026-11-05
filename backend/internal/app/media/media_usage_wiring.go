package media

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// ProvideRoutedMediaStorageUsage accounts for each configured readable resource
// while the media owner retains current scope authority throughout inventory I/O.
func ProvideRoutedMediaStorageUsage(database *gorm.DB, registry *objectstorage.Registry, cfg *config.ObjectStorageConfig) *mediaapp.StorageUsageReader {
	if cfg == nil || registry == nil {
		return mediaapp.NewStorageUsageReader(nil, nil, time.Now)
	}
	validated, err := cfg.Validated()
	if err != nil {
		return mediaapp.NewStorageUsageReader(nil, nil, time.Now)
	}
	buckets := make([]mediaapp.StorageUsageBucket, 0, len(validated.Profiles))
	ids := make([]string, 0, len(validated.Profiles))
	for id := range validated.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := validated.Profiles[id]
		if p.Mode == config.StorageProfileDisabled {
			continue
		}
		sum := sha256.Sum256([]byte(p.ResourceIdentity()))
		buckets = append(buckets, mediaapp.StorageUsageBucket{ProfileID: id, Bucket: p.Bucket, ResourceIdentity: hex.EncodeToString(sum[:])})
	}
	library := pgmedia.NewScopedLibraryStore(database, NewMediaLibraryProjectAccess, time.Now, NewMediaScopeAccess)
	source := pgmedia.NewRoutedStorageUsageStore(library, buckets, func(tx *gorm.DB) mediaapp.StorageUsageLocationSource {
		return pgmedia.NewScopedLibraryStore(tx, NewMediaLibraryProjectAccess, time.Now, NewMediaScopeAccess)
	})
	return mediaapp.NewStorageUsageReader(source, mediaobjects.NewRoutedStorageUsageObjects(registry), time.Now)
}
