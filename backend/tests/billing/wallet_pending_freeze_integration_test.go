package billing_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

func TestWalletReservationPendingFreezeRequiresExactDurableOperationIntent(t *testing.T) {
	for _, scenario := range []string{"accepted", "no_command", "wrong_scope", "wrong_amount", "wrong_caller", "wrong_source", "wrong_action", "wrong_key", "failed_command", "no_outbox", "wrong_event_scope", "wrong_event_type", "wrong_event_identity", "wrong_partition", "unattached_operation", "wrong_quote", "uncapped", "payer_changed", "partial_sync_hold"} {
		t.Run(scenario, func(t *testing.T) {
			_, database := billingDB(t)
			actor, project := billingProject(t, database)
			other, _ := billingProject(t, database)
			seedReserveBudget(t, database, project, 200000)
			scope := bindWallet(t, database, project)
			operationID := seedReserveOperation(t, database, project, 100000)
			if scenario == "wrong_quote" {
				require.NoError(t, database.Exec(`UPDATE operation.operation SET quote_micros=99999 WHERE id=?`, operationID).Error)
			}
			if scenario == "payer_changed" {
				require.NoError(t, database.Exec(`SELECT billing.ensure_wallet_account(?)`, other.ScopeID).Error)
				require.NoError(t, database.Exec(`UPDATE billing.project_wallet SET scope_id=? WHERE project_id=?`, other.ScopeID, project).Error)
			}
			err := database.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
					return err
				}
				reservation := uuid.New()
				if err := tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status,cap_at_quote) VALUES(?,?,?,100000,'held',?)`, reservation, project, operationID, scenario != "uncapped").Error; err != nil {
					return err
				}
				if scenario != "unattached_operation" {
					if err := attachReserveOperation(tx, operationID, project, reservation, actor.ID); err != nil {
						return err
					}
				}
				if scenario == "partial_sync_hold" {
					if err := tx.Exec(`INSERT INTO billing.wallet_hold(reservation_id,scope_id) VALUES(?,?)`, reservation, scope).Error; err != nil {
						return err
					}
				}
				if scenario == "no_command" {
					return nil
				}
				command := billingapp.CreditCommand{CallerID: billingapp.CreditOperationCallerID, IdempotencyKey: "operation/" + operationID.String() + "/freeze", ScopeID: scope, Action: billingapp.CreditFreeze, SourceID: operationID, AmountMicros: 100000, Reason: "pending freeze constraint fixture"}
				switch scenario {
				case "wrong_scope":
					command.ScopeID = other.ScopeID
				case "wrong_amount":
					command.AmountMicros++
				case "wrong_caller":
					command.CallerID = "fixture:other-caller"
				case "wrong_source":
					command.SourceID = uuid.New()
				case "wrong_action":
					command.Action = billingapp.CreditDebit
				case "wrong_key":
					command.IdempotencyKey = "unrelated-freeze"
				}
				receipt, err := billingpg.NewStore(tx).SubmitCreditCommand(t.Context(), command)
				if err != nil {
					return err
				}
				if scenario == "failed_command" {
					return tx.Exec(`UPDATE billing.credit_command SET status='failed' WHERE id=?`, receipt.ID).Error
				}
				if scenario == "no_outbox" {
					return tx.Exec(`DELETE FROM infra.outbox WHERE topic='glowloom.billing.command.v1' AND payload->>'command_id'=?`, receipt.ID.String()).Error
				}
				if scenario == "wrong_event_scope" || scenario == "wrong_event_type" || scenario == "wrong_event_identity" || scenario == "wrong_partition" {
					if err := tx.Exec(`DELETE FROM infra.outbox WHERE topic='glowloom.billing.command.v1' AND payload->>'command_id'=?`, receipt.ID.String()).Error; err != nil {
						return err
					}
					eventID := uuid.New()
					payload := map[string]any{"event_id": eventID, "event_type": "glowloom.billing.command.v1", "command_id": receipt.ID, "scope_id": scope, "occurred_at": time.Now().UTC()}
					partition := scope
					switch scenario {
					case "wrong_event_scope":
						payload["scope_id"] = other.ScopeID
					case "wrong_event_type":
						payload["event_type"] = "glowloom.billing.command_result.v1"
					case "wrong_event_identity":
						payload["event_id"] = uuid.New()
					case "wrong_partition":
						partition = other.ScopeID
					}
					raw, err := json.Marshal(payload)
					if err != nil {
						return err
					}
					return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,'glowloom.billing.command.v1',?,?::jsonb)`, eventID, partition, string(raw)).Error
				}
				return nil
			})
			if scenario == "accepted" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "wallet-backed reservation requires complete funding")
			var reservations int64
			require.NoError(t, database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, operationID).Scan(&reservations).Error)
			require.Zero(t, reservations, "a rejected intent must roll back the whole reservation")
		})
	}
}
