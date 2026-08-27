package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type ProductionBibleVersionInput struct {
	ID, WorkspaceID, ProjectID                 string
	DocumentRevisionID, DocumentRevisionHash   string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateContentHash                       string
	CandidateRevisionNo                        int64
	Version                                    int
	ReviewDecisionID                           string
	Snapshot                                   json.RawMessage
	CreatedBy                                  string
	CreatedAt                                  time.Time
}

type ProductionBibleVersion struct {
	ID                    string          `json:"id"`
	WorkspaceID           string          `json:"workspace_id"`
	ProjectID             string          `json:"project_id"`
	DocumentRevisionID    string          `json:"document_revision_id"`
	DocumentRevisionHash  string          `json:"document_revision_hash"`
	CandidateRevisionID   string          `json:"candidate_revision_id"`
	CandidateRevisionNo   int64           `json:"candidate_revision_no"`
	CandidateRevisionHash string          `json:"candidate_revision_hash"`
	CandidateContentHash  string          `json:"candidate_content_hash"`
	Version               int             `json:"version"`
	ReviewDecisionID      string          `json:"review_decision_id"`
	Snapshot              json.RawMessage `json:"snapshot"`
	ContentHash           string          `json:"content_hash"`
	CreatedBy             string          `json:"created_by"`
	CreatedAt             time.Time       `json:"created_at"`
}

func NewProductionBibleVersion(input ProductionBibleVersionInput) (ProductionBibleVersion, error) {
	for _, identifier := range []string{
		input.ID, input.WorkspaceID, input.ProjectID, input.DocumentRevisionID,
		input.CandidateRevisionID, input.ReviewDecisionID, input.CreatedBy,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ProductionBibleVersion{}, errors.New("invalid Production Bible Version identity")
		}
	}
	if input.CandidateRevisionNo < 1 || input.Version < 1 || input.CreatedAt.IsZero() ||
		!hashPattern.MatchString(input.DocumentRevisionHash) ||
		!hashPattern.MatchString(input.CandidateRevisionHash) ||
		!hashPattern.MatchString(input.CandidateContentHash) {
		return ProductionBibleVersion{}, errors.New("invalid Production Bible Version input")
	}
	var preliminary StoryReconciliationCandidate
	if err := json.Unmarshal(input.Snapshot, &preliminary); err != nil {
		return ProductionBibleVersion{}, errors.New("invalid Production Bible Version snapshot")
	}
	candidate, err := DecodeStoryReconciliationCandidate(
		input.Snapshot,
		StoryReconciliationCandidateEvidence(preliminary),
	)
	if err != nil {
		return ProductionBibleVersion{}, errors.New("invalid Production Bible Version snapshot")
	}
	canonical, err := json.Marshal(candidate)
	canonicalHash, hashErr := canonicalSnapshotHash(canonical)
	if err != nil || hashErr != nil || canonicalHash != input.CandidateContentHash {
		return ProductionBibleVersion{}, errors.New("Production Bible Version snapshot hash has drifted")
	}
	if !bytes.Equal(bytes.TrimSpace(input.Snapshot), canonical) {
		input.Snapshot = canonical
	}
	version := ProductionBibleVersion{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		DocumentRevisionID: input.DocumentRevisionID, DocumentRevisionHash: input.DocumentRevisionHash,
		CandidateRevisionID: input.CandidateRevisionID, CandidateRevisionNo: input.CandidateRevisionNo,
		CandidateRevisionHash: input.CandidateRevisionHash, CandidateContentHash: input.CandidateContentHash,
		Version: input.Version, ReviewDecisionID: input.ReviewDecisionID,
		Snapshot: append(json.RawMessage(nil), input.Snapshot...), CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC(),
	}
	version.ContentHash, err = CanonicalStoryHash(struct {
		Schema                                                           string `json:"schema"`
		ID, WorkspaceID, ProjectID                                       string
		DocumentRevisionID, DocumentRevisionHash                         string
		CandidateRevisionID, CandidateRevisionHash, CandidateContentHash string
		CandidateRevisionNo                                              int64
		Version                                                          int
		ReviewDecisionID, CreatedBy                                      string
	}{
		"production-bible-version-v1", version.ID, version.WorkspaceID, version.ProjectID,
		version.DocumentRevisionID, version.DocumentRevisionHash,
		version.CandidateRevisionID, version.CandidateRevisionHash, version.CandidateContentHash,
		version.CandidateRevisionNo, version.Version, version.ReviewDecisionID, version.CreatedBy,
	})
	if err != nil {
		return ProductionBibleVersion{}, err
	}
	return version, nil
}

func canonicalSnapshotHash(snapshot json.RawMessage) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(snapshot))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return SourceTextHash(string(bytes.TrimSpace(canonical.Bytes()))), nil
}
