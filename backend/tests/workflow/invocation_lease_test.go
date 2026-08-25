package workflow_test

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func TestAgentClaimLeaseHasBoundedConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")

	configuration, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.AgentClaimLease != 30*time.Minute {
		t.Fatalf("default AgentClaimLease = %s", configuration.AgentClaimLease)
	}

	t.Setenv("AGENT_CLAIM_LEASE_SECONDS", "900")
	configuration, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.AgentClaimLease != 15*time.Minute {
		t.Fatalf("configured AgentClaimLease = %s", configuration.AgentClaimLease)
	}
}

func TestAgentInvocationCatalogDeclaresLeaseAndFencing(t *testing.T) {
	t.Parallel()

	record := reflect.TypeOf(model.AgentInvocation{})
	for _, fieldName := range []string{"ClaimVersion", "LeaseExpiresAt"} {
		if _, found := record.FieldByName(fieldName); !found {
			t.Errorf("AgentInvocation must declare %s", fieldName)
		}
	}
}

func TestExpiredInvocationIsReclaimedAndStaleResultIsFenced(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow journey")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	workspaceID := uuid.New()
	requestID := uuid.New()
	taskID := uuid.New()
	invocationID := uuid.New()
	if err = database.Create(&model.Workspace{
		ID: workspaceID, Name: "Workflow Lease Test", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err = database.Create(&model.WorkflowTask{
		ID: taskID, WorkspaceID: workspaceID, TaskType: "storyboard_draft",
		RequestType: "storyboard_draft_batch", RequestID: requestID,
		Scope: datatypes.JSON([]byte(`{}`)), Status: "queued", ProgressStage: "queued",
		CancelStatus: "none", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed workflow task: %v", err)
	}
	if err = database.Create(&model.AgentInvocation{
		ID: invocationID, WorkspaceID: workspaceID, RequestType: "storyboard_draft_batch",
		RequestID: requestID, Kind: "storyboard_draft", InputHash: strings.Repeat("a", 64),
		Payload: datatypes.JSON([]byte(`{}`)), Status: "queued", Attempts: 0,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed agent invocation: %v", err)
	}

	store := storyboardgorm.New(database)
	firstLease := now.Add(time.Minute)
	first, found, err := store.ClaimNext(ctx, now, firstLease)
	if err != nil || !found {
		t.Fatalf("claim queued invocation: found=%v err=%v", found, err)
	}
	if first.ID != invocationID.String() || first.ClaimVersion != 1 || first.LeaseExpiresAt == nil || !first.LeaseExpiresAt.Equal(firstLease) {
		t.Fatalf("unexpected first claim: %#v", first)
	}

	if _, found, err = store.ClaimNext(ctx, now.Add(30*time.Second), now.Add(90*time.Second)); err != nil || found {
		t.Fatalf("unexpired invocation was reclaimed: found=%v err=%v", found, err)
	}

	secondLease := now.Add(2 * time.Minute)
	second, found, err := store.ClaimNext(ctx, now.Add(61*time.Second), secondLease)
	if err != nil || !found {
		t.Fatalf("reclaim expired invocation: found=%v err=%v", found, err)
	}
	if second.ID != first.ID || second.ClaimVersion != 2 || second.LeaseExpiresAt == nil || !second.LeaseExpiresAt.Equal(secondLease) {
		t.Fatalf("unexpected reclaimed invocation: %#v", second)
	}

	resultHash := strings.Repeat("b", 64)
	result := contract.Result{
		InvocationID: second.ID, Kind: second.Kind, InputHash: second.InputHash,
		Status: "succeeded", SchemaVersion: contract.SchemaVersion,
		Candidate: []byte(`{"shots":[]}`), ResultHash: &resultHash,
		Executor: contract.Executor{Name: "lease-test", Version: "1", Model: "deterministic"},
	}
	staleApplied, err := store.CompleteInvocation(
		ctx, first.ID, first.ClaimVersion, result, storyboarddomain.Candidate{}, now.Add(62*time.Second),
	)
	if err != nil || staleApplied {
		t.Fatalf("stale claim completion applied: applied=%v err=%v", staleApplied, err)
	}

	var afterStale model.AgentInvocation
	if err = database.First(&afterStale, "id = ?", invocationID).Error; err != nil {
		t.Fatalf("reload invocation after stale result: %v", err)
	}
	if afterStale.Status != "running" || afterStale.ClaimVersion != second.ClaimVersion {
		t.Fatalf("stale result changed current claim: %#v", afterStale)
	}

	currentApplied, err := store.CompleteInvocation(
		ctx, second.ID, second.ClaimVersion, result, storyboarddomain.Candidate{}, now.Add(63*time.Second),
	)
	if err != nil || !currentApplied {
		t.Fatalf("current claim completion failed: applied=%v err=%v", currentApplied, err)
	}
	if err = database.First(&afterStale, "id = ?", invocationID).Error; err != nil {
		t.Fatalf("reload completed invocation: %v", err)
	}
	if afterStale.Status != "succeeded" || afterStale.LeaseExpiresAt != nil {
		t.Fatalf("current result did not finalize invocation: %#v", afterStale)
	}
}
