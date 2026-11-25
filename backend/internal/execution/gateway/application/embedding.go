package application

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// EmbeddingInput represents exactly one query or source passage.
type EmbeddingInput struct {
	Model   string `json:"model"`
	Text    string `json:"text"`
	Purpose string `json:"purpose"`
}

func prepareEmbedding(c domain.Call, raw json.RawMessage) (domain.Call, error) {
	if c.Origin != OnboardingOrigin && (c.Origin != domain.EmbeddingOrigin || c.Project == uuid.Nil || c.Run == uuid.Nil) {
		return c, domain.ErrInvalid
	}
	var in EmbeddingInput
	if json.Unmarshal(raw, &in) != nil || in.Model != c.Model.Name || strings.TrimSpace(in.Text) == "" || (in.Purpose != "query" && in.Purpose != "document") {
		return c, domain.ErrInvalid
	}
	encoded, _ := json.Marshal(in)
	expected, err := domain.CanonicalJSON(encoded)
	incoming, e := domain.CanonicalJSON(raw)
	if err != nil || e != nil || string(incoming) != string(expected) {
		return c, domain.ErrInvalid
	}
	upper := int64(len(encoded)) + 256
	if upper > c.Model.MaxInput {
		return c, domain.ErrInvalid
	}
	c.Raw = expected
	identity, _ := json.Marshal([]any{c.Scope, c.Actor, c.Project, c.Run, c.Origin, c.Model.ID, in})
	c.Hash = domain.Digest(identity)
	price, err := c.Model.Price.Cost(domain.Usage{Input: upper})
	if err != nil {
		return c, err
	}
	c.Limit, err = domain.Credits(price)
	return c, err
}
