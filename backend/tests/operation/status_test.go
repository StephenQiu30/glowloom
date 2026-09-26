package operation_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestStatusTransitionPaths(t *testing.T) {
	tests := []struct {
		name string
		path []domain.Status
	}{
		{
			name: "asynchronous provider success",
			path: []domain.Status{
				domain.StatusDraft, domain.StatusQuoted, domain.StatusConfirmed,
				domain.StatusSubmitting, domain.StatusSubmitted, domain.StatusSucceeded,
				domain.StatusIngesting, domain.StatusCompleted,
			},
		},
		{
			name: "unknown request reconciled without another submit",
			path: []domain.Status{
				domain.StatusConfirmed, domain.StatusSubmitting, domain.StatusUnknown,
				domain.StatusReconciling, domain.StatusSubmitted, domain.StatusSucceeded,
				domain.StatusIngesting, domain.StatusCompleted,
			},
		},
		{
			name: "manual resolution after uncertain charge",
			path: []domain.Status{
				domain.StatusUnknown, domain.StatusReconciling, domain.StatusManual,
				domain.StatusIngesting, domain.StatusFailed,
			},
		},
		{
			name: "zero cost reuse",
			path: []domain.Status{
				domain.StatusQuoted, domain.StatusConfirmed, domain.StatusCompleted,
			},
		},
		{
			name: "quote expiry",
			path: []domain.Status{domain.StatusQuoted, domain.StatusExpired},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for index := 1; index < len(tt.path); index++ {
				from, to := tt.path[index-1], tt.path[index]
				if err := from.CanTransitionTo(to); err != nil {
					t.Fatalf("%q -> %q: %v", from, to, err)
				}
			}
			for _, status := range tt.path[:len(tt.path)-1] {
				if status.IsTerminal() {
					t.Fatalf("%q must not be terminal", status)
				}
			}
			if !tt.path[len(tt.path)-1].IsTerminal() {
				t.Fatalf("%q must be terminal", tt.path[len(tt.path)-1])
			}
		})
	}
}

func TestStatusRejectsUnsafeTransitionsAndAllowsReplay(t *testing.T) {
	tests := []struct {
		from domain.Status
		to   domain.Status
	}{
		{domain.StatusQuoted, domain.StatusSubmitting},
		{domain.StatusUnknown, domain.StatusSubmitting},
		{domain.StatusUnknown, domain.StatusFailed},
		{domain.StatusSubmitted, domain.StatusCompleted},
		{domain.StatusIngesting, domain.StatusCancelling},
		{domain.StatusIngesting, domain.StatusCancelled},
		{domain.StatusManual, domain.StatusSubmitting},
		{domain.StatusCompleted, domain.StatusFailed},
		{domain.StatusFailed, domain.StatusConfirmed},
		{domain.StatusExpired, domain.StatusQuoted},
	}
	for _, tt := range tests {
		if err := tt.from.CanTransitionTo(tt.to); !errors.Is(err, domain.ErrIllegalTransition) {
			t.Errorf("%q -> %q: got %v, want ErrIllegalTransition", tt.from, tt.to, err)
		}
	}
	if err := domain.StatusSubmitted.CanTransitionTo(domain.StatusSubmitted); err != nil {
		t.Fatalf("replayed transition: %v", err)
	}
}

func TestCancellationRequiresAnExplicitOutcome(t *testing.T) {
	for _, path := range [][]domain.Status{
		{domain.StatusConfirmed, domain.StatusCancelling, domain.StatusCancelled},
		{domain.StatusSubmitted, domain.StatusCancelling, domain.StatusCancelled},
		{domain.StatusSubmitted, domain.StatusCancelling, domain.StatusSubmitted},
		{domain.StatusSubmitted, domain.StatusReconciling, domain.StatusManual},
	} {
		for index := 1; index < len(path); index++ {
			if err := path[index-1].CanTransitionTo(path[index]); err != nil {
				t.Fatalf("%q -> %q: %v", path[index-1], path[index], err)
			}
		}
	}
	if err := domain.StatusSubmitted.CanTransitionTo(domain.StatusCancelled); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("direct submitted -> cancelled: got %v, want ErrIllegalTransition", err)
	}
}

func TestStatusRejectsUnknownValues(t *testing.T) {
	for _, tt := range []struct {
		from domain.Status
		to   domain.Status
	}{
		{"not_a_status", "not_a_status"},
		{"not_a_status", domain.StatusConfirmed},
		{domain.StatusConfirmed, "not_a_status"},
	} {
		if err := tt.from.CanTransitionTo(tt.to); !errors.Is(err, domain.ErrInvalidStatus) {
			t.Errorf("%q -> %q: got %v, want ErrInvalidStatus", tt.from, tt.to, err)
		}
	}
}
