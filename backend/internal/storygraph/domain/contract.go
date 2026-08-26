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

const SchemaVersion = "storygraph-v1"

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
	NodeTypeSourceRevision               NodeType = "source_revision"
	NodeTypeSourceEvidence               NodeType = "source_evidence"
	NodeTypePolicySnapshot               NodeType = "policy_snapshot"
	NodeTypeEffectiveStyleSnapshot       NodeType = "effective_style_snapshot"
	NodeTypeAssetIdentity                NodeType = "asset_identity"
	NodeTypeCharacterSpecification       NodeType = "character_specification"
	NodeTypeLocationSpecification        NodeType = "location_specification"
	NodeTypePropSpecification            NodeType = "prop_specification"
	NodeTypeAssetState                   NodeType = "asset_state"
	NodeTypeProductionBinding            NodeType = "production_binding"
	NodeTypeWorldRule                    NodeType = "world_rule"
	NodeTypeStoryArc                     NodeType = "story_arc"
	NodeTypePlotThread                   NodeType = "plot_thread"
	NodeTypeRelationshipClaim            NodeType = "relationship_claim"
	NodeTypeForeshadowingClaim           NodeType = "foreshadowing_claim"
	NodeTypePayoffClaim                  NodeType = "payoff_claim"
	NodeTypeEpisode                      NodeType = "episode"
	NodeTypeScene                        NodeType = "scene"
	NodeTypeDialogue                     NodeType = "dialogue"
	NodeTypeNarrativeBeat                NodeType = "narrative_beat"
	NodeTypeOccurrence                   NodeType = "occurrence"
	NodeTypeContinuityClaim              NodeType = "continuity_claim"
	NodeTypeCausalClaim                  NodeType = "causal_claim"
	NodeTypeShot                         NodeType = "shot"
	NodeTypeShotContinuityClaim          NodeType = "shot_continuity_claim"
	NodeTypeGenerationTarget             NodeType = "generation_target"
	NodeTypeArtifact                     NodeType = "artifact"
	NodeTypeAssetVersion                 NodeType = "asset_version"
	NodeTypeShotProductionBindingVersion NodeType = "shot_production_binding_version"
	NodeTypeShotImageBindingVersion      NodeType = "shot_image_binding_version"
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
	NodeTypeGenerationTarget: "generation",
}

type OwnerRef struct {
	OwnerKind      string `json:"owner_kind"`
	OwnerLogicalID string `json:"owner_logical_id"`
	FragmentKey    string `json:"fragment_key,omitempty"`
	OwnerVersionID string `json:"owner_version_id"`
	OwnerRevision  int64  `json:"owner_revision"`
	ContentHash    string `json:"content_hash"`
}

func (value OwnerRef) validate(nodeType NodeType) error {
	expectedOwner, ok := nodeOwners[nodeType]
	if !ok || value.OwnerKind != expectedOwner || strings.TrimSpace(value.OwnerLogicalID) == "" || value.OwnerRevision < 1 || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid StoryGraph owner reference")
	}
	if _, err := uuid.Parse(value.OwnerVersionID); err != nil {
		return errors.New("invalid StoryGraph owner reference")
	}
	return nil
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
	}{"story-node-key-v1", nodeType, owner.OwnerKind, owner.OwnerLogicalID, owner.FragmentKey}
	return prefixedHash("sgn_", material)
}

type EdgeType string

const (
	EdgeTypeContains               EdgeType = "contains"
	EdgeTypeDerivedFrom            EdgeType = "derived_from"
	EdgeTypeDescribesIdentity      EdgeType = "describes_identity"
	EdgeTypeHasState               EdgeType = "has_state"
	EdgeTypePrecedes               EdgeType = "precedes"
	EdgeTypeAnchorsOccurrence      EdgeType = "anchors_occurrence"
	EdgeTypeInstantiatesOccurrence EdgeType = "instantiates_occurrence"
	EdgeTypeRealizes               EdgeType = "realizes"
	EdgeTypeInforms                EdgeType = "informs"
	EdgeTypeConstrains             EdgeType = "constrains"
	EdgeTypeMaterializes           EdgeType = "materializes"
	EdgeTypeBindsInput             EdgeType = "binds_input"
	EdgeTypeFeedsGeneration        EdgeType = "feeds_generation"
	EdgeTypeBindsOutput            EdgeType = "binds_output"
	EdgeTypeSupports               EdgeType = "supports"
	EdgeTypeClaimParticipant       EdgeType = "claim_participant"
	EdgeTypeClaimAnchor            EdgeType = "claim_anchor"
	EdgeTypeSupersedes             EdgeType = "supersedes"
)

var edgeTypes = []EdgeType{
	EdgeTypeContains, EdgeTypeDerivedFrom, EdgeTypeDescribesIdentity, EdgeTypeHasState, EdgeTypePrecedes,
	EdgeTypeAnchorsOccurrence, EdgeTypeInstantiatesOccurrence, EdgeTypeRealizes, EdgeTypeInforms, EdgeTypeConstrains,
	EdgeTypeMaterializes, EdgeTypeBindsInput, EdgeTypeFeedsGeneration, EdgeTypeBindsOutput, EdgeTypeSupports,
	EdgeTypeClaimParticipant, EdgeTypeClaimAnchor, EdgeTypeSupersedes,
}

type EdgeQualifier struct {
	BindingRole     string `json:"binding_role,omitempty"`
	ParticipantRole string `json:"participant_role,omitempty"`
	AnchorRole      string `json:"anchor_role,omitempty"`
	SequenceKey     string `json:"sequence_key,omitempty"`
}

func (value EdgeQualifier) validate(edgeType EdgeType) error {
	if !slices.Contains(edgeTypes, edgeType) {
		return errors.New("unknown StoryGraph edge type")
	}
	switch edgeType {
	case EdgeTypeMaterializes:
		if !oneOf(value.BindingRole, "specification", "state", "asset", "asset_version") || value.ParticipantRole != "" || value.AnchorRole != "" || value.SequenceKey != "" {
			return errors.New("invalid materializes qualifier")
		}
	case EdgeTypeClaimParticipant:
		if !oneOf(value.ParticipantRole, "subject", "object", "participant") || value.BindingRole != "" || value.AnchorRole != "" || value.SequenceKey != "" {
			return errors.New("invalid claim participant qualifier")
		}
	case EdgeTypeClaimAnchor:
		if strings.TrimSpace(value.AnchorRole) == "" || value.BindingRole != "" || value.ParticipantRole != "" || value.SequenceKey != "" {
			return errors.New("invalid claim anchor qualifier")
		}
	case EdgeTypeContains, EdgeTypePrecedes:
		if value.BindingRole != "" || value.ParticipantRole != "" || value.AnchorRole != "" {
			return errors.New("invalid sequence qualifier")
		}
	default:
		if value != (EdgeQualifier{}) {
			return errors.New("edge type does not accept a qualifier")
		}
	}
	return nil
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
	}{"story-edge-key-v1", edgeType, fromNodeKey, toNodeKey, qualifier}
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
	if snapshot.SchemaVersion != SchemaVersion || snapshot.Nodes == nil || snapshot.Edges == nil {
		return CanonicalSnapshot{}, errors.New("invalid StoryGraph snapshot envelope")
	}
	nodes := append(make([]Node, 0, len(snapshot.Nodes)), snapshot.Nodes...)
	seenNodes := make(map[string]struct{}, len(nodes))
	for index := range nodes {
		node, err := canonicalizeNode(nodes[index])
		if err != nil {
			return CanonicalSnapshot{}, err
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

	keys := make([]string, 0, len(nodes))
	for _, node := range nodes {
		keys = append(keys, node.StoryNodeKey)
	}
	if _, err := TopologicalOrder(keys, edges); err != nil {
		return CanonicalSnapshot{}, err
	}
	if err := validateClaimEdges(nodes, edges); err != nil {
		return CanonicalSnapshot{}, err
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
	}{SchemaVersion, topologyNodes, topologyEdges})
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	contentHash, err := canonicalValueHash(struct {
		SchemaVersion string `json:"schema_version"`
		Nodes         []Node `json:"nodes"`
		Edges         []Edge `json:"edges"`
	}{SchemaVersion, nodes, edges})
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	return CanonicalSnapshot{SchemaVersion: SchemaVersion, Nodes: nodes, Edges: edges, TopologyHash: topologyHash, ContentHash: contentHash}, nil
}

func canonicalizeNode(node Node) (Node, error) {
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
	if isClaimNode(node.NodeType) {
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
