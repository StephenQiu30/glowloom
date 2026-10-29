package domain

import "testing"

func TestPlanRejectsAmbiguousOrUnauthorizedActions(t *testing.T) {
	for _, raw := range []string{`{"action":"reply","action":"generate_image","message":"计划","prompt":"图片"}`, `{"action":"shell","message":"执行","prompt":"ls"}`, `{"action":"generate_image","message":"计划","prompt":"图","project_id":"another-project"}`, `{"action":"reply","message":"答复","prompt":""} {}`, `{"action":"generate_image","message":"计划","prompt":""}`} {
		if _, err := ParsePlan(raw); err == nil {
			t.Fatalf("accepted unsafe plan: %s", raw)
		}
	}
}
