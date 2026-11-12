package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func shotReceipts(tx *gorm.DB, s domain.Scope) *gorm.DB {
	return tx.Table("agent_business.call_receipt AS c").Joins("JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id").Where("c.kind=? AND c.name=? AND c.status=? AND a.scope->>'actor_id'=? AND a.scope->>'org_id'=? AND a.scope->>'scope_id'=? AND a.scope->>'project_id'=?", domain.CallKindTool, shotDeliveryCreateTool, domain.CallStatusCompleted, s.ActorID.String(), s.OrgID.String(), s.ScopeID.String(), s.ProjectID.String())
}

func readShotReceipt(tx *gorm.DB, s domain.Scope, id uuid.UUID) (shotDelivery, error) {
	var row struct{ Result []byte }
	q := shotReceipts(tx, s).Select("c.result").Where("c.result->>'delivery_id'=?", id.String()).Order("c.created_at,c.receipt_id").Limit(1).Scan(&row)
	if q.Error != nil {
		return shotDelivery{}, q.Error
	}
	if q.RowsAffected != 1 {
		return shotDelivery{}, domain.ErrNotFound
	}
	var result shotDelivery
	if json.Unmarshal(row.Result, &result) != nil || result.DeliveryID != id || result.VideoExportID == uuid.Nil || result.ProjectExportID == uuid.Nil {
		return result, domain.ErrConflict
	}
	if s.CanvasID != nil && *s.CanvasID != result.Input.CanvasID {
		return shotDelivery{}, domain.ErrNotFound
	}
	return result, nil
}

func refreshShotDelivery(ctx context.Context, tx *gorm.DB, s domain.Scope, result shotDelivery) (shotDelivery, error) {
	result.Jobs = nil
	result.Status = domain.DeliveryCompleted
	artifactMissing := false
	for _, id := range []uuid.UUID{result.VideoExportID, result.ProjectExportID} {
		job, err := exportStore(tx).Get(ctx, s.Principal(), s.ProjectID, id)
		if err != nil {
			return result, businessError(err)
		}
		if job.Source.CanvasID != result.Input.CanvasID || job.Source.NodeID != result.TimelineNodeID || job.Source.Revision != int64(result.CanvasRevision) {
			return result, domain.ErrConflict
		}
		result.Jobs = append(result.Jobs, job)
		if job.Status == tooldomain.Succeeded {
			var evidenceErr error
			if id == result.VideoExportID {
				_, asset, e := exportStore(tx).PreviewAsset(ctx, s.Principal(), s.ProjectID, id)
				evidenceErr = e
				if e == nil && (job.AssetID == nil || asset.ID != *job.AssetID || !asset.CanReference() || !asset.ObjectLocation.Readable(asset.ObjectKey)) {
					evidenceErr = mediatool.ErrConflict
				}
			} else {
				_, artifact, e := exportStore(tx).PreviewArtifact(ctx, s.Principal(), s.ProjectID, id)
				evidenceErr = e
				if e == nil && (job.ArtifactID == nil || artifact.ID != *job.ArtifactID || !artifact.ObjectLocation.Readable(artifact.ObjectKey)) {
					evidenceErr = mediatool.ErrConflict
				}
			}
			if evidenceErr != nil {
				if !errors.Is(evidenceErr, mediatool.ErrNotFound) && !errors.Is(evidenceErr, mediatool.ErrConflict) && !errors.Is(evidenceErr, media.ErrNotFound) && !errors.Is(evidenceErr, media.ErrStorageIdentityConflict) {
					return result, businessError(evidenceErr)
				}
				artifactMissing = true
			}
		}
		if job.Status != tooldomain.Succeeded || job.SHA256 == nil || (id == result.VideoExportID && job.AssetID == nil) || (id == result.ProjectExportID && job.ArtifactID == nil) {
			result.Status = domain.DeliveryPending
		}
	}
	for _, job := range result.Jobs {
		if job.Status == tooldomain.ReviewRequired && result.Status != domain.DeliveryCompleted {
			result.Status = domain.DeliveryAwaitingReview
		}
	}
	for _, job := range result.Jobs {
		if job.Status == tooldomain.Failed || job.Status == tooldomain.Cancelled || job.Status == tooldomain.CancelRequested {
			result.Status = domain.DeliveryIncomplete
		}
	}
	if artifactMissing {
		result.Status = domain.DeliveryIncomplete
	}
	return result, nil
}

func readShotDelivery(ctx context.Context, tx *gorm.DB, s domain.Scope, id uuid.UUID) (shotDelivery, error) {
	result, err := readShotReceipt(tx, s, id)
	if err != nil {
		return result, err
	}
	return refreshShotDelivery(ctx, tx, s, result)
}

// shotDeliverySnapshot is run-scoped and bounded. It exposes original receipt
// identities and live job facts, never private storage keys or source prompts.
func shotDeliverySnapshot(ctx context.Context, tx *gorm.DB, g domain.Grant) ([]map[string]any, error) {
	var rows []struct{ Result []byte }
	if err := shotReceipts(tx, g.Scope).Select("c.result").Where("c.root_run_id=?", g.RootRunID).Order("c.created_at,c.receipt_id").Limit(domain.MaxGoalDeliveries + 1).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > domain.MaxGoalDeliveries {
		return nil, domain.ErrBudget
	}
	result := []map[string]any{}
	seen := map[uuid.UUID]int{}
	for _, row := range rows {
		var saved shotDelivery
		if json.Unmarshal(row.Result, &saved) != nil {
			return nil, domain.ErrConflict
		}
		if _, ok := seen[saved.DeliveryID]; ok {
			continue
		}
		seen[saved.DeliveryID] = len(result)
		live, err := refreshShotDelivery(ctx, tx, g.Scope, saved)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"delivery_id": live.DeliveryID, "video_export_id": live.VideoExportID, "project_export_id": live.ProjectExportID, "status": live.Status, "canvas_id": live.Input.CanvasID, "node_id": live.Input.NodeID, "timeline_node_id": live.TimelineNodeID, "operation_id": live.Input.OperationID, "output_id": live.Input.OutputID, "asset_id": live.AssetID, "canvas_revision": live.CanvasRevision, "source": live.Source, "delivery": deliverySpec(live.Input), "approved": false, "jobs": live.Jobs})
	}
	// Approval establishes the obligation before a successful create receipt.
	// A model ending early or a failed create must not turn this into pure chat.
	var approvals []struct{ Body []byte }
	if err := tx.Table("agent_business.proposal p").Select("DISTINCT p.body").Joins("JOIN agent_business.control_decision d ON d.root_run_id=p.root_run_id AND d.action='approve' AND d.body->>'proposal_id'=p.proposal_id::text").Where("p.root_run_id=? AND p.body->'candidate'->>'kind'=?", g.RootRunID, shotDeliveryCandidateKind).Limit(domain.MaxGoalDeliveries + 1).Scan(&approvals).Error; err != nil {
		return nil, err
	}
	if len(approvals) > domain.MaxGoalDeliveries {
		return nil, domain.ErrBudget
	}
	for _, row := range approvals {
		var proposal domain.ProposalInput
		var candidate shotDeliveryCandidate
		if json.Unmarshal(row.Body, &proposal) != nil || json.Unmarshal(proposal.Candidate, &candidate) != nil || !candidate.Input.valid() {
			return nil, domain.ErrConflict
		}
		id := deliveryIdentity(g.Scope, candidate.Input)
		if index, ok := seen[id]; ok {
			if domain.ContentHash(result[index]["source"]) != domain.ContentHash(candidate.Source) || domain.ContentHash(result[index]["delivery"]) != domain.ContentHash(deliverySpec(candidate.Input)) {
				return nil, domain.ErrConflict
			}
			result[index]["approved"] = true
			continue
		}
		// An approved proposal alone cannot borrow another root's completed
		// receipt. Explicit create/reuse must commit a receipt in this root.
		seen[id] = len(result)
		result = append(result, map[string]any{"delivery_id": id, "status": domain.DeliveryIncomplete, "reason": "approved_delivery_not_admitted", "source": candidate.Source, "delivery": deliverySpec(candidate.Input), "approved": true})
	}
	if len(result) > domain.MaxGoalDeliveries {
		return nil, domain.ErrBudget
	}
	return result, nil
}
