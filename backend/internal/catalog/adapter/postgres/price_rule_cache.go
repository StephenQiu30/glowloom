package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// PriceRuleCache stores immutable rule JSON by the exact published price version ID.
type PriceRuleCache interface {
	GetPriceRules(context.Context, []uuid.UUID) (map[uuid.UUID]json.RawMessage, error)
	PutPriceRules(context.Context, map[uuid.UUID]json.RawMessage) error
}

// WithPriceRuleCache injects a best-effort read cache for public model catalog rules.
func (s *Store) WithPriceRuleCache(cache PriceRuleCache) *Store {
	bound := *s
	bound.priceRuleCache = cache
	return &bound
}
