package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"github.com/google/uuid"
)

type ReferenceExecutionProgressReader interface {
	Get(context.Context, genapp.Actor, string, string) (gen.ReferenceJobProgress, error)
}
type ReferenceExecutionAuthoring interface {
	Create(context.Context, authoringapp.Actor, authoringapp.CreateCommand) (authoring.Draft, error)
	Publish(context.Context, authoringapp.Actor, authoringapp.PublishCommand) (authoring.Revision, error)
}
type ReferenceExecutionRunStarter interface {
	Start(context.Context, Actor, StartCommand) (domain.WorkflowRun, error)
}

type StartReferenceExecutionCommand struct {
	ProjectID      string
	ExecutionRef   gen.GenerationRevisionRef
	IdempotencyKey string
}

type ReferenceExecutionStartService struct {
	reader    ReferenceExecutionProgressReader
	authoring ReferenceExecutionAuthoring
	starter   ReferenceExecutionRunStarter
}

func NewReferenceExecutionStartService(reader ReferenceExecutionProgressReader, authoring ReferenceExecutionAuthoring, starter ReferenceExecutionRunStarter) (*ReferenceExecutionStartService, error) {
	if reader == nil || authoring == nil || starter == nil {
		return nil, errors.New("Reference execution start Owners are required")
	}
	return &ReferenceExecutionStartService{reader: reader, authoring: authoring, starter: starter}, nil
}

func (service *ReferenceExecutionStartService) Start(ctx context.Context, actor Actor, command StartReferenceExecutionCommand) (domain.WorkflowRun, error) {
	if service == nil || !command.ExecutionRef.Valid() || command.ExecutionRef.Revision != 1 || actor.TokenVersion < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return domain.WorkflowRun{}, invalid("Invalid Reference execution start")
	}
	for _, id := range []string{actor.UserID, command.ProjectID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return domain.WorkflowRun{}, invalid("Invalid Reference execution start scope")
		}
	}
	progress, err := service.reader.Get(ctx, genapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, command.ProjectID, command.ExecutionRef.ID)
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	if progress.ExecutionRef != command.ExecutionRef {
		return domain.WorkflowRun{}, invalid("Reference execution start identity has drifted")
	}
	graph, err := BuildReferenceExecutionGraph(progress)
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	// Owner receipts bind each stage's input. The key excludes mutable progress,
	// so retries after any commit reuse the same draft, revision and run.
	keyHash, err := platformcommand.InputHash(struct{ ProjectID, Key string }{command.ProjectID, command.IdempotencyKey})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	key := "reference-execution:" + keyHash
	author := authoringapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	draft, err := service.authoring.Create(ctx, author, authoringapp.CreateCommand{ProjectID: command.ProjectID, AuthoringMode: "GUIDED", Graph: graph, Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{{Kind: "reference_execution", ID: command.ExecutionRef.ID, Version: "1", Hash: command.ExecutionRef.ContentHash}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: key + ":draft"})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	revision, err := service.authoring.Publish(ctx, author, authoringapp.PublishCommand{DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: key + ":publish"})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	run, err := service.starter.Start(ctx, actor, StartCommand{AuthoringRevisionID: revision.ID, IdempotencyKey: key + ":start"})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	return run, nil
}
