package bible_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func TestBibleCommandRejectsVoiceRemovalFromLocationAndProp(t *testing.T) {
	for _, kind := range []domain.Kind{domain.KindLocation, domain.KindProp} {
		c := app.Command{ProjectID: uuid.New(), Key: uuid.New(), RequestID: uuid.New(), Kind: kind, Action: "voice_unbind", EntryID: uuid.New(), ExpectedRevision: 1}
		if _, err := c.Fingerprint(); !errors.Is(err, domain.ErrInvalidContent) {
			t.Fatalf("%s voice action accepted before persistence: %v", kind, err)
		}
	}
}
