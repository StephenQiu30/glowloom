package authoring

import (
	"context"
	"errors"

	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoringdomain "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type service interface {
	CompilationSource(context.Context, authoringapp.Actor, string) (authoringdomain.Revision, authoringdomain.Catalog, error)
}

type Source struct{ authoring service }

func New(authoring service) *Source { return &Source{authoring: authoring} }

func (source *Source) Resolve(ctx context.Context, actor workflowapp.Actor, revisionID string) (workflowdomain.CompilationSource, error) {
	if source == nil || source.authoring == nil {
		return workflowdomain.CompilationSource{}, &workflowapp.Error{
			Code: "dependency_unavailable", Message: "Authoring revision source is unavailable", Status: 503,
		}
	}
	revision, catalog, err := source.authoring.CompilationSource(ctx, authoringapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, revisionID)
	if err != nil {
		var sourceError *authoringapp.Error
		if errors.As(err, &sourceError) {
			return workflowdomain.CompilationSource{}, &workflowapp.Error{
				Code: sourceError.Code, Message: sourceError.Message, NextAction: sourceError.NextAction, Status: sourceError.Status,
			}
		}
		return workflowdomain.CompilationSource{}, err
	}
	return workflowdomain.CompilationSource{Revision: revision, Catalog: catalog}, nil
}
