package projectaccess

import (
	"context"
	"errors"

	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
)

type ProjectService interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type Authorizer struct{ projects ProjectService }

func New(projects ProjectService) *Authorizer { return &Authorizer{projects: projects} }

func (authorizer *Authorizer) Get(ctx context.Context, actor searchapp.Actor, projectID string) (projectdomain.Project, error) {
	value, err := authorizer.projects.Get(ctx, projectapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, projectID)
	if err == nil {
		return value, nil
	}
	var projectError *projectapp.Error
	if !errors.As(err, &projectError) {
		return projectdomain.Project{}, err
	}
	return projectdomain.Project{}, &searchapp.Error{
		Code: projectError.Code, Message: projectError.Message, Status: projectError.Status,
		NextAction: projectError.NextAction, Details: projectError.Details,
	}
}
