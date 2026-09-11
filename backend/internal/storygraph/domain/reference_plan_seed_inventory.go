package domain

import (
	"errors"
	"slices"
	"sort"
	"strings"
)

// ReferencePlanSeedInventoryInput freezes the exact P0 graph and P1 scope from
// which Backend derives the only planning facts an Agent may consume.
type ReferencePlanSeedInventoryInput struct {
	OwnerSetHash string   `json:"owner_set_hash"`
	P1ScopeKeys  []string `json:"p1_scope_keys"`
	Graph        Snapshot `json:"graph"`
}

type ReferencePlanCharacterStateSeed struct {
	StateRef              OwnerRef   `json:"state_ref"`
	AppearanceBusinessKey string     `json:"appearance_business_key"`
	CoverageScopeKeys     []string   `json:"coverage_scope_keys"`
	OccurrenceRefs        []OwnerRef `json:"occurrence_refs"`
}

type ReferencePlanCharacterSeed struct {
	AnchorBusinessKey string                            `json:"anchor_business_key"`
	IdentityRef       OwnerRef                          `json:"identity_ref"`
	SpecificationRef  OwnerRef                          `json:"specification_ref"`
	CoverageScopeKeys []string                          `json:"coverage_scope_keys"`
	StateOptions      []ReferencePlanCharacterStateSeed `json:"state_options"`
}

type ReferencePlanTargetOwnerRefs struct {
	Identity      []OwnerRef `json:"identity"`
	Specification []OwnerRef `json:"specification"`
	State         []OwnerRef `json:"state"`
	Scene         []OwnerRef `json:"scene"`
	Occurrence    []OwnerRef `json:"occurrence"`
	Interaction   []OwnerRef `json:"interaction"`
}

type ReferencePlanCharacterDependencySeed struct {
	AnchorBusinessKey     string   `json:"anchor_business_key"`
	StateRef              OwnerRef `json:"state_ref"`
	AppearanceBusinessKey string   `json:"appearance_business_key"`
}

type ReferencePlanFixedTargetSeed struct {
	TargetBusinessKey           string                                 `json:"target_business_key"`
	TargetKind                  string                                 `json:"target_kind"`
	OwnerRefs                   ReferencePlanTargetOwnerRefs           `json:"owner_refs"`
	CoverageScopeKeys           []string                               `json:"coverage_scope_keys"`
	FixedDependencyBusinessKeys []string                               `json:"fixed_dependency_business_keys"`
	CharacterDependencies       []ReferencePlanCharacterDependencySeed `json:"character_dependencies"`
}

type ReferencePlanSeedInventory struct {
	OwnerSetHash     string                         `json:"owner_set_hash"`
	P1ScopeKeys      []string                       `json:"p1_scope_keys"`
	CharacterSeeds   []ReferencePlanCharacterSeed   `json:"character_seeds"`
	FixedTargetSeeds []ReferencePlanFixedTargetSeed `json:"fixed_target_seeds"`
}

type referencePlanCharacterDependency struct {
	anchorKey     string
	state         Node
	appearanceKey string
}

// BuildReferencePlanSeedInventory mechanically derives a plan-free seed
// inventory. Approved Plan, Target, Provider, and media facts are never read.
func BuildReferencePlanSeedInventory(input ReferencePlanSeedInventoryInput) (ReferencePlanSeedInventory, error) {
	if !hashPattern.MatchString(input.OwnerSetHash) || validateSortedProductionStrings(input.P1ScopeKeys, 1) != nil {
		return ReferencePlanSeedInventory{}, errors.New("invalid_reference_plan_seed_input")
	}
	canonical, err := Canonicalize(input.Graph)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	refIndex, sceneByScope, err := expectedReferenceIndexes(canonical.Nodes, input.P1ScopeKeys)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	bindingTriples, err := productionBindingTriples(canonical.Nodes, refIndex)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	bindings, err := expectedReferenceBindings(canonical.Nodes, refIndex)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	occurrences, err := expectedReferenceOccurrences(canonical.Nodes, refIndex, bindings, sceneByScope)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}

	characters, characterDependencies, baseTargets, baseKeys, err := buildReferencePlanBaseSeeds(occurrences)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	nodeByKey := make(map[string]Node, len(canonical.Nodes))
	for _, node := range canonical.Nodes {
		nodeByKey[node.StoryNodeKey] = node
	}
	fixed := append([]ReferencePlanFixedTargetSeed(nil), baseTargets...)
	composition, err := buildReferencePlanCompositionSeeds(
		canonical.Nodes, refIndex, bindingTriples, sceneByScope, nodeByKey, characterDependencies, baseKeys,
	)
	if err != nil {
		return ReferencePlanSeedInventory{}, err
	}
	fixed = append(fixed, composition...)
	slices.SortFunc(fixed, func(left, right ReferencePlanFixedTargetSeed) int {
		return strings.Compare(left.TargetBusinessKey, right.TargetBusinessKey)
	})
	return ReferencePlanSeedInventory{
		OwnerSetHash: input.OwnerSetHash, P1ScopeKeys: append([]string(nil), input.P1ScopeKeys...),
		CharacterSeeds: characters, FixedTargetSeeds: fixed,
	}, nil
}

func buildReferencePlanBaseSeeds(occurrences []expectedReferenceOccurrence) (
	[]ReferencePlanCharacterSeed,
	map[string]referencePlanCharacterDependency,
	[]ReferencePlanFixedTargetSeed,
	map[string]string,
	error,
) {
	characterGroups := make(map[string][]expectedReferenceOccurrence)
	baseGroups := make(map[string][]expectedReferenceOccurrence)
	baseKinds := make(map[string]string)
	for _, occurrence := range occurrences {
		triple := productionBindingTripleKey(
			occurrence.identity.StoryNodeKey, occurrence.specification.StoryNodeKey, occurrence.state.StoryNodeKey,
		)
		if occurrence.assetKind == "character" {
			characterGroups[occurrence.identity.StoryNodeKey] = append(characterGroups[occurrence.identity.StoryNodeKey], occurrence)
			continue
		}
		kind := map[string]string{"location": "location_board", "prop": "prop_sheet"}[occurrence.assetKind]
		if kind == "" {
			return nil, nil, nil, nil, errors.New("invalid_reference_plan_asset_kind")
		}
		baseGroups[triple] = append(baseGroups[triple], occurrence)
		baseKinds[triple] = kind
	}

	characters := make([]ReferencePlanCharacterSeed, 0, len(characterGroups))
	characterDependencies := make(map[string]referencePlanCharacterDependency)
	for _, values := range characterGroups {
		anchorKey, err := expectedReferenceBusinessKey("character_identity_anchor", values[0].identity)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		stateGroups := make(map[string][]expectedReferenceOccurrence)
		for _, occurrence := range values {
			stateGroups[occurrence.state.StoryNodeKey] = append(stateGroups[occurrence.state.StoryNodeKey], occurrence)
		}
		options := make([]ReferencePlanCharacterStateSeed, 0, len(stateGroups))
		for _, stateValues := range stateGroups {
			first := stateValues[0]
			appearanceKey, keyErr := expectedReferenceBusinessKey(
				"character_appearance", first.identity, first.specification, first.state,
			)
			if keyErr != nil {
				return nil, nil, nil, nil, keyErr
			}
			options = append(options, ReferencePlanCharacterStateSeed{
				StateRef: first.state.OwnerRef, AppearanceBusinessKey: appearanceKey,
				CoverageScopeKeys: expectedReferenceScopes(stateValues, ""),
				OccurrenceRefs:    referencePlanOccurrenceRefs(stateValues),
			})
			triple := productionBindingTripleKey(
				first.identity.StoryNodeKey, first.specification.StoryNodeKey, first.state.StoryNodeKey,
			)
			characterDependencies[triple] = referencePlanCharacterDependency{
				anchorKey: anchorKey, state: first.state, appearanceKey: appearanceKey,
			}
		}
		slices.SortFunc(options, func(left, right ReferencePlanCharacterStateSeed) int {
			return strings.Compare(referencePlanOwnerRefSortKey(left.StateRef), referencePlanOwnerRefSortKey(right.StateRef))
		})
		characters = append(characters, ReferencePlanCharacterSeed{
			AnchorBusinessKey: anchorKey, IdentityRef: values[0].identity.OwnerRef,
			SpecificationRef: values[0].specification.OwnerRef, CoverageScopeKeys: expectedReferenceScopes(values, ""),
			StateOptions: options,
		})
	}
	slices.SortFunc(characters, func(left, right ReferencePlanCharacterSeed) int {
		return strings.Compare(left.AnchorBusinessKey, right.AnchorBusinessKey)
	})

	baseTargets := make([]ReferencePlanFixedTargetSeed, 0, len(baseGroups))
	baseKeys := make(map[string]string, len(baseGroups))
	for triple, values := range baseGroups {
		first := values[0]
		key, err := expectedReferenceBusinessKey(baseKinds[triple], first.identity, first.specification, first.state)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		baseKeys[triple] = key
		baseTargets = append(baseTargets, ReferencePlanFixedTargetSeed{
			TargetBusinessKey: key, TargetKind: baseKinds[triple],
			OwnerRefs: ReferencePlanTargetOwnerRefs{
				Identity: []OwnerRef{first.identity.OwnerRef}, Specification: []OwnerRef{first.specification.OwnerRef},
				State: []OwnerRef{first.state.OwnerRef}, Scene: referencePlanSceneRefs(values),
				Occurrence: referencePlanOccurrenceRefs(values), Interaction: []OwnerRef{},
			},
			CoverageScopeKeys:           expectedReferenceScopes(values, ""),
			FixedDependencyBusinessKeys: []string{}, CharacterDependencies: []ReferencePlanCharacterDependencySeed{},
		})
	}
	return characters, characterDependencies, baseTargets, baseKeys, nil
}

func buildReferencePlanCompositionSeeds(
	nodes []Node,
	refIndex map[string][]Node,
	bindingTriples map[string]struct{},
	sceneByScope map[string]Node,
	nodeByKey map[string]Node,
	characterDependencies map[string]referencePlanCharacterDependency,
	baseKeys map[string]string,
) ([]ReferencePlanFixedTargetSeed, error) {
	result := make([]ReferencePlanFixedTargetSeed, 0, len(sceneByScope))
	scopes := make([]string, 0, len(sceneByScope))
	for scope := range sceneByScope {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		scene := sceneByScope[scope]
		closure, err := buildProductionSceneCompositionClosure(nodes, scene, refIndex, bindingTriples)
		if err != nil {
			return nil, err
		}
		key, err := expectedReferenceBusinessKey("scene_composition", scene)
		if err != nil {
			return nil, err
		}
		seed, err := referencePlanCompositionSeed(key, "scene_composition", []string{scope}, closure, nodeByKey, characterDependencies, baseKeys)
		if err != nil {
			return nil, err
		}
		result = append(result, seed)
	}
	for _, interaction := range nodes {
		if interaction.NodeType != NodeTypeContinuityClaim {
			continue
		}
		var payload productionInteractionPayload
		if decodeStrictObject(interaction.Payload, &payload) != nil || payload.ClaimType != "interaction" {
			continue
		}
		scene, err := resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
		if err != nil {
			return nil, err
		}
		if _, included := sceneByScope[scene.OwnerRef.OwnerLogicalID]; !included {
			continue
		}
		closure, err := buildProductionInteractionCompositionClosure(nodes, interaction, refIndex, bindingTriples)
		if err != nil {
			return nil, err
		}
		key, err := expectedReferenceBusinessKey("interaction_composition", interaction)
		if err != nil {
			return nil, err
		}
		seed, err := referencePlanCompositionSeed(
			key, "interaction_composition", []string{scene.OwnerRef.OwnerLogicalID}, closure,
			nodeByKey, characterDependencies, baseKeys,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, seed)
	}
	return result, nil
}

func referencePlanCompositionSeed(
	key, kind string,
	coverage []string,
	closure productionCompositionClosure,
	nodeByKey map[string]Node,
	characterDependencies map[string]referencePlanCharacterDependency,
	baseKeys map[string]string,
) (ReferencePlanFixedTargetSeed, error) {
	fixedDependencies := make(map[string]struct{})
	character := make([]ReferencePlanCharacterDependencySeed, 0)
	for tripleKey, triple := range closure.triples {
		if dependency, exists := characterDependencies[tripleKey]; exists {
			character = append(character, ReferencePlanCharacterDependencySeed{
				AnchorBusinessKey: dependency.anchorKey, StateRef: dependency.state.OwnerRef,
				AppearanceBusinessKey: dependency.appearanceKey,
			})
			continue
		}
		baseKey, exists := baseKeys[tripleKey]
		if !exists || closure.assetKinds[triple.asset] == "character" {
			return ReferencePlanFixedTargetSeed{}, errors.New("reference_plan_composition_dependency_missing")
		}
		fixedDependencies[baseKey] = struct{}{}
	}
	slices.SortFunc(character, func(left, right ReferencePlanCharacterDependencySeed) int {
		leftKey := left.AnchorBusinessKey + "\x00" + referencePlanOwnerRefSortKey(left.StateRef)
		rightKey := right.AnchorBusinessKey + "\x00" + referencePlanOwnerRefSortKey(right.StateRef)
		return strings.Compare(leftKey, rightKey)
	})
	return ReferencePlanFixedTargetSeed{
		TargetBusinessKey: key, TargetKind: kind,
		OwnerRefs: ReferencePlanTargetOwnerRefs{
			Identity:      referencePlanOwnerRefsFromSet(closure.identities, nodeByKey),
			Specification: referencePlanOwnerRefsFromSet(closure.specifications, nodeByKey),
			State:         referencePlanOwnerRefsFromSet(closure.states, nodeByKey),
			Scene:         referencePlanOwnerRefsFromSet(closure.scenes, nodeByKey),
			Occurrence:    referencePlanOwnerRefsFromSet(closure.occurrences, nodeByKey),
			Interaction:   referencePlanOwnerRefsFromSet(closure.interactions, nodeByKey),
		},
		CoverageScopeKeys: coverage, FixedDependencyBusinessKeys: referencePlanStringSet(fixedDependencies),
		CharacterDependencies: character,
	}, nil
}

func referencePlanOwnerRefsFromSet(keys map[string]struct{}, nodeByKey map[string]Node) []OwnerRef {
	refs := make([]OwnerRef, 0, len(keys))
	for key := range keys {
		refs = append(refs, nodeByKey[key].OwnerRef)
	}
	slices.SortFunc(refs, func(left, right OwnerRef) int {
		return strings.Compare(referencePlanOwnerRefSortKey(left), referencePlanOwnerRefSortKey(right))
	})
	return refs
}

func referencePlanSceneRefs(values []expectedReferenceOccurrence) []OwnerRef {
	refs := make(map[string]OwnerRef)
	for _, value := range values {
		refs[value.scene.StoryNodeKey] = value.scene.OwnerRef
	}
	return referencePlanOwnerRefMap(refs)
}

func referencePlanOccurrenceRefs(values []expectedReferenceOccurrence) []OwnerRef {
	refs := make(map[string]OwnerRef)
	for _, value := range values {
		refs[value.node.StoryNodeKey] = value.node.OwnerRef
	}
	return referencePlanOwnerRefMap(refs)
}

func referencePlanOwnerRefMap(values map[string]OwnerRef) []OwnerRef {
	refs := make([]OwnerRef, 0, len(values))
	for _, ref := range values {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(left, right OwnerRef) int {
		return strings.Compare(referencePlanOwnerRefSortKey(left), referencePlanOwnerRefSortKey(right))
	})
	return refs
}

func referencePlanOwnerRefSortKey(value OwnerRef) string {
	ref, err := productionOwnerNodeRefFromOwner(value)
	if err != nil {
		return ""
	}
	return ref.sortKey()
}

func referencePlanStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
