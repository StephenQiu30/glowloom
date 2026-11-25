// Package redis caches immutable catalog price rules for repeated model reads.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

const priceRuleTTL = time.Hour

// PriceRuleCache caches rule JSON only; PostgreSQL remains authoritative for version selection.
type PriceRuleCache struct{ client *redisclient.Client }

// NewPriceRuleCache injects the shared Redis client.
func NewPriceRuleCache(client *redisclient.Client) *PriceRuleCache {
	return &PriceRuleCache{client: client}
}

// GetPriceRules returns rules for immutable price-version IDs; absent keys are misses.
func (c *PriceRuleCache) GetPriceRules(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]json.RawMessage, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("catalog price cache unavailable")
	}
	if len(ids) == 0 || len(ids) > 200 {
		return nil, errors.New("invalid catalog price cache read")
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		if id == uuid.Nil {
			return nil, errors.New("invalid catalog price version ID")
		}
		keys[index] = priceRuleKey(id)
	}
	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("read catalog price cache: %w", err)
	}
	if len(values) != len(ids) {
		return nil, errors.New("catalog price cache returned an unexpected result count")
	}
	result := make(map[uuid.UUID]json.RawMessage, len(values))
	for index, value := range values {
		if value == nil {
			continue
		}
		var raw []byte
		switch value := value.(type) {
		case string:
			raw = []byte(value)
		case []byte:
			raw = value
		default:
			return nil, fmt.Errorf("decode catalog price cache value: unexpected type %T", value)
		}
		if !validRuleJSON(raw) {
			return nil, errors.New("decode catalog price cache value: invalid rule JSON")
		}
		result[ids[index]] = append(json.RawMessage(nil), raw...)
	}
	return result, nil
}

// PutPriceRules writes rules under immutable version keys with a bounded TTL.
func (c *PriceRuleCache) PutPriceRules(ctx context.Context, rules map[uuid.UUID]json.RawMessage) error {
	if c == nil || c.client == nil {
		return errors.New("catalog price cache unavailable")
	}
	if len(rules) == 0 || len(rules) > 200 {
		return errors.New("invalid catalog price cache write")
	}
	pipe := c.client.Pipeline()
	for id, raw := range rules {
		if id == uuid.Nil || !validRuleJSON(raw) {
			return errors.New("invalid catalog price cache value")
		}
		pipe.Set(ctx, priceRuleKey(id), raw, priceRuleTTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write catalog price cache: %w", err)
	}
	return nil
}

func priceRuleKey(id uuid.UUID) string { return "catalog:price-rule:v1:" + id.String() }

func validRuleJSON(raw []byte) bool {
	var fields map[string]json.RawMessage
	return len(raw) > 1 && json.Unmarshal(raw, &fields) == nil && fields != nil
}
