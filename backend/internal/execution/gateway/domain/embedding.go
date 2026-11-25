package domain

import (
	"bytes"
	"encoding/json"
	"math"
)

// The native Ark text retrieval contract pins a single embedding space.
const (
	EmbeddingCapability = "text.embed"
	EmbeddingOrigin     = "project_rag"
	EmbeddingProtocol   = "ark_embedding"
	EmbeddingModel      = "doubao-embedding-vision-251215"
	EmbeddingBaseURL    = "https://ark.cn-beijing.volces.com/api/v3"
	EmbeddingDimensions = 1024
)

// EmbeddingVector decodes the bounded normalized receipt used by search and onboarding.
func EmbeddingVector(raw json.RawMessage) ([]float32, error) {
	var v []float32
	if bytes.Contains(raw, []byte("null")) {
		return nil, ErrInvalid
	}
	if err := json.Unmarshal(raw, &v); err != nil || len(v) != EmbeddingDimensions {
		return nil, ErrInvalid
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, ErrInvalid
	}
	return v, nil
}
