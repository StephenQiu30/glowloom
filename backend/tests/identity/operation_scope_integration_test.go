package identity_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"

	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"

	pgaudit "github.com/StephenQiu30/glowloom-server/backend/internal/audit/adapter/postgres"
	audit "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type operationScopeGate struct {
	store *pgworkspace.ProjectContentAccessStore
}

func (a operationScopeGate) AuthorizeOperation(ctx context.Context, p identity.Principal, id uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, p, id, write)
	return err
}
func operationFixture(t *testing.T) (*gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, *pgoperation.Store) {
	tx, a, team, scope, project, factory := projectConsumerFixture(t)
	a.ScopeID = scope
	request := func(tx *gorm.DB) operation.ProjectAccess {
		return operationScopeGate{pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))}
	}
	worker := func(tx *gorm.DB) operation.ProjectAccess {
		return operationScopeGate{pgworkspace.NewWorkerProjectContentAccessStore(tx, workspace.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })))}
	}
	id := uuid.New()
	if err := tx.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,confirmed_by,confirmed_at) VALUES(?,?,'free','image.generate','standard','scope-test','upload','confirmed',?,now())`, id, project, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	return tx, a, team, id, pgoperation.NewScopedStore(tx, request, worker)
}
func TestOperationRequestAndDurableWorkerScope(t *testing.T) {
	tx, a, team, id, store := operationFixture(t)
	var row struct{ ProjectID uuid.UUID }
	if err := tx.Raw(`SELECT project_id FROM operation.operation WHERE id=?`, id).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindOperation(t.Context(), a, row.ProjectID, id); err != nil {
		t.Fatalf("team member without legacy role: %v", err)
	}
	other := a
	other.ScopeID = uuid.New()
	if _, err := store.FindOperation(t.Context(), other, row.ProjectID, id); err == nil {
		t.Fatal("cross scope operation read")
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, a.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindOperation(t.Context(), a, row.ProjectID, id); err == nil {
		t.Fatal("revoked session operation read")
	}
	if _, err := store.LoadWorkflowOperation(t.Context(), id); err != nil {
		t.Fatalf("worker incorrectly requires request session: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadWorkflowOperation(t.Context(), id); err == nil {
		t.Fatal("removed confirming actor still dispatches")
	}
}
func TestOperationMissingConfirmedActorFailsClosed(t *testing.T) {
	tx, _, _, id, store := operationFixture(t)
	if err := tx.Exec(`UPDATE operation.operation SET confirmed_by=NULL WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadWorkflowOperation(t.Context(), id); err == nil {
		t.Fatal("missing confirmed actor fell back to project author")
	}
}
func TestPromptPreferencesDoNotRequireLegacyRole(t *testing.T) {
	tx, a, _, _, _, _ := projectConsumerFixture(t)
	if _, err := pgprompt.NewScopedStore(tx, func(db *gorm.DB) prompt.ActorAccess { return pgidentity.NewStore(db) }).ReadCustomizations(t.Context(), a); err != nil {
		t.Fatalf("active self preferences require legacy role: %v", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, a.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := pgprompt.NewScopedStore(tx, func(db *gorm.DB) prompt.ActorAccess { return pgidentity.NewStore(db) }).ReadCustomizations(t.Context(), a); err == nil {
		t.Fatal("revoked exact session read preferences")
	}
}

func (a operationScopeGate) AuthorizeCatalog(ctx context.Context, p identity.Principal, id uuid.UUID) error {
	return a.AuthorizeOperation(ctx, p, id, false)
}
func TestCatalogPlatformEpochAndProjectScope(t *testing.T) {
	tx, a, team, scope, project, factory := projectConsumerFixture(t)
	migration, err := os.ReadFile("../../db/migrations/002_ai_catalog_expand.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ReplaceAll(strings.ReplaceAll(string(migration), "BEGIN;", ""), "COMMIT;", "")
	if err := tx.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}

	a.ScopeID = scope
	store := pgcatalog.NewScopedStore(tx, func(db *gorm.DB) catalog.ActorAccess { return pgidentity.NewStore(db) }, func(db *gorm.DB) catalog.ProjectAccess {
		return operationScopeGate{pgworkspace.NewScopedProjectContentAccessStore(db, factory(db))}
	})
	if _, err := store.ListModelsForProject(t.Context(), a, catalog.ListModelsInput{ProjectID: project, Limit: 1}); err != nil {
		t.Fatalf("team model availability: %v", err)
	}
	if _, err := store.ListProvidersForAdmin(t.Context(), a, 1, nil); err == nil {
		t.Fatal("legacy role became platform admin")
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListProvidersForAdmin(t.Context(), a, 1, nil); err != nil {
		t.Fatalf("live platform administrator: %v", err)
	}
	request := catalog.CredentialTestRequest{TestID: uuid.New(), ProviderID: uuid.New(), CredentialID: uuid.New(), ActorID: a.ID, OrgID: a.OrgID, RequestID: uuid.NewString()}
	request.TestID = uuid.NewSHA1(uuid.MustParse(request.RequestID), []byte(a.ID.String()+a.OrgID.String()+request.ProviderID.String()+request.CredentialID.String()))
	eventID := uuid.NewSHA1(request.TestID, []byte("credential_test_requested"))
	receipt, err := json.Marshal(map[string]any{"test_id": request.TestID, "provider_id": request.ProviderID, "credential_id": request.CredentialID, "event_id": eventID, "accepted": true, "platform_epoch": a.PlatformEpoch})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,'scope-test',202,?::jsonb,now()+interval '1 hour')`, uuid.New(), a.ID, request.RequestID, string(receipt)).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyCredentialTestRequest(t.Context(), request, eventID); err != nil {
		t.Fatalf("current accepted credential test: %v", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListProvidersForAdmin(t.Context(), a, 1, nil); err == nil {
		t.Fatal("stale platform epoch accepted")
	}
	if err := store.VerifyCredentialTestRequest(t.Context(), request, eventID); err == nil {
		t.Fatal("queued credential test survived platform epoch change")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListModelsForProject(t.Context(), a, catalog.ListModelsInput{ProjectID: project, Limit: 1}); err == nil {
		t.Fatal("platform privilege bypassed project membership")
	}
}

func TestAuditPlatformMetadataDoesNotExposeContent(t *testing.T) {
	tx, a, _, _, _, _ := projectConsumerFixture(t)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := tx.Exec(`INSERT INTO audit.audit_log(id,org_id,actor_id,actor_kind,action,object_type,object_id,before,after) VALUES(?,?,?,'user','scope.test','project','private-id','{"content":"private before"}','{"prompt":"private after"}')`, id, a.OrgID, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := pgaudit.NewScopedStore(tx, func(db *gorm.DB) audit.ActorAccess { return pgidentity.NewStore(db) })
	row, err := store.GetForAdmin(t.Context(), a, a.OrgID, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.Before) > 0 || len(row.After) > 0 {
		t.Fatal("platform metadata exposed private content")
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, a.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetForAdmin(t.Context(), a, a.OrgID, id); err == nil {
		t.Fatal("revoked platform session read audit")
	}
}

func TestOperationRevocationBlocksDispatchAndLateWriteback(t *testing.T) {
	tx, a, team, id, store := operationFixture(t)
	key := "operation/" + id.String()
	if _, err := store.TransitionWorkflowOperation(t.Context(), operation.TransitionInput{OperationID: id, From: []operationdomain.Status{operationdomain.StatusConfirmed}, To: operationdomain.StatusSubmitting, ProviderRequestKey: &key}); err != nil {
		t.Fatalf("authorized dispatch transition: %v", err)
	}
	receipt := `{"provider_task_id":"retained-task"}`
	if err := tx.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason,detail) VALUES(?,?,'submitting','submitting','scope_test_receipt',?::jsonb)`, uuid.New(), id, receipt).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	task := "retained-task"
	if _, err := store.TransitionWorkflowOperation(t.Context(), operation.TransitionInput{OperationID: id, From: []operationdomain.Status{operationdomain.StatusSubmitting}, To: operationdomain.StatusSubmitted, ProviderTaskID: &task}); err == nil {
		t.Fatal("removed confirming actor wrote late project status")
	}
	var row struct{ Status, Task string }
	if err := tx.Raw(`SELECT o.status,e.detail->>'provider_task_id' AS task FROM operation.operation o JOIN operation.operation_event e ON e.operation_id=o.id AND e.reason='scope_test_receipt' WHERE o.id=?`, id).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "submitting" || row.Task != task {
		t.Fatalf("late write changed status or discarded supplier receipt: %+v", row)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CheckSubmissionInputs(t.Context(), id); err == nil {
		t.Fatal("disabled confirmed actor permitted provider inputs")
	}
}

func TestRevokedActorCanCloseFinancialReservationButCannotPublish(t *testing.T) {
	tx, a, team, id, store := operationFixture(t)
	facts, err := store.ReadSettlementFacts(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	reservation := uuid.New()
	if err := tx.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros,settled_micros,is_overrun,revision) VALUES(?,?,100,10,0,false,1)`, uuid.New(), facts.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,10,'held')`, reservation, facts.ProjectID, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE operation.operation SET status='ingesting',reservation_id=? WHERE id=?`, reservation, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadSettlementFacts(t.Context(), id); err != nil {
		t.Fatalf("revocation hid existing financial liability: %v", err)
	}
	// These are rollback-only supplier facts; no network request is made.
	if err := tx.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,cost_micros,region) VALUES(?,?,?,1,'submit','scope-fixture','{}','{"state":"accepted"}','ok',0,'domestic'),(?,?,?,1,'query','scope-fixture','{}','{"state":"succeeded"}','ok',7,'domestic')`, uuid.New(), facts.ProjectID, id, uuid.New(), facts.ProjectID, id).Error; err != nil {
		t.Fatal(err)
	}
	finalizer := app.NewOperationFinalizer(tx, store, pgbilling.NewStore(tx))
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{OperationID: id.String(), From: operationdomain.StatusIngesting, To: operationdomain.StatusCompleted, ActualCostMicros: 7}); err == nil {
		t.Fatal("revoked actor published completed content")
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{OperationID: id.String(), From: operationdomain.StatusIngesting, To: operationdomain.StatusFailed, FailureCode: "authority_revoked", ActualCostMicros: 7}); err != nil {
		t.Fatalf("revocation blocked financial close: %v", err)
	}
	var row struct {
		Status                        string
		ReservedMicros, SettledMicros int64
	}
	if err := tx.Raw(`SELECT o.status,b.reserved_micros,o.settled_micros FROM operation.operation o JOIN billing.budget b ON b.project_id=o.project_id WHERE o.id=?`, id).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "failed" || row.ReservedMicros != 0 || row.SettledMicros != 7 {
		t.Fatalf("financial closure diverged: %+v", row)
	}
}
