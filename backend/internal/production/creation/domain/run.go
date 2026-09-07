package domain

import "time"

const (
	FlowType = "lanverse.creation.text-storyboard.production"
	Queued   = "queued"
	Unknown  = "delivery_unknown"
	Blocked  = "delivery_blocked"
	Accepted = "accepted"
)

type Source struct {
	DocumentID  string `json:"document_id"`
	RevisionID  string `json:"revision_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
	SpanIndexID string `json:"span_index_id"`
}

type Command struct {
	Schema      string `json:"schema"`
	CommandID   string `json:"command_id"`
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
	ActorID     string `json:"actor_id"`
	Source      Source `json:"source"`
	FlowType    string `json:"flow_type"`
	WorkflowID  string `json:"workflow_id"`
}

type Acceptance struct {
	Schema      string    `json:"schema"`
	CommandID   string    `json:"command_id"`
	RunID       string    `json:"run_id"`
	PayloadHash string    `json:"payload_hash"`
	FlowType    string    `json:"flow_type"`
	WorkflowID  string    `json:"workflow_id"`
	ReceiptID   string    `json:"receipt_id"`
	AcceptedAt  time.Time `json:"accepted_at"`
}

type Run struct {
	Command                                          Command
	InputHash, PayloadHash, IdempotencyKey, Endpoint string
	TokenVersion                                     int
	Status, LastError                                string
	Revision                                         int64
	Acceptance                                       *Acceptance
	CreatedAt, UpdatedAt                             time.Time
}

type Delivery struct {
	Run      Run
	Fence    int64
	Attempts int
}
