package microservices_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func awaitFrozenBlock(ctx context.Context, t *testing.T, admin *gorm.DB, holder int, pattern string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := admin.WithContext(ctx).Raw(`SELECT EXISTS(SELECT FROM pg_stat_activity WHERE datname=current_database() AND ?=ANY(pg_blocking_pids(pid)) AND query LIKE ?)`, holder, pattern).Scan(&blocked).Error; err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("no actual frozen-input PostgreSQL blocking edge", ctx.Err())
		}
	}
}

func TestFrozenInputPostgresFundingGateUsesRealReservationAndKeepsOriginalAmounts(t *testing.T) {
	for _, reserved := range []bool{false, true} {
		t.Run(map[bool]string{false: "quoted only", true: "real held reservation"}[reserved], func(t *testing.T) {
			f := newFrozenFixture(t)
			envelope := f.freeze(t)
			key := executionKey(envelope)
			target := executionpg.NewFrozenInputStore(f.execution, localFrozenSource(f.store))
			receipt, err := target.AcceptFrozenInput(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			command := f.cancelCommand(t)
			tx := f.execution.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer func() { _ = tx.Rollback().Error }()
			if err := target.CheckFrozenInputFunding(t.Context(), tx, key, f.request.Command.SHA256); err != nil {
				t.Fatal(err)
			}
			raw, saved, err := target.AcceptedFrozenInput(t.Context(), tx, key)
			if err != nil || saved != receipt {
				t.Fatalf("accessor=%+v err=%v", saved, err)
			}
			original, _, _ := envelope.Input.Snapshot()
			if string(raw) != string(original) {
				t.Fatal("local accepted accessor changed full content")
			}
			if err := tx.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,quote_micros,quote_expires_at) VALUES(?,?,'free','image.generate','text_to_image',?,'upload','quoted',1234,now()+interval '1 hour')`, receipt.OperationID, f.request.ProjectID, envelope.Input.InputHash).Error; err != nil {
				t.Fatal(err)
			}
			var holder int
			if err := tx.Raw(`SELECT pg_backend_pid()`).Scan(&holder).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.BlockFrozenInput(t.Context(), envelope.Key(), command); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := target.CancelFrozenInput(ctx, key, command); done <- err }()
			awaitFrozenBlock(ctx, t, f.admin, holder, "%pg_advisory_xact_lock(69368%")
			reservation := uuid.New()
			if reserved {
				if err := tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,1234,'held')`, reservation, f.request.ProjectID, receipt.OperationID).Error; err != nil {
					t.Fatal(err)
				}
				if err := tx.Exec(`UPDATE operation.operation SET reservation_id=?,confirmed_at=now(),status='confirmed' WHERE id=?`, reservation, receipt.OperationID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
			err = <-done
			if reserved {
				if !errors.Is(err, executionapp.ErrSubmissionAccepted) {
					t.Fatalf("actual reservation was discarded: %v", err)
				}
				var amount int64
				var status string
				if err := f.execution.Raw(`SELECT amount_micros FROM billing.reservation WHERE id=?`, reservation).Scan(&amount).Error; err != nil || amount != 1234 {
					t.Fatalf("reservation amount=%d err=%v", amount, err)
				}
				if err := f.execution.Raw(`SELECT status FROM operation.operation WHERE id=?`, receipt.OperationID).Scan(&status).Error; err != nil || status != "confirmed" {
					t.Fatalf("confirmed operation status=%q err=%v", status, err)
				}
			} else {
				if err != nil {
					t.Fatal("receiving snapshot/quoted operation incorrectly treated as reserved", err)
				}
				if err := f.execution.Transaction(func(tx *gorm.DB) error {
					return target.CheckFrozenInputFunding(t.Context(), tx, key, f.request.Command.SHA256)
				}); !errors.Is(err, executionapp.ErrFrozenInputCancelled) {
					t.Fatalf("cancel-first allowed a late reserve: %v", err)
				}
			}
		})
	}
}
func TestFrozenInputPostgresFundingGateRejectsMissingTxHashAndRevokedEpoch(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	key := executionKey(envelope)
	target := executionpg.NewFrozenInputStore(f.execution, localFrozenSource(f.store))
	if _, err := target.AcceptFrozenInput(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if err := target.CheckFrozenInputFunding(t.Context(), f.execution, key, f.request.Command.SHA256); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("missing Tx err=%v", err)
	}
	if err := f.execution.Transaction(func(tx *gorm.DB) error { return target.CheckFrozenInputFunding(t.Context(), tx, key, "wrong") }); !errors.Is(err, executionapp.ErrFrozenInputConflict) {
		t.Fatalf("wrong original SHA err=%v", err)
	}
	authority := f.request.Command.Authorities[0]
	authority.Epoch++
	if err := pgfence.New(f.execution).Install(t.Context(), writefence.Control{Authority: authority, Blocked: true, CoordinationID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := f.execution.Transaction(func(tx *gorm.DB) error {
		return target.CheckFrozenInputFunding(t.Context(), tx, key, f.request.Command.SHA256)
	}); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("committed input ACK authorized fresh funds after revocation: %v", err)
	}
}
