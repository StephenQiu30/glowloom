package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type reviewWorkerGate struct{ operationScopeGate }

func (a reviewWorkerGate) AuthorizeOperation(ctx context.Context, p identity.Principal, id uuid.UUID, write bool) error {
	err := a.operationScopeGate.AuthorizeOperation(ctx, p, id, write)
	if errors.Is(err, workspace.ErrScopeNotFound) || errors.Is(err, workspace.ErrProjectNotFound) || errors.Is(err, identity.ErrForbidden) {
		return operation.ErrExecutionAuthorityRevoked
	}
	return err
}
func reviewOperationStore(tx *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(tx, nil, func(db *gorm.DB) operation.ProjectAccess {
		return reviewWorkerGate{operationScopeGate{pgworkspace.NewWorkerProjectContentAccessStore(db, workspace.NewScopeAccess(pgworkspace.NewScopeStore(db, func(db *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(db) })))}}
	})
}

func TestManagedWorkflowRevocationClosesPersistedReservation(t *testing.T) {
	for _, point := range []string{"after_query", "before_complete", "succeeded_before_ingest", "unknown_after_accept", "unknown_before_reconcile"} {
		t.Run(point, func(t *testing.T) {
			tx, actor, team, id, _ := operationFixture(t)
			store := reviewOperationStore(tx)
			facts, err := store.ReadSettlementFacts(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			reservation := uuid.New()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(tx.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros,settled_micros,is_overrun,revision) VALUES(?,?,100,10,0,false,1)`, uuid.New(), facts.ProjectID).Error)
			must(tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,10,'held')`, reservation, facts.ProjectID, id).Error)
			status := domain.StatusSubmitted
			switch point {
			case "before_complete":
				status = domain.StatusIngesting
			case "succeeded_before_ingest":
				status = domain.StatusSucceeded
			case "unknown_after_accept":
				status = domain.StatusSubmitting
			case "unknown_before_reconcile":
				status = domain.StatusUnknown
			}
			must(tx.Exec(`UPDATE operation.operation SET status=?,reservation_id=?,provider_request_key=? WHERE id=?`, status, reservation, "operation/"+id.String(), id).Error)
			must(tx.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,cost_micros,region,provider_task_id) VALUES(?,?,?,1,'submit','scope-fixture','{}','{"state":"accepted"}','ok',0,'domestic','accepted-task')`, uuid.New(), facts.ProjectID, id).Error)
			recordQuery := func(ctx context.Context) error {
				return tx.WithContext(ctx).Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,cost_micros,region,provider_task_id) VALUES(?,?,?,1,'query','scope-fixture','{}','{"state":"succeeded"}','ok',7,'domestic','accepted-task')`, uuid.New(), facts.ProjectID, id).Error
			}
			revoke := func(ctx context.Context) error {
				return tx.WithContext(ctx).Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error
			}
			if point == "before_complete" || point == "succeeded_before_ingest" {
				must(recordQuery(t.Context()))
			}
			finalizer := app.NewOperationFinalizer(tx, store, pgbilling.NewStore(tx))
			activities := operationflow.NewActivities(store, finalizer)
			env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
			register := func(name string, fn any) { env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name}) }
			model, price := uuid.New(), uuid.New()
			loaded := operation.WorkflowOperation{Operation: domain.Operation{ID: id, ProjectID: facts.ProjectID, TargetType: "free", Status: status, ModelProfileVersionID: &model, PriceRuleVersionID: &price}, Provider: operation.WorkflowProvider{AdapterKey: "openrouter", Queue: "provider"}, ProviderRequestKey: "operation/" + id.String()}
			register("flow.LoadOperation", func(ctx context.Context, _ string) (operation.WorkflowOperation, error) {
				if point == "succeeded_before_ingest" || point == "unknown_before_reconcile" {
					if err := revoke(ctx); err != nil {
						return loaded, err
					}
				}
				return loaded, nil
			})
			register("flow.Transition", activities.Transition)
			register("flow.SettleOperation", activities.SettleOperation)
			register("flow.ResolveRevokedOperation", activities.ResolveRevokedOperation)
			register("flow.LoadProviderCost", activities.LoadProviderCost)
			register("flow.BeginProviderCall", func(context.Context, operation.BeginProviderCallInput) error { return nil })
			register("flow.CompleteProviderCall", func(context.Context, operation.CompleteProviderCallInput) error { return nil })
			submitted, queried, ingested := 0, 0, 0
			register("provider.managed", func(ctx context.Context, in operation.ManagedStep) (operation.ManagedProgress, error) {
				if in.Action == "submit" {
					submitted++
					return operation.ManagedProgress{}, errors.New("unexpected provider submission")
				}
				if point == "unknown_after_accept" {
					return operation.ManagedProgress{State: "accepted", TaskID: "accepted-task"}, revoke(ctx)
				}
				if in.Action == "query" {
					queried++
					if err := recordQuery(ctx); err != nil {
						return operation.ManagedProgress{}, err
					}
					if err := revoke(ctx); err != nil {
						return operation.ManagedProgress{}, err
					}
					return operation.ManagedProgress{State: "succeeded", TaskID: "accepted-task"}, nil
				}
				return operation.ManagedProgress{State: "accepted", TaskID: "accepted-task"}, nil
			})
			register("media.IngestManaged", func(ctx context.Context, _ uuid.UUID) error { ingested++; return revoke(ctx) })
			env.SetTestTimeout(10 * time.Second)
			env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			var row struct {
				Status                            string
				Reserved, Settled, Outputs, Calls int64
				Task                              string
			}
			must(tx.Raw(`SELECT o.status,b.reserved_micros AS reserved,b.settled_micros AS settled,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM operation.provider_call WHERE operation_id=o.id) AS calls,(SELECT provider_task_id FROM operation.provider_call WHERE operation_id=o.id AND action='submit') AS task FROM operation.operation o JOIN billing.budget b ON b.project_id=o.project_id WHERE o.id=?`, id).Scan(&row).Error)
			unknown := point == "unknown_after_accept" || point == "unknown_before_reconcile"
			if unknown {
				if row.Status != "unknown" || row.Reserved != 10 || row.Settled != 0 || row.Calls != 1 {
					t.Fatalf("uncertain evidence was fabricated or released: %+v", row)
				}
			} else {
				if row.Status != "failed" || row.Reserved != 0 || row.Settled != 7 || row.Calls != 2 {
					t.Fatalf("workflow failed to close known spend: %+v", row)
				}
			}
			if row.Outputs != 0 || row.Task != "accepted-task" || submitted != 0 || queried > 1 || ingested > 1 {
				t.Fatalf("repeated submission/publication/lost receipt: %+v submits %d queries %d ingest %d", row, submitted, queried, ingested)
			}
		})
	}
}

func TestRevokedDeliveryValidatesReceiptBeforeNoop(t *testing.T) {
	tx, actor, team, id, _ := operationFixture(t)
	store := reviewOperationStore(tx)
	facts, err := store.ReadSettlementFacts(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	requestID, eventID := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]any{"accepted": true, "event_id": eventID, "target_id": id, "target_type": "operation", "action": "cancel"})
	if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,'scope-review',202,?::jsonb,now()+interval '1 hour')`, uuid.New(), actor.ID, requestID.String(), string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	control := operation.WorkflowControlDelivery{EventID: eventID, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: facts.ProjectID, TargetID: id, TargetType: "operation", Action: "cancel", RequestID: requestID}
	if _, err := store.CanDeliverWorkflowControl(t.Context(), control); !errors.Is(err, operation.ErrExecutionAuthorityRevoked) {
		t.Fatalf("bound obsolete control: %v", err)
	}
	control.EventID = uuid.New()
	if _, err := store.CanDeliverWorkflowControl(t.Context(), control); !errors.Is(err, operation.ErrInvalidWorkflowControl) {
		t.Fatalf("forged receipt treated obsolete: %v", err)
	}
	request := catalog.CredentialTestRequest{ActorID: actor.ID, OrgID: actor.OrgID, ProviderID: uuid.New(), CredentialID: uuid.New(), RequestID: uuid.NewString()}
	request.TestID = uuid.NewSHA1(uuid.MustParse(request.RequestID), []byte(actor.ID.String()+actor.OrgID.String()+request.ProviderID.String()+request.CredentialID.String()))
	eventID = uuid.NewSHA1(request.TestID, []byte("credential_test_requested"))
	body, _ = json.Marshal(map[string]any{"accepted": true, "event_id": eventID, "test_id": request.TestID, "provider_id": request.ProviderID, "credential_id": request.CredentialID, "platform_epoch": actor.PlatformEpoch})
	if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,'scope-review',202,?::jsonb,now()+interval '1 hour')`, uuid.New(), actor.ID, request.RequestID, string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	catalogStore := pgcatalog.NewScopedStore(tx, func(db *gorm.DB) catalog.ActorAccess { return pgidentity.NewStore(db) }, nil)
	if err := catalogStore.VerifyCredentialTestRequest(t.Context(), request, eventID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := catalogStore.VerifyCredentialTestRequest(t.Context(), request, eventID); !errors.Is(err, catalog.ErrCredentialTestRevoked) {
		t.Fatalf("bound obsolete catalog: %v", err)
	}
	if err := catalogStore.VerifyCredentialTestRequest(t.Context(), request, uuid.New()); !errors.Is(err, catalog.ErrInvalidCredentialTest) {
		t.Fatalf("forged catalog event treated obsolete: %v", err)
	}
	// The actual key-owning activity must deny before touching DB config, key or HTTP.
	probe := providerprobe.NewScopedActivities(nil, nil, nil, catalogStore)
	if _, err := probe.TestCredential(t.Context(), catalogflow.AgentTestInput{TestID: request.TestID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: request.RequestID, ProviderID: request.ProviderID, Credential: catalogflow.AgentCredential{ID: request.CredentialID}}); !errors.Is(err, catalog.ErrCredentialTestRevoked) {
		t.Fatalf("actual probe bypassed current epoch: %v", err)
	}
}
