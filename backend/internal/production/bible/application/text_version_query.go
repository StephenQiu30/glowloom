package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type TextWorldReader interface {
	ReadTextWorld(context.Context, string, string, string) (domain.TextWorldVersion, error)
}
type TextProjectReader interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}
type TextWorldQuery struct {
	versions TextWorldReader
	projects TextProjectReader
}

func NewTextWorldQuery(versions TextWorldReader, projects TextProjectReader) *TextWorldQuery {
	return &TextWorldQuery{versions: versions, projects: projects}
}
func (s *TextWorldQuery) Get(ctx context.Context, actor Actor, projectID, versionID string) (domain.TextWorldVersion, error) {
	if !contract.ValidTextID(projectID) || !contract.ValidTextID(versionID) {
		return domain.TextWorldVersion{}, &Error{Code: "validation_failed", Message: "Invalid version reference", Status: 422}
	}
	project, err := s.projects.Get(ctx, projectapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, projectID)
	if err != nil {
		var problem *projectapp.Error
		if errors.As(err, &problem) {
			return domain.TextWorldVersion{}, fmt.Errorf("project access: %w: %w", &Error{Code: problem.Code, Message: problem.Message, Status: problem.Status}, err)
		}
		return domain.TextWorldVersion{}, err
	}
	value, err := s.versions.ReadTextWorld(ctx, project.WorkspaceID, projectID, versionID)
	if errors.Is(err, ErrNotFound) {
		return domain.TextWorldVersion{}, &Error{Code: "not_found", Message: "Version not found", Status: 404}
	}
	if err != nil {
		return domain.TextWorldVersion{}, err
	}
	if project.ID != projectID || value.ID != versionID || value.ProjectID != projectID || value.WorkspaceID != project.WorkspaceID || ValidateTextWorldVersion(value) != nil {
		return domain.TextWorldVersion{}, &Error{Code: "formal_version_drift", Message: "Stored version does not match its evidence", Status: 409}
	}
	return value, nil
}
func ValidateTextWorldVersion(value domain.TextWorldVersion) error {
	if value.Revision != 1 || !contract.ValidTextID(value.ID) || value.CreatedAt.IsZero() {
		return errors.New("invalid formal version")
	}
	expected := value.ContentHash
	value.ContentHash = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return err
	}
	if hash != expected {
		return errors.New("formal version hash differs")
	}
	return nil
}
