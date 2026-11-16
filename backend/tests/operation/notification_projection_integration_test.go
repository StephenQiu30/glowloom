package operation_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"gorm.io/gorm"

	identityevent "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/event"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	kafkainbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/application"
	outbox "github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox"
	kafkaoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox/kafka"
)

type notificationSourceSink struct {
	event string
	calls atomic.Int32
}

func (s *notificationSourceSink) Publish(_ context.Context, event realtime.Event) error {
	if event.ID == s.event {
		s.calls.Add(1)
	}
	return nil
}

type notificationSourceConsumer struct {
	next  inbox.Handler
	event uuid.UUID
	seen  int
}

func (h *notificationSourceConsumer) Handle(ctx context.Context, record inbox.Record) error {
	if err := h.next.Handle(ctx, record); err != nil {
		return err
	}
	var e struct {
		EventID uuid.UUID `json:"event_id"`
	}
	if err := json.Unmarshal(record.Value, &e); err != nil {
		return err
	}
	if e.EventID == h.event {
		h.seen++
	}
	return nil
}

func assertOperationNotificationKafka(t *testing.T, database *gorm.DB, actor identity.Principal, project, operation uuid.UUID, payload []byte) {
	t.Helper()
	brokers := os.Getenv("GLOWLOOM_TEST_NOTIFICATION_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("dedicated disposable notification Kafka absent")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var event struct {
		EventID uuid.UUID `json:"event_id"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	producer, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	create := kmsg.NewPtrCreateTopicsRequest()
	create.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: realtime.OperationStatusTopic, NumPartitions: 1, ReplicationFactor: 1}}
	create.TimeoutMillis = 10000
	response, err := producer.Request(ctx, create)
	if err != nil {
		t.Fatal("prepare isolated source topic", err)
	}
	for _, topic := range response.(*kmsg.CreateTopicsResponse).Topics {
		if topic.ErrorCode != 0 && topic.ErrorCode != 36 {
			t.Fatal("isolated Kafka topic", topic.ErrorCode)
		}
	}
	sink := &notificationSourceSink{event: event.EventID.String()}
	svc := identity.NewNotifications(pgidentity.NewNotificationStore(database), time.Now)
	handler := &notificationSourceConsumer{next: identityevent.WithOperationNotifications(realtime.NewHandler(pginbox.NewStore(database), sink), svc), event: event.EventID}
	consumer, err := kafkainbox.NewConsumer(brokers, "notification-realtime-test-"+uuid.NewString(), []string{realtime.OperationStatusTopic}, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	publisher := kafkaoutbox.NewPublisher(producer)
	for i := 0; i < 2; i++ {
		if err = publisher.Publish(ctx, outbox.Event{ID: event.EventID.String(), Topic: realtime.OperationStatusTopic, PartitionKey: project.String(), Payload: payload}); err != nil {
			t.Fatal("publish actual committed source envelope", err)
		}
	}
	for i := 0; i < 200 && handler.seen < 2; i++ {
		if _, err = consumer.RunOnce(ctx); err != nil {
			t.Fatal("consume actual source envelope", err)
		}
	}
	if handler.seen != 2 || sink.calls.Load() != 1 {
		t.Fatal("existing realtime effect was not replay safe", handler.seen, sink.calls.Load())
	}
	page, err := svc.Inbox(ctx, actor, identity.NotificationFilter{Category: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].SourceID == nil || *page.Items[0].SourceID != operation || page.Items[0].Title != "任务已完成" || page.Items[0].AnnouncementID != nil {
		t.Fatal("source Outbox/Kafka/Identity projection", page)
	}
	var count int64
	if err = database.Table("infra.processed_event").Where("consumer=? AND event_id=?", "identity.notification.operation:"+actor.ID.String(), event.EventID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("event projection marker", count, err)
	}
}
