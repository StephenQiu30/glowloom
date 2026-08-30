package eventing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

func TestDeadLetterReplayUsesOriginalTopicEnvelopeAndEventID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	envelope := eventingFixture(t)
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	letter := eventingapp.DeadLetter{
		ID: "00000000-0000-0000-0000-000000000401", Schema: "lanverse.event.dead-letter",
		ConsumerGroup: "lanverse.search-projector", EventID: envelope.EventID,
		EventType: envelope.EventType, ProjectID: envelope.ProjectID, AggregateKind: envelope.AggregateKind,
		AggregateID: envelope.AggregateID, AggregateRevision: envelope.AggregateRevision,
		OriginalTopic: "lanverse.business.storygraph-version.published",
		DLQTopic:      "lanverse.business.storygraph-version.dead-letter", PayloadHash: envelope.PayloadHash,
		Replayable: true, Envelope: encoded, FailedAt: now.Add(-time.Hour),
	}
	repository := &replayRepository{letter: letter}
	broker := &recordingBroker{failures: 1}
	replayer := eventingapp.NewReplayer(repository, broker, eventingapp.ReplayConfig{
		Now: func() time.Time { return now }, NewID: func() string { return "replay-claim" },
		Lease: time.Minute, BatchSize: 10,
	})
	filter := eventingapp.ReplayFilter{
		ConsumerGroup: letter.ConsumerGroup, ProjectID: letter.ProjectID,
		EventTypes: []string{eventing.StoryGraphVersionPublished}, FailedAfter: now.Add(-2 * time.Hour),
		FailedBefore: now,
	}
	if _, err = replayer.ReplayOnce(context.Background(), filter); err == nil {
		t.Fatal("unknown replay publication must remain retryable")
	}
	replayed, err := replayer.ReplayOnce(context.Background(), filter)
	if err != nil || replayed != 1 || !repository.replayed || repository.attempts != 2 {
		t.Fatalf("dead letter was not replayed: count=%d error=%v repository=%#v", replayed, err, repository)
	}
	if len(broker.messages) != 2 || broker.messages[0].Topic != letter.OriginalTopic ||
		broker.messages[0].Key != envelope.EventID || broker.messages[1].Key != envelope.EventID ||
		string(broker.messages[0].Value) != string(encoded) || string(broker.messages[1].Value) != string(encoded) {
		t.Fatalf("replay changed the original event: %#v", broker.messages)
	}
}

type replayRepository struct {
	letter     eventingapp.DeadLetter
	claimed    bool
	replayed   bool
	attempts   int
	claimToken string
}

func (repo *replayRepository) ClaimReplay(_ context.Context, _ eventingapp.ReplayFilter, _ time.Time, _ time.Duration, _ int, newID func() string) ([]eventingapp.ReplayClaim, error) {
	if repo.replayed || repo.claimed {
		return nil, nil
	}
	repo.claimed = true
	repo.attempts++
	repo.claimToken = newID()
	return []eventingapp.ReplayClaim{{Letter: repo.letter, ClaimToken: repo.claimToken}}, nil
}

func (repo *replayRepository) MarkReplayed(_ context.Context, id, claimToken string, _ time.Time) error {
	if id != repo.letter.ID || claimToken != repo.claimToken || !repo.claimed {
		return errors.New("invalid replay claim")
	}
	repo.claimed = false
	repo.replayed = true
	return nil
}

func (repo *replayRepository) ReleaseReplay(_ context.Context, id, claimToken, _ string, _ time.Time) error {
	if id != repo.letter.ID || claimToken != repo.claimToken || !repo.claimed {
		return errors.New("invalid replay claim")
	}
	repo.claimed = false
	return nil
}
