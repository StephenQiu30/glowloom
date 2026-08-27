package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const StoryCandidateRepairOperation = "production_bible.candidate_repair.apply"

type StoryCandidateRepairCommand struct {
	WorkspaceID                   string
	StageInstanceKey              string
	ExpectedRevisionID            string
	ExpectedCandidateRevisionHash string
	ExpectedHeadRevision          int64
	RepairInvocationID            string
	IdempotencyKey                string
}

type StoryCandidateRepairSeed struct {
	ParentCandidate  json.RawMessage
	RepairInput      agentcontract.StoryGraphRepairStageInput
	RepairPatch      agentcontract.CandidateRepairPatch
	RepairResultHash string
}

type StoryCandidateRepairPreparation struct {
	Command   StoryCandidateRepairCommand
	InputHash string
	ReceiptID string
	Candidate json.RawMessage
	CreatedAt time.Time
}

type StoryCandidateRepairResult struct {
	ReceiptID              string   `json:"receipt_id"`
	CandidateRevisionID    string   `json:"candidate_revision_id"`
	CandidateRevisionHash  string   `json:"candidate_revision_hash"`
	CandidateRevisionNo    int64    `json:"candidate_revision_no"`
	StaleStageInstanceKeys []string `json:"stale_stage_instance_keys"`
}

type StoryCandidateRepairRepository interface {
	LoadStoryCandidateRepair(context.Context, Actor, StoryCandidateRepairCommand) (StoryCandidateRepairSeed, error)
	ApplyStoryCandidateRepair(context.Context, Actor, StoryCandidateRepairPreparation) (StoryCandidateRepairResult, error)
}

type StoryCandidateRepairService struct {
	repository StoryCandidateRepairRepository
	config     Config
}

func NewStoryCandidateRepairService(
	repository StoryCandidateRepairRepository,
	config Config,
) *StoryCandidateRepairService {
	return &StoryCandidateRepairService{repository: repository, config: config}
}

func (service *StoryCandidateRepairService) Apply(
	ctx context.Context,
	actor Actor,
	command StoryCandidateRepairCommand,
) (StoryCandidateRepairResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.StageInstanceKey = strings.TrimSpace(command.StageInstanceKey)
	command.ExpectedRevisionID = strings.TrimSpace(command.ExpectedRevisionID)
	command.ExpectedCandidateRevisionHash = strings.TrimSpace(command.ExpectedCandidateRevisionHash)
	command.RepairInvocationID = strings.TrimSpace(command.RepairInvocationID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || actor.TokenVersion < 1 || command.ExpectedHeadRevision < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 ||
		!candidateHash(command.StageInstanceKey) || !candidateHash(command.ExpectedCandidateRevisionHash) {
		return StoryCandidateRepairResult{}, invalid("Invalid Story candidate repair request")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ExpectedRevisionID, command.RepairInvocationID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return StoryCandidateRepairResult{}, invalid("Invalid Story candidate repair request")
		}
	}

	seed, err := service.repository.LoadStoryCandidateRepair(ctx, actor, command)
	if err != nil {
		return StoryCandidateRepairResult{}, err
	}
	candidate, err := ApplyStoryCandidateRepairPatch(seed.ParentCandidate, seed.RepairInput, seed.RepairPatch)
	if err != nil {
		return StoryCandidateRepairResult{}, err
	}
	candidateHashValue, err := agentcontract.CanonicalHash(candidate)
	if err != nil {
		return StoryCandidateRepairResult{}, err
	}
	inputHash, err := platformcommand.InputHash(struct {
		WorkspaceID, StageInstanceKey, ExpectedRevisionID, ExpectedCandidateRevisionHash string
		ExpectedHeadRevision                                                             int64
		RepairInvocationID, RepairResultHash, CandidateContentHash                       string
	}{
		command.WorkspaceID, command.StageInstanceKey, command.ExpectedRevisionID,
		command.ExpectedCandidateRevisionHash, command.ExpectedHeadRevision,
		command.RepairInvocationID, seed.RepairResultHash, candidateHashValue,
	})
	if err != nil {
		return StoryCandidateRepairResult{}, err
	}
	return service.repository.ApplyStoryCandidateRepair(ctx, actor, StoryCandidateRepairPreparation{
		Command: command, InputHash: inputHash, ReceiptID: service.config.NewID(),
		Candidate: candidate, CreatedAt: service.config.Now().UTC(),
	})
}

type CandidateRevisionRef struct {
	ID   string
	Hash string
}

type CandidateStageDependency struct {
	InvocationID       string
	StageInstanceKey   string
	UpstreamCandidates []agentcontract.StageUpstreamCandidateRef
	CandidateRevisions []CandidateRevisionRef
}

type CandidateStageStaleness struct {
	InvocationID               string
	StageInstanceKey           string
	CauseCandidateRevisionID   string
	CauseCandidateRevisionHash string
}

func ApplyStoryCandidateRepairPatch(
	parent json.RawMessage,
	input agentcontract.StoryGraphRepairStageInput,
	patch agentcontract.CandidateRepairPatch,
) (json.RawMessage, error) {
	if err := agentcontract.ValidateCandidateRepairPatch(input, patch); err != nil {
		return nil, err
	}
	var candidate map[string]any
	decoder := json.NewDecoder(bytes.NewReader(parent))
	decoder.UseNumber()
	if err := decoder.Decode(&candidate); err != nil || candidate == nil {
		return nil, errors.New("Candidate repair parent must be one JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("Candidate repair parent contains trailing JSON")
	}

	targetKeys := make(map[string]struct{}, len(input.AllowedTargets))
	for _, allowed := range input.AllowedTargets {
		targetKeys[allowed.CandidateKey] = struct{}{}
	}
	indexed, err := indexCandidateRepairFragments(candidate, targetKeys)
	if err != nil {
		return nil, err
	}
	targets := make(map[string]map[string]any, len(input.AllowedTargets))
	for _, allowed := range input.AllowedTargets {
		fragment, exists := indexed[allowed.CandidateKey]
		if !exists {
			return nil, errors.New("Candidate repair allowed target is absent from the parent revision")
		}
		encoded, marshalErr := json.Marshal(fragment)
		if marshalErr != nil {
			return nil, marshalErr
		}
		actualHash, hashErr := agentcontract.StoryGraphCandidateFragmentHash(encoded)
		if hashErr != nil || actualHash != allowed.BaseFragmentHash {
			return nil, errors.New("Candidate repair allowed fragment does not match the parent revision")
		}
		targets[allowed.CandidateKey] = fragment
	}

	for _, operation := range patch.Operations {
		target := targets[operation.TargetCandidateKey]
		if _, exists := target[operation.FieldName]; !exists {
			return nil, errors.New("Candidate repair field is absent from its frozen fragment")
		}
		switch {
		case operation.Replacement.Text != nil:
			target[operation.FieldName] = *operation.Replacement.Text
		case operation.Replacement.Strings != nil:
			values := make([]any, len(*operation.Replacement.Strings))
			for index, value := range *operation.Replacement.Strings {
				values[index] = value
			}
			target[operation.FieldName] = values
		default:
			return nil, errors.New("Candidate repair replacement type is unsupported for Bible candidates")
		}
	}

	repaired, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	var preliminary domain.StoryReconciliationCandidate
	if err = json.Unmarshal(repaired, &preliminary); err != nil {
		return nil, err
	}
	if _, err = domain.DecodeStoryReconciliationCandidate(
		repaired,
		domain.StoryReconciliationCandidateEvidence(preliminary),
	); err != nil {
		return nil, fmt.Errorf("validate repaired Story candidate: %w", err)
	}
	return json.RawMessage(repaired), nil
}

func StoryCandidateRepairAllowedTarget(
	candidate json.RawMessage,
	candidateKey string,
) (agentcontract.StoryGraphRepairAllowedTarget, error) {
	if strings.TrimSpace(candidateKey) == "" {
		return agentcontract.StoryGraphRepairAllowedTarget{}, errors.New("Story candidate repair target key is required")
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(candidate))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return agentcontract.StoryGraphRepairAllowedTarget{}, errors.New("Story candidate repair parent must be one JSON object")
	}
	indexed, err := indexCandidateRepairFragments(value, map[string]struct{}{candidateKey: {}})
	if err != nil {
		return agentcontract.StoryGraphRepairAllowedTarget{}, err
	}
	fragment, exists := indexed[candidateKey]
	if !exists {
		return agentcontract.StoryGraphRepairAllowedTarget{}, errors.New("Story candidate repair target is absent from its frozen candidate")
	}
	encoded, err := json.Marshal(fragment)
	if err != nil {
		return agentcontract.StoryGraphRepairAllowedTarget{}, err
	}
	fields, err := agentcontract.StoryGraphRepairableFields(encoded)
	if err != nil {
		return agentcontract.StoryGraphRepairAllowedTarget{}, err
	}
	hash, err := agentcontract.StoryGraphCandidateFragmentHash(encoded)
	if err != nil {
		return agentcontract.StoryGraphRepairAllowedTarget{}, err
	}
	return agentcontract.StoryGraphRepairAllowedTarget{
		CandidateKey: candidateKey, AllowedFields: fields, BaseFragmentHash: hash, Fragment: encoded,
	}, nil
}

func StoryCandidateStaleClosure(
	root CandidateRevisionRef,
	appliedRepairInvocationID string,
	existingStageInstanceKeys []string,
	dependencies []CandidateStageDependency,
) ([]CandidateStageStaleness, error) {
	if _, err := uuid.Parse(root.ID); err != nil || !candidateHash(root.Hash) {
		return nil, errors.New("invalid Candidate stale closure root")
	}
	if _, err := uuid.Parse(appliedRepairInvocationID); err != nil {
		return nil, errors.New("invalid applied Candidate repair invocation")
	}

	ordered := append([]CandidateStageDependency(nil), dependencies...)
	slices.SortFunc(ordered, func(left, right CandidateStageDependency) int {
		if compared := strings.Compare(left.StageInstanceKey, right.StageInstanceKey); compared != 0 {
			return compared
		}
		return strings.Compare(left.InvocationID, right.InvocationID)
	})
	knownStages := make(map[string]struct{}, len(ordered))
	knownInvocations := make(map[string]struct{}, len(ordered))
	for _, dependency := range ordered {
		if dependency.InvocationID != "" {
			if _, err := uuid.Parse(dependency.InvocationID); err != nil {
				return nil, errors.New("invalid Candidate stage dependency identity")
			}
		}
		if !candidateHash(dependency.StageInstanceKey) {
			return nil, errors.New("invalid Candidate stage dependency identity")
		}
		if _, exists := knownStages[dependency.StageInstanceKey]; exists {
			return nil, errors.New("duplicate Candidate stage dependency")
		}
		if dependency.InvocationID != "" {
			if _, exists := knownInvocations[dependency.InvocationID]; exists {
				return nil, errors.New("duplicate Candidate invocation dependency")
			}
			knownInvocations[dependency.InvocationID] = struct{}{}
		}
		knownStages[dependency.StageInstanceKey] = struct{}{}
		for _, revision := range dependency.CandidateRevisions {
			if _, err := uuid.Parse(revision.ID); err != nil || !candidateHash(revision.Hash) {
				return nil, errors.New("invalid Candidate dependency output revision")
			}
		}
	}

	staleStages := make(map[string]struct{}, len(existingStageInstanceKeys)+len(ordered))
	for _, key := range existingStageInstanceKeys {
		if !candidateHash(key) {
			return nil, errors.New("invalid existing Candidate stage staleness")
		}
		staleStages[key] = struct{}{}
	}
	staleRevisions := map[string]struct{}{candidateRevisionIdentity(root.ID, root.Hash): {}}
	for _, dependency := range ordered {
		if _, exists := staleStages[dependency.StageInstanceKey]; exists {
			addCandidateRevisions(staleRevisions, dependency.CandidateRevisions)
		}
	}

	closure := make([]CandidateStageStaleness, 0)
	for changed := true; changed; {
		changed = false
		for _, dependency := range ordered {
			if dependency.InvocationID == appliedRepairInvocationID {
				continue
			}
			if _, exists := staleStages[dependency.StageInstanceKey]; exists {
				continue
			}
			upstreams := append([]agentcontract.StageUpstreamCandidateRef(nil), dependency.UpstreamCandidates...)
			slices.SortFunc(upstreams, func(left, right agentcontract.StageUpstreamCandidateRef) int {
				return strings.Compare(
					candidateRevisionIdentity(left.CandidateRevisionID, left.CandidateRevisionHash),
					candidateRevisionIdentity(right.CandidateRevisionID, right.CandidateRevisionHash),
				)
			})
			for _, upstream := range upstreams {
				if _, exists := staleRevisions[candidateRevisionIdentity(
					upstream.CandidateRevisionID,
					upstream.CandidateRevisionHash,
				)]; !exists {
					continue
				}
				staleStages[dependency.StageInstanceKey] = struct{}{}
				addCandidateRevisions(staleRevisions, dependency.CandidateRevisions)
				closure = append(closure, CandidateStageStaleness{
					InvocationID: dependency.InvocationID, StageInstanceKey: dependency.StageInstanceKey,
					CauseCandidateRevisionID:   upstream.CandidateRevisionID,
					CauseCandidateRevisionHash: upstream.CandidateRevisionHash,
				})
				changed = true
				break
			}
		}
	}
	return closure, nil
}

func indexCandidateRepairFragments(
	candidate map[string]any,
	targetKeys map[string]struct{},
) (map[string]map[string]any, error) {
	result := map[string]map[string]any{}
	var visit func(any, string) error
	visit = func(value any, parentEntityKey string) error {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if err := visit(item, parentEntityKey); err != nil {
					return err
				}
			}
		case map[string]any:
			currentEntityKey := parentEntityKey
			identities := make([]string, 0, 2)
			for _, field := range []string{"entity_key", "entry_key", "claim_key", "arc_key", "issue_key"} {
				if identity, ok := typed[field].(string); ok && strings.TrimSpace(identity) != "" {
					identities = append(identities, identity)
					if field == "entity_key" {
						currentEntityKey = identity
					}
				}
			}
			if stateKey, ok := typed["state_key"].(string); ok && strings.TrimSpace(stateKey) != "" {
				if currentEntityKey == "" {
					identities = append(identities, stateKey)
				} else {
					identities = append(identities, currentEntityKey+":"+stateKey)
				}
			}
			for _, identity := range identities {
				if _, wanted := targetKeys[identity]; !wanted {
					continue
				}
				if _, exists := result[identity]; exists {
					return errors.New("Candidate repair parent contains a duplicate stable key")
				}
				result[identity] = typed
			}
			for _, item := range typed {
				if err := visit(item, currentEntityKey); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(candidate, ""); err != nil {
		return nil, err
	}
	return result, nil
}

func addCandidateRevisions(target map[string]struct{}, revisions []CandidateRevisionRef) {
	for _, revision := range revisions {
		target[candidateRevisionIdentity(revision.ID, revision.Hash)] = struct{}{}
	}
}

func candidateRevisionIdentity(identifier, hash string) string { return identifier + "\x00" + hash }

func candidateHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
