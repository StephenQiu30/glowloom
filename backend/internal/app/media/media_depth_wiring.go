package media

import (
	"fmt"

	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/videodepth"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func provideMediaToolSourceWithAccess(tx *gorm.DB, scopes pgmedia.ScopeAccessFactory) toolapp.TranscriptionSourceReader {
	return canvasapp.NewTranscriptionSourceReader(pgcanvas.NewScopedStore(tx,
		func(mediaTx *gorm.DB) canvasapp.MediaReader {
			return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(mediaTx, scopes), nil)
		}, func(tx *gorm.DB) canvasapp.ProjectAccess { return canvasScopedAccess{ScopeAccess: scopes(tx), tx: tx} }))
}

// ProvideMediaDepthStore retains read, cancellation and exact reconciliation
// while unconfigured inference refuses new creation and retry before writes.
func ProvideMediaDepthStore(database *gorm.DB, enabled bool) *pgtool.DepthStore {
	return ProvideMediaDepthStoreWithAccess(database, enabled, NewMediaScopeAccess)
}

// ProvideMediaDepthStoreWithAccess assembles the depth store with explicit scope access.
func ProvideMediaDepthStoreWithAccess(database *gorm.DB, enabled bool, scopes pgmedia.ScopeAccessFactory) *pgtool.DepthStore {
	var source pgtool.TranscriptionSourceFactory
	if enabled {
		source = func(tx *gorm.DB) toolapp.TranscriptionSourceReader {
			return provideMediaToolSourceWithAccess(tx, scopes)
		}
	}
	return pgtool.NewDepthStore(database, source,
		func(tx *gorm.DB) toolapp.DepthDerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
		})
}

// ProvideRoutedMediaDepthStore freezes physical output identity at admission.
func ProvideRoutedMediaDepthStore(database *gorm.DB, enabled bool, scopes pgmedia.ScopeAccessFactory) *pgtool.DepthStore {
	return ProvideMediaDepthStoreWithAccess(database, enabled, scopes).WithStorageAdmission(func(tx *gorm.DB) toolapp.ToolStorageLedger {
		return pgmedia.NewStorageRoutes(tx)
	})
}

// ProvideMediaDepthWorker owns one Runner for every native job on this media
// worker, so concurrent Activities share the same physical inference slot.
func ProvideMediaDepthWorker(database *gorm.DB, storage *objectstorage.Client, cfg config.Config) (*toolapp.DepthWorker, *pgtool.DepthStore, error) {
	return provideMediaDepthWorker(database, nil, storage, cfg)
}

// ProvideMediaDepthWorkerRouted retains one native inference slot while each
// durable attempt opens a separately pinned physical object view.
func ProvideMediaDepthWorkerRouted(database *gorm.DB, registry *objectstorage.Registry, legacy *objectstorage.Client, cfg config.Config) (*toolapp.DepthWorker, *pgtool.DepthStore, error) {
	return provideMediaDepthWorker(database, registry, legacy, cfg)
}

func provideMediaDepthWorker(database *gorm.DB, registry *objectstorage.Registry, storage *objectstorage.Client, cfg config.Config) (*toolapp.DepthWorker, *pgtool.DepthStore, error) {
	verifier, err := videodepth.NewPreprocessor("ffmpeg", "ffprobe")
	if err != nil {
		return nil, nil, fmt.Errorf("configure video depth verification: %w", err)
	}
	var processor toolapp.DepthProcessor
	if cfg.VideoDepthPythonPath != "" {
		processor, err = videodepth.NewRunner(videodepth.Config{
			PythonPath: cfg.VideoDepthPythonPath, SourceDir: cfg.VideoDepthSourceDir,
			ModelPath: cfg.VideoDepthModelPath, Device: cfg.VideoDepthDevice,
			FFmpegPath: "ffmpeg", FFprobePath: "ffprobe",
		}, mediaflow.FFProber{})
		if err != nil {
			return nil, nil, fmt.Errorf("configure offline video depth inference: %w", err)
		}
	}
	store := ProvideMediaDepthStoreWithAccess(database, processor != nil, NewMediaWorkerScopeAccess)
	if registry != nil {
		store = ProvideRoutedMediaDepthStore(database, processor != nil, NewMediaWorkerScopeAccess)
		consumer := toolapp.NewRoutedDepthWorker(store, ToolObjects(database, registry, storage), processor, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, verifier)
		return consumer, store, nil
	}
	consumer := toolapp.NewDepthWorker(store, toolflow.NewObjects(storage), processor, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, verifier)
	return consumer, store, nil
}
