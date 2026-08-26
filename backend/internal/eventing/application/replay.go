package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

type ReplayFilter struct {
	ConsumerGroup string
	ProjectID     string
	EventTypes    []string
	FailedAfter   time.Time
	FailedBefore  time.Time
}

type ReplayClaim struct {
	Letter     DeadLetter
	ClaimToken string
}

type ReplayRepository interface {
	ClaimReplay(context.Context, ReplayFilter, time.Time, time.Duration, int, func() string) ([]ReplayClaim, error)
	MarkReplayed(context.Context, string, string, time.Time) error
	ReleaseReplay(context.Context, string, string, string, time.Time) error
}

type ReplayConfig struct {
	Now       func() time.Time
	NewID     func() string
	Lease     time.Duration
	BatchSize int
}

type Replayer struct {
	repository ReplayRepository
	broker     PublisherBroker
	config     ReplayConfig
}

func NewReplayer(repository ReplayRepository, broker PublisherBroker, config ReplayConfig) *Replayer {
	return &Replayer{repository: repository, broker: broker, config: config}
}

func (replayer *Replayer) ReplayOnce(ctx context.Context, filter ReplayFilter) (int, error) {
	if replayer == nil || replayer.repository == nil || replayer.broker == nil || replayer.config.Now == nil ||
		replayer.config.NewID == nil || replayer.config.Lease <= 0 || replayer.config.BatchSize < 1 ||
		replayer.config.BatchSize > 200 || strings.TrimSpace(filter.ConsumerGroup) == "" ||
		strings.TrimSpace(filter.ProjectID) == "" || len(filter.EventTypes) == 0 || len(filter.EventTypes) > 10 ||
		filter.FailedAfter.IsZero() || filter.FailedBefore.IsZero() || !filter.FailedAfter.Before(filter.FailedBefore) {
		return 0, errors.New("dead letter replay request is invalid")
	}
	now := replayer.config.Now().UTC()
	claims, err := replayer.repository.ClaimReplay(
		ctx, filter, now, replayer.config.Lease, replayer.config.BatchSize, replayer.config.NewID,
	)
	if err != nil {
		return 0, fmt.Errorf("claim dead letters for replay: %w", err)
	}
	replayed := 0
	for _, claim := range claims {
		if err = replayer.replayClaim(ctx, now, claim); err != nil {
			return replayed, err
		}
		replayed++
	}
	return replayed, nil
}

func (replayer *Replayer) replayClaim(ctx context.Context, now time.Time, claim ReplayClaim) error {
	letter := claim.Letter
	if !letter.Replayable || len(letter.Envelope) == 0 || letter.OriginalTopic == "" || claim.ClaimToken == "" {
		return replayer.release(ctx, now, claim, errors.New("dead letter is not replayable"))
	}
	envelope, err := eventing.DecodeEnvelope(letter.Envelope)
	if err != nil {
		return replayer.release(ctx, now, claim, fmt.Errorf("decode replay envelope: %w", err))
	}
	if envelope.EventID != letter.EventID || envelope.EventType != letter.EventType ||
		envelope.ProjectID != letter.ProjectID || envelope.PayloadHash != letter.PayloadHash {
		return replayer.release(ctx, now, claim, errors.New("dead letter envelope identity has drifted"))
	}
	if err = replayer.broker.Publish(ctx, Message{
		Topic: letter.OriginalTopic, Key: envelope.EventID, Value: append([]byte(nil), letter.Envelope...),
		Headers: map[string]string{
			"lanverse-schema": envelope.Schema, "lanverse-event-id": envelope.EventID,
			"lanverse-event-type": envelope.EventType, "lanverse-replay": "true",
		},
	}); err != nil {
		return replayer.release(ctx, now, claim, fmt.Errorf("replay event %s: %w", envelope.EventID, err))
	}
	if err = replayer.repository.MarkReplayed(ctx, letter.ID, claim.ClaimToken, now); err != nil {
		return fmt.Errorf("mark dead letter %s replayed: %w", letter.ID, err)
	}
	return nil
}

func (replayer *Replayer) release(ctx context.Context, now time.Time, claim ReplayClaim, replayErr error) error {
	releaseErr := replayer.repository.ReleaseReplay(
		ctx, claim.Letter.ID, claim.ClaimToken, safeError(replayErr), now,
	)
	if releaseErr != nil {
		return errors.Join(replayErr, fmt.Errorf("release dead letter %s: %w", claim.Letter.ID, releaseErr))
	}
	return replayErr
}
