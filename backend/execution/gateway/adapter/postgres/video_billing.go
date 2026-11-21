package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// SettlementKind owns evidence-only funds completion.
const SettlementKind = "ai.settle"

// DeliveryKind owns archive work for an original receipt.
const DeliveryKind = "ai.deliver"

type videoUsageEvidence struct {
	Reconciliation
	PurchaseMicros *int64 `json:"purchase_micros,omitempty"`
}

func saveVideoReceipt(ctx context.Context, tx *gorm.DB, request, attempt uuid.UUID, attemptState string, route domain.Route, receipt domain.Receipt, raw []byte, digest string) error {
	var result operation.GenerationResult
	if receipt.Pending || receipt.UpstreamID == "" || json.Unmarshal(receipt.Body, &result) != nil ||
		result.TaskID != receipt.UpstreamID || len(result.Images)+len(result.Audio) != 0 {
		return domain.ErrInvalid
	}
	if receipt.DeliveryFailed {
		if len(result.URLs) != 0 || result.State != operation.ProviderTaskFailed &&
			result.State != operation.ProviderTaskCancelled && result.State != operation.ProviderTaskExpired {
			return domain.ErrInvalid
		}
	} else if result.State != operation.ProviderTaskSucceeded || result.Outcome != operation.ProviderOutcomeCompleted || len(result.URLs) != 1 {
		return domain.ErrInvalid
	}
	if receipt.DeliveryFailed || attemptState == "pending" || attemptState == "unknown" {
		var original string
		q := tx.Raw("SELECT task_id FROM ai.pending WHERE request_id=? AND attempt_id=?", request, attempt).Scan(&original)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 && receipt.DeliveryFailed || q.RowsAffected > 0 && original != receipt.UpstreamID {
			return domain.ErrConflict
		}
	}
	state := "received"
	if receipt.PurchasePending {
		if receipt.Usage != (domain.Usage{}) || receipt.PurchaseMicros != 0 {
			return domain.ErrInvalid
		}
		if !receipt.DeliveryFailed {
			state = "unknown"
		}
	} else {
		cost, err := route.Purchase.Cost(receipt.Usage)
		if !receipt.Usage.IsVideoUsage() || err != nil || cost != receipt.PurchaseMicros {
			return domain.ErrInvalid
		}
	}
	if err := tx.Exec("UPDATE ai.attempt SET receipt=?::jsonb,digest=?,state='received' WHERE id=?", string(raw), digest, attempt).Error; err != nil {
		return err
	}
	q := tx.Exec("UPDATE ai.request SET state=?,updated_at=clock_timestamp() WHERE id=? AND state IN ('sending','pending','unknown')", state, request)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrConflict
	}
	work := DeliveryKind
	if receipt.DeliveryFailed {
		work = SettlementKind
	}
	return taskruntime.Enqueue(ctx, tx, work, request.String(), time.Now())
}

func videoDeliveryReady(tx *gorm.DB, request uuid.UUID) (bool, error) {
	var ready bool
	err := tx.Raw(`SELECT EXISTS(
 SELECT 1 FROM ai.output o JOIN media.media_asset a ON a.id=o.asset_id AND a.source_ai_request_id=o.request_id AND a.sha256=o.sha256
 WHERE o.request_id=? AND o.kind='video' AND o.state='ready' AND a.kind='video'
 AND a.status='ready' AND a.moderation_status='passed' AND NOT a.is_delete)`, request).Scan(&ready).Error
	return ready, err
}

func (s *Store) reconcileVideoUsage(ctx context.Context, tx *gorm.DB, in Reconciliation, digest, state string) (domain.Record, error) {
	if state != "unknown" || in.Usage == nil || !in.Usage.IsVideoUsage() {
		return domain.Record{}, domain.ErrInvalid
	}
	var attempt struct{ Contract, Receipt []byte }
	q := tx.Raw("SELECT contract,receipt FROM ai.attempt WHERE id=? AND request_id=? AND state='received' FOR UPDATE", in.Attempt, in.Request).Scan(&attempt)
	if q.Error != nil {
		return domain.Record{}, q.Error
	}
	var original domain.Receipt
	var route domain.Route
	if q.RowsAffected != 1 || json.Unmarshal(attempt.Receipt, &original) != nil || !original.PurchasePending ||
		in.Reference != original.UpstreamID || json.Unmarshal(attempt.Contract, &route) != nil {
		return domain.Record{}, domain.ErrConflict
	}
	cost, err := route.Purchase.Cost(*in.Usage)
	if err != nil {
		return domain.Record{}, err
	}
	evidence := videoUsageEvidence{Reconciliation: in, PurchaseMicros: &cost}
	if err := tx.Exec("INSERT INTO ai.reconciliation(request_id,attempt_id,actor_id,digest,evidence) VALUES(?,?,?,?,?::jsonb)", in.Request, in.Attempt, s.actor.ID, digest, string(mustJSON(evidence))).Error; err != nil {
		return domain.Record{}, err
	}
	if err := tx.Exec("UPDATE ai.request SET state='received',updated_at=clock_timestamp() WHERE id=?", in.Request).Error; err != nil {
		return domain.Record{}, err
	}
	inner := *s
	inner.db = tx
	return inner.Settle(ctx, in.Request)
}
