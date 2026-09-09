package gormdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type productionGraph struct {
	nodes []storygraph.Node
	edges []storygraph.Edge
}

type productionProjection struct {
	material     productionSnapshotMaterial
	owners       map[string]storygraph.OwnerVersionIdentity
	graph        productionGraph
	nodeKeys     map[string]string
	edgeKeys     map[string]struct{}
	evidenceKeys map[string]string
	sourceKey    string
	bibleOwner   storygraph.OwnerVersionIdentity
}

func buildProductionGraph(
	material productionSnapshotMaterial,
	collections []storygraph.OwnerCollectionRef,
) (productionGraph, error) {
	projection := &productionProjection{
		material: material, owners: make(map[string]storygraph.OwnerVersionIdentity),
		nodeKeys: make(map[string]string), edgeKeys: make(map[string]struct{}), evidenceKeys: make(map[string]string),
	}
	for _, collection := range collections {
		for _, owner := range collection.Members {
			projection.owners[owner.VersionFamily+"\x00"+owner.LogicalID] = owner
		}
	}
	var ok bool
	projection.bibleOwner, ok = projection.owner("bible_production_world_set", material.bibleVersion.ProjectID.String())
	if !ok {
		return productionGraph{}, errors.New("Production Bible Owner Version is missing")
	}
	if err := projection.addSource(); err != nil {
		return productionGraph{}, err
	}
	if err := projection.addEpisodes(); err != nil {
		return productionGraph{}, err
	}
	if err := projection.addAssets(); err != nil {
		return productionGraph{}, err
	}
	if err := projection.addBibleFacts(); err != nil {
		return productionGraph{}, err
	}
	if err := projection.addPlanningFacts(); err != nil {
		return productionGraph{}, err
	}
	return projection.graph, nil
}

func (projection *productionProjection) addSource() error {
	owner, ok := projection.owner("script_source_set", projection.material.revision.DocumentID.String())
	if !ok {
		return errors.New("Script Source Owner Version is missing")
	}
	node, err := newNode(storygraph.NodeTypeSourceRevision, productionOwnerRef(owner, "", ""), "", nil, nil, projectionPayload("storygraph-production/source_revision-ref-payload-contract", owner.ContentHash, nil))
	if err != nil {
		return err
	}
	projection.sourceKey = node.StoryNodeKey
	projection.addNode("source", node)
	return nil
}

func (projection *productionProjection) addEpisodes() error {
	episodeByID := make(map[string]model.Episode, len(projection.material.episodes))
	for _, episode := range projection.material.episodes {
		episodeByID[episode.ID.String()] = episode
	}
	for _, reference := range projection.material.episodeRefs {
		owner, ok := projection.owner("project_episode_set", reference.EpisodeID)
		if !ok {
			return errors.New("Project Episode Owner Version is missing")
		}
		episode := episodeByID[reference.EpisodeID]
		evidence, evidenceKey, err := projection.ensureEvidenceRange(reference.SourceStart, reference.SourceEnd)
		if err != nil {
			return err
		}
		node, err := newNode(storygraph.NodeTypeEpisode, productionOwnerRef(owner, "", ""), episode.Name,
			mustProjectionJSON(map[string]any{"position": reference.Position}), []storygraph.EvidenceRef{evidence},
			projectionPayload("storygraph-production/episode-ref-payload-contract", owner.ContentHash, nil))
		if err != nil {
			return err
		}
		projection.addNode("episode:"+reference.EpisodeID, node)
		if err = projection.addEdge(storygraph.EdgeTypeDerivedFrom, evidenceKey, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
			return err
		}
	}
	return nil
}

func (projection *productionProjection) addAssets() error {
	for _, asset := range projection.material.assets {
		owner, ok := projection.owner("asset_identity_state_set", asset.IdentityKey)
		if !ok {
			return errors.New("AssetIdentity Owner Version is missing")
		}
		node, err := newNode(storygraph.NodeTypeAssetIdentity, productionOwnerRef(owner, "", ""), asset.IdentityKey, nil, nil,
			projectionPayload("storygraph-production/asset-identity-payload-contract", owner.ContentHash, map[string]any{"asset_kind": asset.Kind, "creator_decision_ref": nil}))
		if err != nil {
			return err
		}
		projection.addNode("asset:"+asset.ID.String(), node)
	}
	assetByID := make(map[string]model.Asset, len(projection.material.assets))
	for _, asset := range projection.material.assets {
		assetByID[asset.ID.String()] = asset
	}
	for _, state := range projection.material.states {
		owner, ok := projection.owner("asset_identity_state_set", state.StateKey)
		if !ok {
			return errors.New("AssetState Owner Version is missing")
		}
		asset := assetByID[state.AssetID.String()]
		var fragment agentcontract.ProductionStateFragment
		if json.Unmarshal(state.Snapshot, &fragment) != nil {
			return errors.New("AssetState production snapshot has drifted")
		}
		evidence, evidenceKeys, err := projection.ensureBasisEvidence(fragment.Basis)
		if err != nil {
			return err
		}
		node, err := newNode(storygraph.NodeTypeAssetState, productionOwnerRef(owner, "", ""), state.Label, nil, evidence,
			projectionPayload("storygraph-production/asset-state-payload-contract", owner.ContentHash, map[string]any{
				"asset_kind": asset.Kind, "state_key": state.StateKey, "story_time_range": nil, "creator_decision_ref": nil,
			}))
		if err != nil {
			return err
		}
		projection.addNode("state:"+state.ID.String(), node)
		if err = projection.addEdge(storygraph.EdgeTypeHasState, projection.nodeKeys["asset:"+state.AssetID.String()], node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
			return err
		}
		for _, key := range evidenceKeys {
			if err = projection.addEdge(storygraph.EdgeTypeDerivedFrom, key, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (projection *productionProjection) addBibleFacts() error {
	evidenceByID := make(map[string]model.ProductionWorldEvidence, len(projection.material.bibleEvidence))
	for _, value := range projection.material.bibleEvidence {
		evidenceByID[value.ID.String()] = value
	}
	for _, specification := range projection.material.specifications {
		var basis agentcontract.ProductionSourceBasis
		if json.Unmarshal(evidenceByID[specification.EvidenceID.String()].Basis, &basis) != nil {
			return errors.New("Production Specification Evidence has drifted")
		}
		evidence, evidenceKeys, err := projection.ensureBasisEvidence(basis)
		if err != nil {
			return err
		}
		nodeType := map[string]storygraph.NodeType{"character": storygraph.NodeTypeCharacterSpecification, "location": storygraph.NodeTypeLocationSpecification, "prop": storygraph.NodeTypePropSpecification}[specification.Kind]
		node, err := newNode(nodeType, productionOwnerRef(projection.bibleOwner, "specification:"+specification.ID.String(), specification.ContentHash), specification.SpecificationKey, nil, evidence,
			projectionPayload("storygraph-production/specification-payload-contract", specification.ContentHash, map[string]any{"asset_kind": specification.Kind, "creator_decision_ref": nil}))
		if err != nil {
			return err
		}
		projection.addNode("specification:"+specification.ID.String(), node)
		projection.attachEvidence("asset:"+specification.AssetID.String(), evidence)
		if err = projection.addEdge(storygraph.EdgeTypeDescribesIdentity, projection.nodeKeys["asset:"+specification.AssetID.String()], node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
			return err
		}
		for _, key := range evidenceKeys {
			if err = projection.addEdge(storygraph.EdgeTypeDerivedFrom, key, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
				return err
			}
			if err = projection.addEdge(storygraph.EdgeTypeDerivedFrom, key, projection.nodeKeys["asset:"+specification.AssetID.String()], storygraph.EdgeQualifier{}); err != nil {
				return err
			}
		}
	}
	bindingStates := make(map[string][]model.ProductionWorldBindingState)
	for _, value := range projection.material.bindingStates {
		bindingStates[value.BindingID.String()] = append(bindingStates[value.BindingID.String()], value)
	}
	for _, binding := range projection.material.bindings {
		node, err := newNode(storygraph.NodeTypeProductionBinding, productionOwnerRef(projection.bibleOwner, "binding:"+binding.ID.String(), binding.ContentHash), binding.IdentityKey, nil, nil,
			projectionPayload("storygraph-production/production-binding-payload-contract", binding.ContentHash, nil))
		if err != nil {
			return err
		}
		projection.addNode("binding:"+binding.ID.String(), node)
		for _, input := range []struct {
			key, role string
		}{
			{projection.nodeKeys["asset:"+binding.AssetID.String()], "asset"},
			{projection.nodeKeys["specification:"+binding.SpecificationID.String()], "specification"},
		} {
			if err = projection.addEdge(storygraph.EdgeTypeMaterializes, input.key, node.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: input.role}); err != nil {
				return err
			}
		}
		for _, state := range bindingStates[binding.ID.String()] {
			if err = projection.addEdge(storygraph.EdgeTypeMaterializes, projection.nodeKeys["state:"+state.AssetStateID.String()], node.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}); err != nil {
				return err
			}
		}
	}
	for _, claim := range projection.material.bibleClaims {
		if err := projection.addBibleClaim(claim, evidenceByID[claim.EvidenceID.String()]); err != nil {
			return err
		}
	}
	return nil
}

func (projection *productionProjection) addBibleClaim(claim model.ProductionWorldClaim, evidenceRecord model.ProductionWorldEvidence) error {
	var basis agentcontract.ProductionSourceBasis
	var subjects []bibledomain.ProductionWorldClaimSubject
	if json.Unmarshal(evidenceRecord.Basis, &basis) != nil || json.Unmarshal(claim.Subjects, &subjects) != nil {
		return errors.New("Production Bible Claim has drifted")
	}
	evidence, evidenceKeys, err := projection.ensureBasisEvidence(basis)
	if err != nil {
		return err
	}
	nodeType := map[string]storygraph.NodeType{
		"world_rule": storygraph.NodeTypeWorldRule, "relationship": storygraph.NodeTypeRelationshipClaim,
		"story_arc": storygraph.NodeTypeStoryArc, "plot_thread": storygraph.NodeTypePlotThread,
	}[claim.ClaimType]
	contractID := "storygraph-production/auditable-bible-fact-payload-contract"
	if nodeType == storygraph.NodeTypeRelationshipClaim || nodeType == storygraph.NodeTypeForeshadowingClaim || nodeType == storygraph.NodeTypePayoffClaim {
		contractID = "storygraph-production/narrative-claim-payload-contract"
	}
	node, err := newNode(nodeType, productionOwnerRef(projection.bibleOwner, "claim:"+claim.ID.String(), claim.ContentHash), claim.Statement, nil, evidence,
		projectionPayload(contractID, claim.ContentHash, map[string]any{"claim_type": claim.ClaimType, "creator_decision_ref": nil}))
	if err != nil {
		return err
	}
	projection.addNode("bible-claim:"+claim.ID.String(), node)
	for _, key := range evidenceKeys {
		edgeType := storygraph.EdgeTypeDerivedFrom
		if nodeType == storygraph.NodeTypeRelationshipClaim {
			edgeType = storygraph.EdgeTypeSupports
		}
		if err = projection.addEdge(edgeType, key, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
			return err
		}
	}
	if nodeType == storygraph.NodeTypeRelationshipClaim {
		for _, subject := range subjects {
			if err = projection.addEdge(storygraph.EdgeTypeClaimParticipant, projection.nodeKeys["asset:"+subject.AssetID], node.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: "participant"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (projection *productionProjection) addPlanningFacts() error {
	facts := append([]planningdomain.ProductionWorldPlanningFact(nil), projection.material.planningFacts...)
	slices.SortFunc(facts, func(left, right planningdomain.ProductionWorldPlanningFact) int {
		return strings.Compare(left.Kind+"\x00"+left.BusinessKey, right.Kind+"\x00"+right.BusinessKey)
	})
	for _, kind := range []string{"scene", "dialogue", "narrative_beat", "occurrence", "continuity_claim"} {
		for _, fact := range facts {
			if fact.Kind == kind {
				if err := projection.addPlanningFact(fact); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (projection *productionProjection) addPlanningFact(fact planningdomain.ProductionWorldPlanningFact) error {
	owner, ok := projection.owner("planning_scene_set", fact.BusinessKey)
	if !ok {
		return errors.New("Planning Owner Version is missing")
	}
	switch fact.Kind {
	case "scene":
		var payload planningdomain.SceneFactPayload
		if json.Unmarshal(fact.Payload, &payload) != nil {
			return errors.New("Planning Scene payload has drifted")
		}
		evidence, evidenceKey, err := projection.ensureEvidenceRange(payload.SourceStart, payload.SourceEnd)
		if err != nil {
			return err
		}
		node, err := newNode(storygraph.NodeTypeScene, productionOwnerRef(owner, "", ""), payload.SceneOwnerLogicalID, mustProjectionJSON(map[string]any{"story_time_key": payload.StoryTimeKey}), []storygraph.EvidenceRef{evidence}, projectionPayload("storygraph-production/scene-ref-payload-contract", owner.ContentHash, nil))
		if err != nil {
			return err
		}
		projection.addNode("planning:"+fact.ID, node)
		projection.nodeKeys["scene-logical:"+payload.SceneOwnerLogicalID] = node.StoryNodeKey
		if err = projection.addEdge(storygraph.EdgeTypeContains, projection.nodeKeys["episode:"+fact.EpisodeID], node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: payload.StoryTimeKey}); err != nil {
			return err
		}
		return projection.addEdge(storygraph.EdgeTypeDerivedFrom, evidenceKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
	case "dialogue":
		var payload planningdomain.DialogueFactPayload
		var fragment agentcontract.SceneDialogueFragment
		if json.Unmarshal(fact.Payload, &payload) != nil || json.Unmarshal(payload.Fragment, &fragment) != nil {
			return errors.New("Planning Dialogue payload has drifted")
		}
		return projection.addOrderedSceneFact(fact, owner, payload.Scene.ID, payload.SequenceKey, fragment.Text, fragment.Evidence, storygraph.NodeTypeDialogue, "storygraph-production/dialogue-ref-payload-contract")
	case "narrative_beat":
		var payload planningdomain.NarrativeBeatFactPayload
		var fragment agentcontract.SceneBeatFragment
		if json.Unmarshal(fact.Payload, &payload) != nil || json.Unmarshal(payload.Fragment, &fragment) != nil {
			return errors.New("Planning Beat payload has drifted")
		}
		return projection.addOrderedSceneFact(fact, owner, payload.Scene.ID, payload.SequenceKey, fragment.Text, fragment.Evidence, storygraph.NodeTypeNarrativeBeat, "storygraph-production/narrative_beat-ref-payload-contract")
	case "occurrence":
		return projection.addOccurrence(fact, owner)
	case "continuity_claim":
		return projection.addPlanningClaim(fact, owner)
	default:
		return errors.New("unknown Planning StoryGraph fact")
	}
}

func (projection *productionProjection) addOrderedSceneFact(
	fact planningdomain.ProductionWorldPlanningFact,
	owner storygraph.OwnerVersionIdentity,
	sceneID string,
	sequence int,
	label string,
	span agentcontract.SourceEvidenceSpan,
	nodeType storygraph.NodeType,
	payloadContractID string,
) error {
	evidence, evidenceKey, err := projection.ensureEvidence(span)
	if err != nil {
		return err
	}
	node, err := newNode(nodeType, productionOwnerRef(owner, "", ""), label, mustProjectionJSON(map[string]any{"sequence_key": sequence}), []storygraph.EvidenceRef{evidence}, projectionPayload(payloadContractID, owner.ContentHash, nil))
	if err != nil {
		return err
	}
	projection.addNode("planning:"+fact.ID, node)
	if err = projection.addEdge(storygraph.EdgeTypeContains, projection.nodeKeys["planning:"+sceneID], node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("%012d", sequence)}); err != nil {
		return err
	}
	return projection.addEdge(storygraph.EdgeTypeDerivedFrom, evidenceKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
}

func (projection *productionProjection) addOccurrence(fact planningdomain.ProductionWorldPlanningFact, owner storygraph.OwnerVersionIdentity) error {
	var payload planningdomain.OccurrenceFactPayload
	var fragment agentcontract.SceneOccurrenceFragment
	if json.Unmarshal(fact.Payload, &payload) != nil || json.Unmarshal(payload.Fragment, &fragment) != nil {
		return errors.New("Planning Occurrence payload has drifted")
	}
	evidence, evidenceKey, err := projection.ensureEvidence(fragment.Evidence)
	if err != nil {
		return err
	}
	node, err := newNode(storygraph.NodeTypeOccurrence, productionOwnerRef(owner, "", ""), fragment.OccurrenceKey, mustProjectionJSON(map[string]any{"sequence_key": payload.SequenceKey}), []storygraph.EvidenceRef{evidence}, projectionPayload("storygraph-production/occurrence-payload-contract", owner.ContentHash, map[string]any{"creator_decision_ref": nil}))
	if err != nil {
		return err
	}
	projection.addNode("planning:"+fact.ID, node)
	projection.nodeKeys["occurrence:"+fragment.OccurrenceKey] = node.StoryNodeKey
	if err = projection.addEdge(storygraph.EdgeTypeAnchorsOccurrence, projection.nodeKeys["planning:"+payload.Scene.ID], node.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}); err != nil {
		return err
	}
	if err = projection.addEdge(storygraph.EdgeTypeInstantiatesOccurrence, projection.nodeKeys["state:"+payload.State.ID], node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
		return err
	}
	return projection.addEdge(storygraph.EdgeTypeDerivedFrom, evidenceKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
}

func (projection *productionProjection) addPlanningClaim(fact planningdomain.ProductionWorldPlanningFact, owner storygraph.OwnerVersionIdentity) error {
	var payload planningdomain.PlanningClaimFactPayload
	if json.Unmarshal(fact.Payload, &payload) != nil {
		return errors.New("Planning Claim payload has drifted")
	}
	var evidence []storygraph.EvidenceRef
	var evidenceKeys []string
	if payload.ClaimType == "interaction" {
		var fragment agentcontract.InteractionFragment
		if json.Unmarshal(payload.Fragment, &fragment) != nil {
			return errors.New("Interaction Claim fragment has drifted")
		}
		value, key, err := projection.ensureEvidence(fragment.Evidence)
		if err != nil {
			return err
		}
		evidence, evidenceKeys = []storygraph.EvidenceRef{value}, []string{key}
	} else {
		var fragment agentcontract.ContinuityFragment
		if json.Unmarshal(payload.Fragment, &fragment) != nil {
			return errors.New("Continuity Claim fragment has drifted")
		}
		for _, span := range fragment.Evidence {
			value, key, err := projection.ensureEvidence(span)
			if err != nil {
				return err
			}
			evidence, evidenceKeys = append(evidence, value), append(evidenceKeys, key)
		}
	}
	contractID, contractErr := storygraph.ProductionPayloadContract(storygraph.NodeTypeContinuityClaim, payload.ClaimType)
	if contractErr != nil {
		return contractErr
	}
	node, err := newNode(storygraph.NodeTypeContinuityClaim, productionOwnerRef(owner, "", ""), fact.BusinessKey, mustProjectionJSON(map[string]any{"story_time_key": payload.StoryTimeKey}), evidence, projectionPayload(contractID, owner.ContentHash, map[string]any{"claim_type": payload.ClaimType, "creator_decision_ref": nil}))
	if err != nil {
		return err
	}
	projection.addNode("planning:"+fact.ID, node)
	for _, key := range evidenceKeys {
		if err = projection.addEdge(storygraph.EdgeTypeSupports, key, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
			return err
		}
	}
	for _, participant := range []struct {
		ref  *planningdomain.ProductionWorldPlanningFactRef
		role string
	}{{payload.ActorOccurrence, "actor"}, {payload.PropOccurrence, "prop"}, {payload.CounterpartyOccurrence, "counterparty"}} {
		if participant.ref != nil {
			if err = projection.addEdge(storygraph.EdgeTypeClaimParticipant, projection.nodeKeys["planning:"+participant.ref.ID], node.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: participant.role}); err != nil {
				return err
			}
		}
	}
	for _, anchor := range []struct {
		ref  planningdomain.ProductionWorldPlanningFactRef
		role string
	}{{payload.SourceScene, "scope_start"}, {payload.TargetScene, "scope_end"}} {
		if err = projection.addEdge(storygraph.EdgeTypeClaimAnchor, projection.nodeKeys["planning:"+anchor.ref.ID], node.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: anchor.role}); err != nil {
			return err
		}
	}
	for _, state := range []struct {
		ref  *planningdomain.ExactPlanningRef
		role string
	}{{payload.BeforeState, "before"}, {payload.AfterState, "after"}} {
		if state.ref != nil {
			if err = projection.addEdge(storygraph.EdgeTypeClaimState, projection.nodeKeys["state:"+state.ref.ID], node.StoryNodeKey, storygraph.EdgeQualifier{StateRole: state.role}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (projection *productionProjection) ensureBasisEvidence(basis agentcontract.ProductionSourceBasis) ([]storygraph.EvidenceRef, []string, error) {
	result := make([]storygraph.EvidenceRef, 0, len(basis.Evidence))
	keys := make([]string, 0, len(basis.Evidence))
	for _, span := range basis.Evidence {
		value, key, err := projection.ensureEvidence(span)
		if err != nil {
			return nil, nil, err
		}
		result, keys = append(result, value), append(keys, key)
	}
	return result, keys, nil
}

func (projection *productionProjection) ensureEvidence(span agentcontract.SourceEvidenceSpan) (storygraph.EvidenceRef, string, error) {
	if err := span.Validate([]rune(projection.material.revision.NormalizedText)); err != nil {
		return storygraph.EvidenceRef{}, "", err
	}
	return projection.ensureEvidenceRange(span.SourceStart, span.SourceEnd)
}

func (projection *productionProjection) ensureEvidenceRange(start, end int) (storygraph.EvidenceRef, string, error) {
	evidence, err := evidenceRef(projection.material.revision, start, end)
	if err != nil {
		return storygraph.EvidenceRef{}, "", err
	}
	identity := fmt.Sprintf("%012d:%012d:%s", start, end, evidence.TextHash)
	if key, exists := projection.evidenceKeys[identity]; exists {
		return evidence, key, nil
	}
	owner := productionOwnerRef(projection.bibleOwner, "source-range:"+identity, evidence.TextHash)
	node, err := newNode(storygraph.NodeTypeSourceEvidence, owner, "", nil, nil, projectionPayload("storygraph-production/source_evidence-ref-payload-contract", evidence.TextHash, nil))
	if err != nil {
		return storygraph.EvidenceRef{}, "", err
	}
	projection.addNode("evidence:"+identity, node)
	projection.evidenceKeys[identity] = node.StoryNodeKey
	if err = projection.addEdge(storygraph.EdgeTypeDerivedFrom, projection.sourceKey, node.StoryNodeKey, storygraph.EdgeQualifier{}); err != nil {
		return storygraph.EvidenceRef{}, "", err
	}
	return evidence, node.StoryNodeKey, nil
}

func (projection *productionProjection) owner(family, logicalID string) (storygraph.OwnerVersionIdentity, bool) {
	value, ok := projection.owners[family+"\x00"+logicalID]
	return value, ok
}

func (projection *productionProjection) addNode(index string, node storygraph.Node) {
	projection.graph.nodes = append(projection.graph.nodes, node)
	projection.nodeKeys[index] = node.StoryNodeKey
}

func (projection *productionProjection) attachEvidence(index string, values []storygraph.EvidenceRef) {
	key := projection.nodeKeys[index]
	for nodeIndex := range projection.graph.nodes {
		if projection.graph.nodes[nodeIndex].StoryNodeKey != key {
			continue
		}
		existing := make(map[storygraph.EvidenceRef]struct{}, len(projection.graph.nodes[nodeIndex].EvidenceRefs))
		for _, value := range projection.graph.nodes[nodeIndex].EvidenceRefs {
			existing[value] = struct{}{}
		}
		for _, value := range values {
			if _, ok := existing[value]; !ok {
				projection.graph.nodes[nodeIndex].EvidenceRefs = append(projection.graph.nodes[nodeIndex].EvidenceRefs, value)
				existing[value] = struct{}{}
			}
		}
		return
	}
}

func (projection *productionProjection) addEdge(edgeType storygraph.EdgeType, from, to string, qualifier storygraph.EdgeQualifier) error {
	if from == "" || to == "" {
		return errors.New("Production StoryGraph edge endpoint is missing")
	}
	edge, err := newEdge(edgeType, from, to, qualifier)
	if err != nil {
		return err
	}
	if _, exists := projection.edgeKeys[edge.EdgeKey]; exists {
		return nil
	}
	projection.edgeKeys[edge.EdgeKey] = struct{}{}
	projection.graph.edges = append(projection.graph.edges, edge)
	return nil
}

func productionOwnerRef(value storygraph.OwnerVersionIdentity, fragmentKey, fragmentHash string) storygraph.OwnerRef {
	return storygraph.OwnerRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: value.OwnerKind,
		VersionFamily: value.VersionFamily, OwnerLogicalID: value.LogicalID,
		FragmentKey: fragmentKey, FragmentContentHash: fragmentHash,
		OwnerVersionID: value.VersionID, OwnerRevision: value.Revision, OwnerContentHash: value.ContentHash,
	}
}

func projectionPayload(contractID, hash string, fields map[string]any) map[string]any {
	result := map[string]any{"payload_contract_id": contractID, "projection_hash": hash}
	for key, value := range fields {
		result[key] = value
	}
	return result
}

func mustProjectionJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
