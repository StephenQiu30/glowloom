package workspace_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestUploadProjectOwnerRejectsMissingTransaction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *pgworkspace.ProjectContentAccessStore
	}{
		{"nil owner", nil},
		{"nil database", pgworkspace.NewProjectContentAccessStore(nil)},
		{"nil statement", pgworkspace.NewProjectContentAccessStore(&gorm.DB{})},
		{"not a transaction", pgworkspace.NewProjectContentAccessStore(&gorm.DB{Statement: &gorm.Statement{}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.store.AuthorizeUploadProject(t.Context(), identity.Principal{}, uuid.New()); !errors.Is(err, workspacesource.ErrProjectDependencyUnavailable) {
				t.Fatalf("upload authority without transaction: %v", err)
			}
			if err := tc.store.AwaitUploadProjectCommit(t.Context(), uuid.New()); !errors.Is(err, workspacesource.ErrProjectDependencyUnavailable) {
				t.Fatalf("cleanup without transaction: %v", err)
			}
		})
	}
}
