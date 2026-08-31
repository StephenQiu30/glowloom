package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
)

var compilerProjectFixtures = struct {
	sync.Mutex
	values []testgorm.OwnedFixture
}{}

func TestMain(tests *testing.M) {
	result := tests.Run()
	if err := cleanupCompilerProjectFixtures(); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup Workflow database fixtures: %v\n", err)
		result = 1
	}
	os.Exit(result)
}

func trackCompilerProjectFixture(fixture compilerProjectFixture) {
	compilerProjectFixtures.Lock()
	defer compilerProjectFixtures.Unlock()
	compilerProjectFixtures.values = append(compilerProjectFixtures.values, testgorm.OwnedFixture{
		UserID: fixture.userID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
	})
}

func cleanupCompilerProjectFixtures() error {
	compilerProjectFixtures.Lock()
	fixtures := append([]testgorm.OwnedFixture(nil), compilerProjectFixtures.values...)
	compilerProjectFixtures.Unlock()
	if len(fixtures) == 0 {
		return nil
	}
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("LANVERSE_TEST_DATABASE_URL is required after Workflow fixtures were created")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		return err
	}
	defer func() { _ = platformdatabase.Close(database) }()
	cleanupErrors := make([]error, 0)
	for index := len(fixtures) - 1; index >= 0; index-- {
		if err = testgorm.DeleteOwnedFixture(database, fixtures[index]); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("workspace %s: %w", fixtures[index].WorkspaceID, err))
		}
	}
	return errors.Join(cleanupErrors...)
}
