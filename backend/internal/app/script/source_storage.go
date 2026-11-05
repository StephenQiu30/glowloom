package script

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// ProvideScriptObjectLocations composes both owners on the caller transaction.
func ProvideScriptObjectLocations(tx *gorm.DB) app.SourceObjectLocations {
	return sourceLocations{routes: pgmedia.NewStorageRoutes(tx)}
}

type sourceLocations struct{ routes *pgmedia.StorageRoutes }

func (s sourceLocations) Admit(ctx context.Context, project, work uuid.UUID, facts []domain.ObjectFact) (uuid.UUID, []domain.ObjectFact, error) {
	intent, err := s.routes.PinStorageWork(ctx, media.StorageWorkOwner{Kind: media.StorageWorkKindScript, OwnerID: project, WorkID: work})
	if err != nil {
		return uuid.Nil, nil, err
	}
	bound := append([]domain.ObjectFact(nil), facts...)
	for i := range bound {
		location, err := s.routes.ReserveWorkLocation(ctx, intent.ID, bound[i].Key)
		if err != nil {
			return uuid.Nil, nil, err
		}
		bound[i].LocationID = location.ID
	}
	return intent.ID, bound, nil
}

func (s sourceLocations) location(ctx context.Context, f domain.ObjectFact) (mediadomain.ObjectLocation, error) {
	l, err := s.routes.ReadLocation(ctx, f.LocationID)
	if err != nil {
		return l, err
	}
	if f.LocationID == uuid.Nil || f.Validate() != nil || l.Ref.Key != f.Key || l.ByteSize != nil && *l.ByteSize != f.ByteSize || l.SHA256 != nil && *l.SHA256 != f.SHA256 || l.MIMEType != nil && *l.MIMEType != f.MIME {
		return l, app.ErrObjectMismatch
	}
	return l, nil
}

func (s sourceLocations) Begin(ctx context.Context, id uuid.UUID, f domain.ObjectFact) error {
	l, err := s.location(ctx, f)
	if err != nil {
		return err
	}
	return s.routes.BeginWorkWrite(ctx, id, l)
}

func (s sourceLocations) Confirm(ctx context.Context, id uuid.UUID, f domain.ObjectFact) error {
	l, err := s.location(ctx, f)
	if err != nil {
		return err
	}
	return s.routes.ConfirmWorkLocation(ctx, id, l, media.ObjectInfo{Size: f.ByteSize, SHA256: f.SHA256, ContentType: f.MIME})
}

func (s sourceLocations) Accept(ctx context.Context, id uuid.UUID, facts []domain.ObjectFact) error {
	ids := make([]uuid.UUID, len(facts))
	for i, f := range facts {
		if _, err := s.location(ctx, f); err != nil {
			return err
		}
		ids[i] = f.LocationID
	}
	return s.routes.AcceptStorageWork(ctx, id, ids)
}

func (s sourceLocations) Cancel(ctx context.Context, id uuid.UUID) error {
	return s.routes.CancelStorageWork(ctx, id)
}

// BeginRemoval returns readiness separately from transaction success. A false
// result retains the durable cancellation clock without authorizing SDK removal.
func (s sourceLocations) BeginRemoval(ctx context.Context, id uuid.UUID, f domain.ObjectFact) (bool, error) {
	l, err := s.location(ctx, f)
	if err != nil {
		return false, err
	}
	if ready, err := s.routes.BeginPrivateCopyRemoval(ctx, id, l); err != nil || ready {
		return ready, err
	}
	if err := s.routes.CancelStorageWork(ctx, id); err != nil {
		return false, err
	}
	return s.routes.BeginWorkRemoval(ctx, id, l)
}

func (s sourceLocations) ConfirmRemoval(ctx context.Context, id uuid.UUID, f domain.ObjectFact) error {
	l, err := s.location(ctx, f)
	if err != nil {
		return err
	}
	return s.routes.ConfirmWorkRemoval(ctx, id, l)
}
