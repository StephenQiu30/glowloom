package kafkaconn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestOpenRejectsMissingBrokers(t *testing.T) {
	_, err := Open(" ")
	if !errors.Is(err, ErrBrokersRequired) {
		t.Fatalf("Open() error = %v, want ErrBrokersRequired", err)
	}
}

func TestOpenRejectsMalformedBrokers(t *testing.T) {
	for _, brokers := range []string{"127.0.0.1:9092,", "127.0.0.1:not-a-port", "redis://127.0.0.1:9092"} {
		_, err := Open(brokers)
		if !errors.Is(err, ErrInvalidBrokers) {
			t.Errorf("Open(%q) error = %v, want ErrInvalidBrokers", brokers, err)
		}
	}
}

func TestPingWithKafka(t *testing.T) {
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("set LV_TEST_KAFKA_BROKERS to a disposable Kafka broker")
	}
	conn, err := Open(brokers)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
