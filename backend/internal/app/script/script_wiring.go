package script

import (
	"time"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptextract "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

var characterReferences func(*gorm.DB) scriptapp.CharacterReferences

// BindCharacterReferences installs bible character lookup. Script does not import the bible assembler.
func BindCharacterReferences(fn func(*gorm.DB) scriptapp.CharacterReferences) {
	characterReferences = fn
}

func ProvideScriptSourceStore(database *gorm.DB) *pgscript.SourceStore {
	if characterReferences == nil {
		panic("script character references are not bound")
	}
	return pgscript.NewSourceStoreWithCharacters(database, ProvideScriptProjectAccess, characterReferences)
}

func ProvideScriptProjectAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return scopewire.ProvideProjectContentAccess(tx)
}

func ProvideScriptImportStore(database *gorm.DB) *pgscript.ImportStore {
	return pgscript.NewImportStoreWithWorkerAccess(database, ProvideScriptProjectAccess, provideScriptWorkerAccess, func(tx *gorm.DB) scriptapp.SourceAssetReader {
		// Admission freezes media facts in this transaction, without object I/O.
		return mediaapp.NewDocumentSources(pgmedia.NewScopedDocumentSourceStore(tx, mediawire.NewMediaScopeAccess), nil)
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

func ProvideServices(database *gorm.DB, storage *objectstorage.Client) Services {
	store := ProvideScriptSourceStore(database)
	sources := scriptapp.NewSourceService(store, scriptobjects.NewStorage(storage), time.Now)
	// Recovery joins the same synchronous I/O owner before unpublished cleanup.
	return Services{Sources: sources, Recovery: scriptapp.NewSourceRecovery(store, sources, time.Now), Episodes: scriptapp.NewEpisodeService(store, sources, time.Now), History: scriptapp.NewHistoryService(store, sources), Adopt: scriptapp.NewAdoptService(store, time.Now), Imports: scriptapp.NewImportService(ProvideScriptImportStore(database), time.Now)}
}

func ProvideScriptImportWorker(database *gorm.DB, storage *objectstorage.Client) (*scriptapp.ImportWorker, *pgscript.ImportStore) {
	store := ProvideScriptImportStore(database)
	objects := scriptobjects.NewStorage(storage)
	services := ProvideServices(database, storage)
	publication := func(authority scriptapp.ImportAuthority) *scriptapp.SourceService {
		return scriptapp.NewSourceServiceSharingIO(pgscript.NewImportSourceStore(database, provideScriptWorkerAccess, authority), objects, time.Now, services.Sources)
	}
	recovery := scriptapp.NewSourceRecovery(pgscript.NewImportRecoveryStore(database, provideScriptWorkerAccess), services.Sources, time.Now)
	// One worker owns extraction and publication I/O until the synchronous
	// publication call and its durable exit proof both finish. Recovery runs
	// after that join and checks the source owner's persistent I/O facts.
	worker := scriptapp.NewImportWorker(store, mediaapp.NewDocumentSources(pgmedia.NewScopedDocumentSourceStore(database, mediawire.NewMediaWorkerScopeAccess), storage), scriptextract.NewExtractor(), publication, recovery, time.Now)
	return worker, store
}

func provideScriptWorkerAccess(tx *gorm.DB) scriptapp.ProjectAccess {
	return scopewire.ProvideWorkerProjectContentAccess(tx)
}
