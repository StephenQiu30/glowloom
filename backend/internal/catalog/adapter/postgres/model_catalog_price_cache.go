package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// loadModelCatalogPriceRules reads immutable price versions from Redis when warm.
// PostgreSQL chooses each active version and remains the authority on cache misses.
func (s *Store) loadModelCatalogPriceRules(ctx context.Context, tx *gorm.DB, rows []modelCatalogRow) error {
	ids := make([]uuid.UUID, 0, len(rows))
	seen := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		if row.PriceID == nil {
			continue
		}
		if _, exists := seen[*row.PriceID]; exists {
			continue
		}
		seen[*row.PriceID] = struct{}{}
		ids = append(ids, *row.PriceID)
	}
	if len(ids) == 0 {
		return nil
	}

	rules := make(map[uuid.UUID]json.RawMessage, len(ids))
	if s.priceRuleCache != nil {
		cached, err := s.priceRuleCache.GetPriceRules(ctx, ids)
		if err == nil {
			for id, rule := range cached {
				if _, expected := seen[id]; expected && validCatalogPriceRuleJSON(rule) {
					rules[id] = append(json.RawMessage(nil), rule...)
				}
			}
		}
	}

	missing := make([]uuid.UUID, 0, len(ids)-len(rules))
	for _, id := range ids {
		if _, ok := rules[id]; !ok {
			missing = append(missing, id)
		}
	}
	loaded := make(map[uuid.UUID]json.RawMessage, len(missing))
	if len(missing) > 0 {
		var dbRules []struct {
			ID   uuid.UUID
			Rule string
		}
		result := tx.WithContext(ctx).Raw(`
			SELECT id,rule::text AS rule FROM catalog.price_rule_version
			WHERE id IN ? AND NOT is_delete`, missing).Scan(&dbRules)
		if result.Error != nil {
			return fmt.Errorf("read current model price rules: %w", result.Error)
		}
		for _, row := range dbRules {
			rule := json.RawMessage(row.Rule)
			if !validCatalogPriceRuleJSON(rule) {
				return fmt.Errorf("read current model price rules: invalid rule for version %s", row.ID)
			}
			rules[row.ID] = rule
			loaded[row.ID] = rule
		}
		for _, id := range missing {
			if _, ok := rules[id]; !ok {
				return fmt.Errorf("read current model price rules: version %s disappeared", id)
			}
		}
	}
	if len(loaded) > 0 && s.priceRuleCache != nil {
		// Cache outages must not prevent the database-backed catalog read.
		_ = s.priceRuleCache.PutPriceRules(ctx, loaded)
	}
	for index := range rows {
		if rows[index].PriceID == nil {
			continue
		}
		rule, ok := rules[*rows[index].PriceID]
		if !ok {
			return fmt.Errorf("read current model price rules: version %s is missing", *rows[index].PriceID)
		}
		value := string(rule)
		rows[index].PriceRule = &value
	}
	return nil
}

func validCatalogPriceRuleJSON(raw []byte) bool {
	var fields map[string]json.RawMessage
	return len(raw) > 1 && json.Unmarshal(raw, &fields) == nil && fields != nil
}
