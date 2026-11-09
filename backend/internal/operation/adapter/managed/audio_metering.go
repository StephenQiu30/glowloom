package managed

import (
	"encoding/json"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func frozenAudioStrategy(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var detail struct {
		Strategy string `json:"pricing_strategy"`
	}
	if json.Unmarshal(raw, &detail) != nil {
		return "", application.ErrInvalidProviderCall
	}
	return detail.Strategy, nil
}

func normalizeVersionedAudioUsage(strategy string, result application.GenerationResult, usage map[string]any) error {
	if strategy != catalogdomain.PricingCharacterCountV1 && strategy != catalogdomain.PricingOriginalDurationV1 {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(result.Usage, &fields) != nil {
		return application.ErrInvalidProviderCall
	}
	switch strategy {
	case catalogdomain.PricingCharacterCountV1:
		var count int64
		if result.ProviderTerminalCode != 20000000 || json.Unmarshal(fields["text_words"], &count) != nil || count <= 0 {
			return application.ErrInvalidProviderCall
		}
		usage["character_count"] = count
	case catalogdomain.PricingOriginalDurationV1:
		seconds, err := application.ParseAudioDurationSeconds(fields["original_duration"], 120)
		if err != nil {
			return application.ErrInvalidProviderCall
		}
		usage["original_duration_seconds"] = seconds
	}
	return nil
}
