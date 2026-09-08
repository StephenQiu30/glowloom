package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

type Issue struct {
	Code     string `json:"code"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}
type RiskResolution struct {
	Code   string `json:"code"`
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}
type ResolvedEvidence struct {
	RevisionID string `json:"revision_id"`
	SourceHash string `json:"source_hash"`
	Block      int    `json:"block"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	TextHash   string `json:"text_hash"`
	Quote      string `json:"quote"`
}
type TextResult struct {
	InvocationID  string             `json:"invocation_id"`
	Stage         string             `json:"stage"`
	InputHash     string             `json:"input_hash"`
	ReleaseHash   string             `json:"release_hash"`
	CandidateHash string             `json:"candidate_hash"`
	Candidate     json.RawMessage    `json:"candidate"`
	Evidence      []ResolvedEvidence `json:"evidence"`
	Issues        []Issue            `json:"issues"`
}
type TextTask struct {
	InvocationID string `json:"invocation_id"`
	Stage        string `json:"stage"`
	ReleaseHash  string `json:"release_hash"`
	Source       struct {
		RevisionID  string `json:"revision_id"`
		ContentHash string `json:"content_hash"`
		Text        string `json:"text"`
	} `json:"source"`
	EpisodeMap json.RawMessage   `json:"episode_map"`
	Analyses   []json.RawMessage `json:"analyses"`
	World      json.RawMessage   `json:"world"`
	EpisodeKey string            `json:"episode_key"`
	SceneKey   string            `json:"scene_key"`
}
type DraftRef struct {
	StepKey       string `json:"step_key,omitempty"`
	OutputRole    string `json:"output_role,omitempty"`
	ItemKey       string `json:"item_key,omitempty"`
	StepID        string `json:"step_id"`
	DraftID       string `json:"draft_id"`
	ResultHash    string `json:"result_hash"`
	CandidateHash string `json:"candidate_hash"`
}
type DraftEnvelope struct {
	Schema           string          `json:"schema"`
	CommandID        string          `json:"command_id"`
	RunID            string          `json:"run_id"`
	PayloadHash      string          `json:"payload_hash"`
	SourceRevisionID string          `json:"source_revision_id"`
	StepID           string          `json:"step_id"`
	StepKey          string          `json:"step_key"`
	DraftID          string          `json:"draft_id"`
	Revision         int             `json:"revision"`
	ResultHash       string          `json:"result_hash"`
	CandidateHash    string          `json:"candidate_hash"`
	Task             json.RawMessage `json:"task"`
	Result           json.RawMessage `json:"result"`
}
type ExecutionSnapshot struct {
	CanResume        bool              `json:"can_resume"`
	Schema           string            `json:"schema"`
	CommandID        string            `json:"command_id"`
	RunID            string            `json:"run_id"`
	PayloadHash      string            `json:"payload_hash"`
	SourceRevisionID string            `json:"source_revision_id"`
	ReleaseHash      string            `json:"release_hash"`
	CallLimit        int               `json:"call_limit"`
	ReservedCalls    int               `json:"reserved_calls"`
	Status           string            `json:"status"`
	Stage            string            `json:"stage"`
	LastError        string            `json:"last_error"`
	Steps            []json.RawMessage `json:"steps"`
	Outputs          []DraftRef        `json:"outputs"`
}

type ResumeReceipt struct {
	Schema    string `json:"schema"`
	CommandID string `json:"command_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
}
type ResourceRef struct {
	Owner       string `json:"owner"`
	Type        string `json:"type"`
	ID          string `json:"id"`
	Revision    int    `json:"revision"`
	ContentHash string `json:"content_hash"`
}
type OwnerReceipt struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Operation string `json:"operation"`
}
type AdoptionReceipt struct {
	Schema           string            `json:"schema"`
	SubmissionID     string            `json:"submission_id"`
	RunID            string            `json:"run_id"`
	ProposalID       string            `json:"proposal_id"`
	StepID           string            `json:"step_id"`
	Gate             string            `json:"gate"`
	ProposalRevision int               `json:"proposal_revision"`
	ProposalHash     string            `json:"proposal_hash"`
	CandidateHash    string            `json:"candidate_hash"`
	DecisionID       string            `json:"decision_id"`
	OwnerReceipts    []OwnerReceipt    `json:"owner_receipts"`
	FormalRefs       []ResourceRef     `json:"formal_refs"`
	IDMapping        map[string]string `json:"id_mapping"`
	RiskResolutions  []RiskResolution  `json:"risk_resolutions"`
	AcceptedAt       time.Time         `json:"accepted_at"`
}
type Proposal struct {
	ID               string             `json:"id"`
	RunID            string             `json:"run_id"`
	StepID           string             `json:"step_id"`
	Stage            string             `json:"stage"`
	StepKey          string             `json:"step_key"`
	Revision         int                `json:"revision"`
	ResultHash       string             `json:"result_hash"`
	CandidateHash    string             `json:"candidate_hash"`
	SourceRevisionID string             `json:"source_revision_id"`
	SourceHash       string             `json:"source_hash"`
	InvocationID     string             `json:"invocation_id"`
	InputHash        string             `json:"input_hash"`
	ReleaseHash      string             `json:"release_hash"`
	Candidate        json.RawMessage    `json:"candidate"`
	Evidence         []ResolvedEvidence `json:"evidence"`
	Issues           []Issue            `json:"issues"`
	HumanTaskID      string             `json:"human_task_id"`
	Status           string             `json:"status"`
	Acceptance       *AdoptionReceipt   `json:"acceptance"`
	CreatedAt        time.Time          `json:"created_at"`
	Draft            DraftEnvelope      `json:"-"`
}
type SelectedScene struct {
	EpisodeKey string `json:"episode_key"`
	SceneKey   string `json:"scene_key"`
}
type GateResult struct {
	Status         string            `json:"status"`
	Gate           string            `json:"gate"`
	HumanTaskIDs   []string          `json:"human_task_ids"`
	Receipts       []AdoptionReceipt `json:"receipts"`
	SelectedScenes []SelectedScene   `json:"selected_scenes"`
}

func ValidStage(stage string) bool {
	return stage == "map_manuscript" || stage == "analyze_episode" || stage == "build_world" || stage == "direct_scene"
}
func TextHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
func ValidateDraft(run Run, draft DraftEnvelope) (Proposal, error) {
	fail := errors.New("creation draft binding or evidence is invalid")
	for _, id := range []string{draft.RunID, draft.StepID, draft.DraftID} {
		if value, err := uuid.Parse(id); err != nil || value == uuid.Nil || value.String() != id {
			return Proposal{}, fail
		}
	}
	if draft.Schema != "creation-draft-production" || draft.RunID != run.Command.RunID || draft.CommandID != run.Command.RunID || draft.PayloadHash != run.PayloadHash || draft.SourceRevisionID != run.Command.Source.RevisionID || draft.Revision != 1 {
		return Proposal{}, fail
	}
	var task TextTask
	var result TextResult
	if json.Unmarshal(draft.Task, &task) != nil || json.Unmarshal(draft.Result, &result) != nil {
		return Proposal{}, fail
	}
	resultHash, err := canonical.Hash(draft.Result)
	candidateHash, cerr := canonical.Hash(result.Candidate)
	taskHash, taskErr := canonical.Hash(draft.Task)
	if taskErr != nil || result.InputHash != taskHash || result.InvocationID != task.InvocationID || result.ReleaseHash != task.ReleaseHash || len(task.ReleaseHash) != 64 {
		return Proposal{}, fail
	}
	if err != nil || cerr != nil || resultHash != draft.ResultHash || candidateHash != draft.CandidateHash || result.CandidateHash != candidateHash || !ValidStage(result.Stage) || task.Stage != result.Stage {
		return Proposal{}, fail
	}
	if task.Source.RevisionID != draft.SourceRevisionID || task.Source.ContentHash != run.Command.Source.ContentHash || !utf8.ValidString(task.Source.Text) || TextHash(task.Source.Text) != task.Source.ContentHash || utf8.RuneCountInString(task.Source.Text) > 2000000 {
		return Proposal{}, fail
	}
	key := strings.Join(nonempty(task.Stage, task.EpisodeKey, task.SceneKey), "/")
	if draft.StepKey != key {
		return Proposal{}, fail
	}
	if task.Stage == "analyze_episode" || task.Stage == "direct_scene" {
		var scope struct {
			EpisodeKey string `json:"episode_key"`
			SceneKey   string `json:"scene_key"`
		}
		if json.Unmarshal(result.Candidate, &scope) != nil || scope.EpisodeKey != task.EpisodeKey || task.EpisodeKey == "" || (task.Stage == "direct_scene" && (scope.SceneKey != task.SceneKey || task.SceneKey == "")) {
			return Proposal{}, fail
		}
	}
	runes := []rune(task.Source.Text)
	blocks := SourceBlocks(task.Source.Text)
	for _, e := range result.Evidence {
		if e.RevisionID != task.Source.RevisionID || e.SourceHash != task.Source.ContentHash || e.Block < 0 || e.Block >= len(blocks) || e.Start < blocks[e.Block][0] || e.End > blocks[e.Block][1] || e.Start >= e.End || e.End > len(runes) || string(runes[e.Start:e.End]) != e.Quote || TextHash(e.Quote) != e.TextHash {
			return Proposal{}, fail
		}
	}
	for _, issue := range result.Issues {
		if strings.TrimSpace(issue.Code) == "" || strings.TrimSpace(issue.Scope) == "" || (issue.Severity != "warning" && issue.Severity != "blocker") {
			return Proposal{}, fail
		}
	}
	return Proposal{ID: draft.DraftID, RunID: draft.RunID, StepID: draft.StepID, Stage: result.Stage, StepKey: draft.StepKey, Revision: 1, ResultHash: resultHash, CandidateHash: candidateHash, SourceRevisionID: draft.SourceRevisionID, SourceHash: task.Source.ContentHash, InvocationID: result.InvocationID, InputHash: result.InputHash, ReleaseHash: result.ReleaseHash, Candidate: result.Candidate, Evidence: result.Evidence, Issues: result.Issues, Status: "needs_review", Draft: draft}, nil
}
func nonempty(parts ...string) []string {
	result := []string{}
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// SourceBlocks matches Python str.splitlines(keepends=True) in Unicode code points.
func SourceBlocks(text string) [][2]int {
	runes := []rune(text)
	result := [][2]int{}
	start := 0
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
			if runes[i] == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			result = append(result, [2]int{start, i + 1})
			start = i + 1
		}
	}
	if start < len(runes) {
		result = append(result, [2]int{start, len(runes)})
	}
	return result
}
func ValidateResolutions(issues []Issue, resolutions []RiskResolution) error {
	required := map[string]bool{}
	for _, i := range issues {
		if i.Severity == "blocker" {
			required[i.Code+"\x00"+i.Scope] = false
		}
	}
	for _, r := range resolutions {
		key := r.Code + "\x00" + r.Scope
		done, ok := required[key]
		if !ok || done || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4000 {
			return errors.New("risk resolution must bind an unresolved blocker and explanation")
		}
		required[key] = true
	}
	for _, done := range required {
		if !done {
			return errors.New("proposal has unresolved blockers")
		}
	}
	return nil
}
