package application

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// UploadProjectAccess is the upload consumer's narrow Creation authority port.
// AuthorizeUploadProject retains identity, membership and project locks until
// the caller's transaction ends. Admission is not a later commit grant: final
// publication reauthorizes inside its receipt transaction. An RPC precheck or
// an expiring admission token does not satisfy this contract.
//
// AwaitUploadProjectCommit serializes ambiguous-outcome cleanup only. It must
// not require a still-valid session or an active project, and grants no upload
// authority. A missing or unresolved project must return an error, never absence.
type UploadProjectAccess interface {
	AuthorizeUploadProject(context.Context, identity.Principal, uuid.UUID) (string, error)
	AwaitUploadProjectCommit(context.Context, uuid.UUID) error
}
