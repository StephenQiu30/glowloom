package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const PlanningSceneCollectionFamily = "planning_scene_set"

var productionWorldPlanningHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProductionWorldPlanningFact struct {
	ID, WorkspaceID, ProjectID, EpisodeID string
	Kind, BusinessKey                     string
	Revision                              int
	Payload                               json.RawMessage
	ContentHash, CreatedBy                string
	CreatedAt                             time.Time
}

type ProductionWorldPlanningFactRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	BusinessKey string `json:"business_key"`
	ContentHash string `json:"content_hash"`
	Revision    int    `json:"revision"`
}

type ExactPlanningRef struct {
	ID          string `json:"id"`
	BusinessKey string `json:"business_key"`
	ContentHash string `json:"content_hash"`
	Revision    int    `json:"revision"`
}

type SceneFactPayload struct {
	SceneScopeKey       string `json:"scene_scope_key"`
	SceneOwnerLogicalID string `json:"scene_owner_logical_id"`
	TemporarySceneID    string `json:"temporary_scene_id"`
	StoryTimeKey        string `json:"story_time_key"`
	SourceStart         int    `json:"source_start"`
	SourceEnd           int    `json:"source_end"`
}

type DialogueFactPayload struct {
	Scene       ProductionWorldPlanningFactRef `json:"scene"`
	SequenceKey int                            `json:"sequence_key"`
	Fragment    json.RawMessage                `json:"fragment"`
}

type NarrativeBeatFactPayload struct {
	Scene       ProductionWorldPlanningFactRef `json:"scene"`
	SequenceKey int                            `json:"sequence_key"`
	Fragment    json.RawMessage                `json:"fragment"`
}

type OccurrenceFactPayload struct {
	Scene         ProductionWorldPlanningFactRef `json:"scene"`
	Asset         ExactPlanningRef               `json:"asset"`
	State         ExactPlanningRef               `json:"state"`
	Specification ExactPlanningRef               `json:"specification"`
	Binding       ExactPlanningRef               `json:"binding"`
	SequenceKey   int                            `json:"sequence_key"`
	Fragment      json.RawMessage                `json:"fragment"`
}

type PlanningClaimFactPayload struct {
	ClaimType              string                          `json:"claim_type"`
	StoryTimeKey           string                          `json:"story_time_key"`
	SourceScene            ProductionWorldPlanningFactRef  `json:"source_scene"`
	TargetScene            ProductionWorldPlanningFactRef  `json:"target_scene"`
	ActorOccurrence        *ProductionWorldPlanningFactRef `json:"actor_occurrence"`
	PropOccurrence         *ProductionWorldPlanningFactRef `json:"prop_occurrence"`
	CounterpartyOccurrence *ProductionWorldPlanningFactRef `json:"counterparty_occurrence"`
	BeforeState            *ExactPlanningRef               `json:"before_state"`
	AfterState             *ExactPlanningRef               `json:"after_state"`
	Fragment               json.RawMessage                 `json:"fragment"`
}

type ProductionWorldPlanningMember struct {
	Position   int
	Fact       ProductionWorldPlanningFactRef
	MemberHash string
}

type ProductionWorldPlanningEpisodeHead struct {
	WorkspaceID, ProjectID, EpisodeID, ScopeKey string
	ScopeRevision, HeadRevision                 int64
	Members                                     []ProductionWorldPlanningMember
	MemberCount                                 int
	ScopeContentHash, MembersHash               string
	CollectionRootHash, HeadContentHash         string
	UpdatedAt                                   time.Time
}

func NewProductionWorldPlanningFact(id, workspaceID, projectID, episodeID, kind, businessKey string, revision int, payload json.RawMessage, createdBy string, createdAt time.Time) (ProductionWorldPlanningFact, error) {
	if !planningUUIDs(id, workspaceID, projectID, episodeID, createdBy) || !planningFactKind(kind) || strings.TrimSpace(businessKey) == "" || revision < 1 || createdAt.IsZero() {
		return ProductionWorldPlanningFact{}, errors.New("invalid Production World Planning fact input")
	}
	payload, err := canonicalPlanningObject(payload)
	if err != nil {
		return ProductionWorldPlanningFact{}, errors.New("invalid Production World Planning fact payload")
	}
	value := ProductionWorldPlanningFact{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
		Kind: kind, BusinessKey: businessKey, Revision: revision, Payload: payload,
		CreatedBy: createdBy, CreatedAt: createdAt.UTC(),
	}
	value.ContentHash = planningHash(struct {
		Schema, Kind, BusinessKey string
		Payload                   json.RawMessage
	}{"production-world-planning-fact", kind, businessKey, payload})
	return value, nil
}

func PlanningFactRef(value ProductionWorldPlanningFact) ProductionWorldPlanningFactRef {
	return ProductionWorldPlanningFactRef{ID: value.ID, Kind: value.Kind, BusinessKey: value.BusinessKey, Revision: value.Revision, ContentHash: value.ContentHash}
}

func ValidateProductionWorldPlanningFact(value ProductionWorldPlanningFact) error {
	rebuilt, err := NewProductionWorldPlanningFact(
		value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID,
		value.Kind, value.BusinessKey, value.Revision, value.Payload,
		value.CreatedBy, value.CreatedAt,
	)
	if err != nil || rebuilt.ContentHash != value.ContentHash {
		return errors.New("Production World Planning fact has drifted")
	}
	return nil
}

func PlanningBusinessKeyRoot(keys []string) (string, error) {
	keys = append([]string(nil), keys...)
	slices.Sort(keys)
	for index, key := range keys {
		if strings.TrimSpace(key) == "" || (index > 0 && keys[index-1] == key) {
			return "", errors.New("invalid Production World Planning business key set")
		}
	}
	return planningHash(keys), nil
}

func NewProductionWorldPlanningEpisodeHead(workspaceID, projectID, episodeID string, revision int64, facts []ProductionWorldPlanningFact, updatedAt time.Time) (ProductionWorldPlanningEpisodeHead, error) {
	if !planningUUIDs(workspaceID, projectID, episodeID) || revision < 1 || len(facts) == 0 || updatedAt.IsZero() {
		return ProductionWorldPlanningEpisodeHead{}, errors.New("invalid Production World Planning Episode input")
	}
	facts = append([]ProductionWorldPlanningFact(nil), facts...)
	slices.SortFunc(facts, func(left, right ProductionWorldPlanningFact) int {
		if compared := strings.Compare(left.Kind, right.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(left.BusinessKey, right.BusinessKey)
	})
	members := make([]ProductionWorldPlanningMember, len(facts))
	for index, fact := range facts {
		rebuilt, err := NewProductionWorldPlanningFact(fact.ID, fact.WorkspaceID, fact.ProjectID, fact.EpisodeID, fact.Kind, fact.BusinessKey, fact.Revision, fact.Payload, fact.CreatedBy, fact.CreatedAt)
		if err != nil || rebuilt.ContentHash != fact.ContentHash || fact.WorkspaceID != workspaceID || fact.ProjectID != projectID || fact.EpisodeID != episodeID ||
			(index > 0 && facts[index-1].Kind == fact.Kind && facts[index-1].BusinessKey == fact.BusinessKey) {
			return ProductionWorldPlanningEpisodeHead{}, errors.New("invalid Production World Planning member set")
		}
		ref := PlanningFactRef(fact)
		members[index] = ProductionWorldPlanningMember{
			Position: index + 1, Fact: ref,
			MemberHash: planningHash(struct {
				Schema string
				Fact   ProductionWorldPlanningFactRef
			}{"production-world-planning-member", ref}),
		}
	}
	membersHash := planningHash(struct {
		Schema  string
		Members []ProductionWorldPlanningMember
	}{"production-world-planning-members", members})
	scopeKey := "episode:" + episodeID
	rootRefs := make([]ProductionWorldPlanningFactRef, len(members))
	for index, member := range members {
		rootRefs[index] = member.Fact
	}
	scopeHash := planningHash(struct {
		Schema, OwnerKind, Family, ScopeKey string
		ScopeRevision                       int64
		RootRefs                            []ProductionWorldPlanningFactRef
	}{"production-world-planning-scope", "production/planning", PlanningSceneCollectionFamily, scopeKey, revision, rootRefs})
	rootHash := planningHash(struct {
		Schema, Family, ScopeKey, ScopeContentHash, MembersHash string
		ScopeRevision                                           int64
		MemberCount                                             int
	}{"production-world-planning-collection", PlanningSceneCollectionFamily, scopeKey, scopeHash, membersHash, revision, len(members)})
	value := ProductionWorldPlanningEpisodeHead{
		WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
		ScopeKey: scopeKey, ScopeRevision: revision, HeadRevision: revision,
		Members: members, MemberCount: len(members), ScopeContentHash: scopeHash, MembersHash: membersHash,
		CollectionRootHash: rootHash, UpdatedAt: updatedAt.UTC(),
	}
	value.HeadContentHash = planningHash(struct {
		Schema, WorkspaceID, ProjectID, EpisodeID, ScopeKey, ScopeContentHash, MembersHash, CollectionRootHash string
		ScopeRevision, HeadRevision                                                                            int64
		MemberCount                                                                                            int
	}{"production-world-planning-head", workspaceID, projectID, episodeID, scopeKey, scopeHash, membersHash, rootHash, revision, revision, len(members)})
	return value, nil
}

func ValidateProductionWorldPlanningMember(value ProductionWorldPlanningMember) error {
	if value.Position < 1 || !planningUUIDs(value.Fact.ID) || !planningFactKind(value.Fact.Kind) || strings.TrimSpace(value.Fact.BusinessKey) == "" || value.Fact.Revision < 1 || !productionWorldPlanningHashPattern.MatchString(value.Fact.ContentHash) || !productionWorldPlanningHashPattern.MatchString(value.MemberHash) ||
		planningHash(struct {
			Schema string
			Fact   ProductionWorldPlanningFactRef
		}{"production-world-planning-member", value.Fact}) != value.MemberHash {
		return errors.New("Production World Planning member has drifted")
	}
	return nil
}

func planningFactKind(value string) bool {
	return slices.Contains([]string{"scene", "dialogue", "narrative_beat", "occurrence", "continuity_claim"}, value)
}

func planningUUIDs(values ...string) bool {
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return false
		}
	}
	return true
}

func canonicalPlanningObject(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("payload must be an object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("multiple JSON values")
	}
	return json.Marshal(value)
}

func planningHash(value any) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
