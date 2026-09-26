// Package kafkaconn owns the Kafka client used by backend event adapters.
package kafkaconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
)

var (
	// ErrBrokersRequired means no Kafka seed broker is configured.
	ErrBrokersRequired = errors.New("LV_KAFKA_BROKERS is required")
	// ErrInvalidBrokers means the broker list is not comma-separated host:port addresses.
	ErrInvalidBrokers = errors.New("invalid LV_KAFKA_BROKERS")
)

// Connection owns a Kafka client. Close releases its background resources.
type Connection struct {
	Client *kgo.Client
}

// Open creates a client from comma-separated broker addresses. Ping confirms reachability.
func Open(brokers string) (*Connection, error) {
	if strings.TrimSpace(brokers) == "" {
		return nil, ErrBrokersRequired
	}
	seeds := strings.Split(brokers, ",")
	for i, seed := range seeds {
		seed = strings.TrimSpace(seed)
		host, port, err := net.SplitHostPort(seed)
		if err != nil || host == "" || strings.Contains(host, "/") {
			return nil, ErrInvalidBrokers
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, ErrInvalidBrokers
		}
		seeds[i] = seed
	}
	client, err := kgo.NewClient(kgo.SeedBrokers(seeds...), kgo.ClientID("lanverse-backend"))
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Connection{Client: client}, nil
}

// Ping sends a broker metadata request without creating topics or writing records.
func (c *Connection) Ping(ctx context.Context) error {
	if err := c.Client.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka: %w", err)
	}
	return nil
}

// Close stops the Kafka client and releases its resources.
func (c *Connection) Close() {
	c.Client.Close()
}
