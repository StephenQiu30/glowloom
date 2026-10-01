package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// depthControlActor resolves only the latest permanent intent of this attempt.
// Current media authorization, rather than a role frozen in a command, determines
// which existing role is valid. A rejected latest actor never falls back to an
// older command or to the creator; revocation must stop cleanup too.
func (s *DepthStore) depthControlActor(ctx context.Context, tx *gorm.DB, r depthRow, action string) (identityapp.Principal, error) {
	var actor identityapp.Principal
	if action != "cancel" && action != "reconcile" && action != "cleanup" {
		return actor, application.ErrInvalidDepthInput
	}
	actions := []string{action}
	cleanup := action == "cleanup"
	if cleanup {
		if !r.CancellationRequested {
			return actor, application.ErrConflict
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
		return actor, res.Error
	}
	if res.RowsAffected != 1 || row.ActorID == uuid.Nil || row.RequestID == uuid.Nil || row.EventID == uuid.Nil || row.Topic != depthTopic || row.PartitionKey != r.ID.String() {
		return actor, application.ErrInvalidDepthInput
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
		return actor, application.ErrInvalidDepthInput
	}
	if cleanup && !response.CancellationRequested {
		return actor, application.ErrInvalidDepthInput
	}
	hash, err := commandHash("depth_"+action, r.ProjectID, r.ID, response.Revision-1)
	if err != nil {
		return actor, err
	}
	d := event.Data
	expectedEvent := uuid.NewSHA1(row.RequestID, []byte("media-depth/"+row.ActorID.String()+"/"+action))
	if row.EventID != expectedEvent || hash != row.RequestHash || event.EventID != row.EventID || event.EventType != depthTopic || event.OrgID != r.OrgID || event.OccurredAt.IsZero() || d.JobID != r.ID || d.Attempt != r.Attempt || d.ExecutionID != uuid.Nil || d.Reconcile != (action == "reconcile") || d.EventID != row.EventID || d.RequestID != row.RequestID || d.ActorID != row.ActorID || d.OrgID != r.OrgID || d.ProjectID != r.ProjectID || d.Action != action {
		return actor, application.ErrInvalidDepthInput
	}
	actor = identityapp.Principal{ID: row.ActorID, OrgID: r.OrgID}
	for _, role := range []identitydomain.Role{identitydomain.RoleAdmin, identitydomain.RoleProducer} {
		candidate := identityapp.Principal{ID: row.ActorID, OrgID: r.OrgID, Role: role}
		err := s.authorize(ctx, tx, candidate, r.ProjectID, true)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, identityapp.ErrForbidden) {
			return actor, err
		}
	}
	return actor, identityapp.ErrForbidden
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
		r, err := readDepth(tx, uuid.Nil, id.JobID, true)
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
		if _, err := s.depthControlActor(ctx, tx, r, "reconcile"); err != nil {
			return err
		}
		return use(tx, r)
	})
}
