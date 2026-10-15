package managed

import (
	"errors"

	"go.temporal.io/sdk/temporal"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func executionAuthorityError(err error) error {
	if errors.Is(err, application.ErrExecutionAuthorityRevoked) {
		return temporal.NewNonRetryableApplicationError("current execution authority is unavailable", "execution_authority_revoked", err)
	}
	return err
}
