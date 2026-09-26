// Package application defines the Kafka record boundary consumed by backend handlers.
package application

import "context"

// Record contains the routing fields and payload needed by event handlers.
type Record struct {
	Topic string
	Key   []byte
	Value []byte
}

// Handler applies one record before its Kafka offset is acknowledged.
type Handler interface {
	Handle(context.Context, Record) error
}
