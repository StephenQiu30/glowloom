package application

import "github.com/google/uuid"

// ProjectCopyPrimaryCandidate identifies a frozen source and its current copied document.
// It contains no private text and does not assert which source was originally primary.
type ProjectCopyPrimaryCandidate struct {
	SourceDocumentID uuid.UUID `json:"source_document_id"`
	TargetDocumentID uuid.UUID `json:"target_document_id"`
	Revision         int64     `json:"revision"`
	Epoch            uuid.UUID `json:"epoch"`
	GraphHash        string    `json:"graph_hash"`
}

// ProjectCopyPrimaryProof binds current candidates to the immutable owner's receipt.
type ProjectCopyPrimaryProof struct {
	Receipt    ProjectCopyReceipt
	Candidates []ProjectCopyPrimaryCandidate
}
