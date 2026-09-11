package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const SceneAnalysisCandidateSchemaSetContractID = "storygraph-scene-analysis-candidate-schema-set-production"

type SceneAnalysisCandidateSchema struct {
	StageKey            string `json:"stage_key"`
	ProfileKey          string `json:"profile_key"`
	CandidateType       string `json:"candidate_type"`
	OutputSchemaVersion string `json:"output_schema_version"`
	SchemaHash          string `json:"schema_hash"`
}

type SceneAnalysisCandidateSchemaManifest struct {
	ContractID    string                         `json:"contract_id"`
	Schemas       []SceneAnalysisCandidateSchema `json:"schemas"`
	SchemaSetHash string                         `json:"schema_set_hash"`
}

var sceneAnalysisCandidateSchemas = []SceneAnalysisCandidateSchema{
	{StageKey: "bind_scene_occurrences", ProfileKey: "default", CandidateType: "scene_binding_fragment_candidate", OutputSchemaVersion: SceneBindingFragmentCandidateSchemaVersion, SchemaHash: "bfcb5ecaf50dcf957254a7a2f8590538c3932102781ffee0c38edd00f4ce3c48"},
	{StageKey: "derive_production_entities", ProfileKey: "default", CandidateType: "production_entity_fragment_candidate", OutputSchemaVersion: ProductionEntityFragmentCandidateSchemaVersion, SchemaHash: "6e64c80934e967535562702878da6a848a563147add8b7449a5008087ea52895"},
	{StageKey: "extract_scene_facts", ProfileKey: "default", CandidateType: "scene_fact_candidate", OutputSchemaVersion: SceneFactCandidateSchemaVersion, SchemaHash: "6ed929feea15117f4759875f7e690757fd99abe345603d905560e8c8b6334658"},
	{StageKey: "propose_script_spans", ProfileKey: "default", CandidateType: "script_span_candidate", OutputSchemaVersion: ScriptSpanCandidateSchemaVersion, SchemaHash: "93dfffd5337ba9f2fe8d4376eb0aaf6d86676aa7888c53c8224646b54f2df59e"},
	{StageKey: "reconcile_interaction_continuity", ProfileKey: "default", CandidateType: "continuity_fragment_candidate", OutputSchemaVersion: InteractionContinuityCandidateSchemaVersion, SchemaHash: "80750bfd0349ccee72006dad00494d181d76ab4fb4ae4e74c8b71faab9ba1806"},
	{StageKey: "resolve_identities", ProfileKey: "default", CandidateType: "identity_resolution_candidate", OutputSchemaVersion: IdentityResolutionCandidateSchemaVersion, SchemaHash: "0e7cdc46d4bdd4335b962166eb783e4789551cd83de634da1b0736729e86552a"},
	{StageKey: "review_candidate", ProfileKey: "structure_identity", CandidateType: "structure_identity_review_candidate", OutputSchemaVersion: StructureIdentityReviewCandidateSchemaVersion, SchemaHash: "8668ace7b898b3136034b1904c2acfa27489064b05a6736c3145f254e3df7281"},
}

func SceneAnalysisCandidateSchemas() []SceneAnalysisCandidateSchema {
	return slices.Clone(sceneAnalysisCandidateSchemas)
}

func DecodeSceneAnalysisCandidateSchemaManifest(raw json.RawMessage) (SceneAnalysisCandidateSchemaManifest, json.RawMessage, error) {
	var manifest SceneAnalysisCandidateSchemaManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return SceneAnalysisCandidateSchemaManifest{}, nil, errors.New("invalid Scene Analysis Candidate Schema manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SceneAnalysisCandidateSchemaManifest{}, nil, errors.New("invalid Scene Analysis Candidate Schema manifest")
	}
	if manifest.ContractID != SceneAnalysisCandidateSchemaSetContractID ||
		!hashPattern.MatchString(manifest.SchemaSetHash) || !slices.Equal(manifest.Schemas, sceneAnalysisCandidateSchemas) {
		return SceneAnalysisCandidateSchemaManifest{}, nil, errors.New("invalid Scene Analysis Candidate Schema manifest")
	}
	for _, schema := range manifest.Schemas {
		variant := SceneAnalysisStageVariant{
			StageKey:            schema.StageKey,
			ProfileKey:          schema.ProfileKey,
			LaneKey:             "primary",
			OutputSchemaVersion: schema.OutputSchemaVersion,
		}
		if variant.Validate() != nil || !hashPattern.MatchString(schema.SchemaHash) {
			return SceneAnalysisCandidateSchemaManifest{}, nil, errors.New("invalid Scene Analysis Candidate Schema manifest")
		}
	}
	hash, err := sceneAnalysisCandidateSchemaSetHash(manifest.Schemas)
	if err != nil || hash != manifest.SchemaSetHash {
		return SceneAnalysisCandidateSchemaManifest{}, nil, errors.New("Scene Analysis Candidate Schema set hash has drifted")
	}
	encoded, err := encodeSceneAnalysisCandidateSchemaManifest(manifest)
	if err != nil {
		return SceneAnalysisCandidateSchemaManifest{}, nil, err
	}
	return manifest, encoded, nil
}

func sceneAnalysisCandidateSchemaSetHash(schemas []SceneAnalysisCandidateSchema) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID string                         `json:"contract_id"`
		Schemas    []SceneAnalysisCandidateSchema `json:"schemas"`
	}{ContractID: SceneAnalysisCandidateSchemaSetContractID, Schemas: schemas})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeSceneAnalysisCandidateSchemaManifest(manifest SceneAnalysisCandidateSchemaManifest) (json.RawMessage, error) {
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
