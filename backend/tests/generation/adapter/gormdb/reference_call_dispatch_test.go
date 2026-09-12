package gormdb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	fixturegorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestReferenceCallDispatchCASAndWorkspaceLimits(t *testing.T) {
	url := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL for Reference dispatch database concurrency")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, url, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	user, workspace, project, targetID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// This fixture proves storage CAS and accounting, not accepted script facts.
	base := []any{
		&model.UserAccount{ID: user, EmailNormalized: user.String() + "@example.test", PasswordHash: "not-a-login-hash", TokenVersion: 1, DisplayName: "Dispatch fixture", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspace, Name: "Dispatch fixture", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Project{ID: project, WorkspaceID: workspace, Name: "Dispatch fixture", AspectRatio: "1:1", Language: "zh", TargetDurationMS: 1000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Transaction(func(tx *gorm.DB) error {
		for _, record := range base {
			if err := tx.Omit(clause.Associations).Create(record).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fixturegorm.RegisterOwnedFixtureCleanup(t, database, fixturegorm.OwnedFixture{UserID: user.String(), WorkspaceID: workspace.String(), ProjectID: project.String()})
	hash := strings.Repeat("a", 64)
	target := model.GenerationTarget{ID: targetID, WorkspaceID: workspace, ProjectID: project, Kind: "reference_plan", SourceOwnerRef: []byte(`{}`), SourceContentHash: hash, PolicySnapshotRef: []byte(`{}`), PolicyContentHash: hash, Payload: []byte(`{}`), TargetHash: hash, Revision: 1, CreatedBy: user, CreatedAt: now}
	if err = database.Omit(clause.Associations).Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	ref := func() domain.GenerationRevisionRef {
		return domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: hash}
	}
	contract := domain.GenerationContractRef{ContractID: "reference-dispatch-fixture", ContentHash: hash}
	policy, err := domain.DefaultReferenceGenerationLimits().Ref()
	if err != nil {
		t.Fatal(err)
	}
	execution, err := domain.BuildInitialReferenceExecution(domain.InitialReferenceExecutionInput{ID: uuid.NewString(), WorkspaceID: workspace.String(), ProjectID: project.String(), CreatedBy: user.String(), MembershipTokenVersion: 1, CreatedAt: now.Add(-time.Minute), ReadSet: domain.ReferenceExecutionReadSet{TargetRef: domain.GenerationRevisionRef{ID: targetID.String(), Revision: 1, ContentHash: hash}, TargetReadSetRoot: hash, BindingRef: ref(), ConnectionRef: ref(), CredentialRef: domain.ReferenceCredentialRef{ID: uuid.NewString(), Revision: 1, Fingerprint: hash}, ProfileRef: ref(), RegistryReleaseHash: hash, AdapterRef: contract, CompilerRef: contract, CapabilityHash: hash, ManifestHash: hash, OperationalPolicyRef: policy, AuthorizationRef: domain.GenerationActionRef{ID: uuid.NewString(), ContentHash: hash}}})
	if err != nil {
		t.Fatal(err)
	}
	inputs := []domain.ReferenceProviderCallInput{}
	for bundle := 0; bundle < 3; bundle++ {
		for _, slot := range []string{"back", "front", "profile"} {
			inputs = append(inputs, domain.ReferenceProviderCallInput{BundleIndex: bundle, SlotKey: slot, CompiledRequestHash: hash})
		}
	}
	job, calls, err := domain.BuildReferenceProviderJob(domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	store := generationgorm.New(database)
	if err = store.WithinReferenceExecution(ctx, func(repo application.ReferenceExecutionRepository) error {
		if err := repo.PublishInitialReferenceExecution(ctx, execution); err != nil {
			return err
		}
		return repo.PublishReferenceProviderJob(ctx, workspace.String(), project.String(), job, calls)
	}); err != nil {
		t.Fatal(err)
	}
	claimBatch := func(keys []string, limit int64) int64 {
		start := make(chan struct{})
		var workers sync.WaitGroup
		var committed atomic.Int64
		errorsFound := make(chan error, len(keys))
		for _, key := range keys {
			workers.Add(1)
			go func(key string) {
				defer workers.Done()
				<-start
				changed := false
				err := store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
					if err := repo.LockProviderWorkspace(ctx, workspace.String()); err != nil {
						return err
					}
					state, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, key)
					if err != nil {
						return err
					}
					if state.Status != domain.ProviderCallPending {
						return nil
					}
					usage, err := repo.ReferenceCallDispatchUsage(ctx, workspace.String(), now.Truncate(24*time.Hour), now.Truncate(24*time.Hour).Add(24*time.Hour))
					if err != nil {
						return err
					}
					if usage.Unresolved >= limit {
						return nil
					}
					next, send, err := domain.ClaimReferenceCall(state, domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: user.String(), MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(3 * time.Minute)})
					if err != nil {
						return err
					}
					if !send {
						return errors.New("pending call did not yield a transition")
					}
					if err = repo.UpdateReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, state, next); err != nil {
						return err
					}
					changed = true
					return nil
				})
				if err == nil {
					if changed {
						committed.Add(1)
					}
					return
				}
				var stateErr interface{ SQLState() string }
				if errors.Is(err, application.ErrReferenceCallStateConflict) || (errors.As(err, &stateErr) && stateErr.SQLState() == "40001") {
					return
				}
				errorsFound <- err
			}(key)
		}
		close(start)
		workers.Wait()
		close(errorsFound)
		for err := range errorsFound {
			t.Error(err)
		}
		return committed.Load()
	}
	if winners := claimBatch([]string{calls[0].CallKey, calls[0].CallKey, calls[0].CallKey, calls[0].CallKey, calls[0].CallKey, calls[0].CallKey}, 2); winners != 1 {
		t.Fatalf("one call had %d committed send rights", winners)
	}
	keys := []string{}
	for _, call := range calls[1:] {
		keys = append(keys, call.CallKey)
	}
	if winners := claimBatch(keys, 2); winners != 1 {
		t.Fatalf("workspace concurrency admitted %d additional calls", winners)
	}
	if err = store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
		usage, err := repo.ReferenceCallDispatchUsage(ctx, workspace.String(), now, now.Add(time.Hour))
		if err != nil {
			return err
		}
		if usage.Unresolved != 2 || usage.Daily != 2 {
			t.Fatalf("incorrect committed usage: %+v", usage)
		}
		before, err := repo.ReferenceCallDispatchUsage(ctx, workspace.String(), now.Add(-time.Hour), now)
		if err != nil {
			return err
		}
		if before.Daily != 0 {
			t.Fatal("daily accounting end was not exclusive")
		}
		state, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, calls[0].CallKey)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if _, err := domain.DecodeReferenceCallState(raw); err != nil {
			return err
		}
		pending, err := domain.NewReferenceCallState(state.CallKey)
		if err != nil {
			return err
		}
		if err = repo.UpdateReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, pending, state); !errors.Is(err, application.ErrReferenceCallStateConflict) {
			t.Fatalf("stale CAS unexpectedly accepted: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Real, independently committed transactions append exactly one receipt and
	// release the unresolved slot without refunding the daily dispatch count.
	var receipt domain.ReferenceCallReceipt
	if err := store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
		state, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, calls[0].CallKey)
		if err != nil {
			return err
		}
		slot := domain.ReferenceOutputSlot{SlotKey: "back", ViewRole: "back", Required: true, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 10 << 20, SemanticRequirements: []string{"preserve identity"}, QCRubricRefs: []domain.ReferenceOutputQCRubricRef{{ContractID: "reference-qc", ContentHash: hash}}}
		input := domain.ReferenceCallReceiptInput{WorkspaceID: workspace.String(), ProjectID: project.String(), Call: calls[0], SubmissionToken: state.Dispatch.SubmissionToken, Slot: slot, ObservedAt: now.Add(time.Second), Disposition: "staged", Usage: domain.ProviderUsageObservation{ImageCount: 1}}
		input.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + execution.ID + "/" + calls[0].CallKey + "/" + input.SubmissionToken + "/image.png", SHA256: hash, MediaType: "image/png", Bytes: 100, Width: 1024, Height: 1024}
		receipt, err = domain.BuildReferenceCallReceipt(input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("injected receipt rollback")
	if err := store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
		before, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, calls[0].CallKey)
		if err != nil {
			return err
		}
		after, _, err := domain.RecordReferenceCallReceipt(before, receipt)
		if err != nil {
			return err
		}
		if err := repo.UpdateReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, before, after); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("receipt rollback: %v", err)
	}
	var workers sync.WaitGroup
	var committed atomic.Int64
	start := make(chan struct{})
	for range 6 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			changed := false
			err := store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
				if err := repo.LockProviderWorkspace(ctx, workspace.String()); err != nil {
					return err
				}
				before, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, calls[0].CallKey)
				if err != nil {
					return err
				}
				after, update, err := domain.RecordReferenceCallReceipt(before, receipt)
				if err != nil {
					return err
				}
				if !update {
					return nil
				}
				changed = true
				return repo.UpdateReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, before, after)
			})
			if err == nil {
				if changed {
					committed.Add(1)
				}
				return
			}
			var stateErr interface{ SQLState() string }
			if !errors.Is(err, application.ErrReferenceCallStateConflict) && !(errors.As(err, &stateErr) && stateErr.SQLState() == "40001") {
				t.Error(err)
			}
		}()
	}
	close(start)
	workers.Wait()
	if committed.Load() != 1 {
		t.Fatalf("receipt had %d committed writers", committed.Load())
	}
	if winners := claimBatch(keys, 2); winners != 1 {
		t.Fatalf("completed receipt released %d new slots", winners)
	}
	if err := store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
		state, err := repo.FindReferenceCallState(ctx, workspace.String(), project.String(), execution.ID, calls[0].CallKey)
		if err != nil {
			return err
		}
		if state.Status != domain.ProviderCallSucceeded || state.Receipt == nil || state.Receipt.ContentHash != receipt.ContentHash {
			return errors.New("receipt was not committed with state")
		}
		usage, err := repo.ReferenceCallDispatchUsage(ctx, workspace.String(), now, now.Add(time.Hour))
		if err != nil {
			return err
		}
		if usage.Unresolved != 2 || usage.Daily != 3 {
			return errors.New("receipt released daily count or incorrect concurrency")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceCallDispatchStoreRejectsInvalidTransactions(t *testing.T) {
	for _, database := range []*gorm.DB{nil, {}, {Config: &gorm.Config{DisableNestedTransaction: true}}} {
		store := generationgorm.New(database)
		err := store.WithinReferenceCallDispatch(context.Background(), func(application.ReferenceCallDispatchRepository) error {
			t.Fatal("dispatch ran without an atomic transaction boundary")
			return nil
		})
		if err == nil {
			t.Fatal("invalid dispatch transaction configuration accepted")
		}
	}
}

func TestReferenceCallExecutionRequiresPhysicalCommit(t *testing.T) {
	for _, database := range []*gorm.DB{nil, {}, {Config: &gorm.Config{}, Statement: &gorm.Statement{ConnPool: &sql.Tx{}}}} {
		store := generationgorm.New(database)
		if err := store.WithinReferenceCallExecution(context.Background(), func(application.ReferenceCallDispatchRepository) error {
			t.Fatal("execution accepted a savepoint as a commit")
			return nil
		}); err == nil {
			t.Fatal("non-standalone execution transaction accepted")
		}
	}
}
