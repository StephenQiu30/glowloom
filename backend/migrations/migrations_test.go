package migrations

import "testing"

func TestEmbeddedMigrationInventoryStartsWithCompatibilityBaseline(t *testing.T) {
	inventory, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 {
		t.Fatalf("migration count = %d, want 1", len(inventory))
	}
	baseline := inventory[0]
	if baseline.Version != 1 || baseline.Name != "compatibility_runtime_baseline" {
		t.Fatalf("baseline identity = %d/%q", baseline.Version, baseline.Name)
	}
	if len(baseline.Checksum) != 64 {
		t.Fatalf("baseline checksum = %q", baseline.Checksum)
	}
	if len(baseline.SQL) < 100_000 {
		t.Fatalf("baseline SQL is unexpectedly small: %d bytes", len(baseline.SQL))
	}
}
