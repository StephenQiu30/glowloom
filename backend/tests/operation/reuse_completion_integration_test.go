package operation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func completedReuseDelivery(t *testing.T, db *gorm.DB, moderation string, unattributed bool) (identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	actor, pid := operationStoreProject(t, db)
	quoted, _, _ := seedQuoteCatalog(t, db, pid, time.Now().UTC())
	hash := uuid.NewString()
	source := seedCompletedOutput(t, db, pid, hash, moderation, "ready", moderation)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET output_count=2 WHERE id=?`, source).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json','{"result":"approved"}',?)`, uuid.New(), pid, source, moderation).Error)
	var assetText string
	require.NoError(t, db.Raw(`SELECT media_asset_id FROM operation.operation_output WHERE operation_id=? AND seq_no=0`, source).Scan(&assetText).Error)
	asset := uuid.MustParse(assetText)
	if unattributed {
		require.NoError(t, db.Exec(`UPDATE media.media_asset SET source_operation_id=NULL WHERE id=?`, asset).Error)
	}
	zero := int64(0)
	quoted.InputHash, quoted.ReusedFromID, quoted.QuoteMicros, quoted.OutputCount = hash, &source, &zero, 2
	item := quotedSnapshotItem(pid, 0)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := operationStore(db)
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}))
	_, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: quoted.ID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	finalizer := app.NewOperationFinalizer(db, store, billingpg.NewStore(db))
	require.NoError(t, finalizer.CompleteFromReuse(t.Context(), quoted.ID.String()))
	require.NoError(t, finalizer.CompleteFromReuse(t.Context(), quoted.ID.String()))
	return actor, pid, quoted.ID, source, asset
}

func TestReuseDeliveryAcceptsHistoricalSkippedAndUnattributedAssets(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid, id, _, _ := completedReuseDelivery(t, db, "skipped", true)
	detail, err := operationStore(db).ReadPublicTask(t.Context(), actor, pid, id)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
	require.Equal(t, "released", detail.BillingOutcome.ChargeState)
	var calls, settles int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=?`, id).Scan(&calls).Error)
	require.Zero(t, calls)
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='settle' AND amount_micros=0`, id).Scan(&settles).Error)
	require.EqualValues(t, 1, settles)
}

func TestReuseDeliveryRejectsBrokenCopyAndFrozenRelationships(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid, id, source, asset := completedReuseDelivery(t, db, "passed", false)
	otherSource := seedCompletedOutput(t, db, pid, uuid.NewString(), "passed", "ready", "passed")
	var otherAssetText string
	require.NoError(t, db.Raw(`SELECT media_asset_id FROM operation.operation_output WHERE operation_id=?`, otherSource).Scan(&otherAssetText).Error)
	otherAsset := uuid.MustParse(otherAssetText)
	_, otherProject := operationStoreProject(t, db)
	foreignSource := seedCompletedOutput(t, db, otherProject, uuid.NewString(), "passed", "ready", "passed")
	var foreignAssetText string
	require.NoError(t, db.Raw(`SELECT media_asset_id FROM operation.operation_output WHERE operation_id=?`, foreignSource).Scan(&foreignAssetText).Error)
	foreignAsset := uuid.MustParse(foreignAssetText)
	for _, tc := range []struct {
		name, sql string
		args      []any
	}{
		{"missing copied output", `UPDATE operation.operation_output SET is_delete=true WHERE operation_id=? AND seq_no=1`, []any{id}},
		{"wrong sequence", `UPDATE operation.operation_output SET seq_no=3 WHERE operation_id=? AND seq_no=1`, []any{id}},
		{"copied output rejected", `UPDATE operation.operation_output SET moderation_status='rejected' WHERE operation_id=? AND seq_no=1`, []any{id}},
		{"wrong shared asset", `UPDATE operation.operation_output SET media_asset_id=? WHERE operation_id=? AND seq_no=0`, []any{otherAsset, id}},
		{"wrong JSON copy", `UPDATE operation.operation_output SET json_payload='{"result":"changed"}' WHERE operation_id=? AND seq_no=1`, []any{id}},
		{"cross project source", `UPDATE operation.operation SET reused_from_id=? WHERE id=?`, []any{foreignSource, id}},
		{"cross project asset", `UPDATE operation.operation_output SET media_asset_id=? WHERE operation_id=? AND seq_no=0`, []any{foreignAsset, id}},
		{"duplicate sequence", `UPDATE operation.operation_output SET seq_no=0 WHERE operation_id=? AND seq_no=1`, []any{id}},
		{"frozen input changed", `UPDATE operation.operation SET input_hash='changed' WHERE id=?`, []any{source}},
		{"source count changed", `UPDATE operation.operation SET output_count=3 WHERE id=?`, []any{source}},
		{"source completion invalid", `UPDATE operation.operation SET status='failed' WHERE id=?`, []any{source}},
		{"source output missing", `DELETE FROM operation.operation_output WHERE operation_id=? AND seq_no=1`, []any{source}},
		{"deleted shared asset", `UPDATE media.media_asset SET is_delete=true WHERE id=?`, []any{asset}},
		{"unready shared asset", `UPDATE media.media_asset SET status='processing' WHERE id=?`, []any{asset}},
		{"unreviewed shared asset", `UPDATE media.media_asset SET moderation_status='pending' WHERE id=?`, []any{asset}},
		{"consent record not reusable", `UPDATE media.media_asset SET consent_record_id=? WHERE id=?`, []any{uuid.New(), asset}},
		{"real person not reusable", `UPDATE media.media_asset SET contains_real_person=true WHERE id=?`, []any{asset}},
		{"missing completion receipt", `DELETE FROM operation.operation_event WHERE operation_id=? AND reason='reused_result'`, []any{id}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rollback := errors.New("fixture rollback")
			err := db.Transaction(func(tx *gorm.DB) error {
				writeErr := tx.Exec(tc.sql, tc.args...).Error
				switch tc.name {
				case "cross project source", "cross project asset":
					require.Error(t, writeErr)
					require.Contains(t, writeErr.Error(), "SQLSTATE 23503")
					return rollback
				case "duplicate sequence":
					require.Error(t, writeErr)
					require.Contains(t, writeErr.Error(), "SQLSTATE 23505")
					return rollback
				}
				require.NoError(t, writeErr)
				detail, err := operationStore(tx).ReadPublicTask(t.Context(), actor, pid, id)
				require.NoError(t, err)
				require.NotEqual(t, "ready", detail.BillingOutcome.DeliveryState)
				require.Equal(t, "blocked", detail.BillingOutcome.Regenerate)
				return rollback
			})
			require.ErrorIs(t, err, rollback)
		})
	}
}

func TestPaidLegacyZeroBasedDeliveryRetainsReadyAndAdmission(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid, _, items := seedConfirmableBatch(t, db, 10)
	id := items[0].Operation.ID
	store := operationStore(db)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET batch_id=NULL,output_count=2 WHERE id=?`, id).Error)
	_, err := store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: id, RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := billingpg.NewStore(tx).SettleInTransaction(t.Context(), tx, billingapp.SettleInput{ProjectID: pid, OperationID: id, ActualCostMicros: 10, Capability: items[0].Operation.Capability, OccurredAt: time.Now()})
		if err != nil {
			return err
		}
		return tx.Exec(`UPDATE operation.operation SET status='completed',settled_micros=10 WHERE id=?`, id).Error
	}))
	asset := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,source_operation_id,moderation_status) VALUES(?,?,'image','generated','ready',?,'image/png',1,?,'passed')`, asset, pid, "legacy-fixture/"+asset.String(), id).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status) VALUES(?,?,?,0,'media',?,'passed')`, uuid.New(), pid, id, asset).Error)
	require.NoError(t, db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json','{"result":"legacy"}','passed')`, uuid.New(), pid, id).Error)
	source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	encoded, _ := json.Marshal(source)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), id).Error)
	next := items[0]
	next.Operation.ID = uuid.New()
	next.Operation.BatchID = nil
	next.Inputs = append([]domain.OperationInput(nil), next.Inputs...)
	next.Inputs[0].ID = uuid.New()
	next.Inputs[0].OperationID = next.Operation.ID
	require.NoError(t, store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{next}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), next.Operation.ID).Error)
	detail, err := store.ReadPublicTask(t.Context(), actor, pid, id)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
	require.Equal(t, "confirm", detail.BillingOutcome.Regenerate)
	for index, change := range []string{
		fmt.Sprintf(`UPDATE operation.operation_output SET is_delete=true WHERE operation_id='%s' AND seq_no=1`, id),
		fmt.Sprintf(`UPDATE operation.operation_output SET seq_no=3 WHERE operation_id='%s' AND seq_no=1`, id),
		fmt.Sprintf(`UPDATE media.media_asset SET is_delete=true WHERE id='%s'`, asset),
		fmt.Sprintf(`UPDATE media.media_asset SET status='processing' WHERE id='%s'`, asset),
		fmt.Sprintf(`UPDATE media.media_asset SET moderation_status='pending' WHERE id='%s'`, asset),
		fmt.Sprintf(`UPDATE media.media_asset SET contains_real_person=true WHERE id='%s'`, asset),
		fmt.Sprintf(`UPDATE media.media_asset SET consent_record_id='%s' WHERE id='%s'`, uuid.New(), asset),
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			rollback := errors.New("fixture rollback")
			err := db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, tx.Exec(change).Error)
				detail, err := operationStore(tx).ReadPublicTask(t.Context(), actor, pid, id)
				require.NoError(t, err)
				require.NotEqual(t, "ready", detail.BillingOutcome.DeliveryState)
				require.Equal(t, "blocked", detail.BillingOutcome.Regenerate)
				_, err = operationStore(tx).ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: next.Operation.ID, RequestID: uuid.NewString()})
				require.ErrorIs(t, err, application.ErrPendingConsumption)
				return rollback
			})
			require.ErrorIs(t, err, rollback)
		})
	}
	// The accepted legacy contract also permits skipped review and absent historical
	// source_operation_id; project-owned output relationships remain authoritative.
	require.NoError(t, db.Exec(`UPDATE media.media_asset SET source_operation_id=NULL,moderation_status='skipped' WHERE id=?`, asset).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation_output SET moderation_status='skipped' WHERE operation_id=?`, id).Error)
	detail, err = store.ReadPublicTask(t.Context(), actor, pid, id)
	require.NoError(t, err)
	require.Equal(t, "ready", detail.BillingOutcome.DeliveryState)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: next.Operation.ID, RequestID: uuid.NewString()})
	require.NoError(t, err)
}
