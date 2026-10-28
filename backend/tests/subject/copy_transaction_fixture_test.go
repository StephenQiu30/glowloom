package subject_test

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// Isolated media-owner tests commit each receipt in a scoped transaction before
// object I/O. Production adds the workspace job lease through its coordinator;
// the owning workspace tests exercise that fence and publication separately.
// Admission Freeze methods still require the caller's explicit transaction.
type locatedCopyTransactions struct {
	*pgmedia.ProjectCopyStore
	db *gorm.DB
}

func locatedCopyRead[T any](ctx context.Context, db *gorm.DB, read func(*pgmedia.ProjectCopyStore) (T, error)) (T, error) {
	var result T
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = read(pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).WithRoutedStorage())
		return err
	})
	return result, err
}

func (s *locatedCopyTransactions) write(ctx context.Context, change func(*pgmedia.ProjectCopyStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return change(pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).WithRoutedStorage())
	})
}

func (s *locatedCopyTransactions) Objects(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot) ([]mediaapp.ProjectCopyObject, error) {
	return locatedCopyRead(ctx, s.db, func(repo *pgmedia.ProjectCopyStore) ([]mediaapp.ProjectCopyObject, error) {
		return repo.Objects(ctx, actor, binding, snapshot)
	})
}

func (s *locatedCopyTransactions) RecordObjectDigest(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	return s.write(ctx, func(repo *pgmedia.ProjectCopyStore) error {
		return repo.RecordObjectDigest(ctx, actor, binding, snapshot, asset, kind, digest, size)
	})
}

func (s *locatedCopyTransactions) BeginObjectWrite(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return s.write(ctx, func(repo *pgmedia.ProjectCopyStore) error {
		return repo.BeginObjectWrite(ctx, actor, binding, snapshot, asset, kind)
	})
}

func (s *locatedCopyTransactions) ConfirmObject(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	return s.write(ctx, func(repo *pgmedia.ProjectCopyStore) error {
		return repo.ConfirmObject(ctx, actor, binding, snapshot, asset, kind, digest, size)
	})
}

func (s *locatedCopyTransactions) AuthorizeObjectRemoval(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return s.write(ctx, func(repo *pgmedia.ProjectCopyStore) error {
		return repo.AuthorizeObjectRemoval(ctx, actor, binding, snapshot, asset, kind)
	})
}

func (s *locatedCopyTransactions) ConfirmObjectRemoved(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return s.write(ctx, func(repo *pgmedia.ProjectCopyStore) error {
		return repo.ConfirmObjectRemoved(ctx, actor, binding, snapshot, asset, kind)
	})
}

func (s *locatedCopyTransactions) Register(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot) (mediaapp.ProjectCopyReceipt, error) {
	return locatedCopyRead(ctx, s.db, func(repo *pgmedia.ProjectCopyStore) (mediaapp.ProjectCopyReceipt, error) {
		return repo.Register(ctx, actor, binding, snapshot)
	})
}

func (s *locatedCopyTransactions) ReferenceCopiedAsset(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, asset uuid.UUID) (mediaapp.AssetSummary, error) {
	return locatedCopyRead(ctx, s.db, func(repo *pgmedia.ProjectCopyStore) (mediaapp.AssetSummary, error) {
		return repo.ReferenceCopiedAsset(ctx, actor, binding, asset)
	})
}

func (s *locatedCopyTransactions) ReadCopyReference(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, fact mediaapp.ReferenceFact) (mediaapp.ProjectCopyReferenceFiles, error) {
	return locatedCopyRead(ctx, s.db, func(repo *pgmedia.ProjectCopyStore) (mediaapp.ProjectCopyReferenceFiles, error) {
		return repo.ReadCopyReference(ctx, actor, binding, snapshot, fact)
	})
}

func (s *locatedCopyTransactions) ReadTransferredCopyReference(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, fact mediaapp.ReferenceFact) (mediaapp.ProjectCopyReferenceFiles, error) {
	return locatedCopyRead(ctx, s.db, func(repo *pgmedia.ProjectCopyStore) (mediaapp.ProjectCopyReferenceFiles, error) {
		return repo.ReadTransferredCopyReference(ctx, actor, binding, snapshot, fact)
	})
}
