package application

import (
	"context"

	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type SourceReader interface {
	ReadFrozenSource(context.Context, Actor, string, domain.Source) (domain.FrozenSource, error)
}

type SourceService struct {
	transactions Transactions
	sources      SourceReader
}

func NewSourceService(transactions Transactions, sources SourceReader) *SourceService {
	return &SourceService{transactions: transactions, sources: sources}
}

// ReadSource only accepts a stored command identity. The requesting machine cannot
// substitute another actor, project, source revision or source URL.
func (service *SourceService) ReadSource(ctx context.Context, runID, payloadHash string) (domain.FrozenSource, error) {
	if !validID(runID) || !validHash(payloadHash) {
		return domain.FrozenSource{}, Problem("validation_failed", 422)
	}
	var frozenRun domain.Run
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		run, err := repo.Get(ctx, runID)
		if err != nil {
			return err
		}
		if run.PayloadHash != payloadHash {
			return Problem("command_hash_drift", 409)
		}
		actualHash, err := PayloadHash(run.Command)
		if err != nil {
			return err
		}
		if actualHash != payloadHash || run.Command.RunID != runID || run.Command.CommandID != runID {
			return Problem("command_hash_drift", 409)
		}
		actor := Actor{UserID: run.Command.ActorID, TokenVersion: run.TokenVersion}
		workspace, err := repo.Authorize(ctx, actor, run.Command.ProjectID, true)
		if err != nil {
			return err
		}
		if workspace != run.Command.WorkspaceID {
			return Problem("forbidden", 403)
		}
		frozenRun = run
		return nil
	})
	if err != nil {
		return domain.FrozenSource{}, err
	}
	// The source owner rechecks current authorization and the immutable source in
	// its own transaction. Release our project lock before entering that boundary:
	// independently injected stores use separate connections, not savepoints.
	actor := Actor{UserID: frozenRun.Command.ActorID, TokenVersion: frozenRun.TokenVersion}
	return service.sources.ReadFrozenSource(ctx, actor, frozenRun.Command.ProjectID, frozenRun.Command.Source)
}
