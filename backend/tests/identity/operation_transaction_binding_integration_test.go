package identity_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// Stores retain a distinct pool handle just as runtime DI does. Any read through
// that handle would borrow a second connection while the caller holds its tx.
// Fail immediately instead of relying on a pool-exhaustion timeout.
type settlementRootPoolSpy struct {
	gorm.ConnPool
	reads atomic.Int64
}

func (p *settlementRootPoolSpy) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	p.reads.Add(1)
	return nil, errors.New("settlement attempted a second root-pool borrow")
}

func TestOperationFinalizerUsesCurrentTransactionWithRootPoolStores(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		name := "normal_finalization"
		if revoked {
			name = "revoked_finalization"
		}
		t.Run(name, func(t *testing.T) {
			tx, actor, team, id, fixtureStore := operationFixture(t)
			facts, err := fixtureStore.ReadSettlementFacts(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			reservation := uuid.New()
			must(tx.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros,settled_micros,is_overrun,revision) VALUES(?,?,100,10,0,false,1)`, uuid.New(), facts.ProjectID).Error)
			must(tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,10,'held')`, reservation, facts.ProjectID, id).Error)
			must(tx.Exec(`UPDATE operation.operation SET status='ingesting',reservation_id=?,provider_request_key=? WHERE id=?`, reservation, "operation/"+id.String(), id).Error)
			must(tx.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,cost_micros,region,provider_task_id) VALUES(?,?,?,1,'submit','scope-fixture','{}','{"state":"accepted"}','ok',0,'domestic','accepted-task'),(?,?,?,1,'query','scope-fixture','{}','{"state":"succeeded"}','ok',7,'domestic','accepted-task')`, uuid.New(), facts.ProjectID, id, uuid.New(), facts.ProjectID, id).Error)
			if revoked {
				must(tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error)
			}

			pool := &settlementRootPoolSpy{}
			root, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			must(err)
			finalizer := app.NewOperationFinalizer(tx, reviewOperationStore(root), pgbilling.NewStore(root))
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if revoked {
				must(finalizer.ResolveRevokedOperation(ctx, id.String()))
			} else {
				must(finalizer.FinalizeOperation(ctx, operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusIngesting, To: domain.StatusFailed, ActualCostMicros: 7, FailureCode: "ingest_failed", Reason: "ingest_failed"}))
			}
			if pool.reads.Load() != 0 {
				t.Fatalf("borrowed root pool %d times", pool.reads.Load())
			}
			var row struct {
				Status                            string
				Reserved, Settled, Outputs, Calls int64
			}
			must(tx.Raw(`SELECT o.status,b.reserved_micros AS reserved,b.settled_micros AS settled,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM operation.provider_call WHERE operation_id=o.id) AS calls FROM operation.operation o JOIN billing.budget b ON b.project_id=o.project_id WHERE o.id=?`, id).Scan(&row).Error)
			if row.Status != "failed" || row.Reserved != 0 || row.Settled != 7 || row.Outputs != 0 || row.Calls != 2 {
				t.Fatalf("financial or publication invariants changed: %+v", row)
			}
		})
	}
}
