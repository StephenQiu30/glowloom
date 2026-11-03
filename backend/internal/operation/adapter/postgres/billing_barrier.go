package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// Consumption identity excludes editable input, model, revision and request key.
const billingTargetSQL = `CASE WHEN o.source_context IS NOT NULL THEN
 'canvas/'||(o.source_context->>'canvas_id')||'/'||(o.source_context->>'node_id')||'/'||COALESCE(o.source_context->>'row_id','')
 WHEN o.target_id IS NOT NULL THEN COALESCE(o.target_type,'')||'/'||o.target_id::text||'/'||COALESCE(o.target_key,'') ELSE '' END`

func billingTarget(op domain.Operation) string {
	if op.Source != nil {
		row := ""
		if op.Source.RowID != nil {
			row = op.Source.RowID.String()
		}
		return "canvas/" + op.Source.CanvasID.String() + "/" + op.Source.NodeID.String() + "/" + row
	}
	if op.TargetID != nil {
		slot := ""
		if op.TargetKey != nil {
			slot = *op.TargetKey
		}
		return op.TargetType + "/" + op.TargetID.String() + "/" + slot
	}
	return ""
}

func checkBillingBarrier(tx *gorm.DB, op domain.Operation) error {
	key := billingTarget(op)
	if key == "" {
		return nil
	}
	// Serialize admission within a project before taking target keys. This also
	// makes batches with overlapping target sets deadlock-free.
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "billing-admission/"+op.ProjectID.String()).Error; err != nil {
		return err
	}
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "billing-target/"+op.ProjectID.String()+"/"+key).Error; err != nil {
		return err
	}
	var candidates []struct {
		ID     uuid.UUID
		Status domain.Status
	}
	err := tx.Raw(`SELECT o.id,o.status FROM operation.operation o WHERE o.project_id=? AND o.id<>? AND NOT o.is_delete AND (`+billingTargetSQL+`)=?
 AND (o.status NOT IN ('draft','quoted','expired') OR EXISTS(SELECT 1 FROM billing.reservation r WHERE r.project_id=o.project_id AND r.operation_id=o.id AND NOT r.is_delete) OR o.settled_micros IS NOT NULL OR EXISTS(SELECT 1 FROM billing.ledger_entry l WHERE l.project_id=o.project_id AND l.operation_id=o.id AND NOT l.is_delete) OR EXISTS(SELECT 1 FROM operation.provider_call c WHERE c.project_id=o.project_id AND c.operation_id=o.id AND c.dispatch_started_at IS NOT NULL AND NOT c.is_delete)) ORDER BY o.id`, op.ProjectID, op.ID, key).Scan(&candidates).Error
	if err != nil {
		return fmt.Errorf("check original consumption: %w", err)
	}
	for _, candidate := range candidates {
		facts, err := readBillingFacts(tx, op.ProjectID, candidate.ID, candidate.Status)
		if err != nil {
			return err
		}
		if application.ConsumptionNeedsReview(facts) {
			return application.ErrPendingConsumption
		}
	}

	return nil
}

func checkStoredBillingBarrier(tx *gorm.DB, projectID, id uuid.UUID) error {
	var row struct {
		TargetType string
		TargetID   *uuid.UUID
		TargetKey  *string
		Source     []byte
	}
	r := tx.Raw(`SELECT target_type,target_id,target_key,source_context AS source FROM operation.operation WHERE id=? AND project_id=? AND NOT is_delete`, id, projectID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrNotFound
	}
	op := domain.Operation{ID: id, ProjectID: projectID, TargetType: row.TargetType, TargetID: row.TargetID, TargetKey: row.TargetKey}
	if len(row.Source) > 0 {
		var source domain.CanvasSource
		if json.Unmarshal(row.Source, &source) != nil || !source.Valid() {
			return application.ErrProviderCallConflict
		}
		op.Source = &source
	}
	return checkBillingBarrier(tx, op)
}
