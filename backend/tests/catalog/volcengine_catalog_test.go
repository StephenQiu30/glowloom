package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestVolcengineCatalogCoversCurrentGenerationFamilies(t *testing.T) {
	catalog := domain.BuiltinModelCatalog("volcengine_ark")
	require.Greater(t, len(catalog.Models), 16, "the full current generation catalog must not be truncated to the old selection limit")
	ids := map[string]domain.ModelCandidatePreset{}
	keys := []string{}
	for _, candidate := range catalog.Models {
		ids[candidate.ProviderModelID] = candidate
		keys = append(keys, candidate.Key)
	}
	for _, id := range []string{"doubao-seed-2-1-turbo-260628", "doubao-seed-2-0-mini-260428", "kimi-k3-1-pro-261024", "deepseek-v4-1-flash-261017", "doubao-seedream-5-0-pro-260628", "doubao-seedream-5-0-flash-260915", "doubao-seedance-2-5-260628", "doubao-seedance-2-0-mini-260615"} {
		require.Contains(t, ids, id)
	}
	selected, err := catalog.Select(catalog.Version, keys)
	require.NoError(t, err)
	require.Len(t, selected, len(keys))
	require.Equal(t, "sunsetting", ids["doubao-seedream-4-5-251128"].Lifecycle)
	_, err = catalog.Select("2026-10-26.1", keys)
	require.Error(t, err)
	var seedance domain.ModelCandidatePreset
	for _, candidate := range selected {
		if candidate.ProviderModelID == "doubao-seedance-2-5-260628" {
			seedance = candidate
		}
	}
	version, err := seedance.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	var fields []map[string]any
	require.NoError(t, json.Unmarshal(version.ParamSchema, &fields))
	found := false
	for _, field := range fields {
		if field["field"] == "aspect_ratio" {
			require.ElementsMatch(t, []any{"text2video", "omni_reference"}, field["for_modes"], "image2video uses adaptive without offering unsupported ratio choices")
		}
		if field["field"] == "duration_ms" {
			require.EqualValues(t, 30000, field["max"])
			found = true
		}
	}
	require.True(t, found)
	var limits struct {
		Roles map[string]json.RawMessage `json:"roles"`
	}
	require.NoError(t, json.Unmarshal(version.Limits, &limits))
	require.JSONEq(t, `10`, string(limits.Roles["_total_videos"]))
	require.JSONEq(t, `10`, string(limits.Roles["_total_audios"]))
}

func TestDoubaoSpeechPresetUsesItsOwnAudioProtocol(t *testing.T) {
	var config domain.ProviderConfig
	for _, preset := range domain.BuiltinPresets() {
		if preset.Slug == "volcengine-speech" {
			config = preset.Config
		}
	}
	require.Equal(t, "volcengine_speech", config.AdapterKey)
	require.Equal(t, "https://openspeech.bytedance.com/api/v3", config.BaseURL)
	require.NoError(t, config.Validate())
	require.Equal(t, []string{"audio"}, config.Capabilities)
	models := domain.ModelCatalogForConfig(config)
	require.NotEmpty(t, models.Models)
	for _, candidate := range models.Models {
		require.Equal(t, "audio.generate", candidate.Capability)
	}
	model := models.Models[0]
	require.Equal(t, "seed-tts-2.0", model.ProviderModelID)
	_, err := model.OnboardingVersion(uuid.New(), uuid.New())
	require.NoError(t, err)
	config.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	require.Error(t, config.Validate())
}

func TestVolcengineVideoPresetsDoNotAdvertiseUnsupportedLegacyOptions(t *testing.T) {
	for _, candidate := range domain.BuiltinModelCatalog("volcengine_ark").Models {
		if candidate.Capability != "video.generate" {
			continue
		}
		version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
		require.NoError(t, err)
		var fields []map[string]any
		require.NoError(t, json.Unmarshal(version.ParamSchema, &fields))
		for _, field := range fields {
			switch candidate.Key {
			case "ark-video-fast", "ark-video-mini":
				if field["field"] == "resolution" {
					require.ElementsMatch(t, []any{"480p", "720p"}, field["enum"])
				}
			case "ark-video-10", "ark-video-10-fast":
				if field["field"] == "duration_ms" {
					require.EqualValues(t, 2000, field["min"])
					require.EqualValues(t, 12000, field["max"])
				}
			}
		}
		if candidate.Key == "ark-video-10-fast" {
			require.NotContains(t, string(version.Limits), "last_frame")
		}
	}
}
