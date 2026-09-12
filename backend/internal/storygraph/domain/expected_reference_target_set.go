package domain

import (
	"errors"
	"fmt"
	"slices"
)

// ExpectedReferenceTargetSetInput freezes the exact Production World and P1
// scope used to prove a candidate Reference Plan is complete.
type ExpectedReferenceTargetSetInput struct {
	OwnerSetHash string   `json:"owner_set_hash"`
	P1ScopeKeys  []string `json:"p1_scope_keys"`
	Graph        Snapshot `json:"graph"`
}

// ExpectedReferenceTargetSet is the Backend-owned completeness proof consumed
// by Gate 3. Business keys are canonical JSON strings sorted by UTF-8 bytes.
type ExpectedReferenceTargetSet struct {
	OwnerSetHash               string   `json:"owner_set_hash"`
	P1ScopeKeys                []string `json:"p1_scope_keys"`
	ExpectedTargetBusinessKeys []string `json:"expected_target_business_keys"`
	ExpectedTargetKeyRoot      string   `json:"expected_target_key_root"`
}

type expectedReferenceOccurrence struct {
	node, identity, specification, state, scene Node
	assetKind                                   string
}

type expectedReferenceTarget struct {
	scopeKeys      []string
	occurrenceKeys map[string]struct{}
}

// BuildExpectedReferenceTargetSet mechanically derives the six Reference
// Target kinds from actual P0 occurrences and interactions in P1 scope. The
// candidate plan may choose the Character identity anchor State, but it cannot
// add, remove, or rescope a target.
func BuildExpectedReferenceTargetSet(input ExpectedReferenceTargetSetInput) (ExpectedReferenceTargetSet, error) {
	if !hashPattern.MatchString(input.OwnerSetHash) || validateSortedProductionStrings(input.P1ScopeKeys, 1) != nil {
		return ExpectedReferenceTargetSet{}, errors.New("invalid_expected_reference_target_input")
	}
	canonical, err := Canonicalize(input.Graph)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}

	refIndex, sceneByScope, err := expectedReferenceIndexes(canonical.Nodes, input.P1ScopeKeys)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	bindingTriples, err := productionBindingTriples(canonical.Nodes, refIndex)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	bindings, err := expectedReferenceBindings(canonical.Nodes, refIndex)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	occurrences, err := expectedReferenceOccurrences(canonical.Nodes, refIndex, bindings, sceneByScope)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	actual, err := expectedReferencePlanTargets(canonical.Nodes, refIndex, bindingTriples)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}

	expected := make(map[string]expectedReferenceTarget)
	anchorStateByIdentity, err := expectedCharacterAnchors(occurrences, actual, expected)
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	if err = expectedBaseReferenceTargets(occurrences, anchorStateByIdentity, expected); err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	if err = expectedCompositionReferenceTargets(canonical.Nodes, refIndex, sceneByScope, expected); err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	if err = validateExpectedReferenceTargets(expected, actual); err != nil {
		return ExpectedReferenceTargetSet{}, err
	}

	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return BuildExpectedReferenceTargetSetProof(input.OwnerSetHash, input.P1ScopeKeys, keys)
}

// BuildExpectedReferenceTargetSetProof freezes an independently derived Target
// key set into the same proof consumed by Gate 3. Callers must derive keys from
// Backend-owned production inputs before using this function.
func BuildExpectedReferenceTargetSetProof(
	ownerSetHash string,
	p1ScopeKeys []string,
	expectedTargetBusinessKeys []string,
) (ExpectedReferenceTargetSet, error) {
	if !hashPattern.MatchString(ownerSetHash) || validateSortedProductionStrings(p1ScopeKeys, 1) != nil ||
		len(expectedTargetBusinessKeys) == 0 {
		return ExpectedReferenceTargetSet{}, errors.New("invalid_expected_reference_target_proof")
	}
	for index, key := range expectedTargetBusinessKeys {
		if key == "" || index > 0 && expectedTargetBusinessKeys[index-1] >= key {
			return ExpectedReferenceTargetSet{}, errors.New("invalid_expected_reference_target_proof")
		}
	}
	proof := ExpectedReferenceTargetSet{
		OwnerSetHash: ownerSetHash, P1ScopeKeys: append([]string(nil), p1ScopeKeys...),
		ExpectedTargetBusinessKeys: append([]string(nil), expectedTargetBusinessKeys...),
	}
	root, err := canonicalValueHash(struct {
		OwnerSetHash               string   `json:"owner_set_hash"`
		P1ScopeKeys                []string `json:"p1_scope_keys"`
		ExpectedTargetBusinessKeys []string `json:"expected_target_business_keys"`
	}{proof.OwnerSetHash, proof.P1ScopeKeys, proof.ExpectedTargetBusinessKeys})
	if err != nil {
		return ExpectedReferenceTargetSet{}, err
	}
	proof.ExpectedTargetKeyRoot = root
	return proof, nil
}

func expectedReferenceIndexes(nodes []Node, scopeKeys []string) (map[string][]Node, map[string]Node, error) {
	refIndex := make(map[string][]Node, len(nodes))
	scopeSet := make(map[string]struct{}, len(scopeKeys))
	for _, scope := range scopeKeys {
		scopeSet[scope] = struct{}{}
	}
	scenes := make(map[string]Node, len(scopeKeys))
	for _, node := range nodes {
		ref, err := productionOwnerNodeRefFromOwner(node.OwnerRef)
		if err != nil {
			return nil, nil, err
		}
		key, err := ref.identityKey()
		if err != nil {
			return nil, nil, err
		}
		refIndex[key] = append(refIndex[key], node)
		if node.NodeType == NodeTypeScene {
			if _, included := scopeSet[node.OwnerRef.OwnerLogicalID]; included {
				if _, duplicate := scenes[node.OwnerRef.OwnerLogicalID]; duplicate {
					return nil, nil, errors.New("ambiguous_expected_reference_scope")
				}
				scenes[node.OwnerRef.OwnerLogicalID] = node
			}
		}
	}
	if len(scenes) != len(scopeKeys) {
		return nil, nil, errors.New("ambiguous_expected_reference_scope")
	}
	return refIndex, scenes, nil
}

func expectedReferenceBindings(nodes []Node, refIndex map[string][]Node) (map[string]Node, error) {
	result := make(map[string]Node)
	for _, node := range nodes {
		if node.NodeType != NodeTypeProductionBinding {
			continue
		}
		var payload productionBindingPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil {
			return nil, err
		}
		identity, err := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		if err != nil {
			return nil, errors.New("ambiguous_production_binding")
		}
		specification, err := resolveProductionNodeRef(refIndex, payload.SpecificationRef,
			NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
		if err != nil {
			return nil, errors.New("ambiguous_production_binding")
		}
		for _, stateRef := range payload.StateRefs {
			state, resolveErr := resolveProductionNodeRef(refIndex, stateRef, NodeTypeAssetState)
			if resolveErr != nil {
				return nil, errors.New("ambiguous_production_binding")
			}
			key := identity.StoryNodeKey + "\x00" + state.StoryNodeKey
			if current, exists := result[key]; exists && current.StoryNodeKey != specification.StoryNodeKey {
				return nil, errors.New("ambiguous_production_binding")
			}
			result[key] = specification
		}
	}
	return result, nil
}

func expectedReferenceOccurrences(
	nodes []Node,
	refIndex map[string][]Node,
	bindings map[string]Node,
	sceneByScope map[string]Node,
) ([]expectedReferenceOccurrence, error) {
	result := make([]expectedReferenceOccurrence, 0)
	for _, node := range nodes {
		if node.NodeType != NodeTypeOccurrence {
			continue
		}
		var payload productionOccurrencePayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil {
			return nil, err
		}
		scene, err := resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
		if err != nil {
			return nil, err
		}
		if _, included := sceneByScope[scene.OwnerRef.OwnerLogicalID]; !included {
			continue
		}
		identity, err := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		if err != nil {
			return nil, err
		}
		state, err := resolveProductionNodeRef(refIndex, payload.AssetStateRef, NodeTypeAssetState)
		if err != nil {
			return nil, err
		}
		specification, ok := bindings[identity.StoryNodeKey+"\x00"+state.StoryNodeKey]
		if !ok {
			return nil, errors.New("ambiguous_production_binding")
		}
		var identityPayload productionAuditableAssetPayload
		if err = decodeStrictObject(identity.Payload, &identityPayload); err != nil || !productionAssetKind(identityPayload.AssetKind) {
			return nil, errors.New("ambiguous_production_binding")
		}
		result = append(result, expectedReferenceOccurrence{
			node: node, identity: identity, specification: specification, state: state, scene: scene,
			assetKind: identityPayload.AssetKind,
		})
	}
	return result, nil
}

func expectedReferencePlanTargets(
	nodes []Node,
	refIndex map[string][]Node,
	bindingTriples map[string]struct{},
) (map[string]productionReferenceTargetResolution, error) {
	planCount := 0
	for _, node := range nodes {
		if node.NodeType == NodeTypeApprovedReferencePlanVersion {
			planCount++
		}
	}
	if planCount != 1 {
		return nil, errors.New("ambiguous_active_reference_plan")
	}
	result := make(map[string]productionReferenceTargetResolution)
	for _, node := range nodes {
		if node.NodeType != NodeTypeReferencePlanTarget {
			continue
		}
		resolved, err := resolveProductionReferenceTarget(node, refIndex, bindingTriples)
		if err != nil {
			return nil, err
		}
		if _, duplicate := result[resolved.businessKey]; duplicate {
			return nil, errors.New("duplicate_reference_target_business_key")
		}
		result[resolved.businessKey] = resolved
	}
	return result, nil
}

func expectedCharacterAnchors(
	occurrences []expectedReferenceOccurrence,
	actual map[string]productionReferenceTargetResolution,
	expected map[string]expectedReferenceTarget,
) (map[string]string, error) {
	byIdentity := make(map[string][]expectedReferenceOccurrence)
	for _, occurrence := range occurrences {
		if occurrence.assetKind == "character" {
			byIdentity[occurrence.identity.StoryNodeKey] = append(byIdentity[occurrence.identity.StoryNodeKey], occurrence)
		}
	}
	anchorStates := make(map[string]string, len(byIdentity))
	for identityKey, values := range byIdentity {
		key, err := expectedReferenceBusinessKey("character_identity_anchor", values[0].identity)
		if err != nil {
			return nil, err
		}
		target, exists := actual[key]
		if !exists {
			return nil, errors.New("missing_expected_reference_target")
		}
		anchorState := target.ownerNodes.state[0].StoryNodeKey
		anchorStates[identityKey] = anchorState
		expected[key] = expectedReferenceTarget{
			scopeKeys:      expectedReferenceScopes(values, ""),
			occurrenceKeys: expectedReferenceOccurrenceKeys(values, anchorState),
		}
	}
	return anchorStates, nil
}

func expectedBaseReferenceTargets(
	occurrences []expectedReferenceOccurrence,
	anchorStates map[string]string,
	expected map[string]expectedReferenceTarget,
) error {
	groups := make(map[string][]expectedReferenceOccurrence)
	for _, occurrence := range occurrences {
		kind := ""
		switch occurrence.assetKind {
		case "character":
			if anchorStates[occurrence.identity.StoryNodeKey] == occurrence.state.StoryNodeKey {
				continue
			}
			kind = "character_appearance"
		case "location":
			kind = "location_board"
		case "prop":
			kind = "prop_sheet"
		}
		key, err := expectedReferenceBusinessKey(kind, occurrence.identity, occurrence.specification, occurrence.state)
		if err != nil {
			return err
		}
		groups[key] = append(groups[key], occurrence)
	}
	for key, values := range groups {
		expected[key] = expectedReferenceTarget{
			scopeKeys: expectedReferenceScopes(values, ""), occurrenceKeys: expectedReferenceOccurrenceKeys(values, ""),
		}
	}
	return nil
}

func expectedCompositionReferenceTargets(
	nodes []Node,
	refIndex map[string][]Node,
	sceneByScope map[string]Node,
	expected map[string]expectedReferenceTarget,
) error {
	for scope, scene := range sceneByScope {
		key, err := expectedReferenceBusinessKey("scene_composition", scene)
		if err != nil {
			return err
		}
		expected[key] = expectedReferenceTarget{scopeKeys: []string{scope}}
	}
	for _, node := range nodes {
		if node.NodeType != NodeTypeContinuityClaim {
			continue
		}
		var payload productionInteractionPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil || payload.ClaimType != "interaction" {
			continue
		}
		scene, err := resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
		if err != nil {
			return err
		}
		if _, included := sceneByScope[scene.OwnerRef.OwnerLogicalID]; !included {
			continue
		}
		key, err := expectedReferenceBusinessKey("interaction_composition", node)
		if err != nil {
			return err
		}
		expected[key] = expectedReferenceTarget{scopeKeys: []string{scene.OwnerRef.OwnerLogicalID}}
	}
	return nil
}

func validateExpectedReferenceTargets(
	expected map[string]expectedReferenceTarget,
	actual map[string]productionReferenceTargetResolution,
) error {
	for key, proof := range expected {
		target, exists := actual[key]
		if !exists {
			return errors.New("missing_expected_reference_target")
		}
		if !slices.Equal(target.payload.CoverageScopeKeys, proof.scopeKeys) {
			return fmt.Errorf("reference_target_input_mismatch: %s", target.node.StoryNodeKey)
		}
		if proof.occurrenceKeys != nil && !productionNodeSetEquals(proof.occurrenceKeys, target.ownerNodes.occurrence) {
			return fmt.Errorf("reference_target_input_mismatch: %s", target.node.StoryNodeKey)
		}
	}
	for key := range actual {
		if _, exists := expected[key]; !exists {
			return errors.New("unexpected_reference_target")
		}
	}
	return nil
}

func expectedReferenceBusinessKey(kind string, nodes ...Node) (string, error) {
	refs := make([]productionOwnerNodeRef, len(nodes))
	for index, node := range nodes {
		ref, err := productionOwnerNodeRefFromOwner(node.OwnerRef)
		if err != nil {
			return "", err
		}
		refs[index] = ref
	}
	payload := productionReferenceTargetPayload{TargetKind: kind}
	switch kind {
	case "character_identity_anchor":
		payload.TargetOwnerRefs.Identity = refs
	case "character_appearance", "location_board", "prop_sheet":
		payload.TargetOwnerRefs.Identity = refs[:1]
		payload.TargetOwnerRefs.Specification = refs[1:2]
		payload.TargetOwnerRefs.State = refs[2:3]
	case "scene_composition":
		payload.TargetOwnerRefs.Scene = refs
	case "interaction_composition":
		payload.TargetOwnerRefs.Interaction = refs
	default:
		return "", errors.New("invalid Reference Target kind")
	}
	return productionReferenceTargetBusinessKey(payload)
}

func expectedReferenceScopes(values []expectedReferenceOccurrence, stateKey string) []string {
	set := make(map[string]struct{})
	for _, value := range values {
		if stateKey == "" || value.state.StoryNodeKey == stateKey {
			set[value.scene.OwnerRef.OwnerLogicalID] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}

func expectedReferenceOccurrenceKeys(values []expectedReferenceOccurrence, stateKey string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range values {
		if stateKey == "" || value.state.StoryNodeKey == stateKey {
			result[value.node.StoryNodeKey] = struct{}{}
		}
	}
	return result
}
