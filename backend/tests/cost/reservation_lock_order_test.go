package cost_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestCostReservationReadLocksBudgetBeforeReservation(t *testing.T) {
	workspaceID, projectID, budgetID, reservationID, actorID :=
		uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	stop := errors.New("stop after observing cost lock order")
	budget := costdomain.BudgetPolicy{
		ID: budgetID, WorkspaceID: workspaceID, ProjectID: projectID,
		LimitAmount: decimal.RequireFromString("100.000000"), Currency: "USD", Revision: 1,
		CreatedBy: actorID, UpdatedBy: actorID,
	}
	var err error
	budget.ContentHash, err = platformcommand.InputHash(struct {
		WorkspaceID, ProjectID, LimitAmount, Currency string
		Revision                                      int64
	}{
		WorkspaceID: workspaceID, ProjectID: projectID, LimitAmount: "100.000000",
		Currency: "USD", Revision: 1,
	})
	if err != nil {
		t.Fatalf("hash cost budget fixture: %v", err)
	}
	repo := &costReservationLockOrderRepository{
		snapshot: costdomain.Reservation{
			ID: reservationID, WorkspaceID: workspaceID, ProjectID: projectID,
			BudgetPolicyID: budgetID,
		},
		scope:  costapp.ProjectScope{WorkspaceID: workspaceID, ProjectID: projectID},
		budget: budget,
		stop:   stop,
	}
	service := costapp.NewService(costLockOrderTransactions{repo: repo}, costapp.Config{})

	_, err = service.GetReservation(context.Background(), costapp.Actor{UserID: actorID, TokenVersion: 1}, reservationID)
	if !errors.Is(err, stop) {
		t.Fatalf("cost reservation lock probe returned %v, want sentinel", err)
	}
	want := []string{"reservation_snapshot", "authorize", "budget", "reservation"}
	if !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("cost reservation lock order = %v, want %v", repo.calls, want)
	}
}

type costLockOrderTransactions struct{ repo costapp.Repository }

func (manager costLockOrderTransactions) WithinCostTransaction(
	ctx context.Context,
	operation func(costapp.Repository) error,
) error {
	return operation(manager.repo)
}

type costReservationLockOrderRepository struct {
	costapp.Repository
	snapshot costdomain.Reservation
	scope    costapp.ProjectScope
	budget   costdomain.BudgetPolicy
	stop     error
	calls    []string
}

func (repo *costReservationLockOrderRepository) FindReservation(
	context.Context,
	string,
) (costdomain.Reservation, error) {
	repo.calls = append(repo.calls, "reservation_snapshot")
	return repo.snapshot, nil
}

func (repo *costReservationLockOrderRepository) AuthorizeProject(
	context.Context,
	costapp.Actor,
	string,
	string,
) (costapp.ProjectScope, error) {
	repo.calls = append(repo.calls, "authorize")
	return repo.scope, nil
}

func (repo *costReservationLockOrderRepository) GetBudgetForUpdate(
	context.Context,
	string,
) (costdomain.BudgetPolicy, error) {
	repo.calls = append(repo.calls, "budget")
	return repo.budget, nil
}

func (repo *costReservationLockOrderRepository) GetReservationForUpdate(
	context.Context,
	string,
) (costdomain.Reservation, error) {
	repo.calls = append(repo.calls, "reservation")
	return costdomain.Reservation{}, repo.stop
}
