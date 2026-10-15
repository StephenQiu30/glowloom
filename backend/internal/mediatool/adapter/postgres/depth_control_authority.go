package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// depthControlActor resolves only the latest permanent intent of this attempt.
// Current media authorization, rather than a role frozen in a command, determines
// which existing role is valid. A rejected latest actor never falls back to an
// older command or to the creator; revocation must stop cleanup too.
func (s *DepthStore) depthControlActor(ctx context.Context, tx *gorm.DB, r depthRow, action string) (identityapp.Principal, error) {
	proof, err := readDepthControl(tx, r, action)
	if err != nil {
		return proof.Actor, err
	}
	return proof.Actor, s.authorize(ctx, tx, proof.Actor, r.ProjectID, true)
}

type depthControlProof struct {
	Actor               identityapp.Principal
	RequestID, EventID  uuid.UUID
	Revision            int64
	Action, RequestHash string
}

func readDepthControl(tx *gorm.DB, r depthRow, action string) (depthControlProof, error) {
	var proof depthControlProof
	if action != "cancel" && action != "reconcile" && action != "cleanup" {
		return proof, application.ErrInvalidDepthInput
	}
	actions := []string{action}
	cleanup := action == "cleanup"
	if cleanup {
		if !r.CancellationRequested {
			return proof, application.ErrConflict
		}
		actions = []string{"cancel", "reconcile"}
	}
	var row struct {
		ActorID, RequestID, EventID                   uuid.UUID
		RequestHash, Topic, PartitionKey, EventAction string
		Response, Payload                             []byte
	}
	res := tx.Raw(`SELECT c.actor_id,c.request_id,c.event_id,c.request_hash,c.response,c.event_action,o.topic,o.partition_key,o.payload
		FROM mediatool.depth_command c LEFT JOIN infra.outbox o ON o.id=c.event_id
		WHERE c.job_id=? AND c.event_action IN ? AND c.event_attempt=?
		ORDER BY (c.response->>'revision')::bigint DESC LIMIT 1`, r.ID, actions, r.Attempt).Scan(&row)
	if res.Error != nil {
		return proof, res.Error
	}
	if res.RowsAffected != 1 || row.ActorID == uuid.Nil || row.RequestID == uuid.Nil || row.EventID == uuid.Nil || row.Topic != depthTopic || row.PartitionKey != r.ID.String() {
		return proof, application.ErrInvalidDepthInput
	}
	action = row.EventAction
	var response domain.DepthJob
	var event struct {
		EventID    uuid.UUID                 `json:"event_id"`
		EventType  string                    `json:"event_type"`
		OccurredAt time.Time                 `json:"occurred_at"`
		OrgID      uuid.UUID                 `json:"org_id"`
		Data       application.DepthDelivery `json:"data"`
	}
	if depthJSON(row.Response, &response) != nil || depthJSON(row.Payload, &event) != nil || response.ID != r.ID || response.ProjectID != r.ProjectID || response.Source != r.job().Source || response.SourceAssetID != r.SourceAssetID || response.SourceAssetRevision != r.SourceAssetRevision || response.SourceSHA256 != r.SourceSHA256 || response.ProfileID != r.ProfileID || response.Attempt != r.Attempt || response.Revision < 2 || response.Revision > r.Revision {
		return proof, application.ErrInvalidDepthInput
	}
	if cleanup && !response.CancellationRequested {
		return proof, application.ErrInvalidDepthInput
	}
	hash, err := commandHash("depth_"+action, r.ProjectID, r.ID, response.Revision-1)
	if err != nil {
		return proof, err
	}
	d := event.Data
	expectedEvent := uuid.NewSHA1(row.RequestID, []byte("media-depth/"+row.ActorID.String()+"/"+action))
	if row.EventID != expectedEvent || hash != row.RequestHash || event.EventID != row.EventID || event.EventType != depthTopic || event.OrgID != r.OrgID || event.OccurredAt.IsZero() || d.JobID != r.ID || d.Attempt != r.Attempt || d.ExecutionID != uuid.Nil || d.Reconcile != (action == "reconcile") || d.EventID != row.EventID || d.RequestID != row.RequestID || d.ActorID != row.ActorID || d.OrgID != r.OrgID || d.ProjectID != r.ProjectID || d.Action != action {
		return proof, application.ErrInvalidDepthInput
	}
	proof = depthControlProof{Actor: identityapp.Principal{ID: row.ActorID, OrgID: r.OrgID}, RequestID: row.RequestID, EventID: row.EventID, Revision: response.Revision, Action: action, RequestHash: row.RequestHash}
	return proof, nil
}

// depthAuthorization keeps a proved permission rejection separate from a stale
// candidate. Claim and Finish may retain their existing safe rejection facts only
// after the job and permanent control proof have both been rechecked under lock.
type depthAuthorization struct {
	actor identityapp.Principal
	err   error
}

const depthPhysicalMode = "physical"

// Only a valid current candidate may ask Finish to restart its database
// transaction. Binding changes, invalid proofs and current-intent permission
// failures cannot.
var errDepthAuthorityChanged = fmt.Errorf("depth authority changed before job lock: %w", application.ErrConflict)

func sameDepthBinding(a, b depthRow) bool {
	return a.ActorID == b.ActorID && a.OrgID == b.OrgID && a.ProjectID == b.ProjectID && a.Attempt == b.Attempt && (a.ActiveWorker == nil) == (b.ActiveWorker == nil) && (a.ActiveWorker == nil || *a.ActiveWorker == *b.ActiveWorker)
}

// lockDepthAuthority follows the request lock order: identity/scope/project, job.
// Candidate reads grant no authority. After obtaining the job lock, both its
// authority scope and the current permanent control identity must still match.
// Physical mode records cessation without adding a permission requirement.
func (s *DepthStore) lockDepthAuthority(ctx context.Context, tx *gorm.DB, id uuid.UUID, mode func(depthRow) string) (depthRow, depthAuthorization, error) {
	var authority depthAuthorization
	candidate, err := readDepth(tx, uuid.Nil, id, false)
	if err != nil {
		return depthRow{}, authority, err
	}
	action := mode(candidate)
	authority.actor = candidate.actor()
	var proof depthControlProof
	if action != "" && action != depthPhysicalMode {
		if action != "worker" {
			proof, err = readDepthControl(tx, candidate, action)
			if err != nil {
				// A committed command can become visible after the candidate
				// read. Only a newer job with the same binding and a valid
				// current proof can request a fresh database transaction.
				latest, readErr := readDepth(tx, uuid.Nil, id, false)
				if readErr == nil && sameDepthBinding(candidate, latest) && latest.Revision > candidate.Revision {
					latestMode := mode(latest)
					if latestMode != "" && latestMode != "worker" && latestMode != depthPhysicalMode {
						if _, proofErr := readDepthControl(tx, latest, latestMode); proofErr == nil {
							return depthRow{}, authority, errDepthAuthorityChanged
						}
					}
				}
				return depthRow{}, authority, err
			}
			authority.actor = proof.Actor
		}
		authority.err = s.authorize(ctx, tx, authority.actor, candidate.ProjectID, true)
	}
	r, err := readDepth(tx, uuid.Nil, id, true)
	if err != nil {
		return depthRow{}, authority, err
	}
	if !sameDepthBinding(candidate, r) {
		return depthRow{}, authority, application.ErrConflict
	}
	currentMode := mode(r)
	if action == depthPhysicalMode && currentMode == depthPhysicalMode {
		return r, authority, nil
	}
	if r.Revision != candidate.Revision || currentMode != action {
		var current depthControlProof
		if currentMode != "" && currentMode != depthPhysicalMode && currentMode != "worker" {
			var err error
			current, err = readDepthControl(tx, r, currentMode)
			if err != nil {
				return depthRow{}, authority, err
			}
		}
		if authority.err != nil && current == proof {
			return depthRow{}, authority, application.ErrConflict
		}
		return depthRow{}, authority, errDepthAuthorityChanged
	}
	if action != "" && action != depthPhysicalMode && action != "worker" {
		current, err := readDepthControl(tx, r, action)
		if err != nil {
			return depthRow{}, authority, err
		}
		if current != proof {
			// The rejected candidate is no longer the current intent. A new
			// transaction must authorize this exact replacement from scratch.
			return depthRow{}, authority, errDepthAuthorityChanged
		}
	}
	return r, authority, nil
}

// depthHasNoOutput proves that an undispatched attempt has no private result
// obligations. A corrupt or unreadable manifest never grants fence release.
func depthHasNoOutput(tx *gorm.DB, r depthRow) (bool, error) {
	var found bool
	err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.depth_result_intent WHERE job_id=? AND attempt=? UNION ALL SELECT 1 FROM mediatool.depth_object WHERE job_id=? AND attempt=?)`, r.ID, r.Attempt, r.ID, r.Attempt).Scan(&found).Error
	return !found, err
}

// withDepthReadAttempt permits an accepted reconciliation only to inspect and
// confirm its original objects. It never grants source, native, Put or publish access.
func (s *DepthStore) withDepthReadAttempt(ctx context.Context, id application.DepthWorkID, use func(*gorm.DB, depthRow) error) error {
	if !id.Reconcile {
		return s.withDepthAttempt(ctx, id, false, use)
	}
	if s == nil || s.db == nil || id.JobID == uuid.Nil || id.Attempt < 1 || id.ExecutionID == uuid.Nil {
		return application.ErrInvalidDepthInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, authority, err := s.lockDepthAuthority(ctx, tx, id.JobID, func(r depthRow) string {
			if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID || r.ExecutionUnconfirmed || r.CancellationRequested || r.ProcessState != "ended" || r.Status != "running" || r.Stage != "reconciling" {
				return ""
			}
			return "reconcile"
		})
		if err != nil {
			return err
		}
		if r.Attempt != id.Attempt || r.ActiveWorker == nil || *r.ActiveWorker != id.ExecutionID || r.ExecutionUnconfirmed {
			return application.ErrWorkerBusy
		}
		if r.CancellationRequested {
			return application.ErrCancelled
		}
		if r.ProcessState != "ended" || r.Status != "running" || r.Stage != "reconciling" {
			return application.ErrConflict
		}
		if authority.err != nil {
			return authority.err
		}
		return use(tx, r)
	})
}
