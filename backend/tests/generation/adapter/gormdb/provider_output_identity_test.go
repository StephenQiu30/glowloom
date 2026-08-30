package gormdb_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
)

func TestGenerationCandidateIdentityIsProviderCallReceiptAndRemoteOutput(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Provider output identity database contract")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Provider output identity database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Provider output identity GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	if err = database.Create(&model.UserAccount{
		ID: userID, EmailNormalized: "provider-output-identity-" + userID.String() + "@example.test",
		PasswordHash: "test", TokenVersion: 1, DisplayName: "Provider Output Identity",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed Provider output identity user: %v", err)
	}
	if err = database.Create(&model.Workspace{
		ID: workspaceID, Name: "Provider Output Identity", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed Provider output identity workspace: %v", err)
	}
	if err = database.Create(&model.Membership{
		ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "editor", Status: "active", JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed Provider output identity membership: %v", err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Provider Output Identity",
		AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000,
		Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed Provider output identity project: %v", err)
	}
	t.Cleanup(func() {
		for _, deletion := range []struct {
			name  string
			value any
			query string
			args  []any
		}{
			{name: "Generation candidates", value: &model.GenerationCandidate{}, query: "workspace_id = ?", args: []any{workspaceID}},
			{name: "Artifacts", value: &model.Artifact{}, query: "workspace_id = ?", args: []any{workspaceID}},
			{name: "Memberships", value: &model.Membership{}, query: "workspace_id = ?", args: []any{workspaceID}},
			{name: "Projects", value: &model.Project{}, query: "id = ?", args: []any{projectID}},
			{name: "Workspaces", value: &model.Workspace{}, query: "id = ?", args: []any{workspaceID}},
			{name: "Users", value: &model.UserAccount{}, query: "id = ?", args: []any{userID}},
		} {
			if deleteErr := generationtestgorm.DeleteWithoutHooks(database, deletion.value, deletion.query, deletion.args...); deleteErr != nil {
				t.Errorf("clean test-owned %s: %v", deletion.name, deleteErr)
			}
		}
	})

	providerJobID := uuid.New()
	providerCallIDs := []uuid.UUID{uuid.New(), uuid.New()}
	providerReceiptIDs := []uuid.UUID{uuid.New(), uuid.New()}
	artifactIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for index := range artifactIDs {
		width, height := 1536, 1024
		if err = database.Create(&model.Artifact{
			ID: artifactIDs[index], WorkspaceID: workspaceID, ProjectID: projectID,
			SourceType: "generation_provider_receipt", SourceID: providerReceiptIDs[index], OutputKey: "output-1",
			MediaType: "image/png", SHA256: strings.Repeat(string(rune('a'+index)), 64), SizeBytes: 1024,
			Status: "READY", Width: &width, Height: &height, Revision: 2, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed Provider output identity Artifact %d: %v", index+1, err)
		}
		if err = database.Create(&model.GenerationCandidate{
			ID: uuid.New(), WorkspaceID: workspaceID, ProjectID: projectID, ProviderJobID: providerJobID,
			ProviderCallID: providerCallIDs[index], ProviderReceiptID: providerReceiptIDs[index], OutputKey: "output-1",
			ArtifactID: artifactIDs[index], ArtifactRevision: 2,
			ArtifactSHA256: strings.Repeat(string(rune('a'+index)), 64), MediaType: "image/png",
			Width: width, Height: height, Status: "QC_PASSED", Revision: 1,
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("persist same-named output for independent Provider Call %d: %v", index+1, err)
		}
	}

	var count int64
	if err = database.Model(&model.GenerationCandidate{}).
		Where("workspace_id = ? AND provider_job_id = ? AND output_key = ?", workspaceID, providerJobID, "output-1").
		Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("same remote OutputKey did not preserve two Call/Receipt candidates: count=%d err=%v", count, err)
	}

	thirdArtifactID, thirdReceiptID := uuid.New(), uuid.New()
	width, height := 1536, 1024
	if err = database.Create(&model.Artifact{
		ID: thirdArtifactID, WorkspaceID: workspaceID, ProjectID: projectID,
		SourceType: "generation_provider_receipt", SourceID: thirdReceiptID, OutputKey: "output-1",
		MediaType: "image/png", SHA256: strings.Repeat("c", 64), SizeBytes: 1024,
		Status: "READY", Width: &width, Height: &height, Revision: 2, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed duplicate Provider output identity Artifact: %v", err)
	}
	if err = database.Create(&model.GenerationCandidate{
		ID: uuid.New(), WorkspaceID: workspaceID, ProjectID: projectID, ProviderJobID: providerJobID,
		ProviderCallID: providerCallIDs[0], ProviderReceiptID: thirdReceiptID, OutputKey: "output-1",
		ArtifactID: thirdArtifactID, ArtifactRevision: 2, ArtifactSHA256: strings.Repeat("c", 64),
		MediaType: "image/png", Width: width, Height: height, Status: "QC_PASSED", Revision: 1,
		CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}).Error; err == nil {
		t.Fatal("duplicate Call/Receipt/remote OutputKey created a second GenerationCandidate")
	}
}
