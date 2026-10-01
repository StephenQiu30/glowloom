package application

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// DerivedRemoval binds cleanup to an unpublished result and its original proof.
// Callers must separately prove their own immutable result intent and worker fence.
type DerivedRemoval struct {
	ProjectID, AssetID             uuid.UUID
	ObjectKey, PrimarySHA256, Kind string
	ByteSize                       int64
}

// AuthorizeRemoval refuses objects already owned by any other asset or ready result.
func (s *DerivedService) AuthorizeRemoval(ctx context.Context, actor identityapp.Principal, in DerivedRemoval) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	if in.ProjectID == uuid.Nil || in.AssetID == uuid.Nil || !strings.HasPrefix(in.ObjectKey, "projects/"+in.ProjectID.String()+"/video/") || len(in.PrimarySHA256) != 64 || in.ByteSize < 1 || in.ByteSize > 500<<20 || (in.Kind != "original" && in.Kind != "poster" && in.Kind != "proxy_720p") {
		return ErrInvalidQuery
	}
	if _, err := hex.DecodeString(in.PrimarySHA256); err != nil {
		return ErrInvalidQuery
	}
	return s.repo.AuthorizeDerivedRemoval(ctx, actor, in)
}
