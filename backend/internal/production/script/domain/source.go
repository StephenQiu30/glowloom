package domain

import "time"

type SourceVersionIdentity struct {
	OwnerKind   string    `json:"owner_kind"`
	LogicalID   string    `json:"logical_id"`
	VersionID   string    `json:"version_id"`
	Revision    int64     `json:"revision"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

type AcceptedSource struct {
	Identity             SourceVersionIdentity `json:"identity"`
	SpanIndexID          string                `json:"span_index_id"`
	SpanIndexHash        string                `json:"span_index_hash"`
	CodepointCount       int                   `json:"codepoint_count"`
	UTF8ByteCount        int                   `json:"utf8_byte_count"`
	NewlineNormalization string                `json:"newline_normalization"`
	CodepointIndexRule   string                `json:"codepoint_index_rule"`
	HeadRevision         int64                 `json:"head_revision"`
	HeadHash             string                `json:"head_hash"`
	CollectionRootHash   string                `json:"collection_root_hash"`
	CollectionReceiptID  string                `json:"collection_receipt_id"`
	CommandReceiptID     string                `json:"command_receipt_id"`
}

type SourceHead struct {
	Exists                 bool
	ProjectID, WorkspaceID string
	DocumentLogicalID      string
	DocumentRevisionID     string
	SpanIndexID, HeadHash  string
	HeadRevision           int64
}

type SourceSpanIndex struct {
	ID, WorkspaceID, ProjectID, DocumentRevisionID string
	SourceHash, NewlineNormalization               string
	CodepointIndexRule                             string
	CodepointCount, UTF8ByteCount                  int
	IndexManifest                                  []byte
	ContentHash, CreatedBy                         string
	CreatedAt                                      time.Time
}

type SourceCollectionReceipt struct {
	ID, WorkspaceID, ProjectID, DocumentRevisionID, SpanIndexID string
	HeadRevision                                                int64
	HeadHash, MembersHash, CollectionRootHash                   string
	Members                                                     []byte
	SourceAcceptanceRef, ReceiptContentHash, CreatedBy          string
	CreatedAt                                                   time.Time
}
