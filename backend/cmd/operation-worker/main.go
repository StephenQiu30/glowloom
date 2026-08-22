package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/adapters/postgres"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/database"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/messaging"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/objectstorage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.Connect(ctx)
	if err != nil {
		fatal("database connection failed", err)
	}
	defer pool.Close()
	storage, err := objectstorage.New(ctx)
	if err != nil {
		fatal("object storage connection failed", err)
	}
	repository := postgres.New(pool, storage)
	writer := &kafka.Writer{Addr: kafka.TCP(messaging.Brokers()...), Topic: messaging.ScriptAnalysisTopic, Balancer: &kafka.Hash{}, AllowAutoTopicCreation: true, BatchTimeout: 100 * time.Millisecond}
	defer writer.Close()
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: messaging.Brokers(), GroupID: "lanverse-operation-worker", Topic: messaging.ScriptAnalysisTopic, MinBytes: 1, MaxBytes: 10e6, StartOffset: kafka.FirstOffset, CommitInterval: 500 * time.Millisecond})
	defer reader.Close()

	go publishOutbox(ctx, repository, writer)
	slog.Info("lanverse operation worker started", "brokers", messaging.Brokers(), "topic", messaging.ScriptAnalysisTopic)
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("fetch kafka message failed", "error", err)
			continue
		}
		messageID := fmt.Sprintf("%s/%d/%d", message.Topic, message.Partition, message.Offset)
		seen, err := repository.HasInboxMessage(ctx, messageID)
		if err != nil {
			slog.Error("check inbox failed", "error", err)
			continue
		}
		if !seen {
			var request postgres.AnalysisRequest
			if err := json.Unmarshal(message.Value, &request); err != nil {
				slog.Error("decode analysis request failed", "error", err)
				continue
			}
			if err := repository.ProcessAnalysis(ctx, request); err != nil {
				slog.Error("process analysis failed", "operation_id", request.OperationID, "error", err)
				continue
			}
			if err := repository.RecordInboxMessage(ctx, messageID, message.Topic); err != nil {
				slog.Error("record inbox failed", "error", err)
				continue
			}
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			slog.Error("commit kafka message failed", "error", err)
		}
	}
}

func publishOutbox(ctx context.Context, repository *postgres.Repository, writer *kafka.Writer) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := repository.PendingOutbox(ctx, 20)
			if err != nil {
				slog.Error("list outbox failed", "error", err)
				continue
			}
			for _, event := range events {
				if err := writer.WriteMessages(ctx, kafka.Message{Key: []byte(event.Key), Value: event.Payload}); err != nil {
					slog.Error("publish outbox failed", "event_id", event.ID, "error", err)
					continue
				}
				if err := repository.MarkOutboxPublished(ctx, event.ID); err != nil {
					slog.Error("mark outbox published failed", "event_id", event.ID, "error", err)
				}
			}
		}
	}
}

func fatal(message string, err error) {
	slog.Error(message, "error", err)
	os.Exit(1)
}
