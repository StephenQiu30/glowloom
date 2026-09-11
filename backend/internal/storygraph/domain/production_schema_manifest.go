package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const (
	ProductionSchemaManifestContractID = "storygraph-schema-manifest-contract"
	ProductionCanonicalJSONContractID  = "storygraph-canonical-json-production"
	StoryNodeKeyDerivationID           = "story-node-key-stable"
	StoryEdgeKeyDerivationID           = "story-edge-key-stable"
	ProductionPayloadMetaContractID    = "storygraph-payload-contract-meta-contract"
)

type ProductionSchemaManifest struct {
	ManifestContractID               string                                      `json:"manifest_contract_id"`
	SchemaID                         string                                      `json:"schema_id"`
	CanonicalJSONContractID          string                                      `json:"canonical_json_contract_id"`
	NodeKeyDerivationID              string                                      `json:"node_key_derivation_id"`
	EdgeKeyDerivationID              string                                      `json:"edge_key_derivation_id"`
	NodeDefinitions                  []ProductionNodeManifestDefinition          `json:"node_definitions"`
	PayloadUnionDefinitions          []ProductionPayloadUnionDefinition          `json:"payload_union_definitions"`
	EdgeAliasDefinitions             []ProductionEdgeAliasDefinition             `json:"edge_alias_definitions"`
	EdgeMatrixDefinitions            []ProductionEdgeMatrixDefinition            `json:"edge_matrix_definitions"`
	OwnerCollectionDefinitions       []ProductionOwnerCollectionDefinition       `json:"owner_collection_definitions"`
	CoverageRules                    []ProductionCoverageRuleDefinition          `json:"coverage_rules"`
	CheckpointOwnerFamilyDefinitions []ProductionCheckpointOwnerFamilyDefinition `json:"checkpoint_owner_family_definitions"`
	ExclusionDefinitions             []ProductionExclusionDefinition             `json:"exclusion_definitions"`
}

type ProductionNodeManifestDefinition struct {
	NodeType                 string   `json:"node_type"`
	OwnerKind                string   `json:"owner_kind"`
	AllowedVersionFamilies   []string `json:"allowed_version_families"`
	PayloadContractIDs       []string `json:"payload_contract_ids"`
	PayloadDiscriminantField *string  `json:"payload_discriminant_field"`
	EvidencePolicyID         string   `json:"evidence_policy_id"`
}

type ProductionPayloadUnionDefinition struct {
	NodeType            string  `json:"node_type"`
	DiscriminantField   *string `json:"discriminant_field"`
	DiscriminantValue   *string `json:"discriminant_value"`
	PayloadContractID   string  `json:"payload_contract_id"`
	PayloadContractHash string  `json:"payload_contract_hash"`
}

type ProductionEdgeAliasDefinition struct {
	Alias             string   `json:"alias"`
	ExpandedNodeTypes []string `json:"expanded_node_types"`
}

type ProductionEdgeMatrixDefinition struct {
	MatrixID          string          `json:"matrix_id"`
	RowIndex          int64           `json:"row_index"`
	RowPayload        json.RawMessage `json:"row_payload"`
	ProjectionRuleID  string          `json:"projection_rule_id"`
	CardinalityRuleID string          `json:"cardinality_rule_id"`
}

type ProductionOwnerCollectionDefinition struct {
	MinimumCoverage                  string `json:"minimum_coverage"`
	PhaseRank                        int64  `json:"phase_rank"`
	OwnerKind                        string `json:"owner_kind"`
	VersionFamily                    string `json:"version_family"`
	ScopeKind                        string `json:"scope_kind"`
	ScopeKeyAndCollectionCardinality string `json:"scope_key_and_collection_cardinality"`
	MemberCompleteness               string `json:"member_completeness"`
	ScopeKeyRuleID                   string `json:"scope_key_rule_id"`
	MemberContractID                 string `json:"member_contract_id"`
	CardinalityRuleID                string `json:"cardinality_rule_id"`
	EmptyCollectionPolicy            string `json:"empty_collection_policy"`
}

type ProductionCoverageRuleDefinition struct {
	CoveragePhase           string   `json:"coverage_phase"`
	PhaseRank               int64    `json:"phase_rank"`
	ScopeSetField           string   `json:"scope_set_field"`
	ActivationCondition     string   `json:"activation_condition"`
	PrerequisitePhases      []string `json:"prerequisite_phases"`
	RequiredVersionFamilies []string `json:"required_version_families"`
	DecisionCheckpointIDs   []string `json:"decision_checkpoint_ids"`
	CoverageRuleID          string   `json:"coverage_rule_id"`
}

type ProductionCheckpointOwnerFamilyDefinition struct {
	DecisionCheckpointID string `json:"decision_checkpoint_id"`
	OwnerKind            string `json:"owner_kind"`
	VersionFamily        string `json:"version_family"`
}

type ProductionExclusionDefinition struct {
	ExclusionKind string `json:"exclusion_kind"`
	Literal       string `json:"literal"`
}

type ProductionPayloadContractDefinition struct {
	PayloadMetaContractID string                               `json:"payload_meta_contract_id"`
	PayloadContractID     string                               `json:"payload_contract_id"`
	AllowedNodeTypes      []string                             `json:"allowed_node_types"`
	DiscriminantField     *string                              `json:"discriminant_field"`
	DiscriminantValue     *string                              `json:"discriminant_value"`
	RequiredFields        []string                             `json:"required_fields"`
	NullableFields        []string                             `json:"nullable_fields"`
	PropertySchemas       []ProductionPropertySchemaDefinition `json:"property_schemas"`
	ArraySortRules        []ProductionArraySortRuleDefinition  `json:"array_sort_rules"`
	Invariants            []ProductionInvariantDefinition      `json:"invariants"`
	AdditionalProperties  bool                                 `json:"additional_properties"`
}

type ProductionPropertySchemaDefinition struct {
	JSONPointer                string            `json:"json_pointer"`
	JSONTypes                  []string          `json:"json_types"`
	Format                     *string           `json:"format"`
	EnumValues                 []json.RawMessage `json:"enum_values"`
	Pattern                    *string           `json:"pattern"`
	IntegerMinimum             *int64            `json:"integer_minimum"`
	IntegerMaximum             *int64            `json:"integer_maximum"`
	MinItems                   *int64            `json:"min_items"`
	MaxItems                   *int64            `json:"max_items"`
	ObjectContractID           *string           `json:"object_contract_id"`
	ArrayItemContractID        *string           `json:"array_item_contract_id"`
	ObjectAdditionalProperties *bool             `json:"object_additional_properties"`
}

type ProductionArraySortRuleDefinition struct {
	JSONPointer         string   `json:"json_pointer"`
	SortMode            string   `json:"sort_mode"`
	SortKeyJSONPointers []string `json:"sort_key_json_pointers"`
	Unique              bool     `json:"unique"`
}

type ProductionInvariantDefinition struct {
	InvariantID string `json:"invariant_id"`
}

// DecodeProductionSchemaManifestMetaContract validates the closed Manifest
// hash-root shape and returns its canonical hash. It does not establish that a
// manifest exhausts the production registry and therefore cannot authorize
// publishing on its own.
func DecodeProductionSchemaManifestMetaContract(raw []byte) (ProductionSchemaManifest, string, error) {
	if _, err := platformcanonical.JSON(raw); err != nil {
		return ProductionSchemaManifest{}, "", err
	}
	var value ProductionSchemaManifest
	if err := decodeRequiredObject(raw, &value, productionSchemaManifestFields); err != nil {
		return ProductionSchemaManifest{}, "", err
	}
	if err := validateProductionSchemaManifest(raw, value); err != nil {
		return ProductionSchemaManifest{}, "", err
	}
	hash, err := platformcanonical.Hash(raw)
	return value, hash, err
}

// DecodeProductionPayloadMetaContract validates the closed Payload Contract
// hash-root shape and returns its canonical hash. Registry completeness is a
// separate publication check.
func DecodeProductionPayloadMetaContract(raw []byte) (ProductionPayloadContractDefinition, string, error) {
	if _, err := platformcanonical.JSON(raw); err != nil {
		return ProductionPayloadContractDefinition{}, "", err
	}
	var value ProductionPayloadContractDefinition
	if err := decodeRequiredObject(raw, &value, productionPayloadContractFields); err != nil {
		return ProductionPayloadContractDefinition{}, "", err
	}
	if err := validateProductionPayloadContract(raw, value); err != nil {
		return ProductionPayloadContractDefinition{}, "", err
	}
	hash, err := platformcanonical.Hash(raw)
	return value, hash, err
}

var productionSchemaManifestFields = []string{
	"manifest_contract_id", "schema_id", "canonical_json_contract_id", "node_key_derivation_id",
	"edge_key_derivation_id", "node_definitions", "payload_union_definitions", "edge_alias_definitions",
	"edge_matrix_definitions", "owner_collection_definitions", "coverage_rules",
	"checkpoint_owner_family_definitions", "exclusion_definitions",
}

var productionPayloadContractFields = []string{
	"payload_meta_contract_id", "payload_contract_id", "allowed_node_types", "discriminant_field",
	"discriminant_value", "required_fields", "nullable_fields", "property_schemas", "array_sort_rules",
	"invariants", "additional_properties",
}

func validateProductionSchemaManifest(raw []byte, value ProductionSchemaManifest) error {
	if value.ManifestContractID != ProductionSchemaManifestContractID || value.SchemaID != ProductionSchemaID ||
		value.CanonicalJSONContractID != ProductionCanonicalJSONContractID || value.NodeKeyDerivationID != StoryNodeKeyDerivationID ||
		value.EdgeKeyDerivationID != StoryEdgeKeyDerivationID || len(value.NodeDefinitions) == 0 ||
		len(value.PayloadUnionDefinitions) == 0 || len(value.EdgeAliasDefinitions) == 0 || len(value.EdgeMatrixDefinitions) == 0 ||
		len(value.OwnerCollectionDefinitions) == 0 || len(value.CoverageRules) == 0 ||
		len(value.CheckpointOwnerFamilyDefinitions) == 0 || len(value.ExclusionDefinitions) == 0 {
		return errors.New("invalid Production Schema Manifest identity")
	}
	fields, err := rawObjectFields(raw)
	if err != nil {
		return err
	}
	if err = validateProductionManifestNodes(fields["node_definitions"], value.NodeDefinitions); err != nil {
		return err
	}
	if err = validateProductionManifestPayloads(fields["payload_union_definitions"], value.PayloadUnionDefinitions, value.NodeDefinitions); err != nil {
		return err
	}
	if err = validateProductionManifestAliases(fields["edge_alias_definitions"], value.EdgeAliasDefinitions); err != nil {
		return err
	}
	if err = validateProductionManifestMatrices(fields["edge_matrix_definitions"], value.EdgeMatrixDefinitions); err != nil {
		return err
	}
	if err = validateProductionManifestCollections(fields["owner_collection_definitions"], value.OwnerCollectionDefinitions); err != nil {
		return err
	}
	if err = validateProductionManifestCoverage(fields["coverage_rules"], value.CoverageRules); err != nil {
		return err
	}
	if err = validateProductionManifestCheckpoints(fields["checkpoint_owner_family_definitions"], value.CheckpointOwnerFamilyDefinitions); err != nil {
		return err
	}
	return validateProductionManifestExclusions(fields["exclusion_definitions"], value.ExclusionDefinitions)
}

func validateProductionManifestNodes(raw json.RawMessage, values []ProductionNodeManifestDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema node definitions")
	}
	previous := ""
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionNodeManifestDefinition{}, []string{
			"node_type", "owner_kind", "allowed_version_families", "payload_contract_ids", "payload_discriminant_field", "evidence_policy_id",
		}); err != nil || value.NodeType <= previous || !productionStableKey(value.NodeType) || !productionStableKey(value.OwnerKind) ||
			!sortedUniqueNFCStrings(value.AllowedVersionFamilies, true) || !sortedUniqueNFCStrings(value.PayloadContractIDs, true) ||
			!nullableStableKey(value.PayloadDiscriminantField) || !oneOf(value.EvidencePolicyID, productionEvidenceNone, productionEvidenceRoot, productionEvidenceRequired, productionEvidenceOrCreatorDecision) {
			return errors.New("invalid Production Schema node definition")
		}
		previous = value.NodeType
	}
	return nil
}

func validateProductionManifestPayloads(raw json.RawMessage, values []ProductionPayloadUnionDefinition, nodes []ProductionNodeManifestDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema payload definitions")
	}
	nodeByType := make(map[string]ProductionNodeManifestDefinition, len(nodes))
	contractsByNode := make(map[string]map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeByType[node.NodeType] = node
		contractsByNode[node.NodeType] = make(map[string]struct{}, len(node.PayloadContractIDs))
	}
	previous := ""
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionPayloadUnionDefinition{}, []string{
			"node_type", "discriminant_field", "discriminant_value", "payload_contract_id", "payload_contract_hash",
		}); err != nil {
			return err
		}
		key := value.NodeType + "\x00" + pointerText(value.DiscriminantField) + "\x00" + pointerText(value.DiscriminantValue) + "\x00" + value.PayloadContractID
		node, exists := nodeByType[value.NodeType]
		if key <= previous || !exists || !productionStableKey(value.PayloadContractID) || !hashPattern.MatchString(value.PayloadContractHash) ||
			!nullableStableKey(value.DiscriminantField) || !nullableStableKey(value.DiscriminantValue) ||
			(value.DiscriminantField == nil) != (value.DiscriminantValue == nil) || !slices.Contains(node.PayloadContractIDs, value.PayloadContractID) ||
			!sameNullableString(node.PayloadDiscriminantField, value.DiscriminantField) {
			return errors.New("invalid Production Schema payload definition")
		}
		contractsByNode[value.NodeType][value.PayloadContractID] = struct{}{}
		previous = key
	}
	for _, node := range nodes {
		contracts := contractsByNode[node.NodeType]
		if len(contracts) != len(node.PayloadContractIDs) {
			return errors.New("Production Schema payload union coverage has drifted")
		}
		for _, contractID := range node.PayloadContractIDs {
			if _, exists := contracts[contractID]; !exists {
				return errors.New("Production Schema payload union coverage has drifted")
			}
		}
	}
	return nil
}

func validateProductionManifestAliases(raw json.RawMessage, values []ProductionEdgeAliasDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema edge aliases")
	}
	previous := ""
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionEdgeAliasDefinition{}, []string{"alias", "expanded_node_types"}); err != nil ||
			value.Alias <= previous || !productionStableKey(value.Alias) || !sortedUniqueNFCStrings(value.ExpandedNodeTypes, true) {
			return errors.New("invalid Production Schema edge alias")
		}
		previous = value.Alias
	}
	return nil
}

var productionMatrixFields = map[string][]string{
	"edge-type-matrix-contract":                            {"edge_type", "allowed_source_to_target", "qualifier_and_cardinality"},
	"claim-cardinality-matrix-contract":                    {"claim_branch", "participant_cardinality", "anchor_cardinality", "state_cardinality"},
	"materialization-matrix-contract":                      {"source", "target", "binding_role", "target_cardinality"},
	"reference-planning-matrix-contract":                   {"source", "reference_role", "target_payload_field"},
	"reference-target-input-compatibility-matrix-contract": {"target_kind", "target_uniqueness_key", "expected_target_coverage", "identity_refs", "specification_refs", "state_refs", "style_refs", "scene_refs", "occurrence_refs", "interaction_refs", "target_dependencies"},
	"reference-target-result-matrix-contract":              {"target_kind", "unique_allowed_result"},
	"reference-target-activation-matrix-contract":          {"target_activation_condition", "required_cardinality", "optional_cardinality", "not_generated_cardinality"},
	"reference-binding-matrix-contract":                    {"target_binding", "source", "reference_role", "target_cardinality"},
}

func validateProductionManifestMatrices(raw json.RawMessage, values []ProductionEdgeMatrixDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema edge matrices")
	}
	previous := ""
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionEdgeMatrixDefinition{}, []string{
			"matrix_id", "row_index", "row_payload", "projection_rule_id", "cardinality_rule_id",
		}); err != nil {
			return err
		}
		rowFields, exists := productionMatrixFields[value.MatrixID]
		key := fmt.Sprintf("%s\x00%020d", value.MatrixID, value.RowIndex)
		prefix := fmt.Sprintf("storygraph-production/%s/row-%d", value.MatrixID, value.RowIndex)
		if !exists || key <= previous || value.RowIndex < 1 || value.RowIndex > productionMaximumSafeInteger ||
			value.ProjectionRuleID != prefix+"-projection-contract" || value.CardinalityRuleID != prefix+"-cardinality-contract" ||
			validateMatrixRow(value.RowPayload, rowFields) != nil {
			return errors.New("invalid Production Schema edge matrix definition")
		}
		previous = key
	}
	return nil
}

func validateMatrixRow(raw json.RawMessage, fields []string) error {
	var value map[string]string
	if err := decodeRequiredObject(raw, &value, fields); err != nil {
		return err
	}
	for _, field := range fields {
		if !productionStableKey(value[field]) {
			return errors.New("Production Schema matrix cell is empty or not NFC")
		}
	}
	return nil
}

func validateProductionManifestCollections(raw json.RawMessage, values []ProductionOwnerCollectionDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema Owner Collections")
	}
	previous := ""
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionOwnerCollectionDefinition{}, []string{
			"minimum_coverage", "phase_rank", "owner_kind", "version_family", "scope_kind", "scope_key_and_collection_cardinality",
			"member_completeness", "scope_key_rule_id", "member_contract_id", "cardinality_rule_id", "empty_collection_policy",
		}); err != nil {
			return err
		}
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s", value.PhaseRank, value.OwnerKind, value.VersionFamily, value.ScopeKind)
		prefix := "storygraph-production/owner-collection/" + value.VersionFamily
		if key <= previous || !validPhaseRank(value.MinimumCoverage, value.PhaseRank) || !productionStableKey(value.OwnerKind) || !productionStableKey(value.VersionFamily) ||
			!oneOf(value.ScopeKind, "project", "episode", "scene", "reference_plan") || !productionStableKey(value.ScopeKeyAndCollectionCardinality) ||
			!productionStableKey(value.MemberCompleteness) || value.ScopeKeyRuleID != prefix+"-scope-key-contract" ||
			value.MemberContractID != prefix+"-members-contract" || value.CardinalityRuleID != prefix+"-cardinality-contract" ||
			!oneOf(value.EmptyCollectionPolicy, "forbidden", "receipt_bound", "scope_rebase_conditional") {
			return errors.New("invalid Production Schema Owner Collection definition")
		}
		previous = key
	}
	return nil
}

func validateProductionManifestCoverage(raw json.RawMessage, values []ProductionCoverageRuleDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema Coverage rules")
	}
	previousRank := int64(-1)
	for index, value := range values {
		if err = decodeRequiredObject(items[index], &ProductionCoverageRuleDefinition{}, []string{
			"coverage_phase", "phase_rank", "scope_set_field", "activation_condition", "prerequisite_phases",
			"required_version_families", "decision_checkpoint_ids", "coverage_rule_id",
		}); err != nil || value.PhaseRank <= previousRank || !validPhaseRank(value.CoveragePhase, value.PhaseRank) ||
			value.ScopeSetField != value.CoveragePhase+"_scope_keys" || value.ActivationCondition != "scope_set_non_empty" ||
			!sortedUniquePhases(value.PrerequisitePhases) || !sortedUniqueNFCStrings(value.RequiredVersionFamilies, true) ||
			!sortedUniqueNFCStrings(value.DecisionCheckpointIDs, true) || value.CoverageRuleID != "storygraph-production/coverage/"+value.CoveragePhase+"-contract" {
			return errors.New("invalid Production Schema Coverage rule")
		}
		previousRank = value.PhaseRank
	}
	return nil
}

func validateProductionManifestCheckpoints(raw json.RawMessage, values []ProductionCheckpointOwnerFamilyDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema checkpoints")
	}
	previous := ""
	for index, value := range values {
		key := value.DecisionCheckpointID + "\x00" + value.OwnerKind + "\x00" + value.VersionFamily
		if err = decodeRequiredObject(items[index], &ProductionCheckpointOwnerFamilyDefinition{}, []string{"decision_checkpoint_id", "owner_kind", "version_family"}); err != nil ||
			key <= previous || !productionStableKey(value.DecisionCheckpointID) || !productionStableKey(value.OwnerKind) || !productionStableKey(value.VersionFamily) {
			return errors.New("invalid Production Schema checkpoint definition")
		}
		previous = key
	}
	return nil
}

func validateProductionManifestExclusions(raw json.RawMessage, values []ProductionExclusionDefinition) error {
	items, err := rawArray(raw)
	if err != nil || len(items) != len(values) {
		return errors.New("invalid Production Schema exclusions")
	}
	previous := ""
	for index, value := range values {
		key := value.ExclusionKind + "\x00" + value.Literal
		if err = decodeRequiredObject(items[index], &ProductionExclusionDefinition{}, []string{"exclusion_kind", "literal"}); err != nil ||
			key <= previous || !oneOf(value.ExclusionKind, "node_type", "compiler_input_kind", "runtime_kind", "query_view_kind") || !productionStableKey(value.Literal) {
			return errors.New("invalid Production Schema exclusion")
		}
		previous = key
	}
	return nil
}

func validateProductionPayloadContract(raw []byte, value ProductionPayloadContractDefinition) error {
	if value.PayloadMetaContractID != ProductionPayloadMetaContractID || !productionStableKey(value.PayloadContractID) ||
		!sortedUniqueNFCStrings(value.AllowedNodeTypes, true) || (value.DiscriminantField == nil) != (value.DiscriminantValue == nil) ||
		!nullableStableKey(value.DiscriminantField) || !nullableStableKey(value.DiscriminantValue) ||
		!sortedUniqueJSONPointers(value.RequiredFields) || !sortedUniqueJSONPointers(value.NullableFields) || len(value.PropertySchemas) == 0 ||
		value.AdditionalProperties {
		return errors.New("invalid Production Payload Contract identity")
	}
	fields, err := rawObjectFields(raw)
	if err != nil {
		return err
	}
	propertyRaw, err := rawArray(fields["property_schemas"])
	if err != nil || len(propertyRaw) != len(value.PropertySchemas) {
		return errors.New("invalid Production Payload property schemas")
	}
	properties := make(map[string]ProductionPropertySchemaDefinition, len(value.PropertySchemas))
	previous := ""
	for index, property := range value.PropertySchemas {
		if err = decodeRequiredObject(propertyRaw[index], &ProductionPropertySchemaDefinition{}, []string{
			"json_pointer", "json_types", "format", "enum_values", "pattern", "integer_minimum", "integer_maximum",
			"min_items", "max_items", "object_contract_id", "array_item_contract_id", "object_additional_properties",
		}); err != nil || (index > 0 && property.JSONPointer <= previous) || !validJSONPointer(property.JSONPointer) ||
			validateProductionPropertySchema(property) != nil {
			return errors.New("invalid Production Payload property schema")
		}
		properties[property.JSONPointer] = property
		previous = property.JSONPointer
	}
	root, exists := properties[""]
	if !exists || !slices.Equal(root.JSONTypes, []string{"object"}) || root.ObjectContractID == nil || *root.ObjectContractID != value.PayloadContractID ||
		!subsetOfProperties(value.RequiredFields, properties) || !subsetOfProperties(value.NullableFields, properties) {
		return errors.New("Production Payload fields do not match property schemas")
	}
	expectedNullable := make([]string, 0)
	for pointer, property := range properties {
		if slices.Contains(property.JSONTypes, "null") {
			expectedNullable = append(expectedNullable, pointer)
		}
	}
	slices.Sort(expectedNullable)
	if !slices.Equal(expectedNullable, value.NullableFields) {
		return errors.New("Production Payload nullable fields have drifted")
	}
	ruleRaw, err := rawArray(fields["array_sort_rules"])
	if err != nil || len(ruleRaw) != len(value.ArraySortRules) {
		return errors.New("invalid Production Payload array sort rules")
	}
	arrayRules := make(map[string]struct{}, len(value.ArraySortRules))
	previous = ""
	for index, rule := range value.ArraySortRules {
		if err = decodeRequiredObject(ruleRaw[index], &ProductionArraySortRuleDefinition{}, []string{"json_pointer", "sort_mode", "sort_key_json_pointers", "unique"}); err != nil ||
			(index > 0 && rule.JSONPointer <= previous) || !validJSONPointer(rule.JSONPointer) || !oneOf(rule.SortMode, "utf8_ascending", "ascii_ascending", "integer_ascending", "canonical_json_ascending", "owner_node_ref_tuple_ascending", "evidence_ref_tuple_ascending") ||
			!sortedUniqueJSONPointers(rule.SortKeyJSONPointers) || !validSortKeyPointers(rule.SortMode, rule.SortKeyJSONPointers) || !rule.Unique {
			return errors.New("invalid Production Payload array sort rule")
		}
		property, exists := properties[rule.JSONPointer]
		if !exists || !slices.Contains(property.JSONTypes, "array") {
			return errors.New("Production Payload sort rule targets a non-array")
		}
		arrayRules[rule.JSONPointer] = struct{}{}
		previous = rule.JSONPointer
	}
	for pointer, property := range properties {
		_, hasRule := arrayRules[pointer]
		if slices.Contains(property.JSONTypes, "array") != hasRule {
			return errors.New("Production Payload array sort coverage has drifted")
		}
	}
	invariantRaw, err := rawArray(fields["invariants"])
	if err != nil || len(invariantRaw) != len(value.Invariants) {
		return errors.New("invalid Production Payload invariants")
	}
	previous = ""
	for index, invariant := range value.Invariants {
		if err = decodeRequiredObject(invariantRaw[index], &ProductionInvariantDefinition{}, []string{"invariant_id"}); err != nil ||
			invariant.InvariantID <= previous || !productionStableKey(invariant.InvariantID) {
			return errors.New("invalid Production Payload invariant")
		}
		previous = invariant.InvariantID
	}
	return nil
}

func validSortKeyPointers(mode string, pointers []string) bool {
	switch mode {
	case "utf8_ascending", "ascii_ascending", "integer_ascending":
		return slices.Equal(pointers, []string{""})
	case "owner_node_ref_tuple_ascending", "evidence_ref_tuple_ascending":
		return len(pointers) == 0
	case "canonical_json_ascending":
		return len(pointers) > 0
	default:
		return false
	}
}

func validateProductionPropertySchema(value ProductionPropertySchemaDefinition) error {
	if !sortedUniqueAllowedStrings(value.JSONTypes, []string{"array", "boolean", "integer", "null", "object", "string"}, true) ||
		!nullableStableKey(value.Format) || !nullableStableKey(value.Pattern) || !nullableStableKey(value.ObjectContractID) ||
		!nullableStableKey(value.ArrayItemContractID) || value.EnumValues == nil || validateCanonicalScalarSet(value.EnumValues) != nil {
		return errors.New("invalid Production Payload property metadata")
	}
	baseTypes := 0
	for _, jsonType := range value.JSONTypes {
		if jsonType != "null" {
			baseTypes++
		}
	}
	if baseTypes != 1 {
		return errors.New("Production Payload property must have exactly one non-null type")
	}
	isObject, isArray, isInteger := slices.Contains(value.JSONTypes, "object"), slices.Contains(value.JSONTypes, "array"), slices.Contains(value.JSONTypes, "integer")
	isString := slices.Contains(value.JSONTypes, "string")
	if value.Format != nil && (!isString || value.Pattern != nil || len(value.EnumValues) != 0 || !oneOf(*value.Format,
		"nfc-nonempty", "uuid-lowercase-hyphenated", "sha256-lowercase-hex", "stable-key-contract",
		"scene-scope-key-contract", "story-time-key-contract", "sequence-key-contract", "descriptor-key-contract",
		"story-node-key-stable", "story-edge-key-stable", "rfc3339-utc-canonical-contract")) {
		return errors.New("Production Payload property format is invalid")
	}
	if !isString && value.Pattern != nil {
		return errors.New("Production Payload non-string has a pattern")
	}
	if isObject {
		if value.ObjectContractID == nil || value.ObjectAdditionalProperties == nil || *value.ObjectAdditionalProperties {
			return errors.New("Production Payload object contract is incomplete")
		}
	} else if value.ObjectContractID != nil || value.ObjectAdditionalProperties != nil {
		return errors.New("Production Payload non-object has object metadata")
	}
	if isArray {
		if value.ArrayItemContractID == nil || value.MinItems == nil || *value.MinItems < 0 || *value.MinItems > productionMaximumSafeInteger ||
			(value.MaxItems != nil && (*value.MaxItems < *value.MinItems || *value.MaxItems > productionMaximumSafeInteger)) {
			return errors.New("Production Payload array contract is incomplete")
		}
	} else if value.ArrayItemContractID != nil || value.MinItems != nil || value.MaxItems != nil {
		return errors.New("Production Payload non-array has array metadata")
	}
	if !isInteger && (value.IntegerMinimum != nil || value.IntegerMaximum != nil) {
		return errors.New("Production Payload non-integer has integer bounds")
	}
	if isInteger && value.IntegerMinimum != nil && value.IntegerMaximum != nil && *value.IntegerMinimum > *value.IntegerMaximum {
		return errors.New("Production Payload integer bounds are reversed")
	}
	return nil
}

func decodeRequiredObject(raw []byte, destination any, required []string) error {
	if err := decodeStrictObject(raw, destination); err != nil {
		return err
	}
	fields, err := rawObjectFields(raw)
	if err != nil || len(fields) != len(required) {
		return errors.New("JSON object field set does not match its meta-contract")
	}
	for _, field := range required {
		if _, exists := fields[field]; !exists {
			return fmt.Errorf("required JSON field %s is missing", field)
		}
	}
	return nil
}

func rawObjectFields(raw []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("JSON object is required")
	}
	return fields, nil
}

func rawArray(raw []byte) ([]json.RawMessage, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, errors.New("JSON array is required")
	}
	return values, nil
}

func sortedUniqueNFCStrings(values []string, requireNonempty bool) bool {
	if values == nil || requireNonempty && len(values) == 0 {
		return false
	}
	for index, value := range values {
		if !productionStableKey(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func sortedUniqueAllowedStrings(values, allowed []string, requireNonempty bool) bool {
	if !sortedUniqueNFCStrings(values, requireNonempty) {
		return false
	}
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return false
		}
	}
	return true
}

func sortedUniquePhases(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !oneOf(value, "p0", "p1", "p2", "p3") || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func sortedUniqueJSONPointers(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !validJSONPointer(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func validJSONPointer(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") || !productionStableKey(value) {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			continue
		}
		if index+1 >= len(value) || value[index+1] != '0' && value[index+1] != '1' {
			return false
		}
		index++
	}
	return true
}

func validateCanonicalScalarSet(values []json.RawMessage) error {
	previous := []byte(nil)
	for _, raw := range values {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || bytes.ContainsAny(trimmed[:1], "[{") {
			return errors.New("enum value must be a JSON scalar")
		}
		canonical, err := platformcanonical.JSON(raw)
		if err != nil || previous != nil && bytes.Compare(previous, canonical) >= 0 {
			return errors.New("enum values are not canonical, sorted, and unique")
		}
		previous = canonical
	}
	return nil
}

func subsetOfProperties(values []string, properties map[string]ProductionPropertySchemaDefinition) bool {
	for _, value := range values {
		if _, exists := properties[value]; !exists {
			return false
		}
	}
	return true
}

func validPhaseRank(phase string, rank int64) bool {
	return map[string]int64{"p0": 0, "p1": 1, "p2": 2, "p3": 3}[phase] == rank && oneOf(phase, "p0", "p1", "p2", "p3")
}

func nullableStableKey(value *string) bool {
	return value == nil || productionStableKey(*value)
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sameNullableString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
