package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"
)

func TestPersistedCatalogDigestIgnoresJSONStorageKeyOrder(t *testing.T) {
	document := domain.PresetDocument{SchemaVersion: 1, Name: "fixture", Config: catalogfixture.BuiltinPresets()[0].Config}
	source := domain.ModelCandidateCatalog{Version: "pinned-version", Models: []domain.ModelCandidatePreset{{Key: "fixture", AdapterKey: "volcengine_ark", Capability: "video.generate", Execution: &domain.ModelCandidateExecution{Limits: json.RawMessage(`{"max_outputs":1,"durations_ms":[1000]}`), ParamSchema: json.RawMessage(`[{"field":"resolution","type":"string"}]`)}}}}
	first := domain.ModelCatalogForPreset(document, source)
	source.Models[0].Execution.Limits = json.RawMessage(`{"durations_ms":[1000],"max_outputs":1}`)
	source.Models[0].Execution.ParamSchema = json.RawMessage(`[{"type":"string","field":"resolution"}]`)
	second := domain.ModelCatalogForPreset(document, source)
	require.Equal(t, first.Version, second.Version, "Postgres jsonb canonical key order must not invalidate a logically identical selection")
	source.Models[0].Execution.Limits = json.RawMessage(`{"durations_ms":[2000],"max_outputs":1}`)
	require.NotEqual(t, first.Version, domain.ModelCatalogForPreset(document, source).Version)
}

func TestPinnedExecutionParametersCannotWidenTheirSourceContract(t *testing.T) {
	const schema = `[{"field":"max_tokens","type":"integer","min":1,"max":32768,"default":4096},{"field":"thinking","type":"string","enum":["disabled"],"default":"disabled"},{"field":"strength","type":"number","min":0,"max":1,"default":0.5,"for_modes":["image2image"]},{"field":"optional","type":"string"}]`
	source := domain.ModelCandidatePreset{ProviderModelID: "fixture/model", Modes: []string{"text_to_text", "image2image"}, MeteringStrategy: "token_io_v1", Execution: &domain.ModelCandidateExecution{ParamSchema: json.RawMessage(schema)}}
	for _, tc := range []struct {
		name   string
		change func([]map[string]json.RawMessage) []map[string]json.RawMessage
		modes  []string
		valid  bool
	}{
		{name: "delete max tokens", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage { return fields[1:] }},
		{name: "delete thinking", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			return append(fields[:1], fields[2:]...)
		}},
		{name: "enable thinking", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[1]["enum"] = json.RawMessage(`["enabled"]`)
			fields[1]["default"] = json.RawMessage(`"enabled"`)
			return fields
		}},
		{name: "auto thinking", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[1]["enum"] = json.RawMessage(`["disabled","auto"]`)
			fields[1]["default"] = json.RawMessage(`"auto"`)
			return fields
		}},
		{name: "omit thinking default without requiring it", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			delete(fields[1], "default")
			return fields
		}},
		{name: "change max tokens type", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[0]["type"] = json.RawMessage(`"number"`)
			return fields
		}},
		{name: "lower minimum", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[0]["min"] = json.RawMessage(`0`)
			return fields
		}},
		{name: "raise maximum", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[0]["max"] = json.RawMessage(`65536`)
			return fields
		}},
		{name: "remove maximum", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			delete(fields[0], "max")
			return fields
		}},
		{name: "remove enum", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			delete(fields[1], "enum")
			return fields
		}},
		{name: "add undeclared field", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			return append(fields, map[string]json.RawMessage{"field": json.RawMessage(`"new_param"`), "type": json.RawMessage(`"string"`)})
		}},
		{name: "expand source modes", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			delete(fields[2], "for_modes")
			return fields
		}},
		{name: "hide required default in retained mode", change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[0]["for_modes"] = json.RawMessage(`["image2image"]`)
			return fields
		}},
		{name: "tighten ranges and change defaults", valid: true, change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			fields[0]["min"] = json.RawMessage(`2`)
			fields[0]["max"] = json.RawMessage(`4096`)
			fields[0]["default"] = json.RawMessage(`128`)
			fields[2]["min"] = json.RawMessage(`0.2`)
			fields[2]["max"] = json.RawMessage(`0.8`)
			fields[2]["default"] = json.RawMessage(`0.3`)
			return fields
		}},
		{name: "require explicit thinking instead of default", valid: true, change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			delete(fields[1], "default")
			fields[1]["required"] = json.RawMessage(`true`)
			return fields
		}},
		{name: "drop mode-specific field with removed mode", modes: []string{"text_to_text"}, valid: true, change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage {
			return append(fields[:2], fields[3:]...)
		}},
		{name: "drop optional field", valid: true, change: func(fields []map[string]json.RawMessage) []map[string]json.RawMessage { return fields[:3] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(schema), &fields))
			changed, err := json.Marshal(tc.change(fields))
			require.NoError(t, err)
			modes := tc.modes
			if len(modes) == 0 {
				modes = source.Modes
			}
			version := domain.ModelVersion{ProviderModelID: source.ProviderModelID, Modes: modes, Limits: json.RawMessage(`{"max_outputs":1,"metering_strategy":"token_io_v1"}`), ParamSchema: changed}
			err = source.ValidatePinnedVersion(version)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrInvalidModelVersion)
			}
		})
	}
}

func TestPinnedExecutionRetainsRequiredFieldsAndAllowsEnumSubsets(t *testing.T) {
	source := domain.ModelCandidatePreset{ProviderModelID: "fixture/model", Modes: []string{"text_to_text"}, Execution: &domain.ModelCandidateExecution{ParamSchema: json.RawMessage(`[{"field":"choice","type":"string","required":true,"enum":["alpha","beta"]},{"field":"quantity","type":"number","enum":[9007199254740990,9007199254740991]}]`)}}
	for _, tc := range []struct {
		name, schema string
		valid        bool
	}{
		{"retain required and narrow enums", `[{"field":"choice","type":"string","required":true,"enum":["beta"],"default":"beta"},{"field":"quantity","type":"number","enum":[9007199254740991.0]}]`, true},
		{"remove required field", `[{"field":"quantity","type":"number","enum":[9007199254740991]}]`, false},
		{"relax required to optional", `[{"field":"choice","type":"string","enum":["alpha","beta"],"default":"alpha"}]`, false},
		{"numeric enum remains exact", `[{"field":"choice","type":"string","required":true,"enum":["beta"]},{"field":"quantity","type":"number","enum":[9007199254740991.1]}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := source.ValidatePinnedVersion(domain.ModelVersion{ProviderModelID: source.ProviderModelID, Modes: source.Modes, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(tc.schema)})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrInvalidModelVersion)
			}
		})
	}
}
