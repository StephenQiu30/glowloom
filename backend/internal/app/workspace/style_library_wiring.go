package workspace

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type stylePersonalImages struct {
	library *pgmedia.LibraryStore
	sources *pgmedia.SubjectSourceStore
}

func (s stylePersonalImages) Image(ctx context.Context, a identity.Principal, id uuid.UUID) (app.StyleLibraryReference, error) {
	scope := mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}
	file, err := s.sources.FindSubjectSource(ctx, a, scope, id)
	if err != nil {
		return app.StyleLibraryReference{}, err
	}
	asset := file.Asset
	if asset.Validate() != nil || asset.Kind != mediadomain.KindImage || asset.IsDelete || asset.Status != mediadomain.StatusReady || asset.ModerationStatus != mediadomain.ModerationPassed || asset.ObjectLocation == nil || !asset.ObjectLocation.Readable(asset.ObjectKey) || asset.Personal == nil || asset.Personal.ActorID != a.ID || asset.Personal.OrgID != a.OrgID || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.SHA256 == nil || len(*asset.SHA256) != 64 || asset.ByteSize < 1 || asset.ByteSize > mediaapp.MaxUploadImageBytes {
		return app.StyleLibraryReference{}, mediaapp.ErrNotFound
	}
	digest, err := hex.DecodeString(*asset.SHA256)
	if err != nil || len(digest) != 32 || strings.ToLower(*asset.SHA256) != *asset.SHA256 {
		return app.StyleLibraryReference{}, mediaapp.ErrNotFound
	}
	item, err := s.library.LibraryDetail(ctx, a, scope, id)
	if err != nil {
		return app.StyleLibraryReference{}, err
	}
	if item.State != mediadomain.LibraryItemStateActive || item.AssetID == nil || *item.AssetID != id || item.Media == nil || item.Media.ID != id || item.Media.Revision != asset.Revision {
		return app.StyleLibraryReference{}, mediaapp.ErrNotFound
	}
	return app.StyleLibraryReference{ID: item.ID, Revision: item.Revision, AssetID: id, AssetRevision: asset.Revision, Title: item.Title}, nil
}

// ProvideStyleLibraryStore reuses personal-media ownership and catalog eligibility.
func ProvideStyleLibraryStore(db *gorm.DB) *pgworkspace.StyleLibraryStore {
	return pgworkspace.NewStyleLibraryStore(db, scopewire.ProvideRequestScopeAccess, func(tx *gorm.DB) pgworkspace.StyleImageReader {
		library := pgmedia.NewScopedLibraryStore(tx, mediawire.NewMediaLibraryProjectAccess, time.Now, mediawire.NewMediaScopeAccess)
		return stylePersonalImages{library: library, sources: pgmedia.NewSubjectSourceStore(tx, library)}
	})
}
