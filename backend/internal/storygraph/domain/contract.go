package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	SchemaVersion      = "storygraph-scene-production"
	ProductionSchemaID = "storygraph-production"
)

var (
	hashPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	storyKeyPattern = regexp.MustCompile(`^sgn_[0-9a-f]{64}$`)
)

type GraphBoundary struct {
	Kind          string `json:"kind"`
	Owner         string `json:"owner"`
	IdentityField string `json:"identity_field"`
	Storage       string `json:"storage"`
	Executable    bool   `json:"executable"`
}

func GraphBoundaries() []GraphBoundary {
	return []GraphBoundary{
		{Kind: "storygraph", Owner: "production/storygraph", IdentityField: "story_node_key", Storage: "postgresql/gorm", Executable: false},
		{Kind: "authoring_graph", Owner: "authoring", IdentityField: "authoring_node_id", Storage: "postgresql/gorm", Executable: false},
		{Kind: "workflow_definition", Owner: "workflow", IdentityField: "workflow_node_id", Storage: "postgresql/gorm", Executable: true},
		{Kind: "temporal_history", Owner: "temporal", IdentityField: "node_run_id", Storage: "temporal", Executable: true},
	}
}

type NodeType string

const (
	NodeTypeSourceRevision                     NodeType = "source_revision"
	NodeTypeSourceEvidence                     NodeType = "source_evidence"
	NodeTypePolicySnapshot                     NodeType = "policy_snapshot"
	NodeTypeEffectiveStyleSnapshot             NodeType = "effective_style_snapshot"
	NodeTypeAssetIdentity                      NodeType = "asset_identity"
	NodeTypeCharacterSpecification             NodeType = "character_specification"
	NodeTypeLocationSpecification              NodeType = "location_specification"
	NodeTypePropSpecification                  NodeType = "prop_specification"
	NodeTypeAssetState                         NodeType = "asset_state"
	NodeTypeProductionBinding                  NodeType = "production_binding"
	NodeTypeWorldRule                          NodeType = "world_rule"
	NodeTypeStoryArc                           NodeType = "story_arc"
	NodeTypePlotThread                         NodeType = "plot_thread"
	NodeTypeRelationshipClaim                  NodeType = "relationship_claim"
	NodeTypeForeshadowingClaim                 NodeType = "foreshadowing_claim"
	NodeTypePayoffClaim                        NodeType = "payoff_claim"
	NodeTypeEpisode                            NodeType = "episode"
	NodeTypeScene                              NodeType = "scene"
	NodeTypeDialogue                           NodeType = "dialogue"
	NodeTypeNarrativeBeat                      NodeType = "narrative_beat"
	NodeTypeOccurrence                         NodeType = "occurrence"
	NodeTypeContinuityClaim                    NodeType = "continuity_claim"
	NodeTypeCausalClaim                        NodeType = "causal_claim"
	NodeTypeShot                               NodeType = "shot"
	NodeTypeShotContinuityClaim                NodeType = "shot_continuity_claim"
	NodeTypeGenerationTarget                   NodeType = "generation_target"
	NodeTypeArtifact                           NodeType = "artifact"
	NodeTypeAssetVersion                       NodeType = "asset_version"
	NodeTypeApprovedReferencePlanVersion       NodeType = "approved_reference_plan_version"
	NodeTypeReferencePlanTarget                NodeType = "reference_plan_target"
	NodeTypeSceneReferenceBindingVersion       NodeType = "scene_reference_binding_version"
	NodeTypeInteractionReferenceBindingVersion NodeType = "interaction_reference_binding_version"
	NodeTypeShotProductionBindingVersion       NodeType = "shot_production_binding_version"
	NodeTypeShotImageBindingVersion            NodeType = "shot_image_binding_version"
)

var nodeOwners = map[NodeType]string{
	NodeTypeSourceRevision: "production/script", NodeTypeSourceEvidence: "production/bible",
	NodeTypePolicySnapshot: "preset", NodeTypeEffectiveStyleSnapshot: "preset",
	NodeTypeAssetIdentity: "asset", NodeTypeAssetState: "asset", NodeTypeArtifact: "asset", NodeTypeAssetVersion: "asset",
	NodeTypeCharacterSpecification: "production/bible", NodeTypeLocationSpecification: "production/bible", NodeTypePropSpecification: "production/bible",
	NodeTypeProductionBinding: "production/bible", NodeTypeWorldRule: "production/bible", NodeTypeStoryArc: "production/bible", NodeTypePlotThread: "production/bible",
	NodeTypeRelationshipClaim: "production/bible", NodeTypeForeshadowingClaim: "production/bible", NodeTypePayoffClaim: "production/bible",
	NodeTypeEpisode: "production/project",
	NodeTypeScene:   "production/planning", NodeTypeDialogue: "production/planning", NodeTypeNarrativeBeat: "production/planning", NodeTypeOccurrence: "production/planning",
	NodeTypeContinuityClaim: "production/planning", NodeTypeCausalClaim: "production/planning",
	NodeTypeShot: "production/storyboard", NodeTypeShotContinuityClaim: "production/storyboard",
	NodeTypeShotProductionBindingVersion: "production/storyboard", NodeTypeShotImageBindingVersion: "production/storyboard",
	NodeTypeGenerationTarget:             "generation",
	NodeTypeApprovedReferencePlanVersion: "production/reference", NodeTypeReferencePlanTarget: "production/reference",
	NodeTypeSceneReferenceBindingVersion: "production/reference", NodeTypeInteractionReferenceBindingVersion: "production/reference",
}

type OwnerRef struct {
	WorkspaceID         string `json:"workspace_id,omitempty"`
	ProjectID           string `json:"project_id,omitempty"`
	OwnerKind           string `json:"owner_kind"`
	VersionFamily       string `json:"version_family,omitempty"`
	OwnerLogicalID      string `json:"owner_logical_id"`
	FragmentKey         string `json:"fragment_key,omitempty"`
	FragmentContentHash string `json:"fragment_content_hash,omitempty"`
	OwnerVersionID      string `json:"owner_version_id"`
	OwnerRevision       int64  `json:"owner_revision"`
	OwnerContentHash    string `json:"owner_content_hash,omitempty"`
	ContentHash         string `json:"content_hash,omitempty"`
}

func (value OwnerRef) validate(nodeType NodeType) error {
	expectedOwner, ok := nodeOwners[nodeType]
	if !ok || value.OwnerKind != expectedOwner || strings.TrimSpace(value.OwnerLogicalID) == "" || value.OwnerRevision < 1 || !hashPattern.MatchString(value.ownerContentHash()) {
		return errors.New("invalid StoryGraph owner reference")
	}
	productionRef := value.VersionFamily != "" || value.WorkspaceID != "" || value.ProjectID != "" || value.OwnerContentHash != "" || value.FragmentContentHash != ""
	if productionRef {
		if _, err := uuid.Parse(value.WorkspaceID); err != nil {
			return errors.New("invalid StoryGraph owner reference")
		}
		if _, err := uuid.Parse(value.ProjectID); err != nil || value.VersionFamily == "" || value.ContentHash != "" || value.OwnerContentHash == "" ||
			(value.FragmentKey == "") != (value.FragmentContentHash == "") ||
			(value.FragmentContentHash != "" && !hashPattern.MatchString(value.FragmentContentHash)) {
			return errors.New("invalid StoryGraph owner reference")
		}
	}
	if _, err := uuid.Parse(value.OwnerVersionID); err != nil {
		return errors.New("invalid StoryGraph owner reference")
	}
	return nil
}

func (value OwnerRef) ownerContentHash() string {
	if value.OwnerContentHash != "" {
		return value.OwnerContentHash
	}
	return value.ContentHash
}

func DeriveStoryNodeKey(nodeType NodeType, owner OwnerRef) (string, error) {
	if err := owner.validate(nodeType); err != nil {
		return "", err
	}
	material := struct {
		Schema         string   `json:"schema"`
		NodeType       NodeType `json:"node_type"`
		OwnerKind      string   `json:"owner_kind"`
		OwnerLogicalID string   `json:"owner_logical_id"`
		FragmentKey    string   `json:"fragment_key"`
	}{"story-node-key-owner-fragment", nodeType, owner.OwnerKind, owner.OwnerLogicalID, owner.FragmentKey}
	return prefixedHash("sgn_", material)
}

type EdgeType string

const (
	EdgeTypeContains                 EdgeType = "contains"
	EdgeTypeDerivedFrom              EdgeType = "derived_from"
	EdgeTypeDescribesIdentity        EdgeType = "describes_identity"
	EdgeTypeHasState                 EdgeType = "has_state"
	EdgeTypePrecedes                 EdgeType = "precedes"
	EdgeTypeAnchorsOccurrence        EdgeType = "anchors_occurrence"
	EdgeTypeInstantiatesOccurrence   EdgeType = "instantiates_occurrence"
	EdgeTypeRealizes                 EdgeType = "realizes"
	EdgeTypeInforms                  EdgeType = "informs"
	EdgeTypeConstrains               EdgeType = "constrains"
	EdgeTypeMaterializes             EdgeType = "materializes"
	EdgeTypeContainsReferenceTarget  EdgeType = "contains_reference_target"
	EdgeTypeDependsOnReferenceTarget EdgeType = "depends_on_reference_target"
	EdgeTypePlansReference           EdgeType = "plans_reference"
	EdgeTypeFulfillsReferenceTarget  EdgeType = "fulfills_reference_target"
	EdgeTypeBindsReferenceInput      EdgeType = "binds_reference_input"
	EdgeTypeBindsReferenceOutput     EdgeType = "binds_reference_output"
	EdgeTypeBindsInput               EdgeType = "binds_input"
	EdgeTypeFeedsGeneration          EdgeType = "feeds_generation"
	EdgeTypeBindsOutput              EdgeType = "binds_output"
	EdgeTypeSupports                 EdgeType = "supports"
	EdgeTypeClaimParticipant         EdgeType = "claim_participant"
	EdgeTypeClaimAnchor              EdgeType = "claim_anchor"
	EdgeTypeClaimState               EdgeType = "claim_state"
	EdgeTypeSupersedes               EdgeType = "supersedes"
)

var edgeTypes = []EdgeType{
	EdgeTypeContains, EdgeTypeDerivedFrom, EdgeTypeDescribesIdentity, EdgeTypeHasState, EdgeTypePrecedes,
	EdgeTypeAnchorsOccurrence, EdgeTypeInstantiatesOccurrence, EdgeTypeRealizes, EdgeTypeInforms, EdgeTypeConstrains,
	EdgeTypeMaterializes, EdgeTypeContainsReferenceTarget, EdgeTypeDependsOnReferenceTarget, EdgeTypePlansReference,
	EdgeTypeFulfillsReferenceTarget, EdgeTypeBindsReferenceInput, EdgeTypeBindsReferenceOutput,
	EdgeTypeBindsInput, EdgeTypeFeedsGeneration, EdgeTypeBindsOutput, EdgeTypeSupports,
	EdgeTypeClaimParticipant, EdgeTypeClaimAnchor, EdgeTypeSupersedes,
	EdgeTypeClaimState,
}

type EdgeQualifier struct {
	BindingRole     string `json:"binding_role,omitempty"`
	ParticipantRole string `json:"participant_role,omitempty"`
	AnchorRole      string `json:"anchor_role,omitempty"`
	StateRole       string `json:"state_role,omitempty"`
	ConstraintRole  string `json:"constraint_role,omitempty"`
	ReferenceRole   string `json:"reference_role,omitempty"`
	InformsRole     string `json:"informs_role,omitempty"`
	SequenceKey     string `json:"sequence_key,omitempty"`
}

func (value EdgeQualifier) validate(edgeType EdgeType) error {
	if !slices.Contains(edgeTypes, edgeType) {
		return errors.New("unknown StoryGraph edge type")
	}
	switch edgeType {
	case EdgeTypeMaterializes:
		if !oneOf(value.BindingRole, "specification", "state", "asset", "asset_version", "identity_anchor", "artifact") || value.hasFieldsOtherThan("binding") {
			return errors.New("invalid materializes qualifier")
		}
	case EdgeTypeBindsInput:
		if value == (EdgeQualifier{}) {
			return nil
		}
		if !oneOf(value.BindingRole, "shot", "occurrence", "asset_version", "scene_reference", "interaction_reference", "style") || value.hasFieldsOtherThan("binding") {
			return errors.New("invalid binds input qualifier")
		}
	case EdgeTypeClaimParticipant:
		if !oneOf(value.ParticipantRole, "subject", "object", "participant", "actor", "prop", "counterparty", "holder_before", "holder_after") || value.hasFieldsOtherThan("participant") {
			return errors.New("invalid claim participant qualifier")
		}
	case EdgeTypeClaimAnchor:
		if !oneOf(value.AnchorRole, "episode", "scene", "beat", "character_occurrence", "prop_occurrence", "scope_start", "scope_end") || value.hasFieldsOtherThan("anchor") {
			return errors.New("invalid claim anchor qualifier")
		}
	case EdgeTypeClaimState:
		if !oneOf(value.StateRole, "before", "after", "prop_before", "prop_after") || value.hasFieldsOtherThan("state") {
			return errors.New("invalid claim state qualifier")
		}
	case EdgeTypeAnchorsOccurrence:
		if (value.AnchorRole != "" && !oneOf(value.AnchorRole, "scene", "beat")) || (value.AnchorRole != "" && value.hasFieldsOtherThan("anchor")) || (value.AnchorRole == "" && value != (EdgeQualifier{})) {
			return errors.New("invalid occurrence anchor qualifier")
		}
	case EdgeTypeContains, EdgeTypePrecedes, EdgeTypeContainsReferenceTarget:
		if value.hasFieldsOtherThan("sequence") {
			return errors.New("invalid sequence qualifier")
		}
	case EdgeTypeConstrains:
		if value == (EdgeQualifier{}) {
			return nil
		}
		if !oneOf(value.ConstraintRole, "world", "policy", "style") || value.hasFieldsOtherThan("constraint") {
			return errors.New("invalid constraint qualifier")
		}
	case EdgeTypePlansReference, EdgeTypeBindsReferenceInput:
		if !oneOf(value.ReferenceRole, "identity", "specification", "state", "style", "scene", "occurrence", "interaction", "character_asset", "location_asset", "prop_asset") || value.hasFieldsOtherThan("reference") {
			return errors.New("invalid reference qualifier")
		}
	case EdgeTypeInforms:
		if value == (EdgeQualifier{}) {
			return nil
		}
		if !oneOf(value.InformsRole, "occurrence", "scene_reference", "interaction_reference") || value.hasFieldsOtherThan("informs") {
			return errors.New("invalid informs qualifier")
		}
	default:
		if value != (EdgeQualifier{}) {
			return errors.New("edge type does not accept a qualifier")
		}
	}
	return nil
}

func (value EdgeQualifier) hasFieldsOtherThan(kind string) bool {
	copy := value
	switch kind {
	case "binding":
		copy.BindingRole = ""
	case "participant":
		copy.ParticipantRole = ""
	case "anchor":
		copy.AnchorRole = ""
	case "state":
		copy.StateRole = ""
	case "constraint":
		copy.ConstraintRole = ""
	case "reference":
		copy.ReferenceRole = ""
	case "informs":
		copy.InformsRole = ""
	case "sequence":
		copy.SequenceKey = ""
	}
	return copy != (EdgeQualifier{})
}

func DeriveEdgeKey(edgeType EdgeType, fromNodeKey, toNodeKey string, qualifier EdgeQualifier) (string, error) {
	if !storyKeyPattern.MatchString(fromNodeKey) || !storyKeyPattern.MatchString(toNodeKey) || fromNodeKey == toNodeKey {
		return "", errors.New("invalid StoryGraph edge endpoints")
	}
	if err := qualifier.validate(edgeType); err != nil {
		return "", err
	}
	material := struct {
		Schema    string        `json:"schema"`
		EdgeType  EdgeType      `json:"edge_type"`
		From      string        `json:"from_node_key"`
		To        string        `json:"to_node_key"`
		Qualifier EdgeQualifier `json:"qualifier"`
	}{"story-edge-key-qualified-endpoints", edgeType, fromNodeKey, toNodeKey, qualifier}
	return prefixedHash("sge_", material)
}

type ClaimParticipant struct {
	Role         string `json:"role"`
	StoryNodeKey string `json:"story_node_key"`
}

type ClaimScope struct {
	Kind           string `json:"kind"`
	OwnerLogicalID string `json:"owner_logical_id"`
}

type ClaimPayload struct {
	Predicate          string             `json:"predicate"`
	Participants       []ClaimParticipant `json:"participants"`
	Anchors            []string           `json:"anchors"`
	ValidScope         ClaimScope         `json:"valid_scope"`
	Polarity           string             `json:"polarity"`
	Status             string             `json:"status"`
	SupersedesClaimRef string             `json:"supersedes_claim_ref,omitempty"`
}

func (value ClaimPayload) Validate() error {
	if strings.TrimSpace(value.Predicate) == "" || !oneOf(value.ValidScope.Kind, "project", "episode", "scene", "beat", "source_range") || strings.TrimSpace(value.ValidScope.OwnerLogicalID) == "" || !oneOf(value.Polarity, "positive", "negative", "neutral") || !oneOf(value.Status, "asserted", "uncertain", "negated") {
		return errors.New("invalid StoryGraph claim payload")
	}
	roles := make(map[string]struct{}, len(value.Participants))
	for _, participant := range value.Participants {
		if !oneOf(participant.Role, "subject", "object", "participant") || !storyKeyPattern.MatchString(participant.StoryNodeKey) {
			return errors.New("invalid StoryGraph claim participant")
		}
		if _, exists := roles[participant.Role]; exists {
			return errors.New("duplicate StoryGraph claim participant role")
		}
		roles[participant.Role] = struct{}{}
	}
	if _, ok := roles["subject"]; !ok {
		return errors.New("claim subject is required")
	}
	if _, ok := roles["object"]; !ok {
		return errors.New("claim object is required")
	}
	if len(value.Anchors) == 0 {
		return errors.New("claim anchor is required")
	}
	seenAnchors := make(map[string]struct{}, len(value.Anchors))
	for _, anchor := range value.Anchors {
		if !storyKeyPattern.MatchString(anchor) {
			return errors.New("invalid StoryGraph claim anchor")
		}
		if _, exists := seenAnchors[anchor]; exists {
			return errors.New("duplicate StoryGraph claim anchor")
		}
		seenAnchors[anchor] = struct{}{}
	}
	if value.SupersedesClaimRef != "" && !storyKeyPattern.MatchString(value.SupersedesClaimRef) {
		return errors.New("invalid superseded claim reference")
	}
	return nil
}

type EvidenceRef struct {
	DocumentRevisionID string `json:"document_revision_id"`
	AbsoluteStart      int    `json:"absolute_start"`
	AbsoluteEnd        int    `json:"absolute_end"`
	TextHash           string `json:"text_hash"`
}

type Node struct {
	StoryNodeKey     string          `json:"story_node_key"`
	NodeType         NodeType        `json:"node_type"`
	OwnerRef         OwnerRef        `json:"owner_ref"`
	Label            string          `json:"label,omitempty"`
	BusinessPosition json.RawMessage `json:"business_position,omitempty"`
	EvidenceRefs     []EvidenceRef   `json:"evidence_refs"`
	Payload          json.RawMessage `json:"payload"`
	ContentHash      string          `json:"content_hash,omitempty"`
}

type Snapshot struct {
	SchemaVersion string `json:"schema_version"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

type CanonicalSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
	TopologyHash  string `json:"topology_hash"`
	ContentHash   string `json:"content_hash"`
}

func DecodeSnapshot(raw []byte) (Snapshot, error) {
	var value Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return Snapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Snapshot{}, errors.New("multiple StoryGraph JSON values are not allowed")
	}
	return value, nil
}

func Canonicalize(snapshot Snapshot) (CanonicalSnapshot, error) {
	if !oneOf(snapshot.SchemaVersion, SchemaVersion, ProductionSchemaID) || snapshot.Nodes == nil || snapshot.Edges == nil {
		return CanonicalSnapshot{}, errors.New("invalid StoryGraph snapshot envelope")
	}
	nodes := append(make([]Node, 0, len(snapshot.Nodes)), snapshot.Nodes...)
	seenNodes := make(map[string]struct{}, len(nodes))
	for index := range nodes {
		node, err := canonicalizeNode(nodes[index], snapshot.SchemaVersion)
		if err != nil {
			return CanonicalSnapshot{}, err
		}
		if snapshot.SchemaVersion == ProductionSchemaID {
			if err = validateProductionNode(node); err != nil {
				return CanonicalSnapshot{}, err
			}
		}
		if _, exists := seenNodes[node.StoryNodeKey]; exists {
			return CanonicalSnapshot{}, errors.New("duplicate StoryGraph node key")
		}
		seenNodes[node.StoryNodeKey] = struct{}{}
		nodes[index] = node
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].StoryNodeKey < nodes[j].StoryNodeKey })

	edges := append(make([]Edge, 0, len(snapshot.Edges)), snapshot.Edges...)
	seenEdges := make(map[string]struct{}, len(edges))
	for index := range edges {
		edge, err := canonicalizeEdge(edges[index])
		if err != nil {
			return CanonicalSnapshot{}, err
		}
		if _, exists := seenEdges[edge.EdgeKey]; exists {
			return CanonicalSnapshot{}, errors.New("duplicate StoryGraph edge key")
		}
		seenEdges[edge.EdgeKey] = struct{}{}
		edges[index] = edge
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeKey < edges[j].EdgeKey })
	if err := validateEdgeEndpoints(nodes, edges); err != nil {
		return CanonicalSnapshot{}, err
	}
	if snapshot.SchemaVersion == ProductionSchemaID {
		if err := validateProductionEdgeEndpoints(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionEvidenceRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionIdentityRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionReferenceTargetRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionAssetVersionRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionSceneReferenceBindingRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionInteractionReferenceBindingRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionNarrativeClaimRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionContinuityRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionInteractionRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
		if err := validateProductionStructureRelations(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
	}

	keys := make([]string, 0, len(nodes))
	for _, node := range nodes {
		keys = append(keys, node.StoryNodeKey)
	}
	if _, err := TopologicalOrder(keys, edges); err != nil {
		return CanonicalSnapshot{}, err
	}
	if snapshot.SchemaVersion != ProductionSchemaID {
		if err := validateClaimEdges(nodes, edges); err != nil {
			return CanonicalSnapshot{}, err
		}
	}

	topologyNodes := make([]struct {
		StoryNodeKey string   `json:"story_node_key"`
		NodeType     NodeType `json:"node_type"`
	}, 0, len(nodes))
	for _, node := range nodes {
		topologyNodes = append(topologyNodes, struct {
			StoryNodeKey string   `json:"story_node_key"`
			NodeType     NodeType `json:"node_type"`
		}{node.StoryNodeKey, node.NodeType})
	}
	topologyEdges := make([]struct {
		EdgeKey     string        `json:"edge_key"`
		EdgeType    EdgeType      `json:"edge_type"`
		FromNodeKey string        `json:"from_node_key"`
		ToNodeKey   string        `json:"to_node_key"`
		Qualifier   EdgeQualifier `json:"qualifier"`
	}, 0, len(edges))
	for _, edge := range edges {
		topologyEdges = append(topologyEdges, struct {
			EdgeKey     string        `json:"edge_key"`
			EdgeType    EdgeType      `json:"edge_type"`
			FromNodeKey string        `json:"from_node_key"`
			ToNodeKey   string        `json:"to_node_key"`
			Qualifier   EdgeQualifier `json:"qualifier"`
		}{edge.EdgeKey, edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier})
	}
	topologyHash, err := canonicalValueHash(struct {
		SchemaVersion string `json:"schema_version"`
		Nodes         any    `json:"nodes"`
		Edges         any    `json:"edges"`
	}{snapshot.SchemaVersion, topologyNodes, topologyEdges})
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	contentHash, err := canonicalValueHash(struct {
		SchemaVersion string `json:"schema_version"`
		Nodes         []Node `json:"nodes"`
		Edges         []Edge `json:"edges"`
	}{snapshot.SchemaVersion, nodes, edges})
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	return CanonicalSnapshot{SchemaVersion: snapshot.SchemaVersion, Nodes: nodes, Edges: edges, TopologyHash: topologyHash, ContentHash: contentHash}, nil
}

func validateEdgeEndpoints(nodes []Node, edges []Edge) error {
	types := make(map[string]NodeType, len(nodes))
	for _, node := range nodes {
		types[node.StoryNodeKey] = node.NodeType
	}
	for _, edge := range edges {
		from, fromExists := types[edge.FromNodeKey]
		to, toExists := types[edge.ToNodeKey]
		if !fromExists || !toExists {
			return errors.New("StoryGraph edge has a dangling endpoint")
		}
		if err := ValidateEdgeEndpoint(edge.EdgeType, from, to, edge.Qualifier); err != nil {
			return err
		}
	}
	return nil
}

func ValidateEdgeEndpoint(edgeType EdgeType, from, to NodeType, qualifier EdgeQualifier) error {
	if err := qualifier.validate(edgeType); err != nil {
		return err
	}
	if !edgeEndpointAllowed(edgeType, from, to, qualifier) {
		return fmt.Errorf("StoryGraph edge type %s does not allow %s -> %s", edgeType, from, to)
	}
	return nil
}

func edgeEndpointAllowed(edgeType EdgeType, from, to NodeType, qualifier EdgeQualifier) bool {
	switch edgeType {
	case EdgeTypeContains:
		return from == NodeTypeEpisode && to == NodeTypeScene ||
			from == NodeTypeScene && (to == NodeTypeDialogue || to == NodeTypeNarrativeBeat)
	case EdgeTypeDerivedFrom:
		return from == NodeTypeSourceRevision && (to == NodeTypeSourceEvidence || to == NodeTypeEpisode) ||
			from == NodeTypeSourceEvidence && derivedFactNode(to)
	case EdgeTypeDescribesIdentity:
		return from == NodeTypeAssetIdentity && oneOfNode(to, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
	case EdgeTypeHasState:
		return from == NodeTypeAssetIdentity && to == NodeTypeAssetState
	case EdgeTypePrecedes:
		return from == to && oneOfNode(from, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat, NodeTypeShot)
	case EdgeTypeAnchorsOccurrence:
		return oneOfNode(from, NodeTypeScene, NodeTypeNarrativeBeat) && to == NodeTypeOccurrence
	case EdgeTypeInstantiatesOccurrence:
		return from == NodeTypeAssetState && to == NodeTypeOccurrence
	case EdgeTypeRealizes:
		return from == NodeTypeNarrativeBeat && to == NodeTypeShot
	case EdgeTypeInforms:
		return from == NodeTypeOccurrence && to == NodeTypeShot ||
			oneOfNode(from, NodeTypeOccurrence, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion) &&
				to == NodeTypeShotProductionBindingVersion
	case EdgeTypeConstrains:
		return oneOfNode(from, NodeTypeWorldRule, NodeTypeEffectiveStyleSnapshot, NodeTypePolicySnapshot) &&
			oneOfNode(to, NodeTypeShot, NodeTypeGenerationTarget, NodeTypeReferencePlanTarget, NodeTypeAssetVersion,
				NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion, NodeTypeShotProductionBindingVersion)
	case EdgeTypeMaterializes:
		if to == NodeTypeProductionBinding {
			return from == NodeTypeAssetIdentity && qualifier.BindingRole == "asset" ||
				from == NodeTypeAssetState && qualifier.BindingRole == "state" ||
				oneOfNode(from, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification) && qualifier.BindingRole == "specification" ||
				from == NodeTypeAssetVersion && qualifier.BindingRole == "asset_version"
		}
		return to == NodeTypeAssetVersion && (from == NodeTypeAssetIdentity && qualifier.BindingRole == "asset" ||
			from == NodeTypeAssetState && qualifier.BindingRole == "state" ||
			oneOfNode(from, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification) && qualifier.BindingRole == "specification" ||
			from == NodeTypeAssetVersion && qualifier.BindingRole == "identity_anchor" ||
			from == NodeTypeArtifact && qualifier.BindingRole == "artifact")
	case EdgeTypeContainsReferenceTarget:
		return from == NodeTypeApprovedReferencePlanVersion && to == NodeTypeReferencePlanTarget
	case EdgeTypeDependsOnReferenceTarget:
		return from == NodeTypeReferencePlanTarget && to == NodeTypeReferencePlanTarget
	case EdgeTypePlansReference:
		return oneOfNode(from, NodeTypeAssetIdentity, NodeTypeCharacterSpecification, NodeTypeLocationSpecification,
			NodeTypePropSpecification, NodeTypeAssetState, NodeTypeEffectiveStyleSnapshot, NodeTypeScene,
			NodeTypeOccurrence, NodeTypeContinuityClaim) && to == NodeTypeReferencePlanTarget
	case EdgeTypeFulfillsReferenceTarget:
		return from == NodeTypeReferencePlanTarget && oneOfNode(to, NodeTypeAssetVersion, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion)
	case EdgeTypeBindsReferenceInput:
		return oneOfNode(from, NodeTypeScene, NodeTypeOccurrence, NodeTypeContinuityClaim, NodeTypeAssetVersion) &&
			oneOfNode(to, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion)
	case EdgeTypeBindsReferenceOutput:
		return from == NodeTypeArtifact && oneOfNode(to, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion)
	case EdgeTypeBindsInput:
		return oneOfNode(from, NodeTypeShot, NodeTypeOccurrence, NodeTypeAssetVersion, NodeTypeSceneReferenceBindingVersion,
			NodeTypeInteractionReferenceBindingVersion, NodeTypeEffectiveStyleSnapshot) && to == NodeTypeShotProductionBindingVersion
	case EdgeTypeFeedsGeneration:
		return from == NodeTypeShotProductionBindingVersion && to == NodeTypeGenerationTarget ||
			from == NodeTypeGenerationTarget && to == NodeTypeArtifact
	case EdgeTypeBindsOutput:
		return oneOfNode(from, NodeTypeShot, NodeTypeArtifact) && to == NodeTypeShotImageBindingVersion
	case EdgeTypeSupports:
		return oneOfNode(from, NodeTypeSourceRevision, NodeTypeSourceEvidence) && isClaimNode(to)
	case EdgeTypeClaimParticipant:
		return oneOfNode(from, NodeTypeAssetIdentity, NodeTypeOccurrence, NodeTypeWorldRule) && isClaimNode(to)
	case EdgeTypeClaimAnchor:
		return oneOfNode(from, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat, NodeTypeOccurrence) && isClaimNode(to)
	case EdgeTypeClaimState:
		return from == NodeTypeAssetState && to == NodeTypeContinuityClaim
	case EdgeTypeSupersedes:
		return isClaimNode(from) && isClaimNode(to)
	default:
		return false
	}
}

func derivedFactNode(value NodeType) bool {
	return oneOfNode(value,
		NodeTypeAssetIdentity, NodeTypeAssetState,
		NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification,
		NodeTypeWorldRule, NodeTypeStoryArc, NodeTypePlotThread,
		NodeTypeRelationshipClaim, NodeTypeForeshadowingClaim, NodeTypePayoffClaim,
		NodeTypeEpisode, NodeTypeScene, NodeTypeDialogue, NodeTypeNarrativeBeat,
		NodeTypeOccurrence, NodeTypeContinuityClaim, NodeTypeCausalClaim, NodeTypeShot,
	)
}

func oneOfNode(value NodeType, candidates ...NodeType) bool {
	return slices.Contains(candidates, value)
}

func canonicalizeNode(node Node, schemaID string) (Node, error) {
	derivedKey, err := DeriveStoryNodeKey(node.NodeType, node.OwnerRef)
	if err != nil || node.StoryNodeKey != derivedKey {
		return Node{}, errors.New("StoryGraph node key does not match its Owner")
	}
	payload, err := canonicalObject(node.Payload)
	if err != nil {
		return Node{}, errors.New("StoryGraph node payload must be an object")
	}
	node.Payload = payload
	if len(node.BusinessPosition) > 0 {
		position, positionErr := canonicalObject(node.BusinessPosition)
		if positionErr != nil {
			return Node{}, errors.New("StoryGraph business position must be an object")
		}
		node.BusinessPosition = position
	}
	node.EvidenceRefs = append(make([]EvidenceRef, 0, len(node.EvidenceRefs)), node.EvidenceRefs...)
	for _, evidence := range node.EvidenceRefs {
		if err = evidence.Validate(); err != nil {
			return Node{}, err
		}
	}
	sort.Slice(node.EvidenceRefs, func(i, j int) bool {
		left, right := node.EvidenceRefs[i], node.EvidenceRefs[j]
		return fmt.Sprintf("%s\x00%012d\x00%012d\x00%s", left.DocumentRevisionID, left.AbsoluteStart, left.AbsoluteEnd, left.TextHash) < fmt.Sprintf("%s\x00%012d\x00%012d\x00%s", right.DocumentRevisionID, right.AbsoluteStart, right.AbsoluteEnd, right.TextHash)
	})
	for index := 1; index < len(node.EvidenceRefs); index++ {
		if node.EvidenceRefs[index] == node.EvidenceRefs[index-1] {
			return Node{}, errors.New("duplicate StoryGraph evidence reference")
		}
	}
	if schemaID != ProductionSchemaID && isClaimNode(node.NodeType) {
		var claim ClaimPayload
		if err = decodeStrictObject(node.Payload, &claim); err != nil || claim.Validate() != nil {
			return Node{}, errors.New("invalid StoryGraph claim payload")
		}
	}
	material := struct {
		StoryNodeKey     string          `json:"story_node_key"`
		NodeType         NodeType        `json:"node_type"`
		OwnerRef         OwnerRef        `json:"owner_ref"`
		Label            string          `json:"label,omitempty"`
		BusinessPosition json.RawMessage `json:"business_position,omitempty"`
		EvidenceRefs     []EvidenceRef   `json:"evidence_refs"`
		Payload          json.RawMessage `json:"payload"`
	}{node.StoryNodeKey, node.NodeType, node.OwnerRef, node.Label, node.BusinessPosition, node.EvidenceRefs, node.Payload}
	computed, err := canonicalValueHash(material)
	if err != nil {
		return Node{}, err
	}
	if node.ContentHash != "" && node.ContentHash != computed {
		return Node{}, errors.New("StoryGraph node content hash mismatch")
	}
	node.ContentHash = computed
	return node, nil
}

func canonicalizeEdge(edge Edge) (Edge, error) {
	derivedKey, err := DeriveEdgeKey(edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier)
	if err != nil || edge.EdgeKey != derivedKey {
		return Edge{}, errors.New("StoryGraph edge key does not match its endpoints")
	}
	material := struct {
		EdgeKey     string        `json:"edge_key"`
		EdgeType    EdgeType      `json:"edge_type"`
		FromNodeKey string        `json:"from_node_key"`
		ToNodeKey   string        `json:"to_node_key"`
		Qualifier   EdgeQualifier `json:"qualifier"`
	}{edge.EdgeKey, edge.EdgeType, edge.FromNodeKey, edge.ToNodeKey, edge.Qualifier}
	computed, err := canonicalValueHash(material)
	if err != nil {
		return Edge{}, err
	}
	if edge.ContentHash != "" && edge.ContentHash != computed {
		return Edge{}, errors.New("StoryGraph edge content hash mismatch")
	}
	edge.ContentHash = computed
	return edge, nil
}

func validateClaimEdges(nodes []Node, edges []Edge) error {
	for _, node := range nodes {
		if !isClaimNode(node.NodeType) {
			continue
		}
		var payload ClaimPayload
		if err := json.Unmarshal(node.Payload, &payload); err != nil {
			return err
		}
		expectedParticipants := make(map[string]struct{}, len(payload.Participants))
		for _, participant := range payload.Participants {
			expectedParticipants[participant.Role+"\x00"+participant.StoryNodeKey] = struct{}{}
		}
		expectedAnchors := make(map[string]struct{}, len(payload.Anchors))
		for _, anchor := range payload.Anchors {
			expectedAnchors[anchor] = struct{}{}
		}
		for _, edge := range edges {
			if edge.ToNodeKey != node.StoryNodeKey {
				continue
			}
			switch edge.EdgeType {
			case EdgeTypeClaimParticipant:
				key := edge.Qualifier.ParticipantRole + "\x00" + edge.FromNodeKey
				if _, ok := expectedParticipants[key]; !ok {
					return errors.New("claim participant edge does not match payload")
				}
				delete(expectedParticipants, key)
			case EdgeTypeClaimAnchor:
				if _, ok := expectedAnchors[edge.FromNodeKey]; !ok {
					return errors.New("claim anchor edge does not match payload")
				}
				delete(expectedAnchors, edge.FromNodeKey)
			}
		}
		if len(expectedParticipants) != 0 || len(expectedAnchors) != 0 {
			return errors.New("claim payload is missing participant or anchor edges")
		}
	}
	return nil
}

func isClaimNode(nodeType NodeType) bool {
	return slices.Contains([]NodeType{NodeTypeRelationshipClaim, NodeTypeForeshadowingClaim, NodeTypePayoffClaim, NodeTypeContinuityClaim, NodeTypeCausalClaim, NodeTypeShotContinuityClaim}, nodeType)
}

func canonicalObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || raw[0] == 'n' {
		return nil, errors.New("JSON object is required")
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("JSON object is required")
	}
	return canonicalJSON(value)
}

func decodeStrictObject(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func canonicalValueHash(value any) (string, error) {
	encoded, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func (value EvidenceRef) Validate() error {
	if _, err := uuid.Parse(value.DocumentRevisionID); err != nil || value.AbsoluteStart < 0 || value.AbsoluteEnd <= value.AbsoluteStart || !hashPattern.MatchString(value.TextHash) {
		return errors.New("invalid StoryGraph evidence reference")
	}
	return nil
}

type Edge struct {
	EdgeKey     string        `json:"edge_key"`
	EdgeType    EdgeType      `json:"edge_type"`
	FromNodeKey string        `json:"from_node_key"`
	ToNodeKey   string        `json:"to_node_key"`
	Qualifier   EdgeQualifier `json:"qualifier"`
	ContentHash string        `json:"content_hash,omitempty"`
}

type CycleError struct {
	Path []string
}

func (value *CycleError) Error() string {
	return fmt.Sprintf("StoryGraph contains a cycle: %s", strings.Join(value.Path, " -> "))
}

func TopologicalOrder(nodeKeys []string, edges []Edge) ([]string, error) {
	nodes := append([]string(nil), nodeKeys...)
	sort.Strings(nodes)
	indegree := make(map[string]int, len(nodes))
	adjacency := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		if _, exists := indegree[node]; exists {
			return nil, errors.New("duplicate StoryGraph node key")
		}
		indegree[node] = 0
	}
	for _, edge := range edges {
		if _, ok := indegree[edge.FromNodeKey]; !ok {
			return nil, errors.New("StoryGraph edge has a dangling source")
		}
		if _, ok := indegree[edge.ToNodeKey]; !ok {
			return nil, errors.New("StoryGraph edge has a dangling target")
		}
		adjacency[edge.FromNodeKey] = append(adjacency[edge.FromNodeKey], edge.ToNodeKey)
		indegree[edge.ToNodeKey]++
	}
	for node := range adjacency {
		sort.Strings(adjacency[node])
	}
	ready := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if indegree[node] == 0 {
			ready = append(ready, node)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		node := ready[0]
		ready = ready[1:]
		order = append(order, node)
		for _, target := range adjacency[node] {
			indegree[target]--
			if indegree[target] == 0 {
				ready = insertSorted(ready, target)
			}
		}
	}
	if len(order) == len(nodes) {
		return order, nil
	}
	remaining := make(map[string]bool)
	for _, node := range nodes {
		if indegree[node] > 0 {
			remaining[node] = true
		}
	}
	return nil, &CycleError{Path: deterministicCycle(nodes, adjacency, remaining)}
}

func deterministicCycle(nodes []string, adjacency map[string][]string, remaining map[string]bool) []string {
	var shortest []string
	for _, node := range nodes {
		if !remaining[node] {
			continue
		}
		queue := []string{node}
		visited := map[string]bool{node: true}
		parent := make(map[string]string, len(remaining))
		var candidate []string
		for len(queue) > 0 && candidate == nil {
			current := queue[0]
			queue = queue[1:]
			for _, target := range adjacency[current] {
				if !remaining[target] {
					continue
				}
				if target == node && current != node {
					path := []string{current}
					for path[len(path)-1] != node {
						path = append(path, parent[path[len(path)-1]])
					}
					slices.Reverse(path)
					candidate = append(path, node)
					break
				}
				if !visited[target] {
					visited[target] = true
					parent[target] = current
					queue = append(queue, target)
				}
			}
		}
		if candidate != nil && (shortest == nil || len(candidate) < len(shortest) || len(candidate) == len(shortest) && strings.Join(candidate, "\x00") < strings.Join(shortest, "\x00")) {
			shortest = candidate
		}
	}
	return shortest
}

func insertSorted(values []string, value string) []string {
	index, _ := slices.BinarySearch(values, value)
	values = append(values, "")
	copy(values[index+1:], values[index:])
	values[index] = value
	return values
}

func prefixedHash(prefix string, value any) (string, error) {
	encoded, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return prefix + hex.EncodeToString(hash[:]), nil
}

func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(decoded); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(canonical.Bytes()), nil
}

func oneOf(value string, candidates ...string) bool {
	return slices.Contains(candidates, value)
}
