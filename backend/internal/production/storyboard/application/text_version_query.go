package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type TextIntentReader interface {
	ReadTextIntent(context.Context, string, string, string) (domain.TextIntentVersion, error)
}
type TextProjectReader interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}
type TextIntentQuery struct {
	versions TextIntentReader
	projects TextProjectReader
}

func NewTextIntentQuery(versions TextIntentReader, projects TextProjectReader) *TextIntentQuery {
	return &TextIntentQuery{versions: versions, projects: projects}
}
func (s *TextIntentQuery) Get(ctx context.Context, actor Actor, projectID, versionID string) (domain.TextIntentVersion, error) {
	if !contract.ValidTextID(projectID) || !contract.ValidTextID(versionID) {
		return domain.TextIntentVersion{}, &Error{Code: "validation_failed", Message: "Invalid version reference", Status: 422}
	}
	project, err := s.projects.Get(ctx, projectapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, projectID)
	if err != nil {
		var problem *projectapp.Error
		if errors.As(err, &problem) {
			return domain.TextIntentVersion{}, fmt.Errorf("project access: %w: %w", &Error{Code: problem.Code, Message: problem.Message, Status: problem.Status}, err)
		}
		return domain.TextIntentVersion{}, err
	}
	value, err := s.versions.ReadTextIntent(ctx, project.WorkspaceID, projectID, versionID)
	if errors.Is(err, ErrNotFound) {
		return domain.TextIntentVersion{}, &Error{Code: "not_found", Message: "Version not found", Status: 404}
	}
	if err != nil {
		return domain.TextIntentVersion{}, err
	}
	if project.ID != projectID || value.ID != versionID || value.ProjectID != projectID || value.WorkspaceID != project.WorkspaceID || ValidateTextIntentVersion(value) != nil {
		return domain.TextIntentVersion{}, &Error{Code: "formal_version_drift", Message: "Stored version does not match its evidence", Status: 409}
	}
	return value, nil
}
func ValidateTextIntentVersion(value domain.TextIntentVersion) error {
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
