package media_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type transaction struct{ repo *repository }

func (t transaction) WithinTransaction(ctx context.Context, use func(app.Repository) error) error {
	return use(t.repo)
}

type repository struct {
	app.Repository
	session domain.UploadSession
	denied  bool
	created int
}

func (r *repository) GetUpload(context.Context, string, bool) (domain.UploadSession, error) {
	return r.session, nil
}
func (r *repository) Authorize(context.Context, app.Actor, string, bool) error {
	if r.denied {
		return errors.New("permission revoked")
	}
	return nil
}
func (r *repository) CreateCompletion(context.Context, domain.MediaObject, domain.MediaVersion, domain.Task) error {
	r.created++
	return nil
}
func (r *repository) SaveUpload(context.Context, domain.UploadSession) error { return nil }

type storage struct {
	app.ObjectStore
	read func() error
}

func (s storage) ReadVerified(context.Context, string, int64, string, int64) ([]byte, error) {
	return []byte("synthetic"), s.read()
}
func TestUploadCompletionRechecksAccessAndTimeAfterObjectRead(t *testing.T) {
	for _, mode := range []string{"revoked", "expired", "cancelled", "valid"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
			repo := &repository{session: domain.UploadSession{ID: "upload", WorkspaceID: "workspace", Status: "pending", Kind: "document", ExpiresAt: now.Add(time.Minute)}}
			store := storage{read: func() error {
				switch mode {
				case "revoked":
					repo.denied = true
				case "expired":
					now = now.Add(2 * time.Minute)
				case "cancelled":
					return context.Canceled
				}
				return nil
			}}
			service := app.NewService(transaction{repo}, store, app.Config{Now: func() time.Time { return now }, NewID: func() string { return "synthetic-id" }})
			_, err := service.Complete(context.Background(), app.Actor{}, "upload")
			if mode == "valid" {
				if err != nil || repo.created != 1 {
					t.Fatalf("valid completion: %v", err)
				}
				return
			}
			if err == nil || repo.created != 0 {
				t.Fatalf("unsafe completion persisted: %d %v", repo.created, err)
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}
