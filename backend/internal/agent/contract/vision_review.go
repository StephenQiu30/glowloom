package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"

	generation "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const VisionReviewCandidateContractID = "vision-review-candidate-production"

var visionReviewCategories = []string{"identity", "interaction_geometry", "state", "style_fidelity", "view_role"}
var visionReviewIssueCode = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

// VisionReviewSubject is a frozen identity summary, not a grant or a substitute
// for the full input. Only the Backend Owner can verify current facts and QC.
type VisionReviewSubject struct {
	WorkspaceID          string                           `json:"workspace_id"`
	ProjectID            string                           `json:"project_id"`
	TargetKind           string                           `json:"target_kind"`
	GenerationRound      int64                            `json:"generation_round"`
	TargetRef            generation.GenerationRevisionRef `json:"generation_target_ref"`
	ExecutionRef         generation.GenerationRevisionRef `json:"execution_ref"`
	BundleInputRef       generation.GenerationActionRef   `json:"bundle_input_ref"`
	CandidateBundleIndex int                              `json:"candidate_bundle_index"`
	BriefRevisionID      string                           `json:"brief_revision_id"`
	BriefRevisionHash    string                           `json:"brief_revision_hash"`
	StageReleaseHash     string                           `json:"stage_release_hash"`
	InputHash            string                           `json:"input_hash"`
	Slots                []VisionReviewSlot               `json:"slots"`
}

type VisionReviewSlot struct {
	SlotKey  string                           `json:"slot_key"`
	ViewRole string                           `json:"view_role"`
	MediaRef generation.GenerationRevisionRef `json:"staged_media_ref"`
	SHA256   string                           `json:"sha256"`
}

type VisionReviewRegion struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type VisionReviewEvidence struct {
	SlotKey string             `json:"slot_key"`
	Region  VisionReviewRegion `json:"region"`
}

type VisionReviewCheck struct {
	Category       string                 `json:"category"`
	Status         string                 `json:"status"`
	IssueCode      string                 `json:"issue_code"`
	ConfidenceBPS  int                    `json:"confidence_bps"`
	Summary        string                 `json:"summary"`
	Recommendation string                 `json:"recommendation"`
	Evidence       []VisionReviewEvidence `json:"evidence"`
}

type VisionReviewCandidate struct {
	ContractID string              `json:"contract_id"`
	Subject    VisionReviewSubject `json:"subject"`
	Checks     []VisionReviewCheck `json:"checks"`
}

func (subject VisionReviewSubject) Validate() error {
	if !referencePlanUUID(subject.WorkspaceID) || !referencePlanUUID(subject.ProjectID) || !referencePlanUUID(subject.BriefRevisionID) ||
		!subject.TargetRef.Valid() || !subject.ExecutionRef.Valid() || !subject.BundleInputRef.Valid() ||
		subject.GenerationRound < 1 || subject.CandidateBundleIndex < 0 || subject.CandidateBundleIndex > 3 ||
		!hashPattern.MatchString(subject.BriefRevisionHash) || !hashPattern.MatchString(subject.StageReleaseHash) || !hashPattern.MatchString(subject.InputHash) {
		return errors.New("invalid Vision Review subject identity")
	}
	roles := ReferenceBriefRequiredViewRoles(subject.TargetKind)
	if len(roles) == 0 || len(subject.Slots) != len(roles) {
		return errors.New("Vision Review requires the complete target view set")
	}
	mediaIDs, digests := map[string]bool{}, map[string]bool{}
	for index, slot := range subject.Slots {
		if slot.SlotKey != roles[index] || slot.ViewRole != roles[index] || !slot.MediaRef.Valid() || slot.MediaRef.Revision != 2 ||
			!hashPattern.MatchString(slot.SHA256) || mediaIDs[slot.MediaRef.ID] || digests[slot.SHA256] {
			return errors.New("Vision Review slot identity is invalid or duplicated")
		}
		mediaIDs[slot.MediaRef.ID], digests[slot.SHA256] = true, true
	}
	return nil
}

func (candidate VisionReviewCandidate) ValidateFor(subject VisionReviewSubject) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	if subject.Validate() != nil || !reflect.DeepEqual(candidate.Subject, subject) {
		return errors.New("Vision Review changed the frozen subject")
	}
	return nil
}

func (candidate VisionReviewCandidate) Validate() error {
	if candidate.ContractID != VisionReviewCandidateContractID || candidate.Subject.Validate() != nil || len(candidate.Checks) != len(visionReviewCategories) {
		return errors.New("invalid Vision Review candidate")
	}
	slots := make(map[string]bool, len(candidate.Subject.Slots))
	for _, slot := range candidate.Subject.Slots {
		slots[slot.SlotKey] = true
	}
	for index, check := range candidate.Checks {
		if check.Category != visionReviewCategories[index] || !visionReviewText(check.Summary, 2000) || len(check.IssueCode) > 100 ||
			!visionReviewIssueCode.MatchString(check.IssueCode) || !visionReviewUnitNumber(check.ConfidenceBPS) || check.Evidence == nil || len(check.Evidence) > 16 {
			return errors.New("invalid Vision Review check")
		}
		seenSlots := map[string]bool{}
		for i, evidence := range check.Evidence {
			region := evidence.Region
			if !slots[evidence.SlotKey] || !visionReviewUnitNumber(region.X) || !visionReviewUnitNumber(region.Y) ||
				!visionReviewUnitNumber(region.Width) || !visionReviewUnitNumber(region.Height) || region.Width == 0 || region.Height == 0 ||
				region.X+region.Width > 10000 || region.Y+region.Height > 10000 || i > 0 && !visionReviewEvidenceLess(check.Evidence[i-1], evidence) {
				return errors.New("invalid Vision Review evidence region")
			}
			seenSlots[evidence.SlotKey] = true
		}
		switch check.Status {
		case "pass":
			if check.IssueCode != "none" || check.Recommendation != "" || len(seenSlots) != len(slots) {
				return errors.New("Vision pass requires complete view evidence and no issue")
			}
		case "warn", "fail":
			if check.IssueCode == "none" || !visionReviewText(check.Recommendation, 2000) || len(check.Evidence) == 0 {
				return errors.New("Vision issue requires advice and evidence")
			}
		case "not_assessable":
			if check.IssueCode == "none" || check.ConfidenceBPS != 0 || !visionReviewText(check.Recommendation, 2000) {
				return errors.New("Vision uncertainty requires an explicit reason and advice")
			}
		default:
			return errors.New("invalid Vision Review status")
		}
	}
	return nil
}

func DecodeVisionReviewCandidate(raw json.RawMessage) (VisionReviewCandidate, json.RawMessage, error) {
	var value VisionReviewCandidate
	if canonical.Decode(raw, &value) != nil || value.Validate() != nil {
		return VisionReviewCandidate{}, nil, errors.New("invalid Vision Review candidate")
	}
	encoded, err := canonical.JSON(raw)
	if err != nil {
		return VisionReviewCandidate{}, nil, err
	}
	// Re-encoding also rejects missing required zero-valued fields and nulls;
	// the canonical wire shape must be exactly the closed typed shape.
	typed, err := json.Marshal(value)
	if err != nil {
		return VisionReviewCandidate{}, nil, err
	}
	typedCanonical, err := canonical.JSON(typed)
	if err != nil || !bytes.Equal(encoded, typedCanonical) {
		return VisionReviewCandidate{}, nil, errors.New("Vision Review wire shape is incomplete")
	}
	return value, encoded, err
}

func visionReviewText(value string, maxBytes int) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maxBytes
}

func visionReviewUnitNumber(value int) bool {
	return value >= 0 && value <= 10000
}

func visionReviewEvidenceLess(a, b VisionReviewEvidence) bool {
	if a.SlotKey != b.SlotKey {
		return a.SlotKey < b.SlotKey
	}
	left, right := [4]int{a.Region.X, a.Region.Y, a.Region.Width, a.Region.Height}, [4]int{b.Region.X, b.Region.Y, b.Region.Width, b.Region.Height}
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
