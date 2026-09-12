package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

func assertReferenceCallDispatchPersistence(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor generationapp.Actor, fixture referencePreparationFixture, configuration referenceExecutionFixture) {
	t.Helper()
	if err := database.SavePoint("reference_dispatch_journey").Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.RollbackTo("reference_dispatch_journey").Error; err != nil {
			t.Error(err)
		}
	}()
	registry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{openaiadapter.NewFactory(nil, nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	now := fixture.execution.CreatedAt.Add(time.Minute)
	preparer := actor
	dispatchUser := uuid.New()
	workspaceID, err := uuid.Parse(fixture.execution.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.UserAccount{ID: dispatchUser, EmailNormalized: dispatchUser.String() + "@example.test", PasswordHash: "not-a-login-hash", TokenVersion: 3, DisplayName: "Reference dispatch editor", Status: "active", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Omit("Workspace", "User").Create(&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: dispatchUser, Role: "editor", Status: "active", JoinedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	actor = generationapp.Actor{UserID: dispatchUser.String(), TokenVersion: 3}
	store := generationgorm.New(database)
	service, err := generationapp.NewReferenceCallDispatchService(store, registry, func() time.Time { return now }, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.GenerationReferenceProviderCall
	if err := database.Where("execution_id = ?", fixture.execution.ID).Order("bundle_index, slot_key").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) < 3 {
		t.Fatal("dispatch fixture needs three independent slots")
	}
	command := generationapp.ClaimReferenceCallCommand{WorkspaceID: fixture.execution.WorkspaceID, ProjectID: fixture.execution.ProjectID, ExecutionRef: domain.GenerationRevisionRef{ID: fixture.execution.ID, Revision: fixture.execution.Revision, ContentHash: fixture.execution.ContentHash}, CallKey: rows[0].CallKey, ExpectedRevision: 1}
	for _, fault := range []string{"actor", "scope", "execution", "call", "revision"} {
		bad, caller := command, actor
		switch fault {
		case "actor":
			caller.TokenVersion++
		case "scope":
			bad.ProjectID = uuid.NewString()
		case "execution":
			bad.ExecutionRef.ContentHash = strings.Repeat("f", 64)
		case "call":
			bad.CallKey = strings.Repeat("f", 64)
		case "revision":
			bad.ExpectedRevision = 2
		}
		if got, err := service.Claim(ctx, caller, bad); err == nil || got.ShouldDispatch {
			t.Fatalf("invalid %s dispatch accepted", fault)
		}
	}
	// A damaged preparation receipt must not be consumed as a send permission.
	if err := database.SavePoint("reference_dispatch_receipt").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.CommandReceipt{}).Where("resource_id = ? AND operation = ?", fixture.execution.ID, generationapp.PrepareInitialReferenceExecutionOperation).UpdateColumns(map[string]any{"input_hash": strings.Repeat("f", 64)}).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := service.Claim(ctx, actor, command); err == nil || got.ShouldDispatch {
		t.Fatal("corrupt preparation receipt authorized dispatch")
	}
	if err := database.RollbackTo("reference_dispatch_receipt").Error; err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"daily_limit", "write_failure", "commit_failure"} {
		failed, err := generationapp.NewReferenceCallDispatchService(referenceDispatchFaultTransactions{ReferenceCallDispatchTransactions: store, fault: fault}, registry, func() time.Time { return now }, uuid.NewString)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := failed.Claim(ctx, actor, command); err == nil || !reflect.DeepEqual(got, generationapp.ReferenceCallDispatchResult{}) {
			t.Fatalf("%s leaked dispatch permission", fault)
		}
		if err := store.WithinReferenceCallDispatch(ctx, func(repo generationapp.ReferenceCallDispatchRepository) error {
			state, err := repo.FindReferenceCallState(ctx, command.WorkspaceID, command.ProjectID, command.ExecutionRef.ID, command.CallKey)
			if err == nil && (state.Status != domain.ProviderCallPending || state.Revision != 1 || state.Dispatch != nil) {
				t.Fatalf("%s left a consumed send boundary", fault)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := service.Claim(ctx, actor, command)
	if err != nil || !first.ShouldDispatch || first.State.Status != domain.ProviderCallDispatching {
		t.Fatalf("first dispatch claim: %v", err)
	}
	if first.State.Dispatch.DispatchedBy != actor.UserID || first.State.Dispatch.MembershipTokenVersion != actor.TokenVersion || fixture.execution.CreatedBy == actor.UserID {
		t.Fatal("dispatch caller overwrote the original preparation actor")
	}
	repeated, err := service.Claim(ctx, actor, command)
	if err != nil || repeated.ShouldDispatch || !reflect.DeepEqual(repeated.State, first.State) {
		t.Fatalf("claim replay granted another send right: %v", err)
	}
	if replayed, err := fixture.service.PrepareInitial(ctx, preparer, fixture.command); err != nil || !reflect.DeepEqual(replayed, fixture.execution) {
		t.Fatalf("preparation replay changed a dispatched execution: %v", err)
	}
	if replayed, err := service.Claim(ctx, actor, command); err != nil || replayed.ShouldDispatch || !reflect.DeepEqual(replayed.State, first.State) {
		t.Fatalf("preparation replay reset a call: %v", err)
	}
	// A new Binding head must not switch an existing Execution's next slot.
	_, err = configuration.configuration.PublishProjectBinding(ctx, preparer, generationapp.PublishProjectProviderBindingCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Purpose: domain.ProviderPurposeReferenceAsset, ConnectionVersionID: configuration.connection.Connection.ID, ModelProfileVersionID: configuration.profile.Profile.ID, ExpectedRevision: configuration.binding.Binding.Revision, ExpectedContentHash: configuration.binding.Binding.ContentHash, IdempotencyKey: "binding-after-reference-dispatch"})
	if err != nil {
		t.Fatal(err)
	}
	secondCommand := command
	secondCommand.CallKey = rows[1].CallKey
	duplicateToken, err := generationapp.NewReferenceCallDispatchService(store, registry, func() time.Time { return now }, func() string { return first.State.Dispatch.SubmissionToken })
	if err != nil {
		t.Fatal(err)
	}
	if got, err := duplicateToken.Claim(ctx, actor, secondCommand); err == nil || !reflect.DeepEqual(got, generationapp.ReferenceCallDispatchResult{}) {
		t.Fatal("duplicate submission token authorized a different call")
	}
	second, err := service.Claim(ctx, actor, secondCommand)
	if err != nil || !second.ShouldDispatch || second.State.Dispatch.SubmissionToken == first.State.Dispatch.SubmissionToken {
		t.Fatalf("frozen Provider dispatch: %v", err)
	}
	third := command
	third.CallKey = rows[2].CallKey
	if got, err := service.Claim(ctx, actor, third); err == nil || got.ShouldDispatch {
		t.Fatal("unresolved concurrency limit was bypassed")
	}
	expire := generationapp.ExpireReferenceCallCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: command.ExecutionRef, CallKey: command.CallKey, SubmissionToken: first.State.Dispatch.SubmissionToken}
	if early, err := service.Expire(ctx, actor, expire); err != nil || !reflect.DeepEqual(early, first.State) {
		t.Fatalf("premature expiry: %v", err)
	}
	badToken := expire
	badToken.SubmissionToken = uuid.NewString()
	if _, err := service.Expire(ctx, actor, badToken); err == nil {
		t.Fatal("wrong submission token recovered another call")
	}
	// Expiry must still be recordable when the original business source is stale.
	if err := database.SavePoint("reference_dispatch_stale").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("current_execution_id = ?", fixture.execution.ID).UpdateColumns(map[string]any{"current_execution_hash": strings.Repeat("f", 64)}).Error; err != nil {
		t.Fatal(err)
	}
	now = first.State.Dispatch.DeadlineAt
	unknown, err := service.Expire(ctx, actor, expire)
	if err != nil || unknown.Status != domain.ProviderCallOutcomeUnknown || unknown.Revision != 3 {
		t.Fatalf("lost outcome recovery: %v", err)
	}
	if again, err := service.Expire(ctx, actor, expire); err != nil || !reflect.DeepEqual(again, unknown) {
		t.Fatalf("repeated recovery changed the state: %v", err)
	}
	if got, err := service.Claim(ctx, actor, command); err != nil || got.ShouldDispatch {
		t.Fatalf("unknown call was resubmitted: %v", err)
	}
	if err := database.RollbackTo("reference_dispatch_stale").Error; err != nil {
		t.Fatal(err)
	}
	// Return to the current Head, mark unknown again, and retain its budget slot.
	if _, err := service.Expire(ctx, actor, expire); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Claim(ctx, actor, third); err == nil || got.ShouldDispatch {
		t.Fatal("unknown outcome automatically freed a concurrency slot")
	}
	if err := store.WithinReferenceCallDispatch(ctx, func(repo generationapp.ReferenceCallDispatchRepository) error {
		usage, err := repo.ReferenceCallDispatchUsage(ctx, command.WorkspaceID, now.UTC().Truncate(24*time.Hour), now.UTC().Truncate(24*time.Hour).Add(24*time.Hour))
		if err == nil && (usage.Unresolved != 2 || usage.Daily != 2) {
			t.Fatalf("dispatch accounting=%+v", usage)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

type referenceDispatchFaultTransactions struct {
	generationapp.ReferenceCallDispatchTransactions
	fault string
}

func (tx referenceDispatchFaultTransactions) WithinReferenceCallDispatch(ctx context.Context, operation func(generationapp.ReferenceCallDispatchRepository) error) error {
	return tx.ReferenceCallDispatchTransactions.WithinReferenceCallDispatch(ctx, func(repo generationapp.ReferenceCallDispatchRepository) error {
		if err := operation(referenceDispatchFaultRepository{ReferenceCallDispatchRepository: repo, fault: tx.fault}); err != nil {
			return err
		}
		if tx.fault == "commit_failure" {
			return errors.New("injected failure after dispatch operation")
		}
		return nil
	})
}

type referenceDispatchFaultRepository struct {
	generationapp.ReferenceCallDispatchRepository
	fault string
}

func (repo referenceDispatchFaultRepository) ReferenceCallDispatchUsage(ctx context.Context, workspace string, start, end time.Time) (generationapp.ReferenceCallDispatchUsage, error) {
	if repo.fault == "daily_limit" {
		return generationapp.ReferenceCallDispatchUsage{Daily: 256}, nil
	}
	return repo.ReferenceCallDispatchRepository.ReferenceCallDispatchUsage(ctx, workspace, start, end)
}

func (repo referenceDispatchFaultRepository) UpdateReferenceCallState(ctx context.Context, workspace, project, execution string, before, after domain.ReferenceCallState) error {
	if err := repo.ReferenceCallDispatchRepository.UpdateReferenceCallState(ctx, workspace, project, execution, before, after); err != nil {
		return err
	}
	if repo.fault == "write_failure" {
		return errors.New("injected failure after dispatch state write")
	}
	return nil
}
