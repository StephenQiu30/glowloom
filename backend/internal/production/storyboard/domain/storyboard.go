package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

type Unit struct {
	ID, SceneID, Kind, Text string
	DialogueID              *string
	Position                int
	Required                bool
}

type DraftInput struct {
	WorkspaceID, ProjectID, EpisodeID, StructureID, ScriptVersionID string
	BibleID, BibleResultHash                                        string
	BibleRevision                                                   int
	TargetDurationMS                                                int
	AspectRatio                                                     string
	VisualStyle                                                     *string
	Units                                                           []Unit
	WorldEntries                                                    []map[string]any
}

type DraftShot struct {
	ProposalKey             string           `json:"proposal_key"`
	Position                int              `json:"position"`
	Title                   string           `json:"title"`
	NarrativeUnitVersionIDs []string         `json:"narrative_unit_version_ids"`
	Spec                    map[string]any   `json:"spec"`
	AssetReferences         []map[string]any `json:"asset_references"`
	RiskCodes               []string         `json:"risk_codes"`
}

type Candidate struct {
	Shots []DraftShot `json:"shots"`
}

type Batch struct {
	ID, WorkspaceID, ProjectID, EpisodeID, StructureID, ScriptVersionID, TaskID string
	Status, InputHash                                                           string
	ResultHash                                                                  *string
	Candidate                                                                   Candidate
	Decisions                                                                   map[string]string
	Error                                                                       json.RawMessage
	Revision                                                                    int
	ApprovedBy                                                                  *string
	ApprovedAt, AppliedAt                                                       *time.Time
	CreatedBy                                                                   string
	CreatedAt, UpdatedAt                                                        time.Time
}

type DraftSetBatch struct {
	BatchID         string  `json:"batch_id"`
	EpisodeID       string  `json:"episode_id"`
	StructureID     string  `json:"structure_id"`
	ScriptVersionID string  `json:"script_version_id"`
	InputHash       string  `json:"input_hash"`
	ResultHash      *string `json:"result_hash"`
}

type DraftSet struct {
	ID, WorkspaceID, ProjectID, StructureCommitID, StructureContentHash string
	StructureRevision                                                   int
	Status, InputHash                                                   string
	ResultHash                                                          *string
	Batches                                                             []DraftSetBatch
	Revision                                                            int
	CreatedBy                                                           string
	CreatedAt, UpdatedAt                                                time.Time
}

type Invocation struct {
	ID, WorkspaceID, RequestID, Kind, InputHash, Status string
	Payload                                             json.RawMessage
	Attempts, ClaimVersion                              int
	LeaseExpiresAt                                      *time.Time
	CreatedAt                                           time.Time
}

type Shot struct {
	ID, WorkspaceID, ProjectID, EpisodeID, BatchID, ProposalKey, Title string
	Position, Revision                                                 int
	NarrativeUnitIDs                                                   []string
	Spec                                                               map[string]any
	ContentHash, Status, CreatedBy                                     string
	CreatedAt, UpdatedAt                                               time.Time
}

type ExportFile struct {
	Name, MediaType, SHA256 string
	Size                    int
}
type Export struct {
	ID, WorkspaceID, ProjectID, EpisodeID, Status, InputHash, ContentHash string
	Manifest                                                              map[string]any
	Files                                                                 []ExportFile
	Package                                                               []byte
	Revision                                                              int
	CreatedBy                                                             string
	CreatedAt, UpdatedAt                                                  time.Time
}

func DecodeAndValidateCandidate(raw json.RawMessage, payload json.RawMessage) (Candidate, error) {
	var candidate Candidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return Candidate{}, errors.New("candidate does not match storyboard schema")
	}
	var input struct {
		Units []struct {
			UnitVersionID string `json:"unit_version_id"`
			Required      bool   `json:"required_for_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return Candidate{}, errors.New("storyboard invocation payload is invalid")
	}
	known := map[string]bool{}
	required := map[string]bool{}
	for _, unit := range input.Units {
		if _, err := uuid.Parse(unit.UnitVersionID); err != nil {
			return Candidate{}, errors.New("storyboard unit id is invalid")
		}
		known[unit.UnitVersionID] = true
		if unit.Required {
			required[unit.UnitVersionID] = false
		}
	}
	if len(candidate.Shots) < 1 || len(candidate.Shots) > 120 {
		return Candidate{}, errors.New("storyboard candidate must contain 1-120 shots")
	}
	keys := map[string]struct{}{}
	for index, shot := range candidate.Shots {
		if shot.Position != index+1 || shot.ProposalKey == "" || shot.Title == "" || len(shot.NarrativeUnitVersionIDs) == 0 || shot.Spec == nil {
			return Candidate{}, errors.New("storyboard candidate contains an invalid shot")
		}
		if _, exists := keys[shot.ProposalKey]; exists {
			return Candidate{}, errors.New("storyboard proposal keys must be unique")
		}
		keys[shot.ProposalKey] = struct{}{}
		seen := map[string]struct{}{}
		for _, id := range shot.NarrativeUnitVersionIDs {
			if !known[id] {
				return Candidate{}, errors.New("storyboard shot references an unknown narrative unit")
			}
			if _, exists := seen[id]; exists {
				return Candidate{}, errors.New("storyboard shot unit references must be unique")
			}
			seen[id] = struct{}{}
			if _, exists := required[id]; exists {
				required[id] = true
			}
		}
		duration, ok := numberAsInt(shot.Spec["duration_ms"])
		if !ok || duration < 500 || duration > 15000 {
			return Candidate{}, errors.New("storyboard shot duration is invalid")
		}
	}
	for _, covered := range required {
		if !covered {
			return Candidate{}, errors.New("storyboard candidate does not cover every required unit")
		}
	}
	return candidate, nil
}

func OrderHash(shots []Shot) (string, error) {
	ordered := append([]Shot(nil), shots...)
	values := make([]struct {
		ID, ContentHash string
		Position        int
	}, len(ordered))
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	for index, shot := range ordered {
		values[index] = struct {
			ID, ContentHash string
			Position        int
		}{shot.ID, shot.ContentHash, shot.Position}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return canonicalHash(encoded)
}
func numberAsInt(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), number == float64(int(number))
	case json.Number:
		value, err := number.Int64()
		return int(value), err == nil
	case int:
		return number, true
	default:
		return 0, false
	}
}
func canonicalHash(raw []byte) (string, error) {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
