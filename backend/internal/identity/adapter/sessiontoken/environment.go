package sessiontoken

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// LoadValues accepts base64 PEM material and a KID-to-base64-PEM JSON map.
// Empty signing material retains verification-only operation during rollback.
func LoadValues(privateValue, verifyValues, kid string) (*Codec, error) {
	if len(privateValue) > 16384 || len(verifyValues) > 65536 {
		return nil, errors.New("identity key configuration too large")
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(verifyValues), &values); err != nil {
		return nil, errors.New("invalid access verification key configuration")
	}
	keys := make(map[string][]byte, len(values))
	for id, value := range values {
		key, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil {
			return nil, errors.New("invalid access verification key encoding")
		}
		keys[id] = key
	}
	if privateValue == "" {
		return NewVerifier(keys)
	}
	private, err := base64.StdEncoding.Strict().DecodeString(privateValue)
	if err != nil {
		return nil, errors.New("invalid access signing key encoding")
	}
	defer clear(private)
	return New(kid, private, keys)
}

// LoadReceiptValue accepts a dedicated base64-encoded 32-byte receipt key.
func LoadReceiptValue(value string) (*ReceiptProtector, error) {
	if len(value) > 128 {
		return nil, errors.New("invalid refresh receipt key encoding")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid refresh receipt key encoding")
	}
	defer clear(key)
	return NewReceiptProtector(key)
}
