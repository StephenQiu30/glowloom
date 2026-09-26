package application

import "encoding/json"

// Message is the transport-independent SSE frame kept in Redis.
type Message struct {
	ID    string          `json:"id"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}
