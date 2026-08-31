package gormdb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

// OwnedFixture identifies every database fact created for one isolated test owner.
type OwnedFixture struct {
	UserID      string
	WorkspaceID string
	ProjectID   string
}

// OwnedWorkspaceFixture identifies a test user and its isolated workspace.
type OwnedWorkspaceFixture struct {
	UserIDs        []string
	WorkspaceID    string
	NodeCatalogIDs []string
}

// OwnedUserFixture identifies one standalone test account that owns no workspace membership.
type OwnedUserFixture struct {
	UserID string
}

// RegisterOwnedFixtureCleanup deletes the exact test owner after the journey ends.
func RegisterOwnedFixtureCleanup(t testing.TB, database *gorm.DB, fixture OwnedFixture) {
	t.Helper()
	t.Cleanup(func() {
		if err := DeleteOwnedFixture(database, fixture); err != nil {
			t.Errorf("delete owned database fixture: %v", err)
		}
	})
}

// RegisterOwnedWorkspaceFixtureCleanup deletes one exact test workspace after the journey ends.
func RegisterOwnedWorkspaceFixtureCleanup(t testing.TB, database *gorm.DB, fixture OwnedWorkspaceFixture) {
	t.Helper()
	t.Cleanup(func() {
		if err := DeleteOwnedWorkspaceFixture(database, fixture); err != nil {
			t.Errorf("delete owned workspace database fixture: %v", err)
		}
	})
}

// RegisterOwnedUserFixtureCleanup deletes one exact standalone test account after the journey ends.
func RegisterOwnedUserFixtureCleanup(t testing.TB, database *gorm.DB, fixture OwnedUserFixture) {
	t.Helper()
	t.Cleanup(func() {
		if err := DeleteOwnedUserFixture(database, fixture); err != nil {
			t.Errorf("delete owned user database fixture: %v", err)
		}
	})
}

// DeleteOwnedFixture removes only facts owned by the exact test scope.
func DeleteOwnedFixture(database *gorm.DB, fixture OwnedFixture) error {
	if database == nil || fixture.UserID == "" || fixture.WorkspaceID == "" || fixture.ProjectID == "" {
		return fmt.Errorf("complete owned fixture identity is required")
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		var projectCount int64
		if err := transaction.Model(&model.Project{}).
			Where("id = ? AND workspace_id = ?", fixture.ProjectID, fixture.WorkspaceID).
			Count(&projectCount).Error; err != nil {
			return err
		}
		if projectCount != 1 {
			return fmt.Errorf("owned project does not belong to the workspace")
		}
		return deleteOwnedScope(transaction, []string{fixture.UserID}, fixture.WorkspaceID, []string{fixture.ProjectID})
	})
}

// DeleteOwnedWorkspaceFixture discovers and removes only projects inside one test workspace.
func DeleteOwnedWorkspaceFixture(database *gorm.DB, fixture OwnedWorkspaceFixture) error {
	if database == nil || fixture.WorkspaceID == "" {
		return fmt.Errorf("complete owned workspace fixture identity is required")
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		var memberships []model.Membership
		if err := transaction.Where("workspace_id = ?", fixture.WorkspaceID).Find(&memberships).Error; err != nil {
			return err
		}
		expectedUsers := make(map[string]bool, len(fixture.UserIDs))
		for _, userID := range fixture.UserIDs {
			if userID == "" || expectedUsers[userID] {
				return fmt.Errorf("owned workspace fixture contains an invalid user identity")
			}
			expectedUsers[userID] = true
		}
		if len(memberships) != len(expectedUsers) {
			return fmt.Errorf("workspace membership set does not match the owned fixture")
		}
		for _, membership := range memberships {
			if !expectedUsers[membership.UserID.String()] {
				return fmt.Errorf("workspace membership set does not match the owned fixture")
			}
		}
		var projectIDs []string
		if err := transaction.Model(&model.Project{}).
			Where("workspace_id = ?", fixture.WorkspaceID).Pluck("id", &projectIDs).Error; err != nil {
			return err
		}
		if err := deleteOwnedScope(transaction, fixture.UserIDs, fixture.WorkspaceID, projectIDs); err != nil {
			return err
		}
		if len(fixture.NodeCatalogIDs) == 0 {
			return nil
		}
		catalogIDs := make([]uuid.UUID, len(fixture.NodeCatalogIDs))
		seenCatalogs := make(map[uuid.UUID]bool, len(fixture.NodeCatalogIDs))
		for index, value := range fixture.NodeCatalogIDs {
			catalogID, err := uuid.Parse(value)
			if err != nil || seenCatalogs[catalogID] {
				return fmt.Errorf("owned workspace fixture contains an invalid node catalog identity")
			}
			seenCatalogs[catalogID] = true
			catalogIDs[index] = catalogID
		}
		deletion := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().
			Where("id IN ?", catalogIDs).Delete(&model.NodeCatalogVersion{})
		if deletion.Error != nil {
			return deletion.Error
		}
		if deletion.RowsAffected != int64(len(catalogIDs)) {
			return fmt.Errorf("owned node catalog fixtures were not deleted exactly once")
		}
		return nil
	})
}

// DeleteOwnedUserFixture removes one exact example.test account after proving it has no memberships.
func DeleteOwnedUserFixture(database *gorm.DB, fixture OwnedUserFixture) error {
	userID, err := uuid.Parse(fixture.UserID)
	if database == nil || err != nil {
		return fmt.Errorf("valid owned user fixture identity is required")
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		var account model.UserAccount
		if err := transaction.First(&account, "id = ?", userID).Error; err != nil {
			return err
		}
		if !strings.HasSuffix(account.EmailNormalized, "@example.test") {
			return fmt.Errorf("owned user fixture must use the reserved test domain")
		}
		var membershipCount int64
		if err := transaction.Model(&model.Membership{}).Where("user_id = ?", userID).Count(&membershipCount).Error; err != nil {
			return err
		}
		if membershipCount != 0 {
			return fmt.Errorf("owned user fixture still has workspace memberships")
		}
		deletion := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().Where("id = ?", userID).
			Delete(&model.UserAccount{})
		if deletion.Error != nil {
			return deletion.Error
		}
		if deletion.RowsAffected != 1 {
			return fmt.Errorf("owned user fixture was not deleted exactly once")
		}
		return nil
	})
}

func deleteOwnedScope(transaction *gorm.DB, userIDs []string, workspaceID string, projectIDs []string) error {
	if len(projectIDs) > 0 {
		bindingIDs := transaction.Model(&model.ProductionBinding{}).
			Select("id").Where("project_id IN ?", projectIDs)
		if err := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().
			Where("production_binding_id IN (?)", bindingIDs).
			Delete(&model.ProductionBindingState{}).Error; err != nil {
			return err
		}
	}

	for index := len(schema.Catalog()) - 1; index >= 0; index-- {
		entry := schema.Catalog()[index]
		statement := &gorm.Statement{DB: transaction}
		if err := statement.Parse(entry); err != nil {
			return err
		}
		query, identifier := "", any(nil)
		switch {
		case statement.Schema.LookUpField("ProjectID") != nil && len(projectIDs) > 0:
			query, identifier = "project_id IN ?", projectIDs
		case statement.Schema.LookUpField("WorkspaceID") != nil:
			query, identifier = "workspace_id = ?", workspaceID
		default:
			continue
		}
		if err := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().
			Where(query, identifier).Delete(entry).Error; err != nil {
			return err
		}
	}

	workspaceDeletion := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().
		Where("id = ?", workspaceID).Delete(&model.Workspace{})
	if workspaceDeletion.Error != nil {
		return workspaceDeletion.Error
	}
	if workspaceDeletion.RowsAffected != 1 {
		return fmt.Errorf("owned workspace fixture was not deleted exactly once")
	}
	if len(userIDs) == 0 {
		return nil
	}
	userDeletion := transaction.Session(&gorm.Session{SkipHooks: true}).Unscoped().
		Where("id IN ?", userIDs).Delete(&model.UserAccount{})
	if userDeletion.Error != nil {
		return userDeletion.Error
	}
	if userDeletion.RowsAffected != int64(len(userIDs)) {
		return fmt.Errorf("owned user fixtures were not deleted exactly once")
	}
	return nil
}
