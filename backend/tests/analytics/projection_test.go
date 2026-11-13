package analytics_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/analytics/domain"
)

func TestAnalyticsFilterRejectsUnboundedAndUnsupportedFacts(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	for _, raw := range []string{"scope=team", "scope=personal&scope=platform", "from=2026-01-01T00:00:00Z", "model_id=bad", "project_id=00000000-0000-0000-0000-000000000000", "from=bad", "currency=CNY", "origin=secret", "to=2027-01-01T00:00:00Z", "from=2026-11-01T00:00:00Z&to=2026-10-01T00:00:00Z"} {
		if _, err := domain.ParseFilter(raw, now); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	f, err := domain.ParseFilter("", now)
	if err != nil || f.Scope != "personal" || f.To.Sub(f.From) != 7*24*time.Hour {
		t.Fatal("defaults", f, err)
	}
}
func TestCreationProjectionCountsOperationsAttemptsAndOutputsSeparately(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	f, _ := domain.ParseFilter("", now)
	model := uuid.New()
	id := uuid.New()
	facts := []domain.Fact{
		{ID: id, AcceptedAt: now.Add(-time.Hour), Status: "completed", ModelID: &model, Origin: "canvas", Attempts: 2, Outputs: 3},
		{ID: id, AcceptedAt: now.Add(-time.Hour), Status: "completed", ModelID: &model, Origin: "canvas", Attempts: 2, Outputs: 3},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "failed", Origin: "agent", Attempts: 1},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "cancelled", Origin: "canvas"},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "unknown", Origin: "canvas", Attempts: 1},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "ingesting", Origin: "canvas", Outputs: 1},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "quoted", Origin: "canvas"},
		{ID: uuid.New(), AcceptedAt: now.Add(-time.Hour), Status: "draft", Origin: "canvas"},
	}
	p := domain.Project(facts, f, now, false)
	if p.AcceptedOperations != 5 || p.Attempts != 4 || p.Outputs != 4 || p.CompletedOperations != 1 || p.FailedOperations != 1 || p.CancelledOperations != 1 || p.UnknownOperations != 1 || p.PartialOperations != 1 {
		t.Fatalf("projection %+v", p)
	}
	if p.TerminalSuccessRate == nil || *p.TerminalSuccessRate != 1.0/3.0 {
		t.Fatal("terminal denominator", p.TerminalSuccessRate)
	}
	if len(p.Daily) != 8 || len(p.Models) != 2 || p.CostMicros != nil || p.RevenueMicros != nil || p.ProfitMicros != nil {
		t.Fatal("missing facts disguised", p)
	}
	if p.Coverage.Status != "partial" {
		t.Fatal("coverage not declared")
	}
}

func TestCreationDailyUsesShanghaiAndPartialExcludesTerminalDenominator(t *testing.T) {
	from := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	f := domain.Filter{Scope: "personal", From: from, To: to}
	facts := []domain.Fact{{ID: uuid.New(), AcceptedAt: from, Status: "completed", Origin: "canvas"}, {ID: uuid.New(), AcceptedAt: from.Add(time.Hour), Status: "failed", Origin: "agent", Outputs: 1}, {ID: uuid.New(), AcceptedAt: to, Status: "completed", Origin: "canvas"}}
	p := domain.Project(facts, f, to, false)
	if p.Timezone != "Asia/Shanghai" || len(p.Daily) != 1 || p.Daily[0].Date != "2026-11-13" || p.Daily[0].Accepted != 2 || p.AcceptedOperations != 2 || p.PartialOperations != 1 || p.TerminalSuccessRate == nil || *p.TerminalSuccessRate != 1 {
		t.Fatalf("cohort %+v", p)
	}
}
