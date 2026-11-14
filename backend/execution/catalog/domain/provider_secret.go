package domain

import (
	"encoding/json"
	"errors"
)

// CredentialValue is used only inside credential-owning workers. Its result
// must never be returned to a client, logged, or placed in workflow history.
func (c ProviderConfig) CredentialValue(secret []byte) (string, error) {
	var value struct {
		APIKey string `json:"api_key"`
		AK     string `json:"access_key_id"`
		SK     string `json:"secret_access_key"`
	}
	if json.Unmarshal(secret, &value) != nil {
		return "", errors.New("invalid credential envelope")
	}
	if c.AdapterKey == "volcengine_music" {
		if value.AK == "" || value.SK == "" || value.APIKey != "" {
			return "", errors.New("missing music signing credential")
		}
		return string(secret), nil
	}
	if value.APIKey == "" {
		return "", errors.New("missing provider credential")
	}
	return value.APIKey, nil
}
