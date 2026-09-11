package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

var (
	ErrProductionWorldBibleHeadNotFound = errors.New("Production World Bible Head not found")
	ErrProductionWorldBibleConflict     = errors.New("Production World Bible Head conflict")
)

type ProductionWorldSpecificationInput struct {
	SpecificationKey, IdentityKey, Kind string
	Slots                               []agentcontract.ProductionSemanticSlot
	Basis                               agentcontract.ProductionSourceBasis
}

type ProductionWorldClaimInput struct {
	ClaimKey, ClaimType, Statement string
	Participants                   []agentcontract.ProductionWorldClaimParticipant
	Narrative                      *agentcontract.ProductionWorldNarrativeClaim
	Basis                          agentcontract.ProductionSourceBasis
}

type ApplyProductionWorldBibleCommand struct {
	WorkspaceID, ProjectID, ActorID           string
	ReviewDecisionID                          string
	ExpectedHeadRevision                      int64
	ExpectedHeadHash, ExpectedBusinessKeyRoot string
	PartitionHash                             string
	StructureIdentitySet, Candidate           domain.ProductionWorldOwnerRef
	Specifications                            []ProductionWorldSpecificationInput
	Claims                                    []ProductionWorldClaimInput
	Assets                                    []assetdomain.Asset
	States                                    []assetdomain.AssetState
}

type ApplyProductionWorldBibleResult struct {
	Version        domain.ProductionWorldBibleVersion
	Head           domain.ProductionWorldBibleHead
	Evidence       []domain.ProductionWorldEvidence
	Specifications []domain.ProductionWorldSpecification
	Claims         []domain.ProductionWorldClaim
	Bindings       []domain.ProductionWorldBinding
}

type ProductionWorldBibleRepository interface {
	GetProductionWorldBibleHead(context.Context, string, string, bool) (domain.ProductionWorldBibleHead, domain.ProductionWorldBibleVersion, error)
	ListProductionWorldEvidence(context.Context, string, string, bool) ([]domain.ProductionWorldEvidence, error)
	ListProductionWorldSpecifications(context.Context, string, string, bool) ([]domain.ProductionWorldSpecification, error)
	ListProductionWorldClaims(context.Context, string, string, bool) ([]domain.ProductionWorldClaim, error)
	ListProductionWorldBindings(context.Context, string, string, bool) ([]domain.ProductionWorldBinding, error)
	CreateProductionWorldEvidence(context.Context, []domain.ProductionWorldEvidence) error
	CreateProductionWorldSpecifications(context.Context, []domain.ProductionWorldSpecification) error
	CreateProductionWorldClaims(context.Context, []domain.ProductionWorldClaim) error
	CreateProductionWorldBindings(context.Context, []domain.ProductionWorldBinding) error
	CreateProductionWorldBibleVersion(context.Context, domain.ProductionWorldBibleVersion) error
	SaveProductionWorldBibleHead(context.Context, domain.ProductionWorldBibleHead, int64, string) error
}

type ProductionWorldBibleOwner struct {
	now   func() time.Time
	newID func() string
}

func NewProductionWorldBibleOwner(now func() time.Time, newID func() string) *ProductionWorldBibleOwner {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &ProductionWorldBibleOwner{now: now, newID: newID}
}

func (owner *ProductionWorldBibleOwner) ApplyProductionWorldBible(ctx context.Context, repository ProductionWorldBibleRepository, command ApplyProductionWorldBibleCommand) (ApplyProductionWorldBibleResult, error) {
	if owner == nil || repository == nil {
		return ApplyProductionWorldBibleResult{}, errors.New("Production World Bible owner is unavailable")
	}
	if err := validateProductionWorldBibleCommand(command); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	head, current, headErr := repository.GetProductionWorldBibleHead(ctx, command.WorkspaceID, command.ProjectID, true)
	if command.ExpectedHeadRevision == 0 {
		if headErr == nil || !errors.Is(headErr, ErrProductionWorldBibleHeadNotFound) {
			return ApplyProductionWorldBibleResult{}, ErrProductionWorldBibleConflict
		}
	} else if headErr != nil || head.HeadRevision != command.ExpectedHeadRevision || head.HeadContentHash != command.ExpectedHeadHash {
		return ApplyProductionWorldBibleResult{}, ErrProductionWorldBibleConflict
	}
	now := owner.now().UTC()
	assets, statesByAsset := productionWorldBibleAssets(command)
	evidence, newEvidence, err := owner.buildProductionWorldEvidence(ctx, repository, command, now)
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	specifications, newSpecifications, err := owner.buildProductionWorldSpecifications(ctx, repository, command, assets, evidence, now)
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	claims, newClaims, err := owner.buildProductionWorldClaims(ctx, repository, command, assets, evidence, now)
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	bindings, newBindings, err := owner.buildProductionWorldBindings(ctx, repository, command, assets, statesByAsset, specifications, now)
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	version, err := domain.NewProductionWorldBibleVersion(domain.ProductionWorldBibleVersion{
		ID: owner.newID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Revision: command.ExpectedHeadRevision + 1,
		StructureIdentitySet: command.StructureIdentitySet, Candidate: command.Candidate, ReviewDecisionID: command.ReviewDecisionID,
		PartitionHash: command.PartitionHash, BusinessKeyRoot: command.ExpectedBusinessKeyRoot,
		Evidence: productionWorldEvidenceRefs(evidence), Specifications: productionWorldSpecificationRefs(specifications),
		Claims: productionWorldClaimRefs(claims), Bindings: productionWorldBindingRefs(bindings), CreatedBy: command.ActorID, CreatedAt: now,
	})
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if headErr == nil && version.ContentHash == current.ContentHash {
		return ApplyProductionWorldBibleResult{Version: current, Head: head, Evidence: evidence, Specifications: specifications, Claims: claims, Bindings: bindings}, nil
	}
	if err = repository.CreateProductionWorldEvidence(ctx, newEvidence); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if err = repository.CreateProductionWorldSpecifications(ctx, newSpecifications); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if err = repository.CreateProductionWorldClaims(ctx, newClaims); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if err = repository.CreateProductionWorldBindings(ctx, newBindings); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if err = repository.CreateProductionWorldBibleVersion(ctx, version); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	nextHead, err := domain.NewProductionWorldBibleHead(command.WorkspaceID, command.ProjectID, version.Revision, version, now)
	if err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	if err = repository.SaveProductionWorldBibleHead(ctx, nextHead, command.ExpectedHeadRevision, command.ExpectedHeadHash); err != nil {
		return ApplyProductionWorldBibleResult{}, err
	}
	return ApplyProductionWorldBibleResult{Version: version, Head: nextHead, Evidence: evidence, Specifications: specifications, Claims: claims, Bindings: bindings}, nil
}

func (owner *ProductionWorldBibleOwner) buildProductionWorldEvidence(ctx context.Context, repo ProductionWorldBibleRepository, command ApplyProductionWorldBibleCommand, now time.Time) ([]domain.ProductionWorldEvidence, []domain.ProductionWorldEvidence, error) {
	inputs := make([]struct {
		key   string
		basis agentcontract.ProductionSourceBasis
	}, 0, len(command.Specifications)+len(command.Claims))
	for _, item := range command.Specifications {
		inputs = append(inputs, struct {
			key   string
			basis agentcontract.ProductionSourceBasis
		}{item.SpecificationKey, item.Basis})
	}
	for _, item := range command.Claims {
		inputs = append(inputs, struct {
			key   string
			basis agentcontract.ProductionSourceBasis
		}{item.ClaimKey, item.Basis})
	}
	slices.SortFunc(inputs, func(left, right struct {
		key   string
		basis agentcontract.ProductionSourceBasis
	}) int {
		if left.key < right.key {
			return -1
		}
		if left.key > right.key {
			return 1
		}
		return 0
	})
	result, created := make([]domain.ProductionWorldEvidence, 0, len(inputs)), []domain.ProductionWorldEvidence{}
	for _, input := range inputs {
		existing, err := repo.ListProductionWorldEvidence(ctx, command.ProjectID, input.key, true)
		if err != nil {
			return nil, nil, err
		}
		basis, _ := json.Marshal(input.basis)
		candidate, err := domain.NewProductionWorldEvidence(owner.newID(), command.WorkspaceID, command.ProjectID, input.key, nextEvidenceRevision(existing), basis, command.ActorID, now)
		if err != nil {
			return nil, nil, err
		}
		value, reused := reuseEvidence(existing, candidate.ContentHash)
		if !reused {
			value, created = candidate, append(created, candidate)
		}
		result = append(result, value)
	}
	return result, created, nil
}

func (owner *ProductionWorldBibleOwner) buildProductionWorldSpecifications(ctx context.Context, repo ProductionWorldBibleRepository, command ApplyProductionWorldBibleCommand, assets map[string]assetdomain.Asset, evidence []domain.ProductionWorldEvidence, now time.Time) ([]domain.ProductionWorldSpecification, []domain.ProductionWorldSpecification, error) {
	evidenceByKey := map[string]domain.ProductionWorldEvidence{}
	for _, value := range evidence {
		evidenceByKey[value.SubjectKey] = value
	}
	result, created := make([]domain.ProductionWorldSpecification, 0, len(command.Specifications)), []domain.ProductionWorldSpecification{}
	for _, input := range command.Specifications {
		asset := assets[input.IdentityKey]
		basis := evidenceByKey[input.SpecificationKey]
		existing, err := repo.ListProductionWorldSpecifications(ctx, command.ProjectID, input.SpecificationKey, true)
		if err != nil {
			return nil, nil, err
		}
		slots, _ := json.Marshal(input.Slots)
		candidate, err := domain.NewProductionWorldSpecification(owner.newID(), command.WorkspaceID, command.ProjectID, input.SpecificationKey, input.IdentityKey, input.Kind, nextSpecificationRevision(existing), domain.FragmentRef(asset.ID, "asset_identity", asset.IdentityKey, asset.ContentHash, asset.Revision), domain.FragmentRef(basis.ID, "source_evidence", basis.SubjectKey, basis.ContentHash, basis.Revision), slots, command.ActorID, now)
		if err != nil {
			return nil, nil, err
		}
		value, reused := reuseSpecification(existing, candidate.ContentHash)
		if !reused {
			value, created = candidate, append(created, candidate)
		}
		result = append(result, value)
	}
	return result, created, nil
}

func (owner *ProductionWorldBibleOwner) buildProductionWorldClaims(ctx context.Context, repo ProductionWorldBibleRepository, command ApplyProductionWorldBibleCommand, assets map[string]assetdomain.Asset, evidence []domain.ProductionWorldEvidence, now time.Time) ([]domain.ProductionWorldClaim, []domain.ProductionWorldClaim, error) {
	evidenceByKey := map[string]domain.ProductionWorldEvidence{}
	for _, value := range evidence {
		evidenceByKey[value.SubjectKey] = value
	}
	result, created := make([]domain.ProductionWorldClaim, 0, len(command.Claims)), []domain.ProductionWorldClaim{}
	for _, input := range command.Claims {
		participants := make([]domain.ProductionWorldClaimParticipant, len(input.Participants))
		for index, item := range input.Participants {
			asset := assets[item.IdentityKey]
			participants[index] = domain.ProductionWorldClaimParticipant{Role: item.Role, IdentityKey: item.IdentityKey, AssetID: asset.ID, AssetContentHash: asset.ContentHash}
		}
		narrative := productionWorldNarrativeClaim(input.Narrative)
		basis := evidenceByKey[input.ClaimKey]
		existing, err := repo.ListProductionWorldClaims(ctx, command.ProjectID, input.ClaimKey, true)
		if err != nil {
			return nil, nil, err
		}
		evidenceRef := domain.FragmentRef(basis.ID, "source_evidence", basis.SubjectKey, basis.ContentHash, basis.Revision)
		if len(existing) > 0 && productionWorldClaimInputMatches(existing[len(existing)-1], input, participants, narrative, evidenceRef) {
			result = append(result, existing[len(existing)-1])
			continue
		}
		revision := nextClaimRevision(existing)
		if narrative != nil && revision > 1 {
			previous := existing[len(existing)-1]
			reference := domain.FragmentRef(previous.ID, previous.ClaimType, previous.ClaimKey, previous.ContentHash, previous.Revision)
			narrative.SupersedesClaim = &reference
		}
		candidate, err := domain.NewProductionWorldClaim(owner.newID(), command.WorkspaceID, command.ProjectID, input.ClaimKey, input.ClaimType, input.Statement, revision, participants, narrative, evidenceRef, command.ActorID, now)
		if err != nil {
			return nil, nil, err
		}
		created = append(created, candidate)
		result = append(result, candidate)
	}
	return result, created, nil
}

func (owner *ProductionWorldBibleOwner) buildProductionWorldBindings(ctx context.Context, repo ProductionWorldBibleRepository, command ApplyProductionWorldBibleCommand, assets map[string]assetdomain.Asset, statesByAsset map[string][]assetdomain.AssetState, specifications []domain.ProductionWorldSpecification, now time.Time) ([]domain.ProductionWorldBinding, []domain.ProductionWorldBinding, error) {
	result, created := make([]domain.ProductionWorldBinding, 0, len(specifications)), []domain.ProductionWorldBinding{}
	for _, specification := range specifications {
		asset := assets[specification.IdentityKey]
		stateValues := statesByAsset[asset.ID]
		states := make([]domain.ProductionWorldStateRef, len(stateValues))
		for index, state := range stateValues {
			states[index] = domain.ProductionWorldStateRef{ID: state.ID, AssetID: state.AssetID, StateKey: state.StateKey, Revision: state.Revision, ContentHash: state.ContentHash}
		}
		existing, err := repo.ListProductionWorldBindings(ctx, command.ProjectID, specification.IdentityKey, true)
		if err != nil {
			return nil, nil, err
		}
		candidate, err := domain.NewProductionWorldBinding(owner.newID(), command.WorkspaceID, command.ProjectID, specification.IdentityKey, nextBindingRevision(existing), domain.FragmentRef(asset.ID, "asset_identity", asset.IdentityKey, asset.ContentHash, asset.Revision), domain.FragmentRef(specification.ID, "specification", specification.SpecificationKey, specification.ContentHash, specification.Revision), states, command.ActorID, now)
		if err != nil {
			return nil, nil, err
		}
		value, reused := reuseBinding(existing, candidate.ContentHash)
		if !reused {
			value, created = candidate, append(created, candidate)
		}
		result = append(result, value)
	}
	return result, created, nil
}

func validateProductionWorldBibleCommand(command ApplyProductionWorldBibleCommand) error {
	for _, id := range []string{command.WorkspaceID, command.ProjectID, command.ActorID, command.ReviewDecisionID} {
		if _, err := uuid.Parse(id); err != nil {
			return errors.New("invalid Production World Bible command identity")
		}
	}
	if command.ExpectedHeadRevision < 0 || (command.ExpectedHeadRevision == 0 && command.ExpectedHeadHash != "") ||
		(command.ExpectedHeadRevision > 0 && !validBibleHash(command.ExpectedHeadHash)) || !validBibleHash(command.ExpectedBusinessKeyRoot) || !validBibleHash(command.PartitionHash) || len(command.Specifications) == 0 || len(command.Assets) != len(command.Specifications) {
		return errors.New("invalid Production World Bible command")
	}
	keys, assetKeys, stateAssets := make([]string, 0, len(command.Specifications)+len(command.Claims)), map[string]string{}, map[string]int{}
	assetsByID := map[string]assetdomain.Asset{}
	for _, asset := range command.Assets {
		if assetdomain.ValidateAsset(asset) != nil || asset.WorkspaceID != command.WorkspaceID || asset.ProjectID != command.ProjectID || assetKeys[asset.IdentityKey] != "" {
			return errors.New("invalid Production World Bible Asset set")
		}
		assetKeys[asset.IdentityKey] = asset.ID
		assetsByID[asset.ID] = asset
	}
	for _, state := range command.States {
		if assetdomain.ValidateAssetState(state) != nil || state.WorkspaceID != command.WorkspaceID || state.ProjectID != command.ProjectID {
			return errors.New("invalid Production World Bible State set")
		}
		if _, exists := assetsByID[state.AssetID]; !exists {
			return errors.New("Production World Bible State has unknown Asset")
		}
		stateAssets[state.AssetID]++
	}
	previous := ""
	for _, item := range command.Specifications {
		assetID := assetKeys[item.IdentityKey]
		if item.IdentityKey <= previous || assetID == "" || assetsByID[assetID].Kind != item.Kind || len(item.Slots) == 0 || !validProductionWorldBasis(item.Basis) {
			return errors.New("Production World Bible Specifications are not canonical")
		}
		keys = append(keys, "specification:"+item.SpecificationKey)
		previous = item.IdentityKey
	}
	previous = ""
	for _, item := range command.Claims {
		if item.ClaimKey <= previous || !validProductionWorldBasis(item.Basis) || len(item.Participants) == 0 {
			return errors.New("Production World Bible Claims are not canonical")
		}
		previousParticipant, subjects, objects := "", 0, 0
		seenIdentities := make(map[string]struct{}, len(item.Participants))
		for _, participant := range item.Participants {
			key := participant.IdentityKey + "\x00" + participant.Role
			if key <= previousParticipant || assetKeys[participant.IdentityKey] == "" || !slices.Contains([]string{"subject", "object", "participant"}, participant.Role) {
				return errors.New("Production World Bible Claim has invalid participants")
			}
			if _, exists := seenIdentities[participant.IdentityKey]; exists {
				return errors.New("Production World Bible Claim repeats a participant identity")
			}
			seenIdentities[participant.IdentityKey] = struct{}{}
			if participant.Role == "subject" {
				subjects++
			}
			if participant.Role == "object" {
				objects++
			}
			previousParticipant = key
		}
		if subjects != 1 || objects > 1 || !validProductionWorldClaimInputNarrative(item) {
			return errors.New("Production World Bible Claim semantics are invalid")
		}
		keys = append(keys, "world_claim:"+item.ClaimKey)
		previous = item.ClaimKey
	}
	for _, assetID := range assetKeys {
		if stateAssets[assetID] == 0 {
			return errors.New("Production World Bible Binding has no State")
		}
	}
	slices.Sort(keys)
	raw, _ := json.Marshal(keys)
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != command.ExpectedBusinessKeyRoot {
		return errors.New("Production World Bible business key coverage has drifted")
	}
	return nil
}

func productionWorldBibleAssets(command ApplyProductionWorldBibleCommand) (map[string]assetdomain.Asset, map[string][]assetdomain.AssetState) {
	assets := map[string]assetdomain.Asset{}
	for _, value := range command.Assets {
		assets[value.IdentityKey] = value
	}
	states := map[string][]assetdomain.AssetState{}
	for _, value := range command.States {
		states[value.AssetID] = append(states[value.AssetID], value)
	}
	return assets, states
}
func validBibleHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validProductionWorldBasis(value agentcontract.ProductionSourceBasis) bool {
	hasEvidence, hasDecision := len(value.Evidence) > 0, value.CreatorDecisionProposal != nil
	if value.Evidence == nil || hasEvidence == hasDecision ||
		(value.Provenance != "source_explicit" && value.Provenance != "inferred" && value.Provenance != "user_supplied") ||
		(value.Provenance == "user_supplied") != hasDecision {
		return false
	}
	if hasDecision {
		return strings.HasPrefix(value.CreatorDecisionProposal.DecisionKey, "decision_") && strings.TrimSpace(value.CreatorDecisionProposal.Rationale) != ""
	}
	for _, evidence := range value.Evidence {
		if evidence.SourceStart < 0 || evidence.SourceEnd <= evidence.SourceStart || evidence.ExactAnchor == "" || !validBibleHash(evidence.TextHash) {
			return false
		}
	}
	return true
}
func nextEvidenceRevision(values []domain.ProductionWorldEvidence) int {
	next := 1
	for _, v := range values {
		if v.Revision >= next {
			next = v.Revision + 1
		}
	}
	return next
}
func nextSpecificationRevision(values []domain.ProductionWorldSpecification) int {
	next := 1
	for _, v := range values {
		if v.Revision >= next {
			next = v.Revision + 1
		}
	}
	return next
}
func nextClaimRevision(values []domain.ProductionWorldClaim) int {
	next := 1
	for _, v := range values {
		if v.Revision >= next {
			next = v.Revision + 1
		}
	}
	return next
}
func nextBindingRevision(values []domain.ProductionWorldBinding) int {
	next := 1
	for _, v := range values {
		if v.Revision >= next {
			next = v.Revision + 1
		}
	}
	return next
}
func reuseEvidence(values []domain.ProductionWorldEvidence, hash string) (domain.ProductionWorldEvidence, bool) {
	for _, v := range values {
		if v.ContentHash == hash {
			return v, true
		}
	}
	return domain.ProductionWorldEvidence{}, false
}
func reuseSpecification(values []domain.ProductionWorldSpecification, hash string) (domain.ProductionWorldSpecification, bool) {
	for _, v := range values {
		if v.ContentHash == hash {
			return v, true
		}
	}
	return domain.ProductionWorldSpecification{}, false
}
func reuseBinding(values []domain.ProductionWorldBinding, hash string) (domain.ProductionWorldBinding, bool) {
	for _, v := range values {
		if v.ContentHash == hash {
			return v, true
		}
	}
	return domain.ProductionWorldBinding{}, false
}

func productionWorldNarrativeClaim(value *agentcontract.ProductionWorldNarrativeClaim) *domain.ProductionWorldNarrativeClaim {
	if value == nil {
		return nil
	}
	anchors := make([]domain.ProductionWorldClaimAnchor, len(value.Anchors))
	for index, anchor := range value.Anchors {
		anchors[index] = domain.ProductionWorldClaimAnchor{Role: anchor.Role, TargetKey: anchor.TargetKey}
	}
	result := &domain.ProductionWorldNarrativeClaim{
		ClaimSeriesKey: value.ClaimSeriesKey,
		Predicate:      value.Predicate,
		Anchors:        anchors,
		ValidScope: domain.ProductionWorldClaimScope{
			Kind: value.ValidScope.Kind, OwnerLogicalID: value.ValidScope.OwnerLogicalID,
		},
		Polarity: value.Polarity,
		Status:   value.Status,
	}
	if value.StoryTimeRange != nil {
		result.StoryTimeRange = &domain.ProductionWorldStoryTimeRange{
			StartKey: value.StoryTimeRange.StartKey, EndKey: value.StoryTimeRange.EndKey,
		}
	}
	return result
}

func productionWorldClaimInputMatches(existing domain.ProductionWorldClaim, input ProductionWorldClaimInput, participants []domain.ProductionWorldClaimParticipant, narrative *domain.ProductionWorldNarrativeClaim, evidence domain.ProductionWorldFragmentRef) bool {
	existingNarrative := existing.Narrative
	if existingNarrative != nil {
		copyValue := *existingNarrative
		copyValue.SupersedesClaim = nil
		existingNarrative = &copyValue
	}
	return existing.ClaimKey == input.ClaimKey && existing.ClaimType == input.ClaimType &&
		existing.Statement == strings.TrimSpace(input.Statement) && reflect.DeepEqual(existing.Participants, participants) &&
		reflect.DeepEqual(existingNarrative, narrative) && existing.Evidence == evidence
}

func validProductionWorldClaimInputNarrative(value ProductionWorldClaimInput) bool {
	narrativeType := slices.Contains([]string{"relationship", "foreshadowing", "payoff"}, value.ClaimType)
	if narrativeType != (value.Narrative != nil) {
		return false
	}
	if !narrativeType {
		return slices.Contains([]string{"world_rule", "story_arc", "plot_thread"}, value.ClaimType)
	}
	return value.Narrative.ClaimSeriesKey == value.ClaimKey
}
func productionWorldEvidenceRefs(values []domain.ProductionWorldEvidence) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for i, v := range values {
		result[i] = domain.FragmentRef(v.ID, "source_evidence", v.SubjectKey, v.ContentHash, v.Revision)
	}
	sortProductionWorldRefs(result)
	return result
}
func productionWorldSpecificationRefs(values []domain.ProductionWorldSpecification) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for i, v := range values {
		result[i] = domain.FragmentRef(v.ID, "specification", v.SpecificationKey, v.ContentHash, v.Revision)
	}
	sortProductionWorldRefs(result)
	return result
}
func productionWorldClaimRefs(values []domain.ProductionWorldClaim) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for i, v := range values {
		result[i] = domain.FragmentRef(v.ID, v.ClaimType, v.ClaimKey, v.ContentHash, v.Revision)
	}
	sortProductionWorldRefs(result)
	return result
}
func productionWorldBindingRefs(values []domain.ProductionWorldBinding) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for i, v := range values {
		result[i] = domain.FragmentRef(v.ID, "production_binding", v.IdentityKey, v.ContentHash, v.Revision)
	}
	sortProductionWorldRefs(result)
	return result
}

func sortProductionWorldRefs(values []domain.ProductionWorldFragmentRef) {
	slices.SortFunc(values, func(left, right domain.ProductionWorldFragmentRef) int {
		return strings.Compare(left.BusinessKey, right.BusinessKey)
	})
}
