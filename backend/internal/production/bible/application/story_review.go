package application

import (
	"errors"
	"slices"
	"strings"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func EvaluateStoryReconciliationGate(
	targetCandidateRevisionID string,
	targetCandidateRevisionHash string,
	candidate domain.StoryReconciliationCandidate,
) (agentcontract.StoryGraphDeterministicGateResult, error) {
	allowedEvidence := domain.StoryReconciliationCandidateEvidence(candidate)
	if err := domain.ValidateStoryReconciliationCandidate(candidate, allowedEvidence); err != nil {
		return agentcontract.StoryGraphDeterministicGateResult{}, errors.New("invalid Story reconciliation candidate for deterministic gate")
	}

	entityKeys := make(map[string]struct{}, len(candidate.CanonicalEntities))
	allKeys := make(map[string]struct{}, len(candidate.CanonicalEntities)+len(candidate.CanonicalWorldEntries)+len(candidate.MergedClaims)+len(candidate.MergedArcs))
	for _, entity := range candidate.CanonicalEntities {
		entityKeys[entity.EntityKey] = struct{}{}
		allKeys[entity.EntityKey] = struct{}{}
		for _, state := range entity.States {
			allKeys[entity.EntityKey+":"+state.StateKey] = struct{}{}
		}
	}
	for _, entry := range candidate.CanonicalWorldEntries {
		allKeys[entry.EntryKey] = struct{}{}
	}
	for _, claim := range candidate.MergedClaims {
		allKeys[claim.ClaimKey] = struct{}{}
	}
	for _, arc := range candidate.MergedArcs {
		allKeys[arc.ArcKey] = struct{}{}
	}

	blockers := make([]agentcontract.StoryGraphGateBlocker, 0)
	appendReferenceBlockers := func(code, duplicateCode, subject string, values []string, known map[string]struct{}) {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if _, exists := seen[value]; exists {
				blockers = append(blockers, agentcontract.StoryGraphGateBlocker{
					Code: duplicateCode, SubjectKey: subject, RelatedKey: value,
					Summary: "候选引用包含重复的确定性 Key",
				})
				continue
			}
			seen[value] = struct{}{}
			if _, exists := known[value]; !exists {
				blockers = append(blockers, agentcontract.StoryGraphGateBlocker{
					Code: code, SubjectKey: subject, RelatedKey: value,
					Summary: "候选引用的 Key 不存在于冻结候选版本",
				})
			}
		}
	}
	for _, entry := range candidate.CanonicalWorldEntries {
		appendReferenceBlockers("world_unknown_entity", "world_duplicate_entity", entry.EntryKey, entry.EntityKeys, entityKeys)
	}
	for _, claim := range candidate.MergedClaims {
		appendReferenceBlockers("claim_unknown_participant", "claim_duplicate_participant", claim.ClaimKey, claim.ParticipantKeys, entityKeys)
		appendReferenceBlockers("claim_unknown_anchor", "claim_duplicate_anchor", claim.ClaimKey, claim.AnchorKeys, allKeys)
	}

	slices.SortFunc(blockers, func(left, right agentcontract.StoryGraphGateBlocker) int {
		return strings.Compare(
			left.Code+"\x00"+left.SubjectKey+"\x00"+left.RelatedKey,
			right.Code+"\x00"+right.SubjectKey+"\x00"+right.RelatedKey,
		)
	})
	gate := agentcontract.StoryGraphDeterministicGateResult{
		GateVersion:                 agentcontract.BibleDeterministicGateVersion,
		TargetCandidateRevisionID:   targetCandidateRevisionID,
		TargetCandidateRevisionHash: targetCandidateRevisionHash,
		Blockers:                    blockers,
	}
	if err := gate.Validate(targetCandidateRevisionID, targetCandidateRevisionHash); err != nil {
		return agentcontract.StoryGraphDeterministicGateResult{}, err
	}
	return gate, nil
}
