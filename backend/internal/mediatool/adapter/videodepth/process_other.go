//go:build !darwin && !linux

package videodepth

import (
	"context"
	"os/exec"

	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// Unverified native process-tree platforms stay explicitly unavailable.
func configureProcess(*exec.Cmd) error { return application.ErrDepthRuntimeUnavailable }
func processGroupRSS(context.Context, int) (int64, error) {
	return 0, application.ErrDepthRuntimeUnavailable
}
func stopProcessGroup(context.Context, int) error { return application.ErrDepthCessationUncertain }
