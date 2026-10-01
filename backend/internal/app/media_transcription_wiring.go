package app

import (
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/whisper"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

// provideMediaTranscriptionStore retains query and cancel access when native
// inference is disabled, while refusing new commands before any side effect.
func provideMediaTranscriptionStore(database *gorm.DB, enabled bool) *pgtool.TranscriptionStore {
	var source pgtool.TranscriptionSourceFactory
	if enabled {
		source = provideMediaToolSource
	}
	return pgtool.NewTranscriptionStore(database, source,
		func(tx *gorm.DB) toolapp.DerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewStore(tx))
		})
}

func provideTranscriber(cfg config.Config) (toolapp.Transcriber, error) {
	if cfg.WhisperEndpoint == "" {
		return nil, nil
	}
	client, err := whisper.NewClient(cfg.WhisperEndpoint, &http.Client{Timeout: 20 * time.Minute})
	if err != nil {
		return nil, fmt.Errorf("configure native speech inference: %w", err)
	}
	return client, nil
}
