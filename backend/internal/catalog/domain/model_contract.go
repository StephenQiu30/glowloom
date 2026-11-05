package domain

import (
	"encoding/json"
	"slices"
)

// ParameterContract is an installed, conservative subset of one model's API.
// It describes publication and input constraints, never account availability.
type ParameterContract struct {
	Type    string
	Values  []string
	Minimum int64
	Maximum int64
	Step    int64
}

// IsSeedanceTokenModel identifies the exact reviewed video SKUs. Model-name
// prefixes must not grant new versions a pricing or estimation contract.
func IsSeedanceTokenModel(model string) bool {
	return slices.Contains([]string{"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"}, model)
}

// ModelParameterContract returns fresh data for the currently installed model
// subsets. Unknown models retain generic validation and require explicit review.
func ModelParameterContract(model string) (map[string]ParameterContract, bool) {
	if IsSeedanceTokenModel(model) {
		maxDuration := int64(15000)
		resolutions := []string{"480p", "720p", "1080p"}
		if model == "doubao-seedance-2-5-260628" {
			maxDuration = 30000
		}
		if model == "doubao-seedance-2-0-fast-260128" || model == "doubao-seedance-2-0-mini-260615" {
			resolutions = []string{"480p", "720p"}
		}
		return map[string]ParameterContract{
			"duration_ms":    {Type: "integer", Minimum: 4000, Maximum: maxDuration, Step: 1000},
			"resolution":     {Type: "string", Values: resolutions},
			"aspect_ratio":   {Type: "string", Values: []string{"adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}},
			"generate_audio": {Type: "boolean"},
			"watermark":      {Type: "boolean"},
		}, true
	}
	switch model {
	case "doubao-seedream-5-0-flash-260915", "doubao-seedream-5-0-pro-260628", "dola-seedream-5-0-flash-260915", "dola-seedream-5-0-pro-260628":
		return map[string]ParameterContract{
			"size":      {Type: "string", Values: []string{"1K", "1.5K", "2K"}},
			"watermark": {Type: "boolean"},
		}, true
	case "gpt-image-2.5-sunburst", "gpt-image-2.5-flare":
		return map[string]ParameterContract{
			"size":    {Type: "string", Values: []string{"1024x1024", "1536x1024", "1024x1536"}},
			"quality": {Type: "string", Values: []string{"low", "medium", "high"}},
		}, true
	default:
		return nil, false
	}
}

// ValidateModelParameters applies installed SKU limits to normalized input even
// when an older model version has a broader form schema. In-flight usage and
// settlement continue to use their frozen price; only new quotes are checked.
func ValidateModelParameters(model string, params map[string]any) error {
	contract, known := ModelParameterContract(model)
	if !known {
		return nil
	}
	for name, value := range params {
		rule, exists := contract[name]
		if !exists {
			return ErrInvalidModelVersion
		}
		switch rule.Type {
		case "string":
			text, ok := value.(string)
			if !ok || len(rule.Values) > 0 && !slices.Contains(rule.Values, text) {
				return ErrInvalidModelVersion
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return ErrInvalidModelVersion
			}
		case "integer":
			number, ok := value.(json.Number)
			if !ok {
				return ErrInvalidModelVersion
			}
			n, err := number.Int64()
			if err != nil || n < rule.Minimum || n > rule.Maximum || rule.Step <= 0 || n%rule.Step != 0 {
				return ErrInvalidModelVersion
			}
		default:
			return ErrInvalidModelVersion
		}
	}
	return nil
}
