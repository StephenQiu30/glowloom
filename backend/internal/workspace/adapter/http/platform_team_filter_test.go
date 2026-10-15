package http

import "testing"

func TestPlatformTeamFilterRequiredQueries(t *testing.T) {
	for _, query := range []string{
		"id=11111111-1111-4111-8111-111111111111",
		"administrator_id=11111111-1111-4111-8111-111111111111",
		"created_from=2026-10-15T00:00:00Z&created_to=2026-10-16T00:00:00Z",
	} {
		if _, err := platformTeamFilter(query); err != nil {
			t.Errorf("required filter rejected %s: %v", query, err)
		}
	}
	for _, query := range []string{
		"id=", "id=invalid", "id=00000000-0000-0000-0000-000000000000", "administrator_id=invalid",
		"created_from=2026-10-15", "created_to=invalid", "created_from=2026-10-16T00:00:00Z&created_to=2026-10-15T00:00:00Z",
		"created_from=2026-10-15T00:00:00Z&created_to=2026-10-15T00:00:00Z", "created_to=2026-10-15T00:00:00Z&created_to=2026-10-15T00:00:00Z",
		"id=11111111-1111-4111-8111-111111111111&id=11111111-1111-4111-8111-111111111111", "unknown_filter=x",
	} {
		if _, err := platformTeamFilter(query); err == nil {
			t.Errorf("invalid filter accepted: %s", query)
		}
	}
}
