package quota_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

func TestQuotaReservationReadLocksPolicyCounterThenReservation(t *testing.T) {
	workspaceID, projectID, policyID, counterID, reservationID, actorID :=
		uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	windowStart := time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC)
	stop := errors.New("stop after observing quota lock order")
	policy := quotadomain.Policy{
		ID: policyID, WorkspaceID: workspaceID, ProjectID: projectID,
		Metric: quotadomain.MetricGenerationImageCall, WindowKind: quotadomain.WindowUTCDay,
		LimitUnits: 10, Revision: 1,
	}
	var err error
	policy.ContentHash, err = platformcommand.InputHash(struct {
		WorkspaceID, ProjectID, Metric, WindowKind string
		LimitUnits, Revision                       int64
	}{
		WorkspaceID: workspaceID, ProjectID: projectID, Metric: quotadomain.MetricGenerationImageCall,
		WindowKind: quotadomain.WindowUTCDay, LimitUnits: 10, Revision: 1,
	})
	if err != nil {
		t.Fatalf("hash quota policy fixture: %v", err)
	}
	repo := &quotaReservationLockOrderRepository{
		snapshot: quotadomain.Reservation{
			ID: reservationID, WorkspaceID: workspaceID, ProjectID: projectID,
			PolicyID: policyID, CounterID: counterID, Metric: quotadomain.MetricGenerationImageCall,
			WindowStart: windowStart,
		},
		policy: policy,
		counter: quotadomain.Counter{
			ID: counterID, WorkspaceID: workspaceID, ProjectID: projectID,
			PolicyID: policyID, Metric: quotadomain.MetricGenerationImageCall,
			WindowStart: windowStart, WindowEnd: windowStart.Add(24 * time.Hour),
		},
		stop: stop,
	}
	service := quotaapp.NewService(quotaLockOrderTransactions{repo: repo}, quotaapp.Config{})

	_, err = service.GetReservation(context.Background(), quotaapp.Actor{UserID: actorID, TokenVersion: 1}, reservationID)
	if !errors.Is(err, stop) {
		t.Fatalf("quota reservation lock probe returned %v, want sentinel", err)
	}
	want := []string{"reservation_snapshot", "authorize", "policy", "counter", "reservation"}
	if !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("quota reservation lock order = %v, want %v", repo.calls, want)
	}
}

type quotaLockOrderTransactions struct{ repo quotaapp.Repository }

func (manager quotaLockOrderTransactions) WithinQuotaTransaction(
	ctx context.Context,
	operation func(quotaapp.Repository) error,
) error {
	return operation(manager.repo)
}

type quotaReservationLockOrderRepository struct {
	quotaapp.Repository
	snapshot quotadomain.Reservation
	policy   quotadomain.Policy
	counter  quotadomain.Counter
	stop     error
	calls    []string
}

func (repo *quotaReservationLockOrderRepository) GetReservation(
	context.Context,
	string,
) (quotadomain.Reservation, error) {
	repo.calls = append(repo.calls, "reservation_snapshot")
	return repo.snapshot, nil
}

func (repo *quotaReservationLockOrderRepository) AuthorizeProject(
	context.Context,
	quotaapp.Actor,
	string,
	string,
	string,
) error {
	repo.calls = append(repo.calls, "authorize")
	return nil
}

func (repo *quotaReservationLockOrderRepository) GetPolicyForUpdate(
	context.Context,
	string,
	string,
) (quotadomain.Policy, error) {
	repo.calls = append(repo.calls, "policy")
	return repo.policy, nil
}

func (repo *quotaReservationLockOrderRepository) GetCounterForUpdate(
	context.Context,
	string,
	time.Time,
) (quotadomain.Counter, error) {
	repo.calls = append(repo.calls, "counter")
	return repo.counter, nil
}

func (repo *quotaReservationLockOrderRepository) GetReservationForUpdate(
	context.Context,
	string,
) (quotadomain.Reservation, error) {
	repo.calls = append(repo.calls, "reservation")
	return quotadomain.Reservation{}, repo.stop
}
