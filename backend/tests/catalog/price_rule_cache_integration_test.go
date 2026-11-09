package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	catalogredis "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/redis"
)

func TestPriceRuleCacheSeparatesImmutableVersions(t *testing.T) {
	address := os.Getenv("GLOWLOOM_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set GLOWLOOM_TEST_REDIS_ADDR to an isolated Redis endpoint")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := redisclient.NewClient(&redisclient.Options{Addr: address})
	defer func() { _ = client.Close() }()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect isolated Redis: %v", err)
	}

	firstID, secondID := uuid.New(), uuid.New()
	defer func() {
		_ = client.Del(context.Background(), "catalog:price-rule:v1:"+firstID.String(), "catalog:price-rule:v1:"+secondID.String()).Err()
	}()
	cache := catalogredis.NewPriceRuleCache(client)
	firstRule := json.RawMessage(`{"base_micros":120000}`)
	secondRule := json.RawMessage(`{"base_micros":150000}`)
	if err := cache.PutPriceRules(ctx, map[uuid.UUID]json.RawMessage{firstID: firstRule, secondID: secondRule}); err != nil {
		t.Fatal(err)
	}
	got, err := cache.GetPriceRules(ctx, []uuid.UUID{secondID, firstID})
	if err != nil || string(got[firstID]) != string(firstRule) || string(got[secondID]) != string(secondRule) {
		t.Fatalf("cache returned mismatched immutable rules: got=%v err=%v", got, err)
	}
	missingID := uuid.New()
	got, err = cache.GetPriceRules(ctx, []uuid.UUID{missingID})
	if err != nil || len(got) != 0 {
		t.Fatalf("missing immutable version should be a cache miss: got=%v err=%v", got, err)
	}
}
