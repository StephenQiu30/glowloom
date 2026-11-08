package config

import (
	"strings"
	"testing"
)

func TestParseCreditServiceCallersValidatesScopeActionAndRotationKeys(t *testing.T) {
	raw := `[{"id":"short-drama","scopes":["00000000-0000-4000-8000-000000000001"],"actions":["freeze","commit","release"],"keys":[{"id":"old","secret":"` + strings.Repeat("o", 40) + `"},{"id":"new","secret":"` + strings.Repeat("n", 40) + `"}]}]`
	callers, err := parseCreditServiceCallers(raw)
	if err != nil {
		t.Fatalf("parse configured callers: %v", err)
	}
	if len(callers) != 1 || callers[0].ID != "short-drama" || len(callers[0].Scopes) != 1 || len(callers[0].Actions) != 3 || len(callers[0].Keys) != 2 {
		t.Fatalf("parsed caller shape = %+v", callers)
	}
}

func TestParseCreditServiceCallersRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "short key", raw: `[{"id":"svc","scopes":["00000000-0000-4000-8000-000000000001"],"actions":["freeze"],"keys":[{"id":"k","secret":"short"}]}]`},
		{name: "unknown action", raw: `[{"id":"svc","scopes":["00000000-0000-4000-8000-000000000001"],"actions":["admin"],"keys":[{"id":"k","secret":"` + strings.Repeat("k", 40) + `"}]}]`},
		{name: "noncanonical scope", raw: `[{"id":"svc","scopes":["00000000-0000-4000-8000-00000000000A"],"actions":["freeze"],"keys":[{"id":"k","secret":"` + strings.Repeat("k", 40) + `"}]}]`},
		{name: "unknown field", raw: `[{"id":"svc","scopes":["00000000-0000-4000-8000-000000000001"],"actions":["freeze"],"keys":[{"id":"k","secret":"` + strings.Repeat("k", 40) + `"}],"allow_all":true}]`},
		{name: "trailing value", raw: `[] []`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseCreditServiceCallers(test.raw); err == nil {
				t.Fatal("unsafe caller configuration was accepted")
			}
		})
	}
}

func TestCreditCommandLimitsRequireSingleAmountWithinDailyCap(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "single limit exceeds daily limit",
			config: Config{
				CreditCommandMaxAmountMicros: 101,
				CreditCommandMaxDailyMicros:  100,
			},
		},
		{
			name: "only single limit configured",
			config: Config{
				CreditCommandMaxAmountMicros: 100,
			},
		},
		{
			name: "service caller without command limits",
			config: Config{
				CreditServiceCallers: []CreditServiceCaller{{}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.validate(); err == nil || !strings.Contains(err.Error(), "credit command") {
				t.Fatalf("validate credit limits error = %v, want a credit command limit error", err)
			}
		})
	}
}
