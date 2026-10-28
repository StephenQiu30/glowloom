package canvas_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type downloadFixtureObjects struct {
	calls     int
	closed    bool
	afterOpen func()
}

func (s *downloadFixtureObjects) Get(context.Context, string) (io.ReadCloser, error) {
	s.calls++
	if s.afterOpen != nil {
		s.afterOpen()
	}
	return &downloadFixtureBody{Reader: strings.NewReader("x"), closed: &s.closed}, nil
}

type downloadFixtureBody struct {
	io.Reader
	closed *bool
}

func (b *downloadFixtureBody) Close() error { *b.closed = true; return nil }
func TestCanvasAssetOriginalRechecksRealOwnerSessionAfterObjectOpen(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	asset := seedCanvasMedia(t, db, project, "image")
	objects := &downloadFixtureObjects{}
	downloads := mediaapp.NewAssetDownloads(pgmedia.NewScopedStore(db, mediawire.NewMediaScopeAccess), nil, objects)
	file, err := downloads.Open(t.Context(), actor, project, asset)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if err != nil || string(got) != "x" || !objects.closed {
		t.Fatal("real authorized owner original missing", err)
	}
	other, _ := seedCanvasActor(t, db)
	if _, err := downloads.Open(t.Context(), other, project, asset); err == nil || objects.calls != 1 {
		t.Fatal("cross-owner download opened physical object", err)
	}
	objects.closed = false
	objects.afterOpen = func() {
		if err := db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
			t.Fatal(err)
		}
	}
	file, err = downloads.Open(t.Context(), actor, project, asset)
	if file != nil || !errors.Is(err, identity.ErrSessionInvalid) || !objects.closed {
		t.Fatal("session revocation after open streamed bytes or leaked reader", file, err)
	}
}

func TestCanvasAssetOriginalUnconfiguredTransportFailsClosed(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	asset := seedCanvasMedia(t, db, project, "image")
	if file, err := mediawire.ProvideAssetDownloads(db, nil, nil).Open(t.Context(), actor, project, asset); file != nil || !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing transport admitted stream", file, err)
	}
}
