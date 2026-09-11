package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const StoryGraphSourceReviewContractID = "storygraph-external-source-review-production"

//go:embed manifests/storygraph-source-review.json
var pinnedStoryGraphSourceReview []byte

type ExternalSkillSourceRisks struct {
	LicenseUnclear           bool `json:"license_unclear"`
	RedistributionProhibited bool `json:"redistribution_prohibited"`
	EmbeddedCredentials      bool `json:"embedded_credentials"`
	ImplicitNetworkAccess    bool `json:"implicit_network_access"`
	InstructionInjection     bool `json:"instruction_injection"`
	ExcessToolAuthority      bool `json:"excess_tool_authority"`
	UntraceableSource        bool `json:"untraceable_source"`
}

type ExternalSkillSourceReview struct {
	SourceID                string                   `json:"source_id"`
	SourceCommit            string                   `json:"source_commit"`
	SourceFileSHA256        string                   `json:"source_file_sha256"`
	LicenseSHA256           string                   `json:"license_sha256"`
	Risks                   ExternalSkillSourceRisks `json:"risks"`
	Findings                []string                 `json:"findings"`
	Decision                string                   `json:"decision"`
	RewriteQueueAllowed     bool                     `json:"rewrite_queue_allowed"`
	ProductionBundleAllowed bool                     `json:"production_bundle_allowed"`
	RuntimeExecutionAllowed bool                     `json:"runtime_execution_allowed"`
}

type StoryGraphSourceReview struct {
	ContractID          string                      `json:"contract_id"`
	SourceInventoryHash string                      `json:"source_inventory_hash"`
	ReviewBasis         string                      `json:"review_basis"`
	ReviewedAt          string                      `json:"reviewed_at"`
	ReviewerID          string                      `json:"reviewer_id"`
	Reviews             []ExternalSkillSourceReview `json:"reviews"`
	SourceReviewRoot    string                      `json:"source_review_root"`
}

func PinnedStoryGraphSourceReview() (StoryGraphSourceReview, json.RawMessage, error) {
	return DecodeStoryGraphSourceReview(pinnedStoryGraphSourceReview)
}

func DecodeStoryGraphSourceReview(raw json.RawMessage) (StoryGraphSourceReview, json.RawMessage, error) {
	var review StoryGraphSourceReview
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&review); err != nil {
		return StoryGraphSourceReview{}, nil, errors.New("invalid StoryGraph Source Review")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return StoryGraphSourceReview{}, nil, errors.New("invalid StoryGraph Source Review")
	}
	if err := validateStoryGraphSourceReview(review); err != nil {
		return StoryGraphSourceReview{}, nil, err
	}
	encoded, err := encodeStoryGraphSourceReview(review)
	if err != nil {
		return StoryGraphSourceReview{}, nil, err
	}
	return review, encoded, nil
}

func validateStoryGraphSourceReview(review StoryGraphSourceReview) error {
	inventory, _, err := PinnedStoryGraphSourceInventory()
	if err != nil {
		return errors.New("invalid StoryGraph Source Review inventory")
	}
	if review.ContractID != StoryGraphSourceReviewContractID ||
		review.SourceInventoryHash != inventory.SourceInventoryHash ||
		review.ReviewBasis != "fixed_source_and_license_bytes" || review.ReviewerID != "project-owner" ||
		len(review.Reviews) != len(inventory.Sources) || !hashPattern.MatchString(review.SourceReviewRoot) {
		return errors.New("invalid StoryGraph Source Review")
	}
	if _, err := time.Parse(time.RFC3339, review.ReviewedAt); err != nil {
		return errors.New("invalid StoryGraph Source Review time")
	}
	for index, sourceReview := range review.Reviews {
		if index > 0 && review.Reviews[index-1].SourceID >= sourceReview.SourceID {
			return errors.New("StoryGraph Source Review is not uniquely sorted")
		}
		if err := validateExternalSkillSourceReview(sourceReview, inventory.Sources[index]); err != nil {
			return err
		}
	}
	hash, err := storyGraphSourceReviewHash(review)
	if err != nil || hash != review.SourceReviewRoot {
		return errors.New("StoryGraph Source Review root has drifted")
	}
	return nil
}

func validateExternalSkillSourceReview(review ExternalSkillSourceReview, source ExternalSkillSource) error {
	if review.SourceID != source.SourceID || review.SourceCommit != source.SourceCommit ||
		review.SourceFileSHA256 != source.SourceFileSHA256 || review.LicenseSHA256 != source.License.SHA256 ||
		review.RewriteQueueAllowed || review.ProductionBundleAllowed || review.RuntimeExecutionAllowed {
		return errors.New("external Skill review escaped its fixed source boundary")
	}
	if err := validateExternalSkillSourceFindings(review.Findings); err != nil {
		return err
	}
	if review.Risks.LicenseUnclear != slices.Contains(review.Findings, "license_unresolved") ||
		review.Risks.RedistributionProhibited != slices.Contains(review.Findings, "redistribution_prohibited") ||
		review.Risks.EmbeddedCredentials != slices.Contains(review.Findings, "embedded_credentials") ||
		review.Risks.ImplicitNetworkAccess != slices.Contains(review.Findings, "external_network_service") ||
		review.Risks.InstructionInjection != slices.Contains(review.Findings, "active_agent_instructions") ||
		review.Risks.UntraceableSource != slices.Contains(review.Findings, "untraceable_source") {
		return errors.New("external Skill review risks do not match findings")
	}
	hasExcessToolFinding := slices.Contains(review.Findings, "executable_script_calls") ||
		slices.Contains(review.Findings, "filesystem_write_instructions") ||
		slices.Contains(review.Findings, "subagent_execution_instructions") ||
		slices.Contains(review.Findings, "browser_review_instructions") ||
		slices.Contains(review.Findings, "skill_installation_instructions")
	if review.Risks.ExcessToolAuthority != hasExcessToolFinding {
		return errors.New("external Skill review tool risk does not match findings")
	}
	unsafe := review.Risks.LicenseUnclear || review.Risks.RedistributionProhibited ||
		review.Risks.EmbeddedCredentials || review.Risks.ImplicitNetworkAccess ||
		review.Risks.InstructionInjection || review.Risks.ExcessToolAuthority || review.Risks.UntraceableSource
	if unsafe && review.Decision != "quarantined" || !unsafe && review.Decision != "reference_only" {
		return errors.New("external Skill review decision does not match risks")
	}
	return nil
}

func validateExternalSkillSourceFindings(findings []string) error {
	allowed := map[string]struct{}{
		"active_agent_instructions": {}, "browser_review_instructions": {},
		"embedded_credentials": {}, "executable_resource_examples": {},
		"executable_script_calls": {}, "external_network_service": {},
		"external_session_control": {}, "filesystem_write_instructions": {},
		"license_unresolved": {}, "local_file_upload": {},
		"network_requirement_examples": {}, "normative_specification": {},
		"redistribution_prohibited": {}, "remote_artifact_download": {},
		"skill_installation_instructions": {}, "subagent_execution_instructions": {},
		"tool_declaration_examples": {}, "untraceable_source": {},
	}
	if len(findings) == 0 || !slices.IsSorted(findings) {
		return errors.New("invalid external Skill review findings")
	}
	for index, finding := range findings {
		if _, exists := allowed[finding]; !exists || index > 0 && findings[index-1] == finding {
			return errors.New("invalid external Skill review findings")
		}
	}
	return nil
}

func storyGraphSourceReviewHash(review StoryGraphSourceReview) (string, error) {
	review.SourceReviewRoot = ""
	raw, err := json.Marshal(review)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "source_review_root")
	raw, err = json.Marshal(root)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeStoryGraphSourceReview(review StoryGraphSourceReview) (json.RawMessage, error) {
	raw, err := json.Marshal(review)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
