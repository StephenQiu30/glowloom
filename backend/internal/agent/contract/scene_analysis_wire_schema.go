package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const SceneAnalysisWireSchemaSetContractID = "storygraph-scene-analysis-wire-schema-set-production"

type SceneAnalysisWireSchema struct {
	ContractID string `json:"contract_id"`
	SchemaHash string `json:"schema_hash"`
}

type SceneAnalysisWireSchemaManifest struct {
	ContractID     string                    `json:"contract_id"`
	WireSchemaID   string                    `json:"wire_schema_id"`
	Schemas        []SceneAnalysisWireSchema `json:"schemas"`
	WireSchemaHash string                    `json:"wire_schema_hash"`
}

var sceneAnalysisWireSchemas = []SceneAnalysisWireSchema{
	{ContractID: "storygraph-dispatch-authorization-claims-production", SchemaHash: "3ffd2bc7c001579261d9e91c0daf3c8332ed6fe1e90b573239d916e18e06f8ef"},
	{ContractID: "storygraph-stage-attempt-result-production", SchemaHash: "3b8294b376964bf52c48d7a2767ac89f5bee40d9d2732e33fc45318fd5f4fb2d"},
	{ContractID: "storygraph-stage-invocation-production", SchemaHash: "6cc980590ed9aacb9a70755fb843b3ac20cd467abbd4572bd1470be66ae044ac"},
}

func SceneAnalysisWireSchemas() []SceneAnalysisWireSchema {
	return slices.Clone(sceneAnalysisWireSchemas)
}

func DecodeSceneAnalysisWireSchemaManifest(raw json.RawMessage) (SceneAnalysisWireSchemaManifest, json.RawMessage, error) {
	var manifest SceneAnalysisWireSchemaManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return SceneAnalysisWireSchemaManifest{}, nil, errors.New("invalid Scene Analysis Wire Schema manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SceneAnalysisWireSchemaManifest{}, nil, errors.New("invalid Scene Analysis Wire Schema manifest")
	}
	if manifest.ContractID != SceneAnalysisWireSchemaSetContractID ||
		manifest.WireSchemaID != SceneAnalysisWireSchemaVersion ||
		!hashPattern.MatchString(manifest.WireSchemaHash) ||
		!slices.Equal(manifest.Schemas, sceneAnalysisWireSchemas) {
		return SceneAnalysisWireSchemaManifest{}, nil, errors.New("invalid Scene Analysis Wire Schema manifest")
	}
	for _, schema := range manifest.Schemas {
		if !hashPattern.MatchString(schema.SchemaHash) {
			return SceneAnalysisWireSchemaManifest{}, nil, errors.New("invalid Scene Analysis Wire Schema manifest")
		}
	}
	hash, err := sceneAnalysisWireSchemaHash(manifest)
	if err != nil || hash != manifest.WireSchemaHash {
		return SceneAnalysisWireSchemaManifest{}, nil, errors.New("Scene Analysis Wire Schema hash has drifted")
	}
	encoded, err := encodeSceneAnalysisWireSchemaManifest(manifest)
	if err != nil {
		return SceneAnalysisWireSchemaManifest{}, nil, err
	}
	return manifest, encoded, nil
}

func sceneAnalysisWireSchemaHash(manifest SceneAnalysisWireSchemaManifest) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID   string                    `json:"contract_id"`
		WireSchemaID string                    `json:"wire_schema_id"`
		Schemas      []SceneAnalysisWireSchema `json:"schemas"`
	}{ContractID: manifest.ContractID, WireSchemaID: manifest.WireSchemaID, Schemas: manifest.Schemas})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeSceneAnalysisWireSchemaManifest(manifest SceneAnalysisWireSchemaManifest) (json.RawMessage, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
