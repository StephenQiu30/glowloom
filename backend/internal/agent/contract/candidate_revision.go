package contract

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type InvocationCandidateOrigin struct {
	SourceInvocationID string `json:"source_invocation_id"`
	SourceResultHash   string `json:"source_result_hash"`
}

type AggregateLeafCandidateRef struct {
	StageInstanceKey      string `json:"stage_instance_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
}

type AggregateCandidateOrigin struct {
	ShardManifestID   string                      `json:"shard_manifest_id"`
	ManifestVersion   int64                       `json:"manifest_version"`
	ShardManifestHash string                      `json:"shard_manifest_hash"`
	LeafCandidates    []AggregateLeafCandidateRef `json:"leaf_candidates"`
}

type RepairCandidateOrigin struct {
	RepairInvocationID string `json:"repair_invocation_id"`
	RepairResultHash   string `json:"repair_result_hash"`
}

type CandidateRevisionMaterial struct {
	StageInstanceKey            string                     `json:"stage_instance_key"`
	RevisionNo                  int64                      `json:"revision_no"`
	ParentCandidateRevisionHash *string                    `json:"parent_candidate_revision_hash"`
	OriginKind                  string                     `json:"origin_kind"`
	InvocationOrigin            *InvocationCandidateOrigin `json:"invocation_origin"`
	AggregateOrigin             *AggregateCandidateOrigin  `json:"aggregate_origin,omitempty"`
	RepairOrigin                *RepairCandidateOrigin     `json:"repair_origin,omitempty"`
	CandidateContentHash        string                     `json:"candidate_content_hash"`
}

func (value CandidateRevisionMaterial) Hash() (string, error) {
	if !hashPattern.MatchString(value.StageInstanceKey) || value.RevisionNo < 1 ||
		!hashPattern.MatchString(value.CandidateContentHash) {
		return "", errors.New("invalid Stage candidate revision material")
	}
	switch value.OriginKind {
	case "invocation":
		if value.RevisionNo != 1 || value.ParentCandidateRevisionHash != nil ||
			value.InvocationOrigin == nil || value.AggregateOrigin != nil || value.RepairOrigin != nil ||
			value.InvocationOrigin.SourceInvocationID == "" ||
			!hashPattern.MatchString(value.InvocationOrigin.SourceResultHash) {
			return "", errors.New("invalid Stage invocation candidate origin")
		}
		if _, err := uuid.Parse(value.InvocationOrigin.SourceInvocationID); err != nil {
			return "", errors.New("invalid Stage invocation candidate origin")
		}
	case "aggregate":
		if value.RevisionNo != 1 || value.ParentCandidateRevisionHash != nil ||
			value.InvocationOrigin != nil || value.AggregateOrigin == nil || value.RepairOrigin != nil ||
			value.AggregateOrigin.ManifestVersion < 1 ||
			!hashPattern.MatchString(value.AggregateOrigin.ShardManifestHash) ||
			len(value.AggregateOrigin.LeafCandidates) == 0 {
			return "", errors.New("invalid Stage aggregate candidate origin")
		}
		if _, err := uuid.Parse(value.AggregateOrigin.ShardManifestID); err != nil {
			return "", errors.New("invalid Stage aggregate manifest identity")
		}
		for _, leaf := range value.AggregateOrigin.LeafCandidates {
			if !hashPattern.MatchString(leaf.StageInstanceKey) || strings.TrimSpace(leaf.ShardKey) == "" ||
				!hashPattern.MatchString(leaf.CandidateRevisionHash) {
				return "", errors.New("invalid Stage aggregate leaf candidate")
			}
			if _, err := uuid.Parse(leaf.CandidateRevisionID); err != nil {
				return "", errors.New("invalid Stage aggregate leaf candidate")
			}
		}
	case "repair":
		if value.RevisionNo < 2 || value.ParentCandidateRevisionHash == nil ||
			!hashPattern.MatchString(*value.ParentCandidateRevisionHash) ||
			value.InvocationOrigin != nil || value.AggregateOrigin != nil || value.RepairOrigin == nil ||
			!hashPattern.MatchString(value.RepairOrigin.RepairResultHash) {
			return "", errors.New("invalid Stage repair candidate origin")
		}
		if _, err := uuid.Parse(value.RepairOrigin.RepairInvocationID); err != nil {
			return "", errors.New("invalid Stage repair candidate origin")
		}
	default:
		return "", errors.New("invalid Stage candidate revision origin")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return CanonicalHash(encoded)
}
