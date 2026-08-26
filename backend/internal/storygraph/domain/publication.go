package domain

import (
	"encoding/json"
	"time"
)

type Version struct {
	ID, WorkspaceID, ProjectID                   string
	VersionNo                                    int64
	ParentVersionID, ParentContentHash           *string
	SourceRevisionID, SourceRevisionHash         string
	OwnerHeads                                   []OwnerHeadRef
	OwnerSetHash, SchemaVersion                  string
	Nodes                                        []Node
	Edges                                        []Edge
	TopologyHash, ContentHash, Status, CreatedBy string
	PublishedAt, CreatedAt                       time.Time
}

type Head struct {
	WorkspaceID, ProjectID, CurrentVersionID, CurrentContentHash string
	Revision                                                     int64
	UpdatedAt                                                    time.Time
}

type PublicationState struct {
	WorkspaceID, ProjectID, CurrentVersionID, CurrentContentHash string
	HeadRevision                                                 int64
}

type OutboxEvent struct {
	ID, EventType, WorkspaceID, ProjectID, AggregateKind, AggregateID string
	EventVersion, Attempts                                            int
	AggregateRevision                                                 int64
	SourceReceiptID, PayloadHash, Status                              string
	Payload                                                           json.RawMessage
	OccurredAt, CreatedAt                                             time.Time
}
