// Package redisconn owns the Redis client shared by backend adapters.
package redisconn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrURLRequired means the process has no Redis connection URL.
	ErrURLRequired = errors.New("LV_REDIS_URL is required")
	// ErrInvalidURL means the Redis URL could not be parsed.
	ErrInvalidURL = errors.New("invalid LV_REDIS_URL")
)

// Connection owns the Redis client and its connection pool.
type Connection struct {
	Client *redis.Client
}

// Open parses a Redis URL and creates a client. Readiness is checked by Ping.
func Open(url string) (*Connection, error) {
	if strings.TrimSpace(url) == "" {
		return nil, ErrURLRequired
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		// ParseURL errors can contain the supplied URL and its password.
		return nil, ErrInvalidURL
	}
	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second
	return &Connection{Client: redis.NewClient(opts)}, nil
}

// Ping checks whether Redis currently accepts commands.
func (c *Connection) Ping(ctx context.Context) error {
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}

// Close releases the Redis client's connections.
func (c *Connection) Close() error {
	if err := c.Client.Close(); err != nil {
		return fmt.Errorf("close Redis client: %w", err)
	}
	return nil
}
