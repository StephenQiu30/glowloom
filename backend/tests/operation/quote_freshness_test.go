package operation_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestQuoteFreshnessReasons(t *testing.T) {
	quoted := validQuotedOperation()
	base := application.QuoteCurrentFacts{
		Now:                   quoted.QuoteExpiresAt.Add(-time.Nanosecond),
		TargetVersionNo:       quoted.TargetVersionNo,
		ModelProfileVersionID: quoted.ModelProfileVersionID,
		PriceRuleVersionID:    quoted.PriceRuleVersionID,
	}
	for _, tc := range []struct {
		name  string
		facts application.QuoteCurrentFacts
		want  []string
	}{
		{"still fresh", base, nil},
		{"at expiry", func() application.QuoteCurrentFacts { v := base; v.Now = *quoted.QuoteExpiresAt; return v }(), []string{"quote_expired"}},
		{"target changed", func() application.QuoteCurrentFacts { v := base; v.TargetVersionNo = int32Ptr(3); return v }(), []string{"target_version_changed"}},
		{"target disappeared", func() application.QuoteCurrentFacts { v := base; v.TargetVersionNo = nil; return v }(), []string{"target_version_changed"}},
		{"model changed", func() application.QuoteCurrentFacts {
			v := base
			v.ModelProfileVersionID = uuidPtr(uuid.New())
			return v
		}(), []string{"model_version_changed"}},
		{"model unavailable", func() application.QuoteCurrentFacts { v := base; v.ModelProfileVersionID = nil; return v }(), []string{"model_unavailable"}},
		{"price changed", func() application.QuoteCurrentFacts { v := base; v.PriceRuleVersionID = uuidPtr(uuid.New()); return v }(), []string{"price_version_changed"}},
		{"price unavailable", func() application.QuoteCurrentFacts { v := base; v.PriceRuleVersionID = nil; return v }(), []string{"price_unavailable"}},
		{"consent revoked", func() application.QuoteCurrentFacts { v := base; v.ConsentRevoked = true; return v }(), []string{"consent_revoked"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := application.QuoteFreshnessReasons(quoted, tc.facts)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("freshness = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	stale := base
	stale.Now = *quoted.QuoteExpiresAt
	stale.TargetVersionNo = int32Ptr(3)
	stale.ModelProfileVersionID = nil
	stale.PriceRuleVersionID = nil
	stale.ConsentRevoked = true
	got, err := application.QuoteFreshnessReasons(quoted, stale)
	want := []string{"quote_expired", "target_version_changed", "model_unavailable", "price_unavailable", "consent_revoked"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("multiple stale reasons = %v, %v; want %v", got, err, want)
	}
}

func TestQuoteFreshnessHandlesReuseAndAgentSession(t *testing.T) {
	quoted := validQuotedOperation()
	quoted.ReusedFromID = uuidPtr(uuid.New())
	zero := int64(0)
	quoted.QuoteMicros = &zero
	facts := application.QuoteCurrentFacts{
		Now:                    quoted.CreateTime,
		TargetVersionNo:        quoted.TargetVersionNo,
		ModelProfileVersionID:  quoted.ModelProfileVersionID,
		PriceRuleVersionID:     quoted.PriceRuleVersionID,
		ReuseSourceUnavailable: true,
	}
	if got, err := application.QuoteFreshnessReasons(quoted, facts); err != nil || !reflect.DeepEqual(got, []string{"reuse_source_unavailable"}) {
		t.Fatalf("revoked reuse source = %v, %v", got, err)
	}
	quoted = validQuotedOperation()
	quoted.TargetType = "agent_session"
	quoted.TargetID = nil
	quoted.TargetVersionNo = nil
	quoted.ModelProfileVersionID = nil
	quoted.PriceRuleVersionID = nil
	facts = application.QuoteCurrentFacts{Now: quoted.CreateTime}
	if got, err := application.QuoteFreshnessReasons(quoted, facts); err != nil || len(got) != 0 {
		t.Fatalf("agent session without model snapshot = %v, %v", got, err)
	}
	quoted.Status = domain.StatusConfirmed
	if _, err := application.QuoteFreshnessReasons(quoted, facts); !errors.Is(err, domain.ErrQuoteNotConfirmable) {
		t.Fatalf("already confirmed quote = %v", err)
	}
}
