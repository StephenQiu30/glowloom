package agenthttp

import (
	"context"
	"net/http"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

func (c *Client) Manifest(ctx context.Context, run domain.Run) (domain.ManifestSnapshot, error) {
	var value domain.ManifestSnapshot
	err := c.proposalRequest(ctx, run, http.MethodGet, "/internal/creation/commands/"+run.Command.RunID+"/manifest", nil, &value)
	if err == nil {
		err = app.ValidateManifest(run, value)
	}
	return value, err
}
func (c *Client) Attempts(ctx context.Context, run domain.Run, stepID string) (domain.AttemptHistory, error) {
	var value domain.AttemptHistory
	err := c.proposalRequest(ctx, run, http.MethodGet, "/internal/creation/commands/"+run.Command.RunID+"/steps/"+stepID+"/attempts", nil, &value)
	if err == nil {
		err = app.ValidateAttempts(run, stepID, value)
	}
	return value, err
}
