package storygraph_test

import (
	"encoding/json"
	"strings"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionSchemaManifestMetaContractIsStrictAndCanonical(t *testing.T) {
	raw := productionSchemaManifestMetaFixture(t)
	manifest, hash, err := storygraph.DecodeProductionSchemaManifestMetaContract(raw)
	if err != nil || len(hash) != 64 || manifest.SchemaID != storygraph.ProductionSchemaID {
		t.Fatalf("decode Production Schema Manifest: hash=%q manifest=%#v error=%v", hash, manifest, err)
	}

	var reordered map[string]any
	if err = json.Unmarshal(raw, &reordered); err != nil {
		t.Fatal(err)
	}
	reorderedRaw, err := json.Marshal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	_, reorderedHash, err := storygraph.DecodeProductionSchemaManifestMetaContract(reorderedRaw)
	if err != nil || reorderedHash != hash {
		t.Fatalf("Manifest hash depends on JSON object order: %q/%q error=%v", hash, reorderedHash, err)
	}

	tests := map[string]func(map[string]any){
		"missing required nullable field": func(value map[string]any) {
			definition := value["node_definitions"].([]any)[0].(map[string]any)
			delete(definition, "payload_discriminant_field")
		},
		"unsorted nested strings": func(value map[string]any) {
			definition := value["node_definitions"].([]any)[0].(map[string]any)
			definition["allowed_version_families"] = []any{"z", "a"}
		},
		"wrong matrix row union": func(value map[string]any) {
			definition := value["edge_matrix_definitions"].([]any)[0].(map[string]any)
			definition["row_payload"] = map[string]any{
				"claim_branch": "relationship_claim", "participant_cardinality": "1",
				"anchor_cardinality": "1", "state_cardinality": "0",
			}
		},
		"derived rule id drift": func(value map[string]any) {
			definition := value["edge_matrix_definitions"].([]any)[0].(map[string]any)
			definition["projection_rule_id"] = "storygraph-production/unrelated-rule"
		},
		"payload contract missing union": func(value map[string]any) {
			definition := value["node_definitions"].([]any)[0].(map[string]any)
			definition["payload_contract_ids"] = []any{
				"storygraph-production/source_revision-ref-payload-contract",
				"storygraph-production/unrepresented-payload-contract",
			}
		},
		"unknown root field": func(value map[string]any) {
			value["schema_label"] = "Production"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if unmarshalErr := json.Unmarshal(raw, &value); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			mutate(value)
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, _, decodeErr := storygraph.DecodeProductionSchemaManifestMetaContract(encoded); decodeErr == nil {
				t.Fatal("invalid Production Schema Manifest was accepted")
			}
		})
	}
}

func TestProductionPayloadContractMetaContractRequiresExactArrayAndNullableRules(t *testing.T) {
	raw := productionPayloadContractMetaFixture(t)
	contract, hash, err := storygraph.DecodeProductionPayloadMetaContract(raw)
	if err != nil || len(hash) != 64 || contract.PayloadContractID != "storygraph-production/example-payload-contract" {
		t.Fatalf("decode Production Payload Contract: hash=%q contract=%#v error=%v", hash, contract, err)
	}

	tests := map[string]func(map[string]any){
		"missing explicit nullable metadata": func(value map[string]any) {
			property := value["property_schemas"].([]any)[0].(map[string]any)
			delete(property, "format")
		},
		"array without sort rule": func(value map[string]any) {
			value["array_sort_rules"] = []any{}
		},
		"nullable set drift": func(value map[string]any) {
			value["nullable_fields"] = []any{"/items"}
		},
		"additional properties": func(value map[string]any) {
			value["additional_properties"] = true
		},
		"unknown property metadata": func(value map[string]any) {
			property := value["property_schemas"].([]any)[0].(map[string]any)
			property["description"] = "mutable prose"
		},
		"root contract id drift": func(value map[string]any) {
			property := value["property_schemas"].([]any)[0].(map[string]any)
			property["object_contract_id"] = "storygraph-production/unrelated-payload-contract"
		},
		"multiple base types": func(value map[string]any) {
			property := value["property_schemas"].([]any)[1].(map[string]any)
			property["json_types"] = []any{"array", "string"}
		},
		"format outside registry": func(value map[string]any) {
			property := value["property_schemas"].([]any)[1].(map[string]any)
			property["format"] = "custom-format"
		},
		"format on array": func(value map[string]any) {
			property := value["property_schemas"].([]any)[1].(map[string]any)
			property["format"] = "nfc-nonempty"
		},
		"scalar sort missing root key": func(value map[string]any) {
			rule := value["array_sort_rules"].([]any)[0].(map[string]any)
			rule["sort_key_json_pointers"] = []any{}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if unmarshalErr := json.Unmarshal(raw, &value); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			mutate(value)
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, _, decodeErr := storygraph.DecodeProductionPayloadMetaContract(encoded); decodeErr == nil {
				t.Fatal("invalid Production Payload Contract was accepted")
			}
		})
	}
}

func productionSchemaManifestMetaFixture(t *testing.T) []byte {
	t.Helper()
	value := map[string]any{
		"manifest_contract_id":       "storygraph-schema-manifest-contract",
		"schema_id":                  storygraph.ProductionSchemaID,
		"canonical_json_contract_id": "storygraph-canonical-json-production",
		"node_key_derivation_id":     "story-node-key-stable",
		"edge_key_derivation_id":     "story-edge-key-stable",
		"node_definitions": []any{map[string]any{
			"node_type": "source_revision", "owner_kind": "production/script",
			"allowed_version_families":   []string{"script_source_set"},
			"payload_contract_ids":       []string{"storygraph-production/source_revision-ref-payload-contract"},
			"payload_discriminant_field": nil, "evidence_policy_id": "none",
		}},
		"payload_union_definitions": []any{map[string]any{
			"node_type": "source_revision", "discriminant_field": nil, "discriminant_value": nil,
			"payload_contract_id":   "storygraph-production/source_revision-ref-payload-contract",
			"payload_contract_hash": strings.Repeat("a", 64),
		}},
		"edge_alias_definitions": []any{map[string]any{
			"alias": "SPECIFICATION", "expanded_node_types": []string{"character_specification", "location_specification", "prop_specification"},
		}},
		"edge_matrix_definitions": []any{map[string]any{
			"matrix_id": "edge-type-matrix-contract", "row_index": 1,
			"row_payload": map[string]any{
				"edge_type": "contains", "allowed_source_to_target": "episode to scene",
				"qualifier_and_cardinality": "one exact parent",
			},
			"projection_rule_id":  "storygraph-production/edge-type-matrix-contract/row-1-projection-contract",
			"cardinality_rule_id": "storygraph-production/edge-type-matrix-contract/row-1-cardinality-contract",
		}},
		"owner_collection_definitions": []any{map[string]any{
			"minimum_coverage": "p0", "phase_rank": 0, "owner_kind": "production/script",
			"version_family": "script_source_set", "scope_kind": "project",
			"scope_key_and_collection_cardinality": "project:<project_id>; exactly one collection",
			"member_completeness":                  "accepted source and span index", "empty_collection_policy": "forbidden",
			"scope_key_rule_id":   "storygraph-production/owner-collection/script_source_set-scope-key-contract",
			"member_contract_id":  "storygraph-production/owner-collection/script_source_set-members-contract",
			"cardinality_rule_id": "storygraph-production/owner-collection/script_source_set-cardinality-contract",
		}},
		"coverage_rules": []any{map[string]any{
			"coverage_phase": "p0", "phase_rank": 0, "scope_set_field": "p0_scope_keys",
			"activation_condition": "scope_set_non_empty", "prerequisite_phases": []string{},
			"required_version_families": []string{"script_source_set"},
			"decision_checkpoint_ids":   []string{"source_revision_accepted"},
			"coverage_rule_id":          "storygraph-production/coverage/p0-contract",
		}},
		"checkpoint_owner_family_definitions": []any{map[string]any{
			"decision_checkpoint_id": "source_revision_accepted", "owner_kind": "production/script", "version_family": "script_source_set",
		}},
		"exclusion_definitions": []any{map[string]any{"exclusion_kind": "node_type", "literal": "project"}},
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func productionPayloadContractMetaFixture(t *testing.T) []byte {
	t.Helper()
	value := map[string]any{
		"payload_meta_contract_id": "storygraph-payload-contract-meta-contract",
		"payload_contract_id":      "storygraph-production/example-payload-contract",
		"allowed_node_types":       []string{"source_revision"},
		"discriminant_field":       nil,
		"discriminant_value":       nil,
		"required_fields":          []string{"", "/items"},
		"nullable_fields":          []string{},
		"property_schemas": []any{
			map[string]any{
				"json_pointer": "", "json_types": []string{"object"}, "format": nil,
				"enum_values": []any{}, "pattern": nil, "integer_minimum": nil, "integer_maximum": nil,
				"min_items": nil, "max_items": nil,
				"object_contract_id":     "storygraph-production/example-payload-contract",
				"array_item_contract_id": nil, "object_additional_properties": false,
			},
			map[string]any{
				"json_pointer": "/items", "json_types": []string{"array"}, "format": nil,
				"enum_values": []any{}, "pattern": nil, "integer_minimum": nil, "integer_maximum": nil,
				"min_items": 0, "max_items": nil, "object_contract_id": nil,
				"array_item_contract_id":       "storygraph-production/array-item/nfc-string-contract",
				"object_additional_properties": nil,
			},
		},
		"array_sort_rules": []any{map[string]any{
			"json_pointer": "/items", "sort_mode": "utf8_ascending",
			"sort_key_json_pointers": []string{""}, "unique": true,
		}},
		"invariants": []any{map[string]any{
			"invariant_id": "storygraph-production/invariant/projection-hash-equals-owner-contract",
		}},
		"additional_properties": false,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
