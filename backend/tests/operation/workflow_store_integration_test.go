package operation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestWorkflowFinalizationRollbackAndReplay(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, reservationID := operationStoreRows(t, database, actor, projectID)
	store := pgoperation.NewStore(database)
	billingStore := pgbilling.NewStore(database)
	if err := database.Exec(`UPDATE billing.budget SET reserved_micros = 10 WHERE project_id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("seed reserved budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, operation_id)
		VALUES (?::uuid, ?::uuid, 'reserve', 10, ?::uuid)
	`, uuid.NewString(), projectID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("seed reserve ledger: %v", err)
	}
	input := application.FinalizeInput{
		OperationID: operationID, From: []domain.Status{domain.StatusConfirmed},
		To: domain.StatusCompleted,
	}
	region := "domestic"
	settlement := billingapp.SettleInput{
		ProjectID: projectID, OperationID: operationID, ActualCostMicros: 7,
		Capability: "image.generate", Region: &region, OccurredAt: time.Now().UTC(),
	}
	rollback := errors.New("simulate failed billing")
	err := database.Transaction(func(tx *gorm.DB) error {
		prepared, err := store.PrepareFinalizationInTransaction(t.Context(), tx, input)
		if err != nil || prepared.AlreadyFinalized || prepared.ProjectID != projectID || prepared.ReservationID != reservationID {
			t.Fatalf("prepare finalization = %+v: %v", prepared, err)
		}
		result, err := billingStore.SettleInTransaction(t.Context(), tx, settlement)
		if err != nil || result.ChargeMicros != 7 {
			t.Fatalf("settle before simulated rollback = %+v: %v", result, err)
		}
		input.SettledMicros = result.ChargeMicros
		if err := store.FinalizeInTransaction(t.Context(), tx, input); err != nil {
			t.Fatalf("finalize before simulated rollback: %v", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("outer transaction error = %v", err)
	}
	var status string
	if err := database.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`, operationID.String()).Scan(&status).Error; err != nil || status != "confirmed" {
		t.Fatalf("rolled-back finalization status=%q err=%v", status, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if _, err := store.PrepareFinalizationInTransaction(t.Context(), tx, input); err != nil {
			return err
		}
		result, err := billingStore.SettleInTransaction(t.Context(), tx, settlement)
		if err != nil {
			return err
		}
		input.SettledMicros = result.ChargeMicros
		return store.FinalizeInTransaction(t.Context(), tx, input)
	}); err != nil {
		t.Fatalf("commit finalization: %v", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		prepared, err := store.PrepareFinalizationInTransaction(t.Context(), tx, input)
		if err != nil || !prepared.AlreadyFinalized || prepared.SettledMicros == nil || *prepared.SettledMicros != 7 {
			t.Fatalf("prepare replay = %+v: %v", prepared, err)
		}
		result, err := billingStore.SettleInTransaction(t.Context(), tx, settlement)
		if err != nil || !result.AlreadySettled {
			t.Fatalf("billing replay = %+v: %v", result, err)
		}
		return store.FinalizeInTransaction(t.Context(), tx, input)
	}); err != nil {
		t.Fatalf("finalization replay: %v", err)
	}
	var events int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_event WHERE operation_id = ?::uuid`, operationID.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatalf("finalization events=%d err=%v", events, err)
	}
	var ledgerSettles, ledgerReleases int64
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'settle'`, operationID.String()).Scan(&ledgerSettles).Error; err != nil {
		t.Fatalf("count settlement ledger: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'release'`, operationID.String()).Scan(&ledgerReleases).Error; err != nil {
		t.Fatalf("count release ledger: %v", err)
	}
	if ledgerSettles != 1 || ledgerReleases != 1 {
		t.Fatalf("replay wrote settle=%d release=%d", ledgerSettles, ledgerReleases)
	}
}

func TestLoadWorkflowOperationUsesFrozenModelAndPrivateTaskID(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, inputID, _ := operationStoreRows(t, database, actor, projectID)
	var frozen struct {
		ModelProfileVersionID uuid.UUID
		PriceRuleVersionID    uuid.UUID
	}
	if err := database.Raw(`SELECT model_profile_version_id, price_rule_version_id FROM operation.operation WHERE id = ?::uuid`, operationID.String()).Scan(&frozen).Error; err != nil {
		t.Fatalf("read frozen version IDs: %v", err)
	}
	providerID, profileID := uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO catalog.provider (id, key, name, adapter_key, region) VALUES (?::uuid, ?, 'Mock', 'mock', 'domestic')`, providerID.String(), "mock-"+providerID.String()).Error; err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if err := database.Exec(`INSERT INTO catalog.capability (id, key, output_type, modes, input_roles) VALUES (?::uuid, ?, 'image', ARRAY['text_to_image'], ARRAY['prompt']) ON CONFLICT (key) DO NOTHING`, uuid.NewString(), "image.generate").Error; err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if err := database.Exec(`INSERT INTO catalog.model_profile (id, model_key, provider_id, capability, display_name, status) VALUES (?::uuid, ?, ?::uuid, 'image.generate', 'Mock image', 'active')`, profileID.String(), "mock-"+profileID.String(), providerID.String()).Error; err != nil {
		t.Fatalf("insert model profile: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits, param_schema,
		   supports_query, supports_cancel, supports_callback, expected_max_ms, moderation, queue)
		VALUES (?::uuid, ?::uuid, 1, 'mock-image-v1', ARRAY['text_to_image'], '{}'::jsonb, '[]'::jsonb,
		        true, false, false, 60000, 'platform', 'agent.mock')
	`, frozen.ModelProfileVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("insert model version: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 1, 'per_image', '{}'::jsonb, now() - interval '1 minute')
	`, frozen.PriceRuleVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("insert price rule: %v", err)
	}
	store := pgoperation.NewStore(database)
	loaded, err := store.LoadWorkflowOperation(t.Context(), operationID)
	if err != nil {
		t.Fatalf("load workflow operation: %v", err)
	}
	if loaded.Operation.ID != operationID || loaded.Provider.AdapterKey != "mock" ||
		loaded.Provider.ModelKey != "mock-"+profileID.String() ||
		loaded.Provider.ProviderModelID != "mock-image-v1" || loaded.Provider.Queue != "agent.mock" ||
		!loaded.Provider.SupportsQuery || loaded.Provider.SupportsCancel || loaded.PriceUnit != "per_image" ||
		loaded.ProviderRequestKey != "operation/"+operationID.String() || loaded.ProviderTaskID != nil ||
		len(loaded.Inputs) != 1 || loaded.Inputs[0].ID != inputID {
		t.Fatalf("loaded frozen operation = %+v", loaded)
	}
	requestKey, taskID := loaded.ProviderRequestKey, "mock-task-1"
	for _, transition := range []application.TransitionInput{
		{OperationID: operationID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey},
		{OperationID: operationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted, ProviderTaskID: &taskID},
	} {
		if _, err := store.TransitionWorkflowOperation(t.Context(), transition); err != nil {
			t.Fatalf("transition %s: %v", transition.To, err)
		}
	}
	loaded, err = store.LoadWorkflowOperation(t.Context(), operationID)
	if err != nil || loaded.ProviderTaskID == nil || *loaded.ProviderTaskID != taskID || loaded.Operation.Status != domain.StatusSubmitted {
		t.Fatalf("reload submitted operation = %+v: %v", loaded, err)
	}
	wrongTaskID := "different-task"
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusSubmitting},
		To: domain.StatusSubmitted, ProviderTaskID: &wrongTaskID,
	}); !errors.Is(err, pgoperation.ErrTransitionConflict) {
		t.Fatalf("different provider task accepted on replay: %v", err)
	}
	var published string
	if err := database.Raw(`
		SELECT payload::text FROM infra.outbox
		WHERE topic = 'lanverse.operation.status_changed.v1'
		  AND payload -> 'aggregate' ->> 'id' = ?
		ORDER BY create_time DESC LIMIT 1
	`, operationID.String()).Scan(&published).Error; err != nil {
		t.Fatalf("read public status event: %v", err)
	}
	if strings.Contains(published, taskID) || strings.Contains(published, requestKey) ||
		!strings.Contains(published, `"kind": "system"`) || !strings.Contains(published, `"org_id"`) {
		t.Fatalf("public status event leaked task data or lacks envelope: %s", published)
	}
	manualTaskID := "mock-task-manually-resolved"
	for _, transition := range []application.TransitionInput{
		{OperationID: operationID, From: []domain.Status{domain.StatusSubmitted}, To: domain.StatusReconciling},
		{OperationID: operationID, From: []domain.Status{domain.StatusReconciling}, To: domain.StatusManual},
		{OperationID: operationID, From: []domain.Status{domain.StatusManual}, To: domain.StatusIngesting, ProviderTaskID: &manualTaskID},
	} {
		if _, err := store.TransitionWorkflowOperation(t.Context(), transition); err != nil {
			t.Fatalf("manual recovery transition %s: %v", transition.To, err)
		}
	}
	loaded, err = store.LoadWorkflowOperation(t.Context(), operationID)
	if err != nil || loaded.ProviderTaskID == nil || *loaded.ProviderTaskID != manualTaskID ||
		loaded.Operation.Status != domain.StatusIngesting {
		t.Fatalf("reload manually recovered operation = %+v: %v", loaded, err)
	}
}

func TestWorkflowTransitionIsConditionalAndReplaySafe(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, _ := operationStoreRows(t, database, actor, projectID)
	store := pgoperation.NewStore(database)
	requestKey := "operation/" + operationID.String()
	start := application.TransitionInput{
		OperationID:        operationID,
		From:               []domain.Status{domain.StatusConfirmed},
		To:                 domain.StatusSubmitting,
		ProviderRequestKey: &requestKey,
	}
	for i := 0; i < 2; i++ {
		status, err := store.TransitionWorkflowOperation(t.Context(), start)
		if err != nil || status != domain.StatusSubmitting {
			t.Fatalf("start attempt %d: status=%q err=%v", i, status, err)
		}
	}
	var events, outbox int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_event WHERE operation_id = ?::uuid`, operationID.String()).Scan(&events).Error; err != nil {
		t.Fatalf("count operation events: %v", err)
	}
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic = 'lanverse.operation.status_changed.v1' AND payload -> 'aggregate' ->> 'id' = ?`, operationID.String()).Scan(&outbox).Error; err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if events != 1 || outbox != 1 {
		t.Fatalf("replay wrote events=%d outbox=%d", events, outbox)
	}
	var persistedKey string
	if err := database.Raw(`SELECT provider_request_key FROM operation.operation WHERE id = ?::uuid`, operationID.String()).Scan(&persistedKey).Error; err != nil || persistedKey != requestKey {
		t.Fatalf("provider request key=%q err=%v", persistedKey, err)
	}
	wrongKey := "operation/" + uuid.NewString()
	start.ProviderRequestKey = &wrongKey
	if _, err := store.TransitionWorkflowOperation(t.Context(), start); !errors.Is(err, pgoperation.ErrTransitionConflict) {
		t.Fatalf("different request key accepted on replay: %v", err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusQuoted}, To: domain.StatusConfirmed,
	}); !errors.Is(err, pgoperation.ErrTransitionConflict) {
		t.Fatalf("stale transition accepted: %v", err)
	}
}

func TestWorkflowTransitionNeverEntersTerminalWithoutSettlement(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, _ := operationStoreRows(t, database, actor, projectID)
	store := pgoperation.NewStore(database)
	if _, err := store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusCompleted,
	}); !errors.Is(err, pgoperation.ErrTerminalTransitionRequiresSettlement) {
		t.Fatalf("terminal transition bypassed settlement: %v", err)
	}
	var status string
	if err := database.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`, operationID.String()).Scan(&status).Error; err != nil || status != "confirmed" {
		t.Fatalf("terminal bypass changed status=%q err=%v", status, err)
	}
}

func TestWorkflowManualEventIsReplaySafe(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, _ := operationStoreRows(t, database, actor, projectID)
	store := pgoperation.NewStore(database)
	requestKey := "operation/" + operationID.String()
	for _, transition := range []application.TransitionInput{
		{OperationID: operationID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey},
		{OperationID: operationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusUnknown},
		{OperationID: operationID, From: []domain.Status{domain.StatusUnknown}, To: domain.StatusReconciling},
		{OperationID: operationID, From: []domain.Status{domain.StatusReconciling}, To: domain.StatusManual},
		{OperationID: operationID, From: []domain.Status{domain.StatusReconciling}, To: domain.StatusManual},
	} {
		if _, err := store.TransitionWorkflowOperation(t.Context(), transition); err != nil {
			t.Fatalf("transition to %s: %v", transition.To, err)
		}
	}
	var manualEvents int64
	if err := database.Raw(`
		SELECT count(*) FROM infra.outbox
		WHERE topic = 'lanverse.operation.manual.v1'
		  AND payload -> 'aggregate' ->> 'id' = ?
	`, operationID.String()).Scan(&manualEvents).Error; err != nil || manualEvents != 1 {
		t.Fatalf("manual events=%d err=%v", manualEvents, err)
	}
}

func TestWorkflowSubmissionChecksFrozenMediaInsideTransition(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, inputID, _ := operationStoreRows(t, database, actor, projectID)
	assetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO media.media_asset
		  (id, project_id, kind, origin, status, object_key, mime_type, byte_size, moderation_status)
		VALUES (?::uuid, ?::uuid, 'image', 'upload', 'processing', ?, 'image/png', 10, 'pending')
	`, assetID.String(), projectID.String(), "workflow-input/"+assetID.String()).Error; err != nil {
		t.Fatalf("create frozen media: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation_input SET media_asset_id = ?::uuid WHERE id = ?::uuid`, assetID.String(), inputID.String()).Error; err != nil {
		t.Fatalf("attach frozen media: %v", err)
	}
	store := pgoperation.NewStore(database)
	requestKey := "operation/" + operationID.String()
	start := application.TransitionInput{
		OperationID: operationID, From: []domain.Status{domain.StatusConfirmed},
		To: domain.StatusSubmitting, ProviderRequestKey: &requestKey,
	}
	if err := store.CheckSubmissionInputs(t.Context(), operationID); !errors.Is(err, pgoperation.ErrWorkflowInputNotReady) {
		t.Fatalf("processing input accepted: %v", err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), start); !errors.Is(err, pgoperation.ErrWorkflowInputNotReady) {
		t.Fatalf("submission bypassed input readiness: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET status = 'ready', moderation_status = 'passed', contains_real_person = true WHERE id = ?::uuid`, assetID.String()).Error; err != nil {
		t.Fatalf("mark real person input: %v", err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), start); !errors.Is(err, pgoperation.ErrWorkflowConsentUnavailable) {
		t.Fatalf("submission bypassed unverified consent: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET contains_real_person = false WHERE id = ?::uuid`, assetID.String()).Error; err != nil {
		t.Fatalf("clear real person flag: %v", err)
	}
	if err := store.CheckSubmissionInputs(t.Context(), operationID); err != nil {
		t.Fatalf("check ready input: %v", err)
	}
	if _, err := store.TransitionWorkflowOperation(t.Context(), start); err != nil {
		t.Fatalf("start with ready input: %v", err)
	}
}
