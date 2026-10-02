package app

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	pgmediatool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
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

func provideMediaStorageUsage(database *gorm.DB, storage *objectstorage.Client) *mediaapp.StorageUsageReader {
	source := pgmedia.NewStorageUsageStore(database, provideMediaLibraryProjectAccess, func(tx *gorm.DB) mediaapp.StorageUsageIntentSource {
		return mediaStorageIntents{
			tools:    pgmediatool.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx))),
			packages: provideMediaPackageStore(tx),
		}
	})
	return mediaapp.NewStorageUsageReader(source, mediaobjects.NewStorageUsageObjects(storage), time.Now)
}
