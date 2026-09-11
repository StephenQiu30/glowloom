package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const SceneAnalysisInputSchemaSetContractID = "storygraph-scene-analysis-input-schema-set-production"

type SceneAnalysisInputSchema struct {
	StageKey        string `json:"stage_key"`
	ProfileKey      string `json:"profile_key"`
	InputContractID string `json:"input_contract_id"`
	SchemaHash      string `json:"schema_hash"`
}

type SceneAnalysisInputSchemaManifest struct {
	ContractID    string                     `json:"contract_id"`
	Schemas       []SceneAnalysisInputSchema `json:"schemas"`
	SchemaSetHash string                     `json:"schema_set_hash"`
}

var sceneAnalysisInputSchemas = []SceneAnalysisInputSchema{
	{StageKey: "bind_scene_occurrences", ProfileKey: "default", InputContractID: "scene-occurrence-binding-input-production", SchemaHash: "fb12a00b10aca86d6f87b8d10625bdacb0f8bc97f7ae010c96fe27c638e83c73"},
	{StageKey: "derive_production_entities", ProfileKey: "default", InputContractID: "production-entity-derivation-input-production", SchemaHash: "731624221a473e22fb22cddc695bcb849a8bf0bd8944c92f00dcd97f4819a2ce"},
	{StageKey: "extract_scene_facts", ProfileKey: "default", InputContractID: "scene-fact-extraction-input-production", SchemaHash: "d5274894af86322cbdfd2336d519c09cfff0c17faaa940063087707c60acb17f"},
	{StageKey: "propose_script_spans", ProfileKey: "default", InputContractID: "script-span-proposal-input-production", SchemaHash: "80b9dcfad4a869820686cce5e63bf21ddfef6db0ab8684d39871452c1e142696"},
	{StageKey: "reconcile_interaction_continuity", ProfileKey: "default", InputContractID: "interaction-continuity-input-production", SchemaHash: "4bbf30524119fcf2d74de34942207980e6cfe95634ec309fa9ecd62a348139ea"},
	{StageKey: "resolve_identities", ProfileKey: "default", InputContractID: "identity-resolution-input-production", SchemaHash: "80e96d4dbc18fcf727c98bbb3e9b54b1b846f7b39df342b7e3518f4df55eeb2f"},
	{StageKey: VisualFoundationStageKey, ProfileKey: "default", InputContractID: VisualFoundationInputContractID, SchemaHash: VisualFoundationInputSchemaHash},
	{StageKey: "review_candidate", ProfileKey: "structure_identity", InputContractID: "structure-identity-review-input-production", SchemaHash: "2d738a97a051aacd164558eac74564100ed7b12546dd397015ed3e2f0f25c9e0"},
}

func SceneAnalysisInputSchemas() []SceneAnalysisInputSchema {
	return slices.Clone(sceneAnalysisInputSchemas)
}

func DecodeSceneAnalysisInputSchemaManifest(raw json.RawMessage) (SceneAnalysisInputSchemaManifest, json.RawMessage, error) {
	var manifest SceneAnalysisInputSchemaManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return SceneAnalysisInputSchemaManifest{}, nil, errors.New("invalid Scene Analysis Input Schema manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SceneAnalysisInputSchemaManifest{}, nil, errors.New("invalid Scene Analysis Input Schema manifest")
	}
	if manifest.ContractID != SceneAnalysisInputSchemaSetContractID ||
		!hashPattern.MatchString(manifest.SchemaSetHash) ||
		!slices.Equal(manifest.Schemas, sceneAnalysisInputSchemas) {
		return SceneAnalysisInputSchemaManifest{}, nil, errors.New("invalid Scene Analysis Input Schema manifest")
	}
	for index, schema := range manifest.Schemas {
		candidate := sceneAnalysisCandidateSchemas[index]
		variant := SceneAnalysisStageVariant{
			StageKey: schema.StageKey, ProfileKey: schema.ProfileKey, LaneKey: "primary",
			OutputSchemaVersion: candidate.OutputSchemaVersion,
		}
		if schema.StageKey != candidate.StageKey || schema.ProfileKey != candidate.ProfileKey ||
			variant.Validate() != nil || !hashPattern.MatchString(schema.SchemaHash) {
			return SceneAnalysisInputSchemaManifest{}, nil, errors.New("invalid Scene Analysis Input Schema manifest")
		}
	}
	hash, err := sceneAnalysisInputSchemaSetHash(manifest.Schemas)
	if err != nil || hash != manifest.SchemaSetHash {
		return SceneAnalysisInputSchemaManifest{}, nil, errors.New("Scene Analysis Input Schema set hash has drifted")
	}
	encoded, err := encodeSceneAnalysisInputSchemaManifest(manifest)
	if err != nil {
		return SceneAnalysisInputSchemaManifest{}, nil, err
	}
	return manifest, encoded, nil
}

func sceneAnalysisInputSchemaSetHash(schemas []SceneAnalysisInputSchema) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID string                     `json:"contract_id"`
		Schemas    []SceneAnalysisInputSchema `json:"schemas"`
	}{ContractID: SceneAnalysisInputSchemaSetContractID, Schemas: schemas})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeSceneAnalysisInputSchemaManifest(manifest SceneAnalysisInputSchemaManifest) (json.RawMessage, error) {
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
