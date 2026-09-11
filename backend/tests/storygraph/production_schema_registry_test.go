package storygraph_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const productionSchemaRegistryFixture = "../fixtures/storygraph/production-schema-registry.json"

type productionSchemaFixture struct {
	SchemaHash       string                              `json:"schema_hash"`
	Manifest         storygraph.ProductionSchemaManifest `json:"manifest"`
	PayloadContracts []productionPayloadFixture          `json:"payload_contracts"`
}

type productionPayloadFixture struct {
	PayloadContractHash string                                         `json:"payload_contract_hash"`
	Definition          storygraph.ProductionPayloadContractDefinition `json:"definition"`
}

func TestProductionSchemaRegistryIsCompleteAndContentAddressed(t *testing.T) {
	t.Parallel()

	registry, err := storygraph.BuildProductionSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}

	if registry.SchemaHash == "" || len(registry.SchemaHash) != 64 {
		t.Fatalf("schema hash = %q", registry.SchemaHash)
	}
	if len(registry.Manifest.NodeDefinitions) != 31 || len(registry.Manifest.PayloadUnionDefinitions) != 32 ||
		len(registry.PayloadContracts) != 25 || len(registry.Manifest.EdgeAliasDefinitions) != 7 ||
		len(registry.Manifest.EdgeMatrixDefinitions) != 65 || len(registry.Manifest.OwnerCollectionDefinitions) != 13 ||
		len(registry.Manifest.CoverageRules) != 4 || len(registry.Manifest.CheckpointOwnerFamilyDefinitions) != 13 ||
		len(registry.Manifest.ExclusionDefinitions) != 22 {
		t.Fatalf("incomplete production registry: nodes=%d unions=%d payloads=%d aliases=%d matrices=%d collections=%d coverage=%d checkpoints=%d exclusions=%d",
			len(registry.Manifest.NodeDefinitions), len(registry.Manifest.PayloadUnionDefinitions), len(registry.PayloadContracts),
			len(registry.Manifest.EdgeAliasDefinitions), len(registry.Manifest.EdgeMatrixDefinitions), len(registry.Manifest.OwnerCollectionDefinitions),
			len(registry.Manifest.CoverageRules), len(registry.Manifest.CheckpointOwnerFamilyDefinitions), len(registry.Manifest.ExclusionDefinitions))
	}

	decoded, decodedHash, err := storygraph.DecodeProductionSchemaManifestMetaContract(registry.CanonicalManifest)
	if err != nil {
		t.Fatal(err)
	}
	if decodedHash != registry.SchemaHash || decoded.SchemaID != storygraph.ProductionSchemaID {
		t.Fatalf("manifest identity drifted: decoded=%q built=%q schema=%q", decodedHash, registry.SchemaHash, decoded.SchemaID)
	}

	for _, contract := range registry.PayloadContracts {
		canonical := registry.CanonicalPayloadContracts[contract.PayloadContractID]
		decodedContract, hash, decodeErr := storygraph.DecodeProductionPayloadMetaContract(canonical)
		if decodeErr != nil {
			t.Fatalf("decode %s: %v", contract.PayloadContractID, decodeErr)
		}
		if decodedContract.PayloadContractID != contract.PayloadContractID || hash != registry.PayloadContractHashes[contract.PayloadContractID] {
			t.Fatalf("payload identity drifted for %s", contract.PayloadContractID)
		}
		contractIDProperty := propertyByJSONPointer(t, contract.PropertySchemas, "/payload_contract_id")
		if len(contractIDProperty.EnumValues) != 1 || string(contractIDProperty.EnumValues[0]) != `"`+contract.PayloadContractID+`"` {
			t.Fatalf("payload_contract_id is not frozen for %s", contract.PayloadContractID)
		}
	}

	second, err := storygraph.BuildProductionSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if second.SchemaHash != registry.SchemaHash || !bytes.Equal(second.CanonicalManifest, registry.CanonicalManifest) {
		t.Fatal("production registry build is not deterministic")
	}
}

func propertyByJSONPointer(t *testing.T, values []storygraph.ProductionPropertySchemaDefinition, pointer string) storygraph.ProductionPropertySchemaDefinition {
	t.Helper()
	for _, value := range values {
		if value.JSONPointer == pointer {
			return value
		}
	}
	t.Fatalf("property %s is missing", pointer)
	return storygraph.ProductionPropertySchemaDefinition{}
}

func TestProductionSchemaRegistryMatchesSharedFixture(t *testing.T) {
	registry, err := storygraph.BuildProductionSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fixture := productionSchemaFixture{SchemaHash: registry.SchemaHash, Manifest: registry.Manifest}
	for _, definition := range registry.PayloadContracts {
		fixture.PayloadContracts = append(fixture.PayloadContracts, productionPayloadFixture{
			PayloadContractHash: registry.PayloadContractHashes[definition.PayloadContractID], Definition: definition,
		})
	}
	if os.Getenv("LANVERSE_UPDATE_PRODUCTION_SCHEMA_FIXTURE") == "1" {
		raw, marshalErr := json.MarshalIndent(fixture, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(productionSchemaRegistryFixture, append(raw, '\n'), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	raw, err := os.ReadFile(filepath.Clean(productionSchemaRegistryFixture))
	if err != nil {
		t.Fatal(err)
	}
	var committed productionSchemaFixture
	if err = json.Unmarshal(raw, &committed); err != nil {
		t.Fatal(err)
	}
	if committed.SchemaHash != fixture.SchemaHash || len(committed.PayloadContracts) != len(fixture.PayloadContracts) {
		t.Fatal("shared production registry fixture identity has drifted")
	}
	committedManifest, _ := json.Marshal(committed.Manifest)
	canonicalCommitted, err := platformcanonical.JSON(committedManifest)
	if err != nil || !bytes.Equal(canonicalCommitted, registry.CanonicalManifest) {
		t.Fatal("shared production manifest differs from Backend registry")
	}
	for index := range fixture.PayloadContracts {
		actual, expected := committed.PayloadContracts[index], fixture.PayloadContracts[index]
		if actual.PayloadContractHash != expected.PayloadContractHash || actual.Definition.PayloadContractID != expected.Definition.PayloadContractID {
			t.Fatalf("shared payload fixture differs at index %d", index)
		}
	}
}
