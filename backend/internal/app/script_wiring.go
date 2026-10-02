package app

import (
	"time"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scriptextract "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	pgscript "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func provideScriptSourceStore(database *gorm.DB) *pgscript.SourceStore {
	return pgscript.NewSourceStore(database, provideScriptProjectAccess)
}

func provideScriptProjectAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return pgworkspace.NewProjectContentAccessStore(tx)
}

func provideScriptImportStore(database *gorm.DB) *pgscript.ImportStore {
	return pgscript.NewImportStore(database, provideScriptProjectAccess, func(tx *gorm.DB) scriptapp.SourceAssetReader {
		// Admission freezes media facts in this transaction, without object I/O.
		return mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(tx), nil)
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
		return scriptapp.NewSourceServiceSharingIO(pgscript.NewImportSourceStore(database, provideScriptProjectAccess, authority), objects, time.Now, services.sources)
	}
	recovery := scriptapp.NewSourceRecovery(pgscript.NewImportRecoveryStore(database, provideScriptProjectAccess), services.sources, time.Now)
	// One worker owns extraction and publication I/O until the synchronous
	// publication call and its durable exit proof both finish. Recovery runs
	// after that join and checks the source owner's persistent I/O facts.
	worker := scriptapp.NewImportWorker(store, mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(database), storage), scriptextract.NewExtractor(), publication, recovery, time.Now)
	return worker, store
}
