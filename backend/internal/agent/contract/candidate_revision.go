package contract

import (
	"encoding/json"
	"errors"
)

type InvocationCandidateOrigin struct {
	SourceInvocationID string `json:"source_invocation_id"`
	SourceResultHash   string `json:"source_result_hash"`
}

type CandidateRevisionMaterial struct {
	StageInstanceKey            string                     `json:"stage_instance_key"`
	RevisionNo                  int64                      `json:"revision_no"`
	ParentCandidateRevisionHash *string                    `json:"parent_candidate_revision_hash"`
	OriginKind                  string                     `json:"origin_kind"`
	InvocationOrigin            *InvocationCandidateOrigin `json:"invocation_origin"`
	CandidateContentHash        string                     `json:"candidate_content_hash"`
}

func (value CandidateRevisionMaterial) Hash() (string, error) {
	if !hashPattern.MatchString(value.StageInstanceKey) || value.RevisionNo < 1 || value.OriginKind != "invocation" || value.InvocationOrigin == nil || value.InvocationOrigin.SourceInvocationID == "" || !hashPattern.MatchString(value.InvocationOrigin.SourceResultHash) || !hashPattern.MatchString(value.CandidateContentHash) {
		return "", errors.New("invalid Stage candidate revision material")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return CanonicalHash(encoded)
}
