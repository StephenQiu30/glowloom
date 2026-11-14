// Package script wires script sources, imports, and project authority.
package script

import (
	"time"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

var characterReferences func(*gorm.DB) scriptapp.CharacterReferences

// BindCharacterReferences installs bible character lookup. Script does not import the bible assembler.
func BindCharacterReferences(fn func(*gorm.DB) scriptapp.CharacterReferences) {
	characterReferences = fn
}

// ProvideScriptSourceStore assembles script sources and their reference dependencies.
func ProvideScriptSourceStore(database *gorm.DB) *pgscript.SourceStore {
	if characterReferences == nil {
		panic("script character references are not bound")
	}
	return pgscript.NewSourceStoreWithCharacters(database, ProvideScriptProjectAccess, characterReferences).WithPublicationAuthorization(ProvideSourcePublicationAuthorization)
}

// ProvideScriptProjectAccess adapts project authority for scripts.
func ProvideScriptProjectAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return scopewire.ProvideProjectContentAccess(tx)
}

// ProvideScriptImportStore assembles script import storage.
func ProvideScriptImportStore(database *gorm.DB) *pgscript.ImportStore {
	return pgscript.NewImportStoreWithWorkerAccess(database, ProvideScriptProjectAccess, provideScriptWorkerAccess, func(tx *gorm.DB) scriptapp.SourceAssetReader {
		// Admission freezes media facts in this transaction, without object I/O.
		return mediaapp.NewRoutedDocumentSources(pgmedia.NewScopedDocumentSourceStore(tx, mediawire.NewMediaScopeAccess), nil)
	})
}

// Services are the script HTTP handlers composed for one process.
type Services struct {
	Sources  *scriptapp.SourceService
	Recovery *scriptapp.SourceRecovery
	Episodes *scriptapp.EpisodeService
	History  *scriptapp.HistoryService
	Adopt    *scriptapp.AdoptService
	Imports  *scriptapp.ImportService
}

// ProvideServicesWithObjects binds script publication to formal fixed locations.
func ProvideServicesWithObjects(database *gorm.DB, registry *objectstorage.Registry) Services {
	store := ProvideScriptSourceStore(database)
	store = store.WithObjectLocations(ProvideScriptObjectLocations)
	objects := scriptobjects.NewRoutedStorage(registry, pgmedia.NewStorageRoutes(database))
	sources := scriptapp.NewSourceService(store, objects, time.Now)
	// Recovery joins the same synchronous I/O owner before unpublished cleanup.
	return Services{Sources: sources, Recovery: scriptapp.NewSourceRecovery(store, sources, time.Now), Episodes: scriptapp.NewEpisodeService(store, sources, time.Now), History: scriptapp.NewHistoryService(store, sources), Adopt: scriptapp.NewAdoptService(store, time.Now), Imports: scriptapp.NewImportService(ProvideScriptImportStore(database), time.Now)}
}

// ProvideScriptImportWorkerWithObjects shares exact source/history location ownership.
func ProvideScriptImportWorkerWithObjects(database *gorm.DB, registry *objectstorage.Registry, extractor scriptapp.SourceExtractor) (*scriptapp.ImportWorker, *pgscript.ImportStore) {
	store := ProvideScriptImportStore(database)
	objects := scriptobjects.NewRoutedStorage(registry, pgmedia.NewStorageRoutes(database))
	services := ProvideServicesWithObjects(database, registry)
	publication := func(authority scriptapp.ImportAuthority) *scriptapp.SourceService {
		sourceStore := pgscript.NewImportSourceStore(database, provideScriptWorkerAccess, authority)
		sourceStore = sourceStore.WithObjectLocations(ProvideScriptObjectLocations).WithPublicationAuthorization(ProvideSourcePublicationAuthorization)
		return scriptapp.NewSourceServiceSharingIO(sourceStore, objects, time.Now, services.Sources)
	}
	recoveryStore := pgscript.NewImportRecoveryStore(database, provideScriptWorkerAccess)
	recoveryStore = recoveryStore.WithObjectLocations(ProvideScriptObjectLocations).WithPublicationAuthorization(ProvideSourcePublicationAuthorization)
	recovery := scriptapp.NewSourceRecovery(recoveryStore, services.Sources, time.Now)
	// One worker owns extraction and publication I/O until the synchronous
	// publication call and its durable exit proof both finish. Recovery runs
	// after that join and checks the source owner's persistent I/O facts.
	documents := mediaapp.NewRoutedDocumentSources(pgmedia.NewScopedDocumentSourceStore(database, mediawire.NewMediaWorkerScopeAccess), mediaobjects.NewRoutedReadObjects(registry))
	worker := scriptapp.NewImportWorker(store, documents, extractor, publication, recovery, time.Now)
	return worker, store
}

func provideScriptWorkerAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return scopewire.ProvideWorkerProjectContentAccess(tx)
}
