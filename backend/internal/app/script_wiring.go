package app

import (
	"time"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptextract "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func provideScriptSourceStore(database *gorm.DB) *pgscript.SourceStore {
	return pgscript.NewSourceStoreWithCharacters(database, provideScriptProjectAccess, provideScriptCharacterReferences)
}

func provideScriptProjectAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return provideProjectContentAccess(tx)
}

func provideScriptImportStore(database *gorm.DB) *pgscript.ImportStore {
	return pgscript.NewImportStoreWithWorkerAccess(database, provideScriptProjectAccess, provideScriptWorkerAccess, func(tx *gorm.DB) scriptapp.SourceAssetReader {
		// Admission freezes media facts in this transaction, without object I/O.
		return mediaapp.NewDocumentSources(pgmedia.NewScopedDocumentSourceStore(tx, NewMediaScopeAccess), nil)
	})
}

type scriptServices struct {
	sources  *scriptapp.SourceService
	recovery *scriptapp.SourceRecovery
	episodes *scriptapp.EpisodeService
	history  *scriptapp.HistoryService
	adopt    *scriptapp.AdoptService
	imports  *scriptapp.ImportService
}

func provideScriptServices(database *gorm.DB, storage *objectstorage.Client) scriptServices {
	store := provideScriptSourceStore(database)
	sources := scriptapp.NewSourceService(store, scriptobjects.NewStorage(storage), time.Now)
	// Recovery joins the same synchronous I/O owner before unpublished cleanup.
	return scriptServices{sources: sources, recovery: scriptapp.NewSourceRecovery(store, sources, time.Now), episodes: scriptapp.NewEpisodeService(store, sources, time.Now), history: scriptapp.NewHistoryService(store, sources), adopt: scriptapp.NewAdoptService(store, time.Now), imports: scriptapp.NewImportService(provideScriptImportStore(database), time.Now)}
}

func provideScriptImportWorker(database *gorm.DB, storage *objectstorage.Client) (*scriptapp.ImportWorker, *pgscript.ImportStore) {
	store := provideScriptImportStore(database)
	objects := scriptobjects.NewStorage(storage)
	services := provideScriptServices(database, storage)
	publication := func(authority scriptapp.ImportAuthority) *scriptapp.SourceService {
		return scriptapp.NewSourceServiceSharingIO(pgscript.NewImportSourceStore(database, provideScriptWorkerAccess, authority), objects, time.Now, services.sources)
	}
	recovery := scriptapp.NewSourceRecovery(pgscript.NewImportRecoveryStore(database, provideScriptWorkerAccess), services.sources, time.Now)
	// One worker owns extraction and publication I/O until the synchronous
	// publication call and its durable exit proof both finish. Recovery runs
	// after that join and checks the source owner's persistent I/O facts.
	worker := scriptapp.NewImportWorker(store, mediaapp.NewDocumentSources(pgmedia.NewScopedDocumentSourceStore(database, NewMediaWorkerScopeAccess), storage), scriptextract.NewExtractor(), publication, recovery, time.Now)
	return worker, store
}

func provideScriptWorkerAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return provideWorkerProjectContentAccess(tx)
}
