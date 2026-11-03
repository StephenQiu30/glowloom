package postgres

import (
	"errors"
	"fmt"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func readBillingOutcome(tx *gorm.DB, projectID, id uuid.UUID, status domain.Status) (application.BillingOutcome, error) {
	facts, err := readBillingFacts(tx, projectID, id, status)
	if err != nil {
		return application.BillingOutcome{}, err
	}
	return application.ComputeBillingOutcome(facts), nil
}

// readBillingFacts is shared by authorized snapshots and lock-internal admission
// vetoes. It validates the immutable ledger against reservation and lifecycle.
func readBillingFacts(tx *gorm.DB, projectID, id uuid.UUID, status domain.Status) (application.BillingOutcomeFacts, error) {
	var row struct {
		Status                                                    string
		Amount, OperationSettled                                  *int64
		ChargedMicros, ReleasedMicros, ReserveMicros              *int64
		Reserves, Settles, Releases                               int64
		AdapterKey, Queue                                         string
		LegacyAmbiguousZero, SupportsQuery, Staged, Task, Receipt bool
	}
	err := tx.Raw(`SELECT COALESCE(r.status,'') AS status,r.amount_micros AS amount,o.settled_micros AS operation_settled,
 l.reserve_micros,l.charged_micros,l.released_micros,l.reserves,l.settles,l.releases,
 p.adapter_key,v.queue,v.supports_query,
 (r.status='released' AND EXISTS(SELECT 1 FROM operation.provider_call c WHERE c.operation_id=o.id AND c.project_id=o.project_id AND c.action='submit' AND NOT c.is_delete AND c.response_summary->>'state' IN ('unknown','rejected') AND COALESCE(c.response_summary->>'price_contract','')<>'mock_free')) AS legacy_ambiguous_zero,
 EXISTS(SELECT 1 FROM operation.provider_result WHERE operation_id=o.id AND result->>'state' IN ('accepted','succeeded') AND result->'identity'->>'request_key'=o.provider_request_key AND result->'identity'->>'operation_id'=o.id::text AND result->'identity'->>'project_id'=o.project_id::text AND result->'identity'->>'model_profile_version_id'=o.model_profile_version_id::text AND result->'identity'->>'price_rule_version_id'=o.price_rule_version_id::text AND result->'identity'->>'attempt'='1' AND result->'identity'->>'action'='submit') AS staged,
 EXISTS(SELECT 1 FROM operation.provider_call WHERE operation_id=o.id AND project_id=o.project_id AND NOT is_delete AND provider_task_id IS NOT NULL AND action='submit') AS task,
 EXISTS(SELECT 1 FROM operation.provider_call WHERE operation_id=o.id AND project_id=o.project_id AND NOT is_delete AND receipt IS NOT NULL AND action='submit' AND attempt=1 AND dispatch_started_at IS NOT NULL AND request_summary->>'dispatch_contract'='v1' AND request_key=o.provider_request_key AND model_profile_version_id=o.model_profile_version_id AND price_rule_version_id=o.price_rule_version_id) AS receipt
 FROM operation.operation o
 LEFT JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id
 LEFT JOIN catalog.model_profile m ON m.id=v.model_profile_id
 LEFT JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN billing.reservation r ON r.operation_id=o.id AND r.project_id=o.project_id AND r.id=o.reservation_id AND NOT r.is_delete
 LEFT JOIN LATERAL(SELECT count(*) FILTER(WHERE entry_type='reserve') AS reserves,count(*) FILTER(WHERE entry_type='settle') AS settles,count(*) FILTER(WHERE entry_type='release') AS releases,
 sum(amount_micros) FILTER(WHERE entry_type='reserve') AS reserve_micros,sum(amount_micros) FILTER(WHERE entry_type='settle') AS charged_micros,sum(amount_micros) FILTER(WHERE entry_type='release') AS released_micros
 FROM billing.ledger_entry WHERE project_id=o.project_id AND operation_id=o.id AND NOT is_delete)l ON true
 WHERE o.id=? AND o.project_id=? AND NOT o.is_delete`, id, projectID).Scan(&row).Error
	if err != nil {
		return application.BillingOutcomeFacts{}, fmt.Errorf("read billing outcome facts: %w", err)
	}
	var observation struct{ State *string }
	if err := tx.Raw(`SELECT response_summary->>'settlement_state' AS state FROM operation.provider_call WHERE operation_id=? AND project_id=? AND NOT is_delete AND jsonb_exists(response_summary,'settlement_state') ORDER BY create_time DESC,attempt DESC,id DESC LIMIT 1`, id, projectID).Scan(&observation).Error; err != nil {
		return application.BillingOutcomeFacts{}, fmt.Errorf("read settlement observation: %w", err)
	}
	ready, deliveryConflict, err := readDeliveryReady(tx, projectID, id, status)
	if err != nil {
		return application.BillingOutcomeFacts{}, err
	}
	recoverable := row.Staged && (status.IsTerminal() || catalog.IsManagedCloud(row.AdapterKey) && row.Queue == "provider") ||
		row.Receipt && row.AdapterKey == "codex" && row.Queue == "agent.codex" ||
		row.Task && row.AdapterKey == "mock" && row.Queue == "agent.mock" && row.SupportsQuery
	f := application.BillingOutcomeFacts{ProviderSettlementState: observation.State, ReservationStatus: row.Status, ChargedMicros: row.ChargedMicros, ReleasedMicros: row.ReleasedMicros, Ready: ready, Recoverable: recoverable, Terminal: status.IsTerminal()}
	if row.Amount == nil {
		f.Conflict = deliveryConflict || row.OperationSettled != nil || row.Settles != 0 || row.Reserves != 0 || row.Releases != 0
		return f, nil
	}
	zero := int64(0)
	f.ReservedMicros = &zero
	if row.Status == "held" {
		f.ReservedMicros = row.Amount
	}
	f.Conflict = deliveryConflict || *row.Amount < 0 || row.Reserves != 1 || row.ReserveMicros == nil || *row.ReserveMicros != *row.Amount
	if row.Status == "held" {
		f.Conflict = f.Conflict || row.OperationSettled != nil || row.Settles != 0 || row.Releases != 0
		return f, nil
	}
	if row.Status != "settled" && row.Status != "released" {
		f.Conflict = true
		return f, nil
	}
	f.Conflict = f.Conflict || !status.IsTerminal() || row.Settles != 1 || row.ChargedMicros == nil || row.OperationSettled == nil
	if row.ChargedMicros == nil || row.OperationSettled == nil {
		return f, nil
	}
	f.Conflict = f.Conflict || *row.ChargedMicros < 0 || *row.OperationSettled != *row.ChargedMicros || (row.Status == "settled" && *row.ChargedMicros == 0) || (row.Status == "released" && *row.ChargedMicros != 0)
	released := int64(0)
	if row.ReleasedMicros != nil {
		released = *row.ReleasedMicros
	}
	expected := max(*row.Amount-*row.ChargedMicros, 0)
	f.Conflict = f.Conflict || released != expected || (expected == 0 && row.Releases != 0) || (expected > 0 && row.Releases != 1)
	if row.LegacyAmbiguousZero {
		// Old versions recorded remote rejection as zero. Preserve the actual
		// ledger and block a new purchase unless original evidence supports it.
		cost, err := readProviderCost(tx.Statement.Context, tx, id, providerCostOperation{ProjectID: projectID, Status: string(status)})
		if err != nil && !errors.Is(err, application.ErrProviderCostUnknown) && !errors.Is(err, application.ErrProviderCallConflict) && !errors.Is(err, application.ErrManualResolutionUnverified) {
			return application.BillingOutcomeFacts{}, err
		}
		f.Conflict = f.Conflict || err != nil || cost.ActualCostMicros != 0
	}
	return f, nil
}
