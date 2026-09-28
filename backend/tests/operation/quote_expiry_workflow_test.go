package operation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestQuoteExpiryWorkflowRunsFlowActivity(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(func(context.Context) (operationapp.ExpiredQuotes, error) {
		return operationapp.ExpiredQuotes{}, nil
	}, activity.RegisterOptions{Name: "flow.ExpireQuotes"})
	env.OnActivity("flow.ExpireQuotes", mock.Anything).Return(operationapp.ExpiredQuotes{Operations: 2, Batches: 1}, nil).Once()
	env.ExecuteWorkflow(operationflow.QuoteExpiryWorkflow)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("quote expiry workflow: %v", err)
	}
	env.AssertExpectations(t)
}

type expiryPageStore struct {
	pages  []operationapp.ExpiredQuotes
	cutoff []time.Time
}

func (s *expiryPageStore) ExpireQuoted(_ context.Context, cutoff time.Time, _ int) (operationapp.ExpiredQuotes, error) {
	s.cutoff = append(s.cutoff, cutoff)
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestQuoteExpiryServiceDrainsOperationsAndBatchesAtOneCutoff(t *testing.T) {
	store := &expiryPageStore{pages: []operationapp.ExpiredQuotes{
		{Operations: 1, Batches: 1},
		{Operations: 1},
		{},
	}}
	cutoff := time.Now().UTC()
	total, err := operationapp.NewQuoteExpiryService(store, 1).Run(t.Context(), cutoff, nil)
	if err != nil || total.Operations != 2 || total.Batches != 1 || len(store.cutoff) != 3 {
		t.Fatalf("drain = %+v, %v, cutoffs=%v", total, err, store.cutoff)
	}
	for _, got := range store.cutoff {
		if !got.Equal(cutoff) {
			t.Fatalf("quote expiry cutoff moved: %v, want %v", got, cutoff)
		}
	}
}
