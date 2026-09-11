package domain

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	productionOwnerNodeRefContract = "storygraph-production/owner-node-ref-contract"
	productionAuditRefContract     = "storygraph-production/content-addressed-audit-ref-contract"
	productionClaimScopeContract   = "storygraph-production/claim-scope-contract"
	productionStoryTimeContract    = "storygraph-production/story-time-range-contract"
	productionRationalContract     = "storygraph-production/positive-rational-contract"
	productionConstraintContract   = "storygraph-production/constraint-refs-contract"
	productionTargetRefsContract   = "storygraph-production/reference-target-owner-refs-contract"
	ownerRefArrayItemContract      = "storygraph-production/array-item/owner-node-ref-contract"
	scopeKeyArrayItemContract      = "storygraph-production/array-item/scene-scope-key-contract"

	projectionInvariant = "storygraph-production/invariant/projection-hash-equals-owner-contract"
	refOnlyInvariant    = "storygraph-production/invariant/ref-payload-no-business-copy-contract"
	evidenceInvariant   = "storygraph-production/invariant/evidence-xor-creator-decision-contract"
)

type productionField struct {
	name      string
	kind      string
	required  bool
	nullable  bool
	format    string
	values    []string
	pattern   string
	minimum   *int64
	maximum   *int64
	minItems  int64
	objectID  string
	arrayItem string
	sortMode  string
	sortKeys  []string
	children  []productionField
}

type productionPayloadSpec struct {
	id            string
	allowed       []string
	discriminant  string
	discriminantV string
	fields        []productionField
	invariants    []string
}

func productionPayloadDefinitions() ([]ProductionPayloadContractDefinition, error) {
	specs := productionPayloadSpecs()
	values := make([]ProductionPayloadContractDefinition, 0, len(specs))
	for _, spec := range specs {
		value, err := buildProductionPayloadDefinition(spec)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	slices.SortFunc(values, func(left, right ProductionPayloadContractDefinition) int {
		if left.PayloadContractID < right.PayloadContractID {
			return -1
		}
		if left.PayloadContractID > right.PayloadContractID {
			return 1
		}
		return 0
	})
	return values, nil
}

func productionPayloadSpecs() []productionPayloadSpec {
	projection := []productionField{
		stringEnum("payload_contract_id"),
		stringFormat("projection_hash", "sha256-lowercase-hex"),
	}
	refOnly := []struct{ node, id string }{
		{"source_revision", "storygraph-production/source_revision-ref-payload-contract"},
		{"source_evidence", "storygraph-production/source_evidence-ref-payload-contract"},
		{"policy_snapshot", "storygraph-production/policy_snapshot-ref-payload-contract"},
		{"effective_style_snapshot", "storygraph-production/effective_style_snapshot-ref-payload-contract"},
		{"episode", "storygraph-production/episode-ref-payload-contract"},
		{"scene", "storygraph-production/scene-ref-payload-contract"},
		{"dialogue", "storygraph-production/dialogue-ref-payload-contract"},
		{"narrative_beat", "storygraph-production/narrative_beat-ref-payload-contract"},
		{"artifact", "storygraph-production/artifact-ref-payload-contract"},
		{"approved_reference_plan_version", "storygraph-production/approved_reference_plan_version-ref-payload-contract"},
	}
	specs := make([]productionPayloadSpec, 0, 25)
	for _, item := range refOnly {
		specs = append(specs, payloadSpec(item.id, []string{item.node}, projection, refOnlyInvariant))
	}
	audit := objectField("creator_decision_ref", productionAuditRefContract, true, true, auditRefFields())
	assetKind := stringEnum("asset_kind", "character", "location", "prop")
	specs = append(specs,
		payloadSpec("storygraph-production/auditable-bible-fact-payload-contract", []string{"plot_thread", "story_arc", "world_rule"}, appendFields(projection, audit), evidenceInvariant),
		payloadSpec("storygraph-production/asset-identity-payload-contract", []string{"asset_identity"}, appendFields(projection, assetKind, audit), evidenceInvariant, "storygraph-production/invariant/asset-kind-and-state-range-contract"),
		payloadSpec("storygraph-production/specification-payload-contract", []string{"character_specification", "location_specification", "prop_specification"}, appendFields(projection, assetKind, audit), evidenceInvariant, "storygraph-production/invariant/asset-kind-and-state-range-contract"),
		payloadSpec("storygraph-production/asset-state-payload-contract", []string{"asset_state"}, appendFields(projection, assetKind, stringFormat("state_key", "stable-key-contract"), objectField("story_time_range", productionStoryTimeContract, true, true, storyTimeFields()), audit), evidenceInvariant, "storygraph-production/invariant/asset-kind-and-state-range-contract"),
		payloadSpec("storygraph-production/production-binding-payload-contract", []string{"production_binding"}, appendFields(projection, ownerRef("asset_identity_ref", true, false), ownerRef("specification_ref", true, false), ownerRefArray("state_refs", 1)), "storygraph-production/invariant/production-binding-edge-equivalence-contract"),
		payloadSpec("storygraph-production/occurrence-payload-contract", []string{"occurrence"}, appendFields(projection, ownerRef("asset_identity_ref", true, false), ownerRef("asset_state_ref", true, false), ownerRef("scene_ref", true, false), ownerRef("beat_ref", true, true), audit), evidenceInvariant, "storygraph-production/invariant/occurrence-edge-equivalence-contract"),
		payloadSpec("storygraph-production/asset-version-payload-contract", []string{"asset_version"}, appendFields(projection,
			stringEnum("purpose", "character_appearance", "character_identity_anchor", "location_board", "prop_sheet"), ownerRef("asset_identity_ref", true, false), ownerRef("specification_ref", true, false), ownerRef("asset_state_ref", true, false), ownerRef("identity_anchor_asset_version_ref", true, true), ownerRef("selected_artifact_ref", true, false), ownerRef("fulfilled_reference_target_ref", true, false), constraintsField()), "storygraph-production/invariant/asset-version-anchor-target-artifact-constraint-contract"),
		payloadSpec("storygraph-production/reference-plan-target-payload-contract", []string{"reference_plan_target"}, appendFields(projection,
			stringEnum("target_kind", "character_appearance", "character_identity_anchor", "interaction_composition", "location_board", "prop_sheet", "scene_composition"), stringEnum("fulfillment", "not_generated", "optional", "required"),
			objectField("target_owner_refs", productionTargetRefsContract, true, false, targetOwnerRefFields()), scopeKeyArray("coverage_scope_keys", 1), ownerRefArray("depends_on_target_refs", 0), constraintsField()), "storygraph-production/invariant/reference-target-scope-dependency-constraint-contract"),
		payloadSpec("storygraph-production/scene-reference-binding-payload-contract", []string{"scene_reference_binding_version"}, appendFields(projection,
			ownerRef("scene_ref", true, false), ownerRefArray("occurrence_refs", 0), ownerRefArray("interaction_claim_refs", 0), ownerRefArray("character_asset_version_refs", 0), ownerRef("location_asset_version_ref", true, false), ownerRefArray("prop_asset_version_refs", 0), ownerRef("selected_composition_artifact_ref", true, false), ownerRef("fulfilled_reference_target_ref", true, false), constraintsField()), "storygraph-production/invariant/scene-reference-binding-edge-equivalence-contract"),
		payloadSpec("storygraph-production/interaction-reference-binding-payload-contract", []string{"interaction_reference_binding_version"}, appendFields(projection,
			ownerRef("interaction_claim_ref", true, false), ownerRefArray("character_asset_version_refs", 1), ownerRef("prop_asset_version_ref", true, false), ownerRef("selected_composition_artifact_ref", true, false), ownerRef("fulfilled_reference_target_ref", true, false), constraintsField()), "storygraph-production/invariant/interaction-reference-binding-edge-equivalence-contract"),
		payloadSpec("storygraph-production/shot-payload-contract", []string{"shot"}, appendFields(projection, ownerRefArray("source_beat_refs", 1)), "storygraph-production/invariant/shot-source-beat-equivalence-contract"),
		payloadSpec("storygraph-production/shot-production-binding-payload-contract", []string{"shot_production_binding_version"}, appendFields(projection,
			ownerRef("shot_ref", true, false), ownerRefArray("occurrence_refs", 0), ownerRefArray("asset_version_refs", 0), ownerRefArray("scene_reference_binding_refs", 0), ownerRefArray("interaction_reference_binding_refs", 0), ownerRef("effective_style_snapshot_ref", true, false), constraintsField()), "storygraph-production/invariant/shot-binding-edge-equivalence-contract"),
	)
	specs = append(specs, narrativeClaimSpec(projection), interactionClaimSpec(projection), continuityClaimSpec(projection))
	return specs
}

func narrativeClaimSpec(projection []productionField) productionPayloadSpec {
	fields := appendFields(projection,
		stringFormat("claim_series_key", "stable-key-contract"), boundedInteger("claim_revision", 1, productionMaximumSafeInteger),
		stringPattern("predicate", "^[a-z][a-z0-9_]{0,63}$"), ownerRef("subject_ref", true, false), ownerRef("object_ref", true, true), ownerRefArray("participant_refs", 0), ownerRefArray("anchor_refs", 1),
		objectField("valid_scope", productionClaimScopeContract, true, false, claimScopeFields()), objectField("story_time_range", productionStoryTimeContract, true, true, storyTimeFields()),
		stringEnum("polarity", "negative", "neutral", "positive"), stringEnum("status", "asserted", "negated"), objectField("creator_decision_ref", productionAuditRefContract, true, true, auditRefFields()), ownerRef("supersedes_claim_ref", true, true))
	return payloadSpec("storygraph-production/narrative-claim-payload-contract", []string{"causal_claim", "foreshadowing_claim", "payoff_claim", "relationship_claim"}, fields,
		evidenceInvariant, "storygraph-production/invariant/narrative-claim-edge-equivalence-contract", "storygraph-production/invariant/claim-supersession-chain-contract")
}

func interactionClaimSpec(projection []productionField) productionPayloadSpec {
	fields := appendFields(projection,
		stringEnum("claim_type", "interaction"), stringFormat("claim_series_key", "stable-key-contract"), boundedInteger("claim_revision", 1, productionMaximumSafeInteger),
		stringEnum("predicate", "break", "carry", "drop", "give", "hold", "open", "place", "receive", "use", "wear"), ownerRefArray("actor_occurrence_refs", 1), ownerRef("prop_occurrence_ref", true, false), ownerRef("counterparty_occurrence_ref", false, false), ownerRef("scene_ref", true, false), ownerRef("beat_ref", false, false),
		objectField("valid_scope", productionClaimScopeContract, true, false, claimScopeFields()), stringFormat("story_time", "story-time-key-contract"), stringEnum("status", "asserted"), stringEnum("hand", "both", "left", "right", "unspecified"),
		optionalFormat("grip_type", "descriptor-key-contract"), optionalFormat("contact_point", "descriptor-key-contract"), optionalFormat("direction", "descriptor-key-contract"), objectField("relative_scale", productionRationalContract, false, false, rationalFields()),
		ownerRef("holder_before", true, true), ownerRef("holder_after", true, true), ownerRef("prop_state_before", true, false), ownerRef("prop_state_after", true, false), objectField("creator_decision_ref", productionAuditRefContract, true, true, auditRefFields()), ownerRef("supersedes_claim_ref", true, true))
	spec := payloadSpec("storygraph-production/continuity-interaction-payload-contract", []string{"continuity_claim"}, fields,
		evidenceInvariant, "storygraph-production/invariant/interaction-predicate-state-machine-contract", "storygraph-production/invariant/claim-supersession-chain-contract")
	spec.discriminant, spec.discriminantV = "claim_type", "interaction"
	return spec
}

func continuityClaimSpec(projection []productionField) productionPayloadSpec {
	fields := appendFields(projection,
		stringEnum("claim_type", "continuity"), stringFormat("claim_series_key", "stable-key-contract"), boundedInteger("claim_revision", 1, productionMaximumSafeInteger), stringEnum("predicate", "state_changes", "state_persists"),
		ownerRef("subject", true, false), ownerRef("state_before", true, false), ownerRef("state_after", true, false), ownerRef("anchor_start", true, false), ownerRef("anchor_end", true, false), objectField("valid_scope", productionClaimScopeContract, true, false, claimScopeFields()),
		stringFormat("story_time_start", "story-time-key-contract"), stringFormat("story_time_end", "story-time-key-contract"), stringEnum("status", "asserted", "negated"), objectField("creator_decision_ref", productionAuditRefContract, true, true, auditRefFields()), ownerRef("supersedes_claim_ref", true, true))
	spec := payloadSpec("storygraph-production/continuity-state-payload-contract", []string{"continuity_claim"}, fields,
		evidenceInvariant, "storygraph-production/invariant/continuity-state-transition-contract", "storygraph-production/invariant/claim-supersession-chain-contract")
	spec.discriminant, spec.discriminantV = "claim_type", "continuity"
	return spec
}

func buildProductionPayloadDefinition(spec productionPayloadSpec) (ProductionPayloadContractDefinition, error) {
	value := ProductionPayloadContractDefinition{
		PayloadMetaContractID: ProductionPayloadMetaContractID, PayloadContractID: spec.id,
		AllowedNodeTypes: append([]string(nil), spec.allowed...), RequiredFields: []string{}, NullableFields: []string{},
		PropertySchemas: []ProductionPropertySchemaDefinition{}, ArraySortRules: []ProductionArraySortRuleDefinition{},
		Invariants: []ProductionInvariantDefinition{}, AdditionalProperties: false,
	}
	slices.Sort(value.AllowedNodeTypes)
	if spec.discriminant != "" {
		value.DiscriminantField, value.DiscriminantValue = stringPointer(spec.discriminant), stringPointer(spec.discriminantV)
	}
	root := propertySchema("", "object")
	root.ObjectContractID, root.ObjectAdditionalProperties = stringPointer(spec.id), boolPointer(false)
	value.PropertySchemas = append(value.PropertySchemas, root)
	value.RequiredFields = append(value.RequiredFields, "")
	for _, field := range spec.fields {
		if field.name == "payload_contract_id" {
			field.values = []string{spec.id}
			field.format = ""
		}
		appendProductionField(&value, "", field)
	}
	for _, invariant := range append([]string{projectionInvariant}, spec.invariants...) {
		value.Invariants = append(value.Invariants, ProductionInvariantDefinition{InvariantID: invariant})
	}
	slices.Sort(value.RequiredFields)
	slices.Sort(value.NullableFields)
	slices.SortFunc(value.PropertySchemas, func(left, right ProductionPropertySchemaDefinition) int {
		return compareString(left.JSONPointer, right.JSONPointer)
	})
	slices.SortFunc(value.ArraySortRules, func(left, right ProductionArraySortRuleDefinition) int {
		return compareString(left.JSONPointer, right.JSONPointer)
	})
	slices.SortFunc(value.Invariants, func(left, right ProductionInvariantDefinition) int {
		return compareString(left.InvariantID, right.InvariantID)
	})
	raw, err := json.Marshal(value)
	if err != nil {
		return ProductionPayloadContractDefinition{}, err
	}
	if _, _, err = DecodeProductionPayloadMetaContract(raw); err != nil {
		return ProductionPayloadContractDefinition{}, fmt.Errorf("build payload contract %s: %w", spec.id, err)
	}
	return value, nil
}

func appendProductionField(value *ProductionPayloadContractDefinition, parent string, field productionField) {
	pointer := parent + "/" + field.name
	property := propertySchema(pointer, field.kind)
	if field.nullable {
		property.JSONTypes = append(property.JSONTypes, "null")
		slices.Sort(property.JSONTypes)
		value.NullableFields = append(value.NullableFields, pointer)
	}
	if field.format != "" {
		property.Format = stringPointer(field.format)
	}
	if field.pattern != "" {
		property.Pattern = stringPointer(field.pattern)
	}
	property.IntegerMinimum, property.IntegerMaximum = field.minimum, field.maximum
	for _, item := range field.values {
		raw, _ := json.Marshal(item)
		property.EnumValues = append(property.EnumValues, raw)
	}
	if field.kind == "object" {
		property.ObjectContractID, property.ObjectAdditionalProperties = stringPointer(field.objectID), boolPointer(false)
	}
	if field.kind == "array" {
		property.ArrayItemContractID, property.MinItems = stringPointer(field.arrayItem), intPointer(field.minItems)
		sortKeys := make([]string, len(field.sortKeys))
		copy(sortKeys, field.sortKeys)
		value.ArraySortRules = append(value.ArraySortRules, ProductionArraySortRuleDefinition{
			JSONPointer: pointer, SortMode: field.sortMode, SortKeyJSONPointers: sortKeys, Unique: true,
		})
	}
	value.PropertySchemas = append(value.PropertySchemas, property)
	if field.required {
		value.RequiredFields = append(value.RequiredFields, pointer)
	}
	for _, child := range field.children {
		appendProductionField(value, pointer, child)
	}
}

func propertySchema(pointer, kind string) ProductionPropertySchemaDefinition {
	return ProductionPropertySchemaDefinition{JSONPointer: pointer, JSONTypes: []string{kind}, EnumValues: []json.RawMessage{}}
}

func payloadSpec(id string, allowed []string, fields []productionField, invariants ...string) productionPayloadSpec {
	return productionPayloadSpec{id: id, allowed: allowed, fields: fields, invariants: invariants}
}

func appendFields(base []productionField, fields ...productionField) []productionField {
	result := append([]productionField(nil), base...)
	return append(result, fields...)
}

func stringEnum(name string, values ...string) productionField {
	format := ""
	if len(values) == 0 {
		format = "nfc-nonempty"
	}
	return productionField{name: name, kind: "string", required: true, values: values, format: format}
}

func stringFormat(name, format string) productionField {
	return productionField{name: name, kind: "string", required: true, format: format}
}

func optionalFormat(name, format string) productionField {
	return productionField{name: name, kind: "string", format: format}
}

func stringPattern(name, pattern string) productionField {
	return productionField{name: name, kind: "string", required: true, pattern: pattern}
}

func boundedInteger(name string, minimum, maximum int64) productionField {
	return productionField{name: name, kind: "integer", required: true, minimum: intPointer(minimum), maximum: intPointer(maximum)}
}

func objectField(name, contract string, required, nullable bool, children []productionField) productionField {
	return productionField{name: name, kind: "object", required: required, nullable: nullable, objectID: contract, children: children}
}

func ownerRef(name string, required, nullable bool) productionField {
	return objectField(name, productionOwnerNodeRefContract, required, nullable, ownerNodeRefFields())
}

func ownerRefArray(name string, minimum int64) productionField {
	return productionField{name: name, kind: "array", required: true, minItems: minimum, arrayItem: ownerRefArrayItemContract, sortMode: "owner_node_ref_tuple_ascending", sortKeys: []string{}}
}

func scopeKeyArray(name string, minimum int64) productionField {
	return productionField{name: name, kind: "array", required: true, minItems: minimum, arrayItem: scopeKeyArrayItemContract, sortMode: "utf8_ascending", sortKeys: []string{""}}
}

func constraintsField() productionField {
	return objectField("constraints", productionConstraintContract, true, false, []productionField{
		ownerRefArray("world_rule_refs", 0), ownerRef("policy_snapshot_ref", true, false), ownerRef("effective_style_snapshot_ref", true, false),
	})
}

func targetOwnerRefFields() []productionField {
	names := []string{"identity", "interaction", "occurrence", "scene", "specification", "state", "style"}
	fields := make([]productionField, 0, len(names))
	for _, name := range names {
		fields = append(fields, ownerRefArray(name, 0))
	}
	return fields
}

func ownerNodeRefFields() []productionField {
	return []productionField{
		stringFormat("workspace_id", "uuid-lowercase-hyphenated"), stringFormat("project_id", "uuid-lowercase-hyphenated"),
		stringEnum("owner_kind"), stringEnum("version_family"), stringEnum("owner_logical_id"),
		optionalFormat("fragment_key", "nfc-nonempty"), optionalFormat("fragment_content_hash", "sha256-lowercase-hex"),
		stringFormat("owner_version_id", "uuid-lowercase-hyphenated"), boundedInteger("owner_revision", 1, productionMaximumSafeInteger), stringFormat("owner_content_hash", "sha256-lowercase-hex"),
	}
}

func auditRefFields() []productionField {
	return []productionField{
		stringFormat("workspace_id", "uuid-lowercase-hyphenated"), stringFormat("project_id", "uuid-lowercase-hyphenated"),
		stringEnum("audit_owner_kind", "asset", "production/bible", "production/planning"), stringEnum("audit_id"), boundedInteger("audit_revision", 1, productionMaximumSafeInteger), stringFormat("audit_content_hash", "sha256-lowercase-hex"),
	}
}

func claimScopeFields() []productionField {
	return []productionField{stringEnum("kind", "beat", "episode", "project", "scene", "source_range"), stringFormat("owner_logical_id", "stable-key-contract")}
}

func storyTimeFields() []productionField {
	return []productionField{stringFormat("start_key", "story-time-key-contract"), stringFormat("end_key", "story-time-key-contract")}
}

func rationalFields() []productionField {
	return []productionField{boundedInteger("numerator", 1, productionMaximumSafeInteger), boundedInteger("denominator", 1, productionMaximumSafeInteger)}
}

func stringPointer(value string) *string { return &value }
func intPointer(value int64) *int64      { return &value }
func boolPointer(value bool) *bool       { return &value }

func compareString(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
