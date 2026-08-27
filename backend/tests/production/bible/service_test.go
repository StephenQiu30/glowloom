package bible_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type fakeBibleStore struct {
	bible        domain.Bible
	confirmation bibleapp.CandidateConfirmation
	versions     map[string]domain.ProductionBibleVersion
	receipts     map[string]platformcommand.Receipt
}

func (store *fakeBibleStore) WithinTransaction(_ context.Context, operation func(bibleapp.Repository) error) error {
	return operation(store)
}

func (store *fakeBibleStore) RevisionInput(context.Context, bibleapp.Actor, string, bool) (bibleapp.RevisionInput, error) {
	return bibleapp.RevisionInput{}, bibleapp.ErrNotFound
}

func (store *fakeBibleStore) FindReceipt(_ context.Context, _, operation, key string) (platformcommand.Receipt, error) {
	receipt, ok := store.receipts[operation+":"+key]
	if !ok {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return receipt, nil
}

func (store *fakeBibleStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receipts[receipt.Operation+":"+receipt.IdempotencyKey] = receipt
	return nil
}

func (store *fakeBibleStore) CreateWorkflow(context.Context, domain.Bible, domain.Invocation) error {
	return nil
}

func (store *fakeBibleStore) GetBible(_ context.Context, _ bibleapp.Actor, bibleID string, _ bool) (domain.Bible, error) {
	if store.bible.ID != bibleID {
		return domain.Bible{}, bibleapp.ErrNotFound
	}
	value := store.bible
	value.ReviewDecisions = make(map[string]string, len(store.bible.ReviewDecisions))
	for key, decision := range store.bible.ReviewDecisions {
		value.ReviewDecisions[key] = decision
	}
	return value, nil
}

func (store *fakeBibleStore) GetCurrentBible(context.Context, bibleapp.Actor, string) (domain.Bible, error) {
	return store.bible, nil
}

func (store *fakeBibleStore) CandidateConfirmation(
	context.Context,
	bibleapp.Actor,
	bibleapp.ConfirmCommand,
	bool,
) (bibleapp.CandidateConfirmation, error) {
	return store.confirmation, nil
}

func (store *fakeBibleStore) GetBibleVersion(
	_ context.Context,
	_ bibleapp.Actor,
	versionID string,
) (domain.ProductionBibleVersion, error) {
	version, ok := store.versions[versionID]
	if !ok {
		return domain.ProductionBibleVersion{}, bibleapp.ErrNotFound
	}
	return version, nil
}

func (store *fakeBibleStore) CreateBibleVersion(_ context.Context, version domain.ProductionBibleVersion) error {
	store.versions[version.ID] = version
	return nil
}

func (store *fakeBibleStore) UpdateReviewDecisions(_ context.Context, bible domain.Bible) error {
	store.bible = bible
	return nil
}

func (store *fakeBibleStore) ResumeBible(_ context.Context, bible domain.Bible) error {
	store.bible = bible
	return nil
}

func TestProductionBibleConfirmationCreatesOneImmutableVersionAndReceipt(t *testing.T) {
	snapshot := []byte(`{"canonical_entities":[],"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[]}`)
	candidateContentHash, err := agentcontract.CanonicalHash(snapshot)
	if err != nil {
		t.Fatalf("hash candidate snapshot: %v", err)
	}
	store := &fakeBibleStore{
		confirmation: bibleapp.CandidateConfirmation{
			WorkspaceID:           "00000000-0000-0000-0000-000000000002",
			ProjectID:             "00000000-0000-0000-0000-000000000004",
			DocumentRevisionID:    "00000000-0000-0000-0000-000000000005",
			DocumentRevisionHash:  "1111111111111111111111111111111111111111111111111111111111111111",
			CandidateRevisionID:   "00000000-0000-0000-0000-000000000006",
			CandidateRevisionHash: "2222222222222222222222222222222222222222222222222222222222222222",
			CandidateContentHash:  candidateContentHash,
			CandidateRevisionNo:   2, Snapshot: snapshot, NextVersion: 1,
		},
		versions: map[string]domain.ProductionBibleVersion{},
		receipts: map[string]platformcommand.Receipt{},
	}
	sequence := 10
	service := bibleapp.NewService(store, bibleapp.Config{
		Now: func() time.Time { return time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC) },
		NewID: func() string {
			sequence++
			return fmt.Sprintf("00000000-0000-0000-0000-%012d", sequence)
		},
	})
	actor := bibleapp.Actor{UserID: "00000000-0000-0000-0000-000000000003", TokenVersion: 1}

	command := bibleapp.ConfirmCommand{
		CandidateRevisionID:       store.confirmation.CandidateRevisionID,
		CandidateRevisionHash:     store.confirmation.CandidateRevisionHash,
		ExpectedCandidateRevision: store.confirmation.CandidateRevisionNo,
		DocumentRevisionID:        store.confirmation.DocumentRevisionID,
		DocumentRevisionHash:      store.confirmation.DocumentRevisionHash,
		ExpectedVersion:           1, ReviewDecisionID: "00000000-0000-0000-0000-000000000007",
		IdempotencyKey: "confirm-1",
	}
	confirmed, err := service.Confirm(context.Background(), actor, command)
	if err != nil || confirmed.Version.Version != 1 ||
		confirmed.Version.CandidateRevisionID != store.confirmation.CandidateRevisionID ||
		confirmed.Receipt.ID == "" || confirmed.Receipt.Operation != "production_bible.confirm" ||
		confirmed.Receipt.ResourceID != confirmed.Version.ID {
		t.Fatalf("confirmed bible = %#v, error = %v", confirmed, err)
	}
	replayed, err := service.Confirm(context.Background(), actor, command)
	if err != nil || replayed.Version.ID != confirmed.Version.ID || replayed.Receipt.ID != confirmed.Receipt.ID ||
		len(store.versions) != 1 {
		t.Fatalf("replayed bible confirmation = %#v, error = %v", replayed, err)
	}
	drifted := command
	drifted.ExpectedVersion = 2
	_, err = service.Confirm(context.Background(), actor, drifted)
	var apiError *bibleapp.Error
	if !errors.As(err, &apiError) || apiError.Code != "resource_conflict" {
		t.Fatalf("drifted replay error = %#v, want resource_conflict", err)
	}
}
