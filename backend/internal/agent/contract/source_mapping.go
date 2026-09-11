package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const StoryGraphSourceMappingContractID = "storygraph-external-source-mapping-production"

//go:embed manifests/storygraph-source-mapping.json
var pinnedStoryGraphSourceMapping []byte

type ExternalSkillSourceMapping struct {
	SourceID                 string   `json:"source_id"`
	SourceCommit             string   `json:"source_commit"`
	ReviewDecision           string   `json:"review_decision"`
	Classification           string   `json:"classification"`
	ArtifactKind             string   `json:"artifact_kind"`
	DispositionReason        string   `json:"disposition_reason"`
	CopiedProductionPaths    []string `json:"copied_production_paths"`
	RewrittenProductionPaths []string `json:"rewritten_production_paths"`
}

type StoryGraphSourceMapping struct {
	ContractID          string                       `json:"contract_id"`
	SourceInventoryHash string                       `json:"source_inventory_hash"`
	SourceReviewRoot    string                       `json:"source_review_root"`
	MappedAt            string                       `json:"mapped_at"`
	MapperID            string                       `json:"mapper_id"`
	Mappings            []ExternalSkillSourceMapping `json:"mappings"`
	SourceMappingRoot   string                       `json:"source_mapping_root"`
}

func PinnedStoryGraphSourceMapping() (StoryGraphSourceMapping, json.RawMessage, error) {
	return DecodeStoryGraphSourceMapping(pinnedStoryGraphSourceMapping)
}

func DecodeStoryGraphSourceMapping(raw json.RawMessage) (StoryGraphSourceMapping, json.RawMessage, error) {
	var mapping StoryGraphSourceMapping
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mapping); err != nil {
		return StoryGraphSourceMapping{}, nil, errors.New("invalid StoryGraph Source Mapping")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return StoryGraphSourceMapping{}, nil, errors.New("invalid StoryGraph Source Mapping")
	}
	if err := validateStoryGraphSourceMapping(mapping); err != nil {
		return StoryGraphSourceMapping{}, nil, err
	}
	encoded, err := encodeStoryGraphSourceMapping(mapping)
	if err != nil {
		return StoryGraphSourceMapping{}, nil, err
	}
	return mapping, encoded, nil
}

func validateStoryGraphSourceMapping(mapping StoryGraphSourceMapping) error {
	review, _, err := PinnedStoryGraphSourceReview()
	if err != nil {
		return errors.New("invalid StoryGraph Source Mapping review")
	}
	if mapping.ContractID != StoryGraphSourceMappingContractID ||
		mapping.SourceInventoryHash != review.SourceInventoryHash || mapping.SourceReviewRoot != review.SourceReviewRoot ||
		mapping.MapperID != "project-owner" || len(mapping.Mappings) != len(review.Reviews) ||
		!hashPattern.MatchString(mapping.SourceMappingRoot) {
		return errors.New("invalid StoryGraph Source Mapping")
	}
	if _, err := time.Parse(time.RFC3339, mapping.MappedAt); err != nil {
		return errors.New("invalid StoryGraph Source Mapping time")
	}
	for index, sourceMapping := range mapping.Mappings {
		if index > 0 && mapping.Mappings[index-1].SourceID >= sourceMapping.SourceID {
			return errors.New("StoryGraph Source Mapping is not uniquely sorted")
		}
		if err := validateExternalSkillSourceMapping(sourceMapping, review.Reviews[index]); err != nil {
			return err
		}
	}
	hash, err := storyGraphSourceMappingHash(mapping)
	if err != nil || hash != mapping.SourceMappingRoot {
		return errors.New("StoryGraph Source Mapping root has drifted")
	}
	return nil
}

func validateExternalSkillSourceMapping(mapping ExternalSkillSourceMapping, review ExternalSkillSourceReview) error {
	if mapping.SourceID != review.SourceID || mapping.SourceCommit != review.SourceCommit ||
		mapping.ReviewDecision != review.Decision || strings.TrimSpace(mapping.DispositionReason) == "" ||
		mapping.CopiedProductionPaths == nil || mapping.RewrittenProductionPaths == nil ||
		len(mapping.CopiedProductionPaths) != 0 || len(mapping.RewrittenProductionPaths) != 0 {
		return errors.New("external Skill Source Mapping escaped its reviewed byte boundary")
	}
	switch review.Decision {
	case "reference_only":
		if mapping.Classification != "reference_only" || mapping.ArtifactKind != "reference_knowledge" ||
			mapping.DispositionReason != "normative_specification_only" ||
			!slices.Contains(review.Findings, "normative_specification") {
			return errors.New("invalid reference-only external Skill mapping")
		}
	case "quarantined":
		if mapping.Classification != "reject" || mapping.ArtifactKind != "none" {
			return errors.New("quarantined external Skill entered the absorption pipeline")
		}
		activeInstructions := slices.Contains(review.Findings, "active_agent_instructions")
		remoteService := slices.Contains(review.Findings, "external_network_service")
		if activeInstructions == remoteService ||
			activeInstructions && mapping.DispositionReason != "quarantined_active_agent_and_tool_instructions" ||
			remoteService && mapping.DispositionReason != "quarantined_remote_service_and_file_transfer" {
			return errors.New("invalid quarantined external Skill disposition")
		}
	default:
		return errors.New("invalid external Skill review decision")
	}
	return nil
}

func storyGraphSourceMappingHash(mapping StoryGraphSourceMapping) (string, error) {
	mapping.SourceMappingRoot = ""
	raw, err := json.Marshal(mapping)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "source_mapping_root")
	raw, err = json.Marshal(root)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeStoryGraphSourceMapping(mapping StoryGraphSourceMapping) (json.RawMessage, error) {
	raw, err := json.Marshal(mapping)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
