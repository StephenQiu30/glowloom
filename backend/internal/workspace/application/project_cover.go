package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

// ErrProjectCoverUnavailable means the selected image is no longer a usable project asset.
var ErrProjectCoverUnavailable = errors.New("project cover unavailable")

// ProjectCoverReference proves current media eligibility in the project transaction.
type ProjectCoverReference interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (mediaapp.AssetSummary, error)
}

// SameProjectCover compares nullable identities without pointer identity or URL facts.
func SameProjectCover(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// FreezeProjectCover validates an existing binding through its owning media reader.
// The caller retains its transaction until the project and other snapshots commit.
func FreezeProjectCover(ctx context.Context, reader ProjectCoverReference, actor identityapp.Principal, project uuid.UUID, asset *uuid.UUID) error {
	if asset == nil {
		return nil
	}
	if *asset == uuid.Nil || project == uuid.Nil {
		return ErrProjectCoverUnavailable
	}
	if reader == nil {
		return ErrProjectDependencyUnavailable
	}
	fact, err := reader.Reference(ctx, actor, project, *asset)
	if err != nil {
		if errors.Is(err, mediaapp.ErrNotFound) {
			return ErrProjectCoverUnavailable
		}
		if errors.Is(err, identityapp.ErrForbidden) {
			return err
		}
		return fmt.Errorf("%w: verify project cover: %w", ErrProjectDependencyUnavailable, err)
	}
	if fact.ID != *asset || fact.ProjectID != project || fact.Kind != "image" || fact.Revision < 1 {
		return ErrProjectCoverUnavailable
	}
	return nil
}

// MapProjectCover maps the frozen source identity using the media owner's actual mapping.
func MapProjectCover(source *uuid.UUID, mapping map[uuid.UUID]uuid.UUID) (*uuid.UUID, error) {
	if source == nil {
		return nil, nil
	}
	id, ok := mapping[*source]
	if !ok || *source == uuid.Nil || id == uuid.Nil || id == *source {
		return nil, ErrProjectCoverUnavailable
	}
	return &id, nil
}

// VerifyProjectCover compares publication facts and rechecks the target's owning media.
func VerifyProjectCover(ctx context.Context, reader ProjectCoverReference, actor identityapp.Principal, project uuid.UUID, expected, actual *uuid.UUID) error {
	if !SameProjectCover(expected, actual) {
		return ErrProjectCoverUnavailable
	}
	return FreezeProjectCover(ctx, reader, actor, project, actual)
}
