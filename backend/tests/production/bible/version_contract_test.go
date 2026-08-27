package bible_test

import (
	"encoding/json"
	"testing"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestProductionBibleVersionSealsTheExactReviewedCandidate(t *testing.T) {
	snapshot := json.RawMessage(`{"canonical_entities":[],"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[]}`)
	candidateContentHash, err := agentcontract.CanonicalHash(snapshot)
	if err != nil {
		t.Fatalf("hash candidate snapshot: %v", err)
	}
	input := domain.ProductionBibleVersionInput{
		ID:                    "bbbe968a-d451-41fd-8673-5d0f19ae146a",
		WorkspaceID:           "a84d7969-0029-4b33-a6c7-17e4b2ca7c6e",
		ProjectID:             "c2f39934-9657-48f8-adf4-a9ed20e6e97f",
		DocumentRevisionID:    "897dc52d-bd9b-4a68-994e-ec629c68a5d0",
		DocumentRevisionHash:  "1111111111111111111111111111111111111111111111111111111111111111",
		CandidateRevisionID:   "de7ca5d8-a33b-4b8c-a714-f8e39fc57c34",
		CandidateRevisionNo:   2,
		CandidateRevisionHash: "2222222222222222222222222222222222222222222222222222222222222222",
		CandidateContentHash:  candidateContentHash,
		Version:               1,
		ReviewDecisionID:      "b39fa453-a622-4be3-951f-2cc95263ac8e",
		Snapshot:              snapshot,
		CreatedBy:             "508c6093-9e57-45e0-a62c-d3c1640ef4b1",
		CreatedAt:             time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC),
	}
	version, err := domain.NewProductionBibleVersion(input)
	if err != nil {
		t.Fatalf("seal Production Bible Version: %v", err)
	}
	if version.Version != 1 || version.CandidateRevisionID != input.CandidateRevisionID ||
		version.CandidateRevisionHash != input.CandidateRevisionHash || version.ContentHash == "" ||
		string(version.Snapshot) != string(snapshot) {
		t.Fatalf("Production Bible Version = %#v", version)
	}

	drifted := input
	drifted.Snapshot = json.RawMessage(`{"canonical_entities":[],"canonical_world_entries":[],"merged_claims":[],"merged_arcs":[],"conflicts":[],"review_issues":[{"issue_key":"drift"}]}`)
	if _, err = domain.NewProductionBibleVersion(drifted); err == nil {
		t.Fatal("Production Bible Version accepted a snapshot that does not match the frozen Candidate content hash")
	}
}
