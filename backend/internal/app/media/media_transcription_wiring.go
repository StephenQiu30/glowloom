package media

import (
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/whisper"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ProvideMediaTranscriptionStore retains query and cancel access when native
// inference is disabled, while refusing new commands before any side effect.
func ProvideMediaTranscriptionStore(database *gorm.DB, enabled bool) *pgtool.TranscriptionStore {
	return ProvideMediaTranscriptionStoreWithAccess(database, enabled, NewMediaScopeAccess)
}

// ProvideMediaTranscriptionStoreWithAccess assembles transcription storage with explicit scope access.
func ProvideMediaTranscriptionStoreWithAccess(database *gorm.DB, enabled bool, scopes pgmedia.ScopeAccessFactory) *pgtool.TranscriptionStore {
	var source pgtool.TranscriptionSourceFactory
	if enabled {
		source = func(tx *gorm.DB) toolapp.TranscriptionSourceReader {
			return provideMediaToolSourceWithAccess(tx, scopes)
		}
	}
	return pgtool.NewTranscriptionStore(database, source,
		func(tx *gorm.DB) toolapp.DerivedMedia {
			return mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
		})
}

// ProvideTranscriber constructs the configured transcription provider.
func ProvideTranscriber(cfg config.Config) (toolapp.Transcriber, error) {
	if cfg.WhisperEndpoint == "" {
		return nil, nil
	}
	client, err := whisper.NewClient(cfg.WhisperEndpoint, &http.Client{Timeout: 20 * time.Minute})
	if err != nil {
		return nil, fmt.Errorf("configure native speech inference: %w", err)
	}
	return client, nil
}

// ProvideRoutedMediaTranscriptionStore pins a real output profile at admission.
func ProvideRoutedMediaTranscriptionStore(database *gorm.DB, enabled bool, scopes pgmedia.ScopeAccessFactory) *pgtool.TranscriptionStore {
	return ProvideMediaTranscriptionStoreWithAccess(database, enabled, scopes).WithStorageAdmission(func(tx *gorm.DB) toolapp.ToolStorageLedger { return pgmedia.NewStorageRoutes(tx) })
}
