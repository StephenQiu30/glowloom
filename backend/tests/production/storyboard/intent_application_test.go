package storyboard_test

import (
	"context"
	"errors"
	"testing"
	"time"

	command "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
)

type intentStore struct {
	app.Repository
	source          app.IntentFreezeSource
	receipt         command.Receipt
	denied          bool
	saves, receipts int
	failReceipt     bool
}

func (s *intentStore) WithinTransaction(ctx context.Context, fn func(app.Repository) error) error {
	before := *s
	if err := fn(s); err != nil {
		*s = before
		return err
	}
	return nil
}
func (s *intentStore) FindReceipt(context.Context, string, string, string) (command.Receipt, error) {
	if s.receipt.ID == "" {
		return command.Receipt{}, command.ErrReceiptNotFound
	}
	return s.receipt, nil
}
func (s *intentStore) GetIntentReceipt(context.Context, string) (command.Receipt, error) {
	if s.receipt.ID == "" {
		return command.Receipt{}, app.ErrNotFound
	}
	return s.receipt, nil
}
func (s *intentStore) GetSet(context.Context, app.Actor, string, bool) (domain.DraftSet, error) {
	if s.denied {
		return domain.DraftSet{}, app.ErrNotFound
	}
	return s.source.Set, nil
}
func (s *intentStore) GetIntentFreezeSource(context.Context, app.Actor, string, string, bool) (app.IntentFreezeSource, error) {
	if s.denied {
		return app.IntentFreezeSource{}, app.ErrNotFound
	}
	return s.source, nil
}
func (s *intentStore) SaveSet(_ context.Context, set domain.DraftSet) error {
	s.source.Set = set
	s.saves++
	return nil
}
func (s *intentStore) CreateReceipt(_ context.Context, r command.Receipt) error {
	if s.failReceipt {
		return errors.New("receipt write failed")
	}
	s.receipt = r
	s.receipts++
	return nil
}
func intentServiceFixture() (*app.Service, *intentStore, app.Actor, app.FreezeIntentSetCommand) {
	set := domain.DraftSet{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), GraphVersionID: uuid.NewString(), GraphVersionNo: 1, GraphContentHash: hashOf("1"), ManifestID: uuid.NewString(), ManifestVersion: 1, ManifestHash: hashOf("2"), Revision: 2, Status: "needs_asset"}
	candidateID, candidateHash := uuid.NewString(), hashOf("3")
	set.CandidateRevisionID, set.CandidateRevisionHash, set.ResultHash = &candidateID, &candidateHash, &candidateHash
	set.Batches = []domain.DraftSetBatch{draftSetBatch("scene:a", uuid.NewString(), uuid.NewString())}
	actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	cmd := app.FreezeIntentSetCommand{WorkspaceID: set.WorkspaceID, ProjectID: set.ProjectID, CandidateRevisionID: candidateID, CandidateRevisionHash: candidateHash, ExpectedCandidateRevision: 1, ReviewDecisionID: uuid.NewString(), IdempotencyKey: "accept-text"}
	store := &intentStore{source: app.IntentFreezeSource{Set: set, Batches: []domain.Batch{intentBatch(set, set.Batches[0], "shot:a", "occurrence:a")}, CandidateRevision: 1, CandidateRevisionID: candidateID, CandidateRevisionHash: candidateHash, ReviewDecisionID: cmd.ReviewDecisionID}}
	return app.NewService(store, app.Config{Now: time.Now, NewID: uuid.NewString}), store, actor, cmd
}
func TestTextIntentAcceptanceReplaysWithoutRepeatingOwnerEffects(t *testing.T) {
	service, store, actor, cmd := intentServiceFixture()
	ctx := context.Background()
	first, err := service.FreezeIntentSet(ctx, actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.FreezeIntentSet(ctx, actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.ID != replay.Receipt.ID || store.saves != 1 || store.receipts != 1 || replay.Set.Status != "intent_frozen" || replay.Approved.Scenes[0].AssetReadiness != "needs_asset" {
		t.Fatalf("duplicate or premature visual acceptance: %+v", replay)
	}
	loaded, err := service.GetApprovedIntents(ctx, actor, first.Set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Approved.ContentHash != first.Approved.ContentHash {
		t.Fatal("recovery changed approved text")
	}
	changed := cmd
	changed.CandidateRevisionHash = hashOf("9")
	if _, err = service.FreezeIntentSet(ctx, actor, changed); err == nil {
		t.Fatal("idempotency input mismatch accepted")
	}
	store.denied = true
	if _, err = service.FreezeIntentSet(ctx, actor, cmd); err == nil {
		t.Fatal("revoked user replayed acceptance")
	}
	if _, err = service.GetApprovedIntents(ctx, actor, first.Set.ID); err == nil {
		t.Fatal("revoked user read accepted text")
	}
}
func TestTextIntentReceiptFailureRollsBackFreeze(t *testing.T) {
	service, store, actor, cmd := intentServiceFixture()
	store.failReceipt = true
	if _, err := service.FreezeIntentSet(context.Background(), actor, cmd); err == nil {
		t.Fatal("receipt failure hidden")
	}
	if store.source.Set.Status != "needs_asset" || store.saves != 0 || store.receipts != 0 {
		t.Fatal("partial freeze committed")
	}
}

func TestTextIntentAcceptanceRejectsMalformedHashBeforeRepositoryAccess(t *testing.T) {
	service, store, actor, cmd := intentServiceFixture()
	for _, hash := range []string{hashOf("z"), hashOf("A")} {
		cmd.CandidateRevisionHash = hash
		_, err := service.FreezeIntentSet(context.Background(), actor, cmd)
		var problem *app.Error
		if !errors.As(err, &problem) || problem.Status != 422 || store.saves != 0 {
			t.Fatalf("invalid candidate hash: %v", err)
		}
	}
}
func TestApprovedTextReadRejectsReceiptDrift(t *testing.T) {
	for _, mutate := range []func(*intentStore){
		func(s *intentStore) { s.receipt.WorkspaceID = uuid.NewString() },
		func(s *intentStore) { s.receipt.Operation = "other" },
		func(s *intentStore) { s.receipt.ResourceID = uuid.NewString() },
		func(s *intentStore) { s.receipt.Result = []byte(`{}`) },
		func(s *intentStore) { s.source.Set.ResultHash = nil },
	} {
		service, store, actor, cmd := intentServiceFixture()
		first, err := service.FreezeIntentSet(context.Background(), actor, cmd)
		if err != nil {
			t.Fatal(err)
		}
		mutate(store)
		if _, err = service.GetApprovedIntents(context.Background(), actor, first.Set.ID); err == nil {
			t.Fatal("drifted receipt exposed")
		}
	}
}
