package eventing_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	eventinggorm "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/gormdb"
	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestGORMOutboxInboxRevisionAndDeadLetterState(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL eventing journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	workspaceID, projectID := uuid.New(), uuid.New()
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "eventing", Status: "active", Revision: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "eventing", AspectRatio: "16:9", Language: "zh-CN",
		TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 11, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"content_hash":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","owner_set_hash":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","topology_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","version_id":"` + uuid.NewString() + `","version_no":2}`)
	payloadHash, err := eventing.HashPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	eventID, receiptID := uuid.New(), uuid.New()
	outbox := model.OutboxEvent{
		ID: eventID, EventType: eventing.StoryGraphVersionPublished, EventVersion: 1,
		WorkspaceID: workspaceID, ProjectID: projectID, AggregateKind: "storygraph",
		AggregateID: projectID.String(), AggregateRevision: 2, SourceReceiptID: receiptID,
		PayloadHash: payloadHash, Status: "pending",
		OccurredAt: now, CreatedAt: now,
	}
	outbox.Payload = append(outbox.Payload, payload...)
	if err = database.Create(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	repository := eventinggorm.New(database)
	ownerlessProjectID := uuid.New()
	ownerlessEnvelope := eventingFixtureForProject(t, workspaceID.String(), ownerlessProjectID.String(), 1)
	ownerlessDelivery := eventingapp.InboxDelivery{
		Group: "lanverse.search-projector.v1." + ownerlessProjectID.String(),
		Message: eventingapp.IncomingMessage{
			Topic: "lanverse.business.storygraph-version.v1", Partition: 0, Offset: 20,
			Key: ownerlessEnvelope.EventID,
		},
		Envelope: ownerlessEnvelope,
	}
	if _, ownerErr := repository.Acquire(ctx, ownerlessDelivery, now, time.Minute, uuid.NewString); !eventingapp.IsPermanent(ownerErr) {
		t.Fatalf("ownerless delivery was not rejected permanently: %v", ownerErr)
	}
	var ownerlessInboxCount, ownerlessCheckpointCount int64
	if err = database.Model(&model.InboxEvent{}).Where("event_id = ?", ownerlessEnvelope.EventID).Count(&ownerlessInboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EventCheckpoint{}).
		Where("consumer_group = ?", ownerlessDelivery.Group).Count(&ownerlessCheckpointCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerlessInboxCount != 0 || ownerlessCheckpointCount != 0 {
		t.Fatalf("ownerless delivery persisted FK-backed state: inbox=%d checkpoint=%d", ownerlessInboxCount, ownerlessCheckpointCount)
	}
	claims, err := repository.ClaimPending(ctx, now, time.Minute, 10, uuid.NewString)
	if err != nil || len(claims) != 1 || claims[0].Event.ID != eventID.String() {
		t.Fatalf("outbox was not claimed: claims=%#v error=%v", claims, err)
	}
	if second, claimErr := repository.ClaimPending(ctx, now.Add(30*time.Second), time.Minute, 10, uuid.NewString); claimErr != nil || len(second) != 0 {
		t.Fatalf("active outbox lease was ignored: claims=%#v error=%v", second, claimErr)
	}
	if err = repository.ReleaseClaim(ctx, eventID.String(), claims[0].ClaimToken, "broker outcome unknown"); err != nil {
		t.Fatal(err)
	}
	claims, err = repository.ClaimPending(ctx, now.Add(31*time.Second), time.Minute, 10, uuid.NewString)
	if err != nil || len(claims) != 1 {
		t.Fatalf("released outbox was not reclaimed: claims=%#v error=%v", claims, err)
	}
	if err = repository.MarkPublished(ctx, eventID.String(), claims[0].ClaimToken, now.Add(32*time.Second)); err != nil {
		t.Fatal(err)
	}
	var persistedOutbox model.OutboxEvent
	if err = database.First(&persistedOutbox, "id = ?", eventID).Error; err != nil || persistedOutbox.Attempts != 2 || persistedOutbox.Status != "published" {
		t.Fatalf("outbox publication state is wrong: %#v error=%v", persistedOutbox, err)
	}

	envelope := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 2)
	delivery := eventingapp.InboxDelivery{
		Group:    "lanverse.search-projector.v1." + projectID.String(),
		Message:  eventingapp.IncomingMessage{Topic: "lanverse.business.storygraph-version.v1", Partition: 0, Offset: 21, Key: envelope.EventID},
		Envelope: envelope,
	}
	claim, err := repository.Acquire(ctx, delivery, now, time.Minute, uuid.NewString)
	if err != nil || claim.Disposition != eventingapp.InboxAcquired {
		t.Fatalf("inbox event was not acquired: %#v error=%v", claim, err)
	}
	if err = repository.Complete(ctx, delivery, claim.ClaimToken, now); err != nil {
		t.Fatal(err)
	}
	duplicate, err := repository.Acquire(ctx, delivery, now.Add(time.Second), time.Minute, uuid.NewString)
	if err != nil || duplicate.Disposition != eventingapp.InboxDuplicate {
		t.Fatalf("duplicate event was not fenced: %#v error=%v", duplicate, err)
	}
	staleEnvelope := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 1)
	staleDelivery := delivery
	staleDelivery.Envelope = staleEnvelope
	staleDelivery.Message.Key = staleEnvelope.EventID
	staleDelivery.Message.Offset++
	stale, err := repository.Acquire(ctx, staleDelivery, now.Add(2*time.Second), time.Minute, uuid.NewString)
	if err != nil || stale.Disposition != eventingapp.InboxStale {
		t.Fatalf("out-of-order aggregate revision was not fenced: %#v error=%v", stale, err)
	}

	poisonEnvelope := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 3)
	poisonDelivery := delivery
	poisonDelivery.Envelope = poisonEnvelope
	poisonDelivery.Message.Key = poisonEnvelope.EventID
	poisonDelivery.Message.Offset += 2
	poisonClaim, err := repository.Acquire(ctx, poisonDelivery, now.Add(3*time.Second), time.Minute, uuid.NewString)
	if err != nil || poisonClaim.Disposition != eventingapp.InboxAcquired {
		t.Fatalf("poison event was not acquired: %#v error=%v", poisonClaim, err)
	}
	encodedEnvelope, err := eventing.EncodeEnvelope(poisonEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	letter := eventingapp.DeadLetter{
		ID: uuid.NewString(), Schema: "lanverse.dead-letter.v1", ConsumerGroup: poisonDelivery.Group,
		EventID: poisonEnvelope.EventID, EventType: poisonEnvelope.EventType, ProjectID: poisonEnvelope.ProjectID,
		AggregateKind: poisonEnvelope.AggregateKind, AggregateID: poisonEnvelope.AggregateID,
		AggregateRevision: poisonEnvelope.AggregateRevision, OriginalTopic: poisonDelivery.Message.Topic,
		DLQTopic: "lanverse.business.storygraph-version.dlq.v1", SourcePartition: 0,
		SourceOffset: poisonDelivery.Message.Offset, PayloadHash: poisonEnvelope.PayloadHash,
		FailureCode: "projection_rejected", FailureMessage: "unsupported projection contract",
		Replayable: true, Envelope: encodedEnvelope, FailedAt: now.Add(3 * time.Second),
	}
	if err = repository.MarkDeadLetter(ctx, poisonDelivery, poisonClaim.ClaimToken, letter, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	var inboxCount, checkpointCount, deadLetterCount int64
	for record, count := range map[any]*int64{
		&model.InboxEvent{}: countPointer(&inboxCount), &model.EventCheckpoint{}: countPointer(&checkpointCount),
		&model.DeadLetter{}: countPointer(&deadLetterCount),
	} {
		if err = database.Model(record).Where("consumer_group = ?", delivery.Group).Count(count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if inboxCount != 3 || checkpointCount != 1 || deadLetterCount != 1 {
		t.Fatalf("event facts drifted: inbox=%d checkpoints=%d dead_letters=%d", inboxCount, checkpointCount, deadLetterCount)
	}
	replayClaims, err := repository.ClaimReplay(ctx, eventingapp.ReplayFilter{
		ConsumerGroup: delivery.Group, ProjectID: projectID.String(),
		EventTypes:  []string{eventing.StoryGraphVersionPublished},
		FailedAfter: now, FailedBefore: now.Add(4 * time.Second),
	}, now.Add(4*time.Second), time.Minute, 10, uuid.NewString)
	if err != nil || len(replayClaims) != 1 || replayClaims[0].Letter.EventID != poisonEnvelope.EventID {
		t.Fatalf("dead letter range was not claimed for replay: %#v error=%v", replayClaims, err)
	}
	if err = repository.MarkReplayed(ctx, replayClaims[0].Letter.ID, replayClaims[0].ClaimToken, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	replayedClaim, err := repository.Acquire(ctx, poisonDelivery, now.Add(5*time.Second), time.Minute, uuid.NewString)
	if err != nil || replayedClaim.Disposition != eventingapp.InboxAcquired || replayedClaim.Attempt != 2 {
		t.Fatalf("replayed event did not reuse its inbox identity: %#v error=%v", replayedClaim, err)
	}
}

func eventingFixtureForProject(t *testing.T, workspaceID, projectID string, revision int64) eventing.Envelope {
	t.Helper()
	value, err := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: uuid.NewString(), EventType: eventing.StoryGraphVersionPublished, EventVersion: 1,
		WorkspaceID: workspaceID, ProjectID: projectID, AggregateKind: "storygraph",
		AggregateID: projectID, AggregateRevision: revision, SourceReceiptID: uuid.NewString(),
		Payload:    json.RawMessage(`{"content_hash":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","owner_set_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","topology_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","version_id":"` + uuid.NewString() + `","version_no":` + jsonNumber(revision) + `}`),
		OccurredAt: time.Date(2026, time.August, 27, 11, int(revision), 0, 0, time.UTC),
	}, eventing.TraceContext{RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func countPointer(value *int64) *int64 { return value }
