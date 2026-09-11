package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type productionReferenceTargetOwnerRefs struct {
	Identity      []productionOwnerNodeRef `json:"identity"`
	Specification []productionOwnerNodeRef `json:"specification"`
	State         []productionOwnerNodeRef `json:"state"`
	Style         []productionOwnerNodeRef `json:"style"`
	Scene         []productionOwnerNodeRef `json:"scene"`
	Occurrence    []productionOwnerNodeRef `json:"occurrence"`
	Interaction   []productionOwnerNodeRef `json:"interaction"`
}

type productionReferenceTargetPayload struct {
	productionProjectionPayload
	TargetKind          string                             `json:"target_kind"`
	Fulfillment         string                             `json:"fulfillment"`
	TargetOwnerRefs     productionReferenceTargetOwnerRefs `json:"target_owner_refs"`
	CoverageScopeKeys   []string                           `json:"coverage_scope_keys"`
	DependsOnTargetRefs []productionOwnerNodeRef           `json:"depends_on_target_refs"`
	Constraints         productionConstraintRefs           `json:"constraints"`
}

type productionReferenceTargetResolution struct {
	node          Node
	payload       productionReferenceTargetPayload
	planKey       string
	businessKey   string
	ownerNodes    productionReferenceTargetOwnerNodes
	constraintSet resolvedProductionConstraints
}

type productionReferenceTargetOwnerNodes struct {
	identity      []Node
	specification []Node
	state         []Node
	style         []Node
	scene         []Node
	occurrence    []Node
	interaction   []Node
}

func validateProductionReferenceTargetRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
	nodeByKey := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		ref, err := productionOwnerNodeRefFromOwner(node.OwnerRef)
		if err != nil {
			return err
		}
		key, err := ref.identityKey()
		if err != nil {
			return err
		}
		refIndex[key] = append(refIndex[key], node)
		nodeByKey[node.StoryNodeKey] = node
	}
	bindingTriples, err := productionBindingTriples(nodes, refIndex)
	if err != nil {
		return err
	}

	resolutions := make(map[string]productionReferenceTargetResolution)
	businessKeys := make(map[string]struct{})
	expected := make(map[productionRelationEdge]struct{})
	for _, node := range nodes {
		if node.NodeType != NodeTypeReferencePlanTarget {
			continue
		}
		resolution, resolveErr := resolveProductionReferenceTarget(node, refIndex, bindingTriples)
		if resolveErr != nil {
			return resolveErr
		}
		uniqueKey := resolution.planKey + "\x00" + resolution.businessKey
		if _, exists := businessKeys[uniqueKey]; exists {
			return errors.New("duplicate_reference_target_business_key")
		}
		businessKeys[uniqueKey] = struct{}{}
		resolutions[node.StoryNodeKey] = resolution
		addProductionReferenceTargetInputEdges(expected, resolution)
	}
	if err := validateProductionReferenceTargetParents(edges, nodeByKey, resolutions); err != nil {
		return err
	}
	for _, resolution := range resolutions {
		if err := validateProductionReferenceTargetDependencies(resolution, resolutions, refIndex, expected); err != nil {
			return err
		}
	}
	return validateProductionReferenceTargetEdges(edges, nodeByKey, expected)
}

func resolveProductionReferenceTarget(
	node Node,
	refIndex map[string][]Node,
	bindingTriples map[string]struct{},
) (productionReferenceTargetResolution, error) {
	var payload productionReferenceTargetPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil || !productionReferenceTargetKind(payload.TargetKind) ||
		!oneOf(payload.Fulfillment, "required", "optional", "not_generated") ||
		validateSortedProductionStrings(payload.CoverageScopeKeys, 1) != nil ||
		payload.DependsOnTargetRefs == nil || validateSortedProductionRefs(payload.DependsOnTargetRefs) != nil {
		return productionReferenceTargetResolution{}, fmt.Errorf("Production StoryGraph Reference Target %s has an invalid payload", node.StoryNodeKey)
	}
	ownerNodes, err := resolveProductionReferenceTargetInputs(payload.TargetOwnerRefs, refIndex)
	if err != nil {
		return productionReferenceTargetResolution{}, fmt.Errorf("Production StoryGraph Reference Target %s has invalid owner refs", node.StoryNodeKey)
	}
	constraints, err := resolveProductionConstraints(payload.Constraints, refIndex)
	if err != nil || len(ownerNodes.style) != 1 || ownerNodes.style[0].StoryNodeKey != constraints.style.StoryNodeKey {
		return productionReferenceTargetResolution{}, fmt.Errorf("Production StoryGraph Reference Target %s has invalid constraints", node.StoryNodeKey)
	}
	if err = validateProductionReferenceTargetInputMatrix(payload, ownerNodes, bindingTriples); err != nil {
		return productionReferenceTargetResolution{}, fmt.Errorf("reference_target_input_mismatch: %s", node.StoryNodeKey)
	}
	if !slices.Equal(payload.CoverageScopeKeys, productionReferenceTargetSceneScopeKeys(ownerNodes.scene)) {
		return productionReferenceTargetResolution{}, fmt.Errorf("reference_target_input_mismatch: %s", node.StoryNodeKey)
	}

	targetRef, _ := productionOwnerNodeRefFromOwner(node.OwnerRef)
	targetRef.FragmentKey, targetRef.FragmentContentHash = nil, nil
	plan, err := resolveProductionNodeRef(refIndex, targetRef, NodeTypeApprovedReferencePlanVersion)
	if err != nil {
		return productionReferenceTargetResolution{}, fmt.Errorf("Production StoryGraph Reference Target %s does not belong to one Plan", node.StoryNodeKey)
	}
	businessKey, err := productionReferenceTargetBusinessKey(payload)
	if err != nil {
		return productionReferenceTargetResolution{}, err
	}
	return productionReferenceTargetResolution{
		node: node, payload: payload, planKey: plan.StoryNodeKey, businessKey: businessKey,
		ownerNodes: ownerNodes, constraintSet: constraints,
	}, nil
}

func resolveProductionReferenceTargetInputs(value productionReferenceTargetOwnerRefs, refIndex map[string][]Node) (productionReferenceTargetOwnerNodes, error) {
	if value.Identity == nil || value.Specification == nil || value.State == nil || value.Style == nil || value.Scene == nil || value.Occurrence == nil || value.Interaction == nil {
		return productionReferenceTargetOwnerNodes{}, errors.New("Reference Target owner ref arrays are required")
	}
	groups := []struct {
		refs    []productionOwnerNodeRef
		allowed []NodeType
		target  *[]Node
	}{
		{value.Identity, []NodeType{NodeTypeAssetIdentity}, nil},
		{value.Specification, []NodeType{NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification}, nil},
		{value.State, []NodeType{NodeTypeAssetState}, nil},
		{value.Style, []NodeType{NodeTypeEffectiveStyleSnapshot}, nil},
		{value.Scene, []NodeType{NodeTypeScene}, nil},
		{value.Occurrence, []NodeType{NodeTypeOccurrence}, nil},
		{value.Interaction, []NodeType{NodeTypeContinuityClaim}, nil},
	}
	result := productionReferenceTargetOwnerNodes{}
	groups[0].target, groups[1].target, groups[2].target = &result.identity, &result.specification, &result.state
	groups[3].target, groups[4].target, groups[5].target, groups[6].target = &result.style, &result.scene, &result.occurrence, &result.interaction
	for _, group := range groups {
		if err := validateSortedProductionRefs(group.refs); err != nil {
			return productionReferenceTargetOwnerNodes{}, err
		}
		for _, ref := range group.refs {
			node, err := resolveProductionNodeRef(refIndex, ref, group.allowed...)
			if err != nil {
				return productionReferenceTargetOwnerNodes{}, err
			}
			*group.target = append(*group.target, node)
		}
	}
	for _, interaction := range result.interaction {
		var branch struct {
			ClaimType string `json:"claim_type"`
		}
		if err := json.Unmarshal(interaction.Payload, &branch); err != nil || branch.ClaimType != "interaction" {
			return productionReferenceTargetOwnerNodes{}, errors.New("Reference Target interaction ref is not an Interaction Claim")
		}
	}
	return result, nil
}

func validateProductionReferenceTargetInputMatrix(
	payload productionReferenceTargetPayload,
	owners productionReferenceTargetOwnerNodes,
	bindingTriples map[string]struct{},
) error {
	counts := []int{len(owners.identity), len(owners.specification), len(owners.state), len(owners.style), len(owners.scene), len(owners.occurrence), len(owners.interaction)}
	if len(owners.style) != 1 {
		return errors.New("Reference Target requires one style")
	}
	switch payload.TargetKind {
	case "character_identity_anchor", "character_appearance", "location_board", "prop_sheet":
		if counts[0] != 1 || counts[1] != 1 || counts[2] != 1 || counts[4] < 1 || counts[6] != 0 ||
			owners.specification[0].NodeType != productionAssetVersionSpecification(payload.TargetKind) ||
			!productionBindingTripleExists(bindingTriples, owners.identity[0].StoryNodeKey, owners.specification[0].StoryNodeKey, owners.state[0].StoryNodeKey) {
			return errors.New("invalid base Reference Target inputs")
		}
		if err := validateProductionTargetOccurrences(owners); err != nil {
			return err
		}
	case "scene_composition":
		if counts[0] < 1 || counts[1] < 1 || counts[2] < 1 || counts[4] != 1 || !productionTargetBindingsCovered(owners, bindingTriples) {
			return errors.New("invalid Scene Composition inputs")
		}
		if err := validateProductionTargetOccurrences(owners); err != nil || validateProductionTargetInteractionScenes(owners) != nil {
			return errors.New("invalid Scene Composition closure")
		}
	case "interaction_composition":
		if counts[0] < 2 || counts[1] < 2 || counts[2] < 2 || counts[4] != 1 || counts[5] < 2 || counts[6] != 1 ||
			!productionTargetBindingsCovered(owners, bindingTriples) || validateProductionTargetOccurrences(owners) != nil || validateProductionTargetInteractionScenes(owners) != nil {
			return errors.New("invalid Interaction Composition closure")
		}
	default:
		return errors.New("invalid Reference Target kind")
	}
	return nil
}

func validateProductionTargetOccurrences(owners productionReferenceTargetOwnerNodes) error {
	identityKeys, stateKeys, sceneKeys := productionNodeKeySet(owners.identity), productionNodeKeySet(owners.state), productionNodeKeySet(owners.scene)
	for _, occurrence := range owners.occurrence {
		var payload productionOccurrencePayload
		if err := decodeStrictObject(occurrence.Payload, &payload); err != nil {
			return err
		}
		asset, assetErr := productionRefNodeKey(payload.AssetIdentityRef)
		state, stateErr := productionRefNodeKey(payload.AssetStateRef)
		scene, sceneErr := productionRefNodeKey(payload.SceneRef)
		if assetErr != nil || stateErr != nil || sceneErr != nil || !sceneKeys[scene] || !identityKeys[asset] || !stateKeys[state] {
			return errors.New("Reference Target occurrence is outside its inputs")
		}
	}
	return nil
}

func validateProductionTargetInteractionScenes(owners productionReferenceTargetOwnerNodes) error {
	scenes := productionNodeKeySet(owners.scene)
	for _, interaction := range owners.interaction {
		var payload productionInteractionPayload
		if err := decodeStrictObject(interaction.Payload, &payload); err != nil {
			return err
		}
		scene, err := productionRefNodeKey(payload.SceneRef)
		if err != nil || !scenes[scene] {
			return errors.New("Reference Target interaction is outside its Scene")
		}
	}
	return nil
}

func productionTargetBindingsCovered(owners productionReferenceTargetOwnerNodes, triples map[string]struct{}) bool {
	for _, identity := range owners.identity {
		covered := false
		for _, specification := range owners.specification {
			for _, state := range owners.state {
				if productionBindingTripleExists(triples, identity.StoryNodeKey, specification.StoryNodeKey, state.StoryNodeKey) {
					covered = true
				}
			}
		}
		if !covered {
			return false
		}
	}
	for _, specification := range owners.specification {
		covered := false
		for _, identity := range owners.identity {
			for _, state := range owners.state {
				if productionBindingTripleExists(triples, identity.StoryNodeKey, specification.StoryNodeKey, state.StoryNodeKey) {
					covered = true
				}
			}
		}
		if !covered {
			return false
		}
	}
	for _, state := range owners.state {
		covered := false
		for _, identity := range owners.identity {
			for _, specification := range owners.specification {
				if productionBindingTripleExists(triples, identity.StoryNodeKey, specification.StoryNodeKey, state.StoryNodeKey) {
					covered = true
				}
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func validateProductionReferenceTargetParents(edges []Edge, nodeByKey map[string]Node, targets map[string]productionReferenceTargetResolution) error {
	parentCount := make(map[string]int)
	sequenceKeys := make(map[string]struct{})
	for _, edge := range edges {
		if edge.EdgeType != EdgeTypeContainsReferenceTarget {
			continue
		}
		target, ok := targets[edge.ToNodeKey]
		parent, parentOK := nodeByKey[edge.FromNodeKey]
		if !ok || !parentOK || parent.NodeType != NodeTypeApprovedReferencePlanVersion || edge.FromNodeKey != target.planKey {
			return errors.New("Production StoryGraph Reference Target has an invalid Plan parent")
		}
		parentCount[edge.ToNodeKey]++
		sequenceIdentity := edge.FromNodeKey + "\x00" + edge.Qualifier.SequenceKey
		if _, exists := sequenceKeys[sequenceIdentity]; exists {
			return errors.New("Production StoryGraph Reference Plan has duplicate Target order keys")
		}
		sequenceKeys[sequenceIdentity] = struct{}{}
	}
	for key := range targets {
		if parentCount[key] != 1 {
			return errors.New("Production StoryGraph Reference Target does not have exactly one Plan parent")
		}
	}
	return nil
}

func validateProductionReferenceTargetDependencies(
	target productionReferenceTargetResolution,
	targets map[string]productionReferenceTargetResolution,
	refIndex map[string][]Node,
	expected map[productionRelationEdge]struct{},
) error {
	dependencies := make([]productionReferenceTargetResolution, 0, len(target.payload.DependsOnTargetRefs))
	for _, ref := range target.payload.DependsOnTargetRefs {
		node, err := resolveProductionNodeRef(refIndex, ref, NodeTypeReferencePlanTarget)
		dependency, ok := targets[node.StoryNodeKey]
		if err != nil || !ok || node.StoryNodeKey == target.node.StoryNodeKey || dependency.planKey != target.planKey ||
			productionFulfillmentRank(dependency.payload.Fulfillment) < productionFulfillmentRank(target.payload.Fulfillment) {
			return errors.New("reference_target_input_mismatch")
		}
		dependencies = append(dependencies, dependency)
		expected[productionRelationEdge{edgeType: EdgeTypeDependsOnReferenceTarget, from: node.StoryNodeKey, to: target.node.StoryNodeKey}] = struct{}{}
	}
	switch target.payload.TargetKind {
	case "character_identity_anchor", "location_board", "prop_sheet":
		if len(dependencies) != 0 {
			return errors.New("reference_target_input_mismatch")
		}
	case "character_appearance":
		if len(dependencies) != 1 || dependencies[0].payload.TargetKind != "character_identity_anchor" ||
			!productionSingleRefEqual(dependencies[0].payload.TargetOwnerRefs.Identity, target.payload.TargetOwnerRefs.Identity[0]) ||
			!productionSingleRefEqual(dependencies[0].payload.TargetOwnerRefs.Specification, target.payload.TargetOwnerRefs.Specification[0]) ||
			productionSingleRefEqual(dependencies[0].payload.TargetOwnerRefs.State, target.payload.TargetOwnerRefs.State[0]) ||
			!productionConstraintsEqual(dependencies[0].payload.Constraints, target.payload.Constraints) ||
			!productionStringSuperset(dependencies[0].payload.CoverageScopeKeys, target.payload.CoverageScopeKeys) {
			return errors.New("reference_target_input_mismatch")
		}
	case "scene_composition", "interaction_composition":
		if (target.payload.Fulfillment == "not_generated" && len(dependencies) != 0) ||
			(target.payload.Fulfillment != "not_generated" && len(dependencies) == 0) {
			return errors.New("reference_target_input_mismatch")
		}
		for _, dependency := range dependencies {
			if !productionBaseReferenceTargetKind(dependency.payload.TargetKind) {
				return errors.New("reference_target_input_mismatch")
			}
		}
	}
	return nil
}

func addProductionReferenceTargetInputEdges(expected map[productionRelationEdge]struct{}, value productionReferenceTargetResolution) {
	to := value.node.StoryNodeKey
	groups := []struct {
		nodes []Node
		role  string
	}{
		{value.ownerNodes.identity, "identity"}, {value.ownerNodes.specification, "specification"},
		{value.ownerNodes.state, "state"}, {value.ownerNodes.style, "style"}, {value.ownerNodes.scene, "scene"},
		{value.ownerNodes.occurrence, "occurrence"}, {value.ownerNodes.interaction, "interaction"},
	}
	for _, group := range groups {
		for _, node := range group.nodes {
			expected[productionRelationEdge{edgeType: EdgeTypePlansReference, from: node.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: group.role}}] = struct{}{}
		}
	}
	for _, world := range value.constraintSet.world {
		expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: world.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "world"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraintSet.policy.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "policy"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraintSet.style.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "style"}}] = struct{}{}
}

func validateProductionReferenceTargetEdges(edges []Edge, nodeByKey map[string]Node, expected map[productionRelationEdge]struct{}) error {
	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeReferencePlanTarget || !oneOfEdge(edge.EdgeType, EdgeTypePlansReference, EdgeTypeConstrains, EdgeTypeDependsOnReferenceTarget) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s Reference Target edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Reference Target edges do not match payloads")
	}
	return nil
}

func productionReferenceTargetBusinessKey(payload productionReferenceTargetPayload) (string, error) {
	var refs []productionOwnerNodeRef
	switch payload.TargetKind {
	case "character_identity_anchor":
		refs = payload.TargetOwnerRefs.Identity
	case "character_appearance", "location_board", "prop_sheet":
		refs = append(refs, payload.TargetOwnerRefs.Identity[0], payload.TargetOwnerRefs.Specification[0], payload.TargetOwnerRefs.State[0])
	case "scene_composition":
		refs = payload.TargetOwnerRefs.Scene
	case "interaction_composition":
		refs = payload.TargetOwnerRefs.Interaction
	default:
		return "", errors.New("invalid Reference Target kind")
	}
	value := make([]any, 1, len(refs)+1)
	value[0] = payload.TargetKind
	for _, ref := range refs {
		fragment := ""
		if ref.FragmentKey != nil {
			fragment = *ref.FragmentKey
		}
		value = append(value, []string{ref.OwnerKind, ref.VersionFamily, ref.OwnerLogicalID, fragment})
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	canonical, err := platformcanonical.JSON(raw)
	return string(canonical), err
}

func productionRefNodeKey(ref productionOwnerNodeRef) (string, error) {
	key, err := ref.identityKey()
	if err != nil {
		return "", err
	}
	return key, nil
}

func productionNodeKeySet(nodes []Node) map[string]bool {
	result := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		ref, err := productionOwnerNodeRefFromOwner(node.OwnerRef)
		if err != nil {
			continue
		}
		key, err := ref.identityKey()
		if err == nil {
			result[key] = true
		}
	}
	return result
}

func validateSortedProductionStrings(values []string, minimum int) error {
	if values == nil || len(values) < minimum {
		return errors.New("Production StoryGraph string array is missing required values")
	}
	for index, value := range values {
		if !productionStableKey(value) || index > 0 && strings.Compare(values[index-1], value) >= 0 {
			return errors.New("Production StoryGraph string array is not sorted and unique")
		}
	}
	return nil
}

func productionReferenceTargetSceneScopeKeys(scenes []Node) []string {
	result := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		result = append(result, scene.OwnerRef.OwnerLogicalID)
	}
	slices.Sort(result)
	return result
}

func productionReferenceTargetKind(value string) bool {
	return productionBaseReferenceTargetKind(value) || value == "scene_composition" || value == "interaction_composition"
}

func productionBaseReferenceTargetKind(value string) bool {
	return oneOf(value, "character_appearance", "character_identity_anchor", "location_board", "prop_sheet")
}

func productionFulfillmentRank(value string) int {
	return slices.Index([]string{"not_generated", "optional", "required"}, value)
}

func productionStringSuperset(superset, subset []string) bool {
	values := make(map[string]struct{}, len(superset))
	for _, value := range superset {
		values[value] = struct{}{}
	}
	for _, value := range subset {
		if _, ok := values[value]; !ok {
			return false
		}
	}
	return true
}
