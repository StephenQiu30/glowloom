package app

import (
	"fmt"

	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/videodepth"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func provideMediaToolSource(tx *gorm.DB) toolapp.TranscriptionSourceReader {
	return canvasapp.NewTranscriptionSourceReader(pgcanvas.NewStore(tx,
		func(mediaTx *gorm.DB) canvasapp.MediaReader {
			return mediaapp.NewAssetQuery(pgmedia.NewStore(mediaTx), nil)
		}))
}

// provideMediaDepthStore retains read, cancellation and exact reconciliation
// while unconfigured inference refuses new creation and retry before writes.
func provideMediaDepthStore(database *gorm.DB, enabled bool) *pgtool.DepthStore {
	var source pgtool.TranscriptionSourceFactory
	if enabled {
		source = provideMediaToolSource
	}
	return pgtool.NewDepthStore(database, source,
		func(tx *gorm.DB) toolapp.DepthDerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewStore(tx))
		})
}

// provideMediaDepthWorker owns one Runner for every native job on this media
// worker, so concurrent Activities share the same physical inference slot.
func provideMediaDepthWorker(database *gorm.DB, storage *objectstorage.Client, cfg config.Config) (*toolapp.DepthWorker, *pgtool.DepthStore, error) {
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
	store := provideMediaDepthStore(database, processor != nil)
	consumer := toolapp.NewDepthWorker(store, toolflow.NewObjects(storage), processor, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, verifier)
	return consumer, store, nil
}
