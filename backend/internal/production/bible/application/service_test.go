package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type fakeBibleStore struct {
	bible    domain.Bible
	receipts map[string]platformcommand.Receipt
}

func (store *fakeBibleStore) WithinTransaction(_ context.Context, operation func(Repository) error) error {
	return operation(store)
}

func (store *fakeBibleStore) RevisionInput(context.Context, Actor, string, bool) (RevisionInput, error) {
	return RevisionInput{}, ErrNotFound
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

func (store *fakeBibleStore) GetBible(_ context.Context, _ Actor, bibleID string, _ bool) (domain.Bible, error) {
	if store.bible.ID != bibleID {
		return domain.Bible{}, ErrNotFound
	}
	value := store.bible
	value.ReviewDecisions = make(map[string]string, len(store.bible.ReviewDecisions))
	for key, decision := range store.bible.ReviewDecisions {
		value.ReviewDecisions[key] = decision
	}
	return value, nil
}

func (store *fakeBibleStore) GetCurrentBible(context.Context, Actor, string) (domain.Bible, error) {
	return store.bible, nil
}

func (store *fakeBibleStore) ConfirmBible(_ context.Context, bible domain.Bible) error {
	store.bible = bible
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

func TestBlockingBibleIssuesRequireExplicitAcceptedDecisions(t *testing.T) {
	resultHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	store := &fakeBibleStore{
		bible: domain.Bible{
			ID: "00000000-0000-0000-0000-000000000001", WorkspaceID: "00000000-0000-0000-0000-000000000002",
			Status: "needs_review", ResultHash: &resultHash, Revision: 1, ReviewDecisions: map[string]string{},
			Candidate: domain.Candidate{ReviewIssues: []domain.ReviewIssue{
				{IssueKey: "issue.one", Severity: "blocking"},
				{IssueKey: "issue.two", Severity: "blocking"},
			}},
		},
		receipts: map[string]platformcommand.Receipt{},
	}
	sequence := 10
	service := NewService(store, Config{
		Now: func() time.Time { return time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC) },
		NewID: func() string {
			sequence++
			return fmt.Sprintf("00000000-0000-0000-0000-%012d", sequence)
		},
	})
	actor := Actor{UserID: "00000000-0000-0000-0000-000000000003", TokenVersion: 1}

	assertConflict := func(err error) {
		t.Helper()
		var apiError *Error
		if !errors.As(err, &apiError) || apiError.Code != "resource_conflict" {
			t.Fatalf("error = %#v, want resource_conflict", err)
		}
	}

	_, err := service.Confirm(context.Background(), actor, ConfirmCommand{BibleID: store.bible.ID, ExpectedResultHash: resultHash, ExpectedRevision: 1, IdempotencyKey: "confirm-1"})
	assertConflict(err)

	first, err := service.DecideReviewIssue(context.Background(), actor, DecideReviewIssueCommand{BibleID: store.bible.ID, IssueKey: "issue.one", Action: "accepted", ExpectedRevision: 1, IdempotencyKey: "issue-one"})
	if err != nil || first.Revision != 2 || first.ReviewDecisions["issue.one"] != "accepted" {
		t.Fatalf("first decision = %#v, error = %v", first, err)
	}
	second, err := service.DecideReviewIssue(context.Background(), actor, DecideReviewIssueCommand{BibleID: store.bible.ID, IssueKey: "issue.two", Action: "rejected", ExpectedRevision: 2, IdempotencyKey: "issue-two-reject"})
	if err != nil || second.ReviewDecisions["issue.two"] != "rejected" {
		t.Fatalf("rejected decision = %#v, error = %v", second, err)
	}
	_, err = service.Confirm(context.Background(), actor, ConfirmCommand{BibleID: store.bible.ID, ExpectedResultHash: resultHash, ExpectedRevision: 3, IdempotencyKey: "confirm-2"})
	assertConflict(err)

	accepted, err := service.DecideReviewIssue(context.Background(), actor, DecideReviewIssueCommand{BibleID: store.bible.ID, IssueKey: "issue.two", Action: "accepted", ExpectedRevision: 3, IdempotencyKey: "issue-two-accept"})
	if err != nil || accepted.Revision != 4 {
		t.Fatalf("accepted decision = %#v, error = %v", accepted, err)
	}
	confirmed, err := service.Confirm(context.Background(), actor, ConfirmCommand{BibleID: store.bible.ID, ExpectedResultHash: resultHash, ExpectedRevision: 4, IdempotencyKey: "confirm-3"})
	if err != nil || confirmed.Status != "confirmed" || confirmed.Revision != 5 {
		t.Fatalf("confirmed bible = %#v, error = %v", confirmed, err)
	}
}
