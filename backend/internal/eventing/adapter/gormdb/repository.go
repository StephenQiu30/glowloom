package gormdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Repository struct {
	database *gorm.DB
}

func New(database *gorm.DB) *Repository { return &Repository{database: database} }

func (repo *Repository) ClaimPending(
	ctx context.Context,
	now time.Time,
	lease time.Duration,
	limit int,
	newID func() string,
) ([]eventingapp.ClaimedOutbox, error) {
	if repo == nil || repo.database == nil || lease <= 0 || limit < 1 || limit > 200 || newID == nil {
		return nil, errors.New("outbox claim configuration is invalid")
	}
	var claimed []eventingapp.ClaimedOutbox
	err := repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []model.OutboxEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)", "pending", now).
			Order("created_at ASC").Order("id ASC").Limit(limit).Find(&records).Error; err != nil {
			return err
		}
		claimed = make([]eventingapp.ClaimedOutbox, 0, len(records))
		for _, record := range records {
			claimToken := newID()
			if claimToken == "" {
				return errors.New("outbox claim token is empty")
			}
			expiresAt := now.Add(lease)
			result := tx.Model(&model.OutboxEvent{}).Where("id = ? AND status = ?", record.ID, "pending").Updates(map[string]any{
				"claim_token": claimToken, "claim_expires_at": expiresAt, "attempts": record.Attempts + 1,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("outbox event claim was lost")
			}
			value, err := outboxDomain(record)
			if err != nil {
				return err
			}
			claimed = append(claimed, eventingapp.ClaimedOutbox{Event: value, ClaimToken: claimToken})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim pending outbox events: %w", err)
	}
	return claimed, nil
}

func (repo *Repository) MarkPublished(ctx context.Context, eventID, claimToken string, now time.Time) error {
	id, err := uuid.Parse(eventID)
	if err != nil || claimToken == "" {
		return errors.New("outbox publication identity is invalid")
	}
	result := repo.database.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "pending", claimToken).
		Updates(map[string]any{
			"status": "published", "published_at": now, "claim_token": "", "claim_expires_at": nil, "last_error": "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("outbox publication claim is stale")
	}
	return nil
}

func (repo *Repository) ReleaseClaim(ctx context.Context, eventID, claimToken, message string) error {
	id, err := uuid.Parse(eventID)
	if err != nil || claimToken == "" {
		return errors.New("outbox release identity is invalid")
	}
	result := repo.database.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "pending", claimToken).
		Updates(map[string]any{"claim_token": "", "claim_expires_at": nil, "last_error": boundedError(message)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("outbox release claim is stale")
	}
	return nil
}

func (repo *Repository) Acquire(
	ctx context.Context,
	delivery eventingapp.InboxDelivery,
	now time.Time,
	lease time.Duration,
	newID func() string,
) (eventingapp.InboxClaim, error) {
	ids, err := parseDeliveryIDs(delivery)
	if err != nil || lease <= 0 || newID == nil {
		return eventingapp.InboxClaim{}, errors.New("inbox delivery identity is invalid")
	}
	var claim eventingapp.InboxClaim
	err = repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if ownerErr := ensureDeliveryOwner(tx, ids); ownerErr != nil {
			return ownerErr
		}
		var inbox model.InboxEvent
		inboxFound := true
		if findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("consumer_group = ? AND event_id = ?", delivery.Group, delivery.Envelope.EventID).
			First(&inbox).Error; findErr != nil {
			if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
			inboxFound = false
		}
		if inboxFound {
			switch inbox.Status {
			case "processed", "dead_lettered":
				claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxDuplicate, Attempt: inbox.Attempts}
				return nil
			case "ignored_stale":
				claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxStale, Attempt: inbox.Attempts}
				return nil
			case "processing":
				if inbox.ClaimExpiresAt != nil && inbox.ClaimExpiresAt.After(now) {
					claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxBusy, Attempt: inbox.Attempts}
					return nil
				}
			}
		}
		checkpoint, checkpointErr := repo.lockCheckpoint(tx, delivery, ids, now, newID)
		if checkpointErr != nil {
			return checkpointErr
		}
		if checkpoint.LastRevision >= delivery.Envelope.AggregateRevision {
			if err = repo.persistStaleInbox(tx, inboxFound, inbox, delivery, ids, now, newID); err != nil {
				return err
			}
			claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxStale}
			return nil
		}
		if checkpoint.PendingEventID != "" && checkpoint.PendingEventID != delivery.Envelope.EventID &&
			checkpoint.ClaimExpiresAt != nil && checkpoint.ClaimExpiresAt.After(now) {
			claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxBusy}
			return nil
		}
		claimToken := newID()
		if claimToken == "" {
			return errors.New("inbox claim token is empty")
		}
		expiresAt := now.Add(lease)
		attempt := 1
		if inboxFound {
			attempt = inbox.Attempts + 1
			if err = tx.Model(&model.InboxEvent{}).Where("id = ?", inbox.ID).Updates(map[string]any{
				"status": "processing", "claim_token": claimToken, "claim_expires_at": expiresAt,
				"attempts": attempt, "last_error": "",
			}).Error; err != nil {
				return err
			}
		} else {
			inboxID, parseErr := nextUUID(newID)
			if parseErr != nil {
				return parseErr
			}
			inbox = model.InboxEvent{
				ID: inboxID, ConsumerGroup: delivery.Group, EventID: delivery.Envelope.EventID,
				EventType: delivery.Envelope.EventType, WorkspaceID: ids.workspaceID, ProjectID: ids.projectID,
				AggregateKind: delivery.Envelope.AggregateKind, AggregateID: delivery.Envelope.AggregateID,
				AggregateRevision: delivery.Envelope.AggregateRevision, PayloadHash: delivery.Envelope.PayloadHash,
				OriginalTopic: delivery.Message.Topic, SourcePartition: delivery.Message.Partition,
				SourceOffset: delivery.Message.Offset, Status: "processing", ClaimToken: claimToken,
				ClaimExpiresAt: &expiresAt, Attempts: attempt, ReceivedAt: now,
			}
			if err = tx.Create(&inbox).Error; err != nil {
				return err
			}
		}
		if err = tx.Model(&model.EventCheckpoint{}).Where("id = ?", checkpoint.ID).Updates(map[string]any{
			"pending_event_id": delivery.Envelope.EventID,
			"pending_revision": delivery.Envelope.AggregateRevision,
			"claim_token":      claimToken, "claim_expires_at": expiresAt, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		claim = eventingapp.InboxClaim{Disposition: eventingapp.InboxAcquired, ClaimToken: claimToken, Attempt: attempt}
		return nil
	})
	if err != nil {
		return eventingapp.InboxClaim{}, err
	}
	return claim, nil
}

func (repo *Repository) Complete(ctx context.Context, delivery eventingapp.InboxDelivery, claimToken string, now time.Time) error {
	ids, err := parseDeliveryIDs(delivery)
	if err != nil || claimToken == "" {
		return errors.New("inbox completion identity is invalid")
	}
	return repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inbox, checkpoint, lockErr := repo.lockDelivery(tx, delivery, ids)
		if lockErr != nil {
			return lockErr
		}
		if inbox.Status != "processing" || inbox.ClaimToken != claimToken || checkpoint.ClaimToken != claimToken ||
			checkpoint.PendingEventID != delivery.Envelope.EventID {
			return errors.New("inbox completion claim is stale")
		}
		status := "processed"
		updates := map[string]any{
			"pending_event_id": "", "pending_revision": 0, "claim_token": "", "claim_expires_at": nil,
			"updated_at": now,
		}
		if checkpoint.LastRevision >= delivery.Envelope.AggregateRevision {
			status = "ignored_stale"
		} else {
			updates["last_revision"] = delivery.Envelope.AggregateRevision
			updates["last_event_id"] = delivery.Envelope.EventID
		}
		if err = tx.Model(&model.EventCheckpoint{}).Where("id = ?", checkpoint.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&model.InboxEvent{}).Where("id = ?", inbox.ID).Updates(map[string]any{
			"status": status, "claim_token": "", "claim_expires_at": nil, "processed_at": now,
		}).Error
	})
}

func (repo *Repository) MarkDeadLetter(
	ctx context.Context,
	delivery eventingapp.InboxDelivery,
	claimToken string,
	letter eventingapp.DeadLetter,
	now time.Time,
) error {
	ids, err := parseDeliveryIDs(delivery)
	if err != nil || claimToken == "" {
		return errors.New("dead letter identity is invalid")
	}
	record, err := deadLetterRecord(letter, now)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inbox, checkpoint, lockErr := repo.lockDelivery(tx, delivery, ids)
		if lockErr != nil {
			return lockErr
		}
		if inbox.Status != "processing" || inbox.ClaimToken != claimToken || checkpoint.ClaimToken != claimToken ||
			checkpoint.PendingEventID != delivery.Envelope.EventID {
			return errors.New("dead letter claim is stale")
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
			return err
		}
		if err = tx.Model(&model.EventCheckpoint{}).Where("id = ?", checkpoint.ID).Updates(map[string]any{
			"pending_event_id": "", "pending_revision": 0, "claim_token": "", "claim_expires_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.InboxEvent{}).Where("id = ?", inbox.ID).Updates(map[string]any{
			"status": "dead_lettered", "claim_token": "", "claim_expires_at": nil,
			"last_error": boundedError(letter.FailureMessage), "processed_at": now,
		}).Error
	})
}

func (repo *Repository) RecordRejected(ctx context.Context, letter eventingapp.DeadLetter, now time.Time) error {
	record, err := deadLetterRecord(letter, now)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error
}

func (repo *Repository) ClaimReplay(
	ctx context.Context,
	filter eventingapp.ReplayFilter,
	now time.Time,
	lease time.Duration,
	limit int,
	newID func() string,
) ([]eventingapp.ReplayClaim, error) {
	if repo == nil || repo.database == nil || filter.ConsumerGroup == "" || filter.ProjectID == "" ||
		len(filter.EventTypes) == 0 || filter.FailedAfter.IsZero() || filter.FailedBefore.IsZero() ||
		lease <= 0 || limit < 1 || limit > 200 || newID == nil {
		return nil, errors.New("dead letter replay claim is invalid")
	}
	var claims []eventingapp.ReplayClaim
	err := repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []model.DeadLetter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("consumer_group = ? AND project_id = ? AND event_type IN ? AND replayable = ?", filter.ConsumerGroup, filter.ProjectID, filter.EventTypes, true).
			Where("failed_at >= ? AND failed_at < ?", filter.FailedAfter, filter.FailedBefore).
			Where("status = ? OR (status = ? AND claim_expires_at <= ?)", "ready", "replay_claimed", now).
			Order("failed_at ASC").Order("id ASC").Limit(limit).Find(&records).Error; err != nil {
			return err
		}
		claims = make([]eventingapp.ReplayClaim, 0, len(records))
		for _, record := range records {
			claimToken := newID()
			if claimToken == "" {
				return errors.New("dead letter replay token is empty")
			}
			expiresAt := now.Add(lease)
			if err := tx.Model(&model.DeadLetter{}).Where("id = ?", record.ID).Updates(map[string]any{
				"status": "replay_claimed", "claim_token": claimToken, "claim_expires_at": expiresAt,
				"replay_count": record.ReplayCount + 1, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			result := tx.Model(&model.InboxEvent{}).
				Where("consumer_group = ? AND event_id = ? AND status = ?", record.ConsumerGroup, record.EventID, "dead_lettered").
				Updates(map[string]any{
					"status": "retryable", "claim_token": "", "claim_expires_at": nil,
					"last_error": "", "processed_at": nil,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("dead letter inbox state is not replayable")
			}
			letter, err := deadLetterDomain(record)
			if err != nil {
				return err
			}
			claims = append(claims, eventingapp.ReplayClaim{Letter: letter, ClaimToken: claimToken})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func (repo *Repository) MarkReplayed(ctx context.Context, deadLetterID, claimToken string, now time.Time) error {
	id, err := uuid.Parse(deadLetterID)
	if err != nil || claimToken == "" {
		return errors.New("dead letter replay identity is invalid")
	}
	result := repo.database.WithContext(ctx).Model(&model.DeadLetter{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "replay_claimed", claimToken).
		Updates(map[string]any{
			"status": "replayed", "claim_token": "", "claim_expires_at": nil,
			"last_replayed_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("dead letter replay claim is stale")
	}
	return nil
}

func (repo *Repository) ReleaseReplay(ctx context.Context, deadLetterID, claimToken, _ string, now time.Time) error {
	id, err := uuid.Parse(deadLetterID)
	if err != nil || claimToken == "" {
		return errors.New("dead letter replay release identity is invalid")
	}
	return repo.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record model.DeadLetter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND claim_token = ?", id, "replay_claimed", claimToken).
			First(&record).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.DeadLetter{}).Where("id = ?", id).Updates(map[string]any{
			"status": "ready", "claim_token": "", "claim_expires_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.InboxEvent{}).
			Where("consumer_group = ? AND event_id = ? AND status = ?", record.ConsumerGroup, record.EventID, "retryable").
			Update("status", "dead_lettered").Error
	})
}

type deliveryIDs struct {
	workspaceID uuid.UUID
	projectID   uuid.UUID
}

func parseDeliveryIDs(delivery eventingapp.InboxDelivery) (deliveryIDs, error) {
	workspaceID, err := uuid.Parse(delivery.Envelope.WorkspaceID)
	if err != nil {
		return deliveryIDs{}, err
	}
	projectID, err := uuid.Parse(delivery.Envelope.ProjectID)
	if err != nil {
		return deliveryIDs{}, err
	}
	if delivery.Group == "" || delivery.Envelope.EventID == "" || delivery.Message.Topic == "" ||
		delivery.Message.Partition < 0 || delivery.Message.Offset < 0 {
		return deliveryIDs{}, errors.New("delivery coordinate is invalid")
	}
	return deliveryIDs{workspaceID: workspaceID, projectID: projectID}, nil
}

func ensureDeliveryOwner(tx *gorm.DB, ids deliveryIDs) error {
	var project model.Project
	err := tx.Select("id").Where("id = ? AND workspace_id = ?", ids.projectID, ids.workspaceID).Take(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return eventingapp.Permanent(eventingapp.ErrEventOwnerNotFound)
	}
	return err
}

func (repo *Repository) lockCheckpoint(
	tx *gorm.DB,
	delivery eventingapp.InboxDelivery,
	ids deliveryIDs,
	now time.Time,
	newID func() string,
) (model.EventCheckpoint, error) {
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"consumer_group = ? AND workspace_id = ? AND aggregate_kind = ? AND aggregate_id = ?",
		delivery.Group, ids.workspaceID, delivery.Envelope.AggregateKind, delivery.Envelope.AggregateID,
	)
	var checkpoint model.EventCheckpoint
	if err := query.First(&checkpoint).Error; err == nil {
		return checkpoint, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EventCheckpoint{}, err
	}
	id, err := nextUUID(newID)
	if err != nil {
		return model.EventCheckpoint{}, err
	}
	checkpoint = model.EventCheckpoint{
		ID: id, ConsumerGroup: delivery.Group, WorkspaceID: ids.workspaceID, ProjectID: ids.projectID,
		AggregateKind: delivery.Envelope.AggregateKind, AggregateID: delivery.Envelope.AggregateID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err = tx.Create(&checkpoint).Error; err != nil {
		return model.EventCheckpoint{}, err
	}
	return checkpoint, nil
}

func (repo *Repository) persistStaleInbox(
	tx *gorm.DB,
	found bool,
	inbox model.InboxEvent,
	delivery eventingapp.InboxDelivery,
	ids deliveryIDs,
	now time.Time,
	newID func() string,
) error {
	if found {
		return tx.Model(&model.InboxEvent{}).Where("id = ?", inbox.ID).Updates(map[string]any{
			"status": "ignored_stale", "claim_token": "", "claim_expires_at": nil, "processed_at": now,
		}).Error
	}
	id, err := nextUUID(newID)
	if err != nil {
		return err
	}
	return tx.Create(&model.InboxEvent{
		ID: id, ConsumerGroup: delivery.Group, EventID: delivery.Envelope.EventID,
		EventType: delivery.Envelope.EventType, WorkspaceID: ids.workspaceID, ProjectID: ids.projectID,
		AggregateKind: delivery.Envelope.AggregateKind, AggregateID: delivery.Envelope.AggregateID,
		AggregateRevision: delivery.Envelope.AggregateRevision, PayloadHash: delivery.Envelope.PayloadHash,
		OriginalTopic: delivery.Message.Topic, SourcePartition: delivery.Message.Partition,
		SourceOffset: delivery.Message.Offset, Status: "ignored_stale", Attempts: 0,
		ReceivedAt: now, ProcessedAt: &now,
	}).Error
}

func (repo *Repository) lockDelivery(
	tx *gorm.DB,
	delivery eventingapp.InboxDelivery,
	ids deliveryIDs,
) (model.InboxEvent, model.EventCheckpoint, error) {
	var inbox model.InboxEvent
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("consumer_group = ? AND event_id = ?", delivery.Group, delivery.Envelope.EventID).
		First(&inbox).Error; err != nil {
		return model.InboxEvent{}, model.EventCheckpoint{}, err
	}
	var checkpoint model.EventCheckpoint
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"consumer_group = ? AND workspace_id = ? AND aggregate_kind = ? AND aggregate_id = ?",
		delivery.Group, ids.workspaceID, delivery.Envelope.AggregateKind, delivery.Envelope.AggregateID,
	).First(&checkpoint).Error; err != nil {
		return model.InboxEvent{}, model.EventCheckpoint{}, err
	}
	return inbox, checkpoint, nil
}

func outboxDomain(record model.OutboxEvent) (eventing.OutboxEvent, error) {
	value := eventing.OutboxEvent{
		ID: record.ID.String(), EventType: record.EventType, EventVersion: record.EventVersion,
		WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		AggregateKind: record.AggregateKind, AggregateID: record.AggregateID,
		AggregateRevision: record.AggregateRevision, SourceReceiptID: record.SourceReceiptID.String(),
		Payload: append([]byte(nil), record.Payload...), PayloadHash: record.PayloadHash, OccurredAt: record.OccurredAt,
	}
	if _, err := eventing.NewEnvelope(value, eventing.TraceContext{RequestID: value.SourceReceiptID}); err != nil {
		return eventing.OutboxEvent{}, fmt.Errorf("persisted outbox event is invalid: %w", err)
	}
	return value, nil
}

func deadLetterRecord(letter eventingapp.DeadLetter, now time.Time) (model.DeadLetter, error) {
	id, err := uuid.Parse(letter.ID)
	if err != nil || letter.ConsumerGroup == "" || letter.EventID == "" || letter.OriginalTopic == "" ||
		letter.DLQTopic == "" || letter.PayloadHash == "" || letter.FailureCode == "" || letter.FailedAt.IsZero() {
		return model.DeadLetter{}, errors.New("dead letter is invalid")
	}
	return model.DeadLetter{
		ID: id, ConsumerGroup: letter.ConsumerGroup, EventID: letter.EventID, EventType: letter.EventType,
		ProjectID: letter.ProjectID, AggregateKind: letter.AggregateKind, AggregateID: letter.AggregateID,
		AggregateRevision: letter.AggregateRevision, OriginalTopic: letter.OriginalTopic, DLQTopic: letter.DLQTopic,
		SourcePartition: letter.SourcePartition, SourceOffset: letter.SourceOffset, PayloadHash: letter.PayloadHash,
		FailureCode: letter.FailureCode, FailureMessage: boundedError(letter.FailureMessage),
		Replayable: letter.Replayable, Envelope: datatypes.JSON(letter.Envelope), Status: "ready",
		FailedAt: letter.FailedAt, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func deadLetterDomain(record model.DeadLetter) (eventingapp.DeadLetter, error) {
	letter := eventingapp.DeadLetter{
		ID: record.ID.String(), Schema: "lanverse.event.dead-letter", ConsumerGroup: record.ConsumerGroup,
		EventID: record.EventID, EventType: record.EventType, ProjectID: record.ProjectID,
		AggregateKind: record.AggregateKind, AggregateID: record.AggregateID,
		AggregateRevision: record.AggregateRevision, OriginalTopic: record.OriginalTopic,
		DLQTopic: record.DLQTopic, SourcePartition: record.SourcePartition, SourceOffset: record.SourceOffset,
		PayloadHash: record.PayloadHash, FailureCode: record.FailureCode, FailureMessage: record.FailureMessage,
		Replayable: record.Replayable, Envelope: append([]byte(nil), record.Envelope...), FailedAt: record.FailedAt,
	}
	if letter.Replayable {
		envelope, err := eventing.DecodeEnvelope(letter.Envelope)
		if err != nil || envelope.EventID != letter.EventID || envelope.PayloadHash != letter.PayloadHash {
			return eventingapp.DeadLetter{}, errors.New("persisted dead letter envelope has drifted")
		}
	}
	return letter, nil
}

func nextUUID(newID func() string) (uuid.UUID, error) {
	value, err := uuid.Parse(newID())
	if err != nil {
		return uuid.Nil, errors.New("generated eventing id is not a UUID")
	}
	return value, nil
}

func boundedError(message string) string {
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
