package database

import (
	"testing"

	"github.com/StephenQiu30/lanverse/backend/migrations"
)

func TestAppliedMigrationVerificationAcceptsOnlyAuditedBaselineAdoption(t *testing.T) {
	inventory, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	baseline := inventory[0]

	if err = verifyAppliedMigration(baseline, appliedMigration{
		Name:   baseline.Name,
		Source: "adopted",
	}); err != nil {
		t.Fatal(err)
	}
	wrongName := appliedMigration{Name: "wrong", Source: "adopted"}
	if err = verifyAppliedMigration(baseline, wrongName); err == nil {
		t.Fatal("baseline name drift was accepted")
	}
	wrongChecksum := "wrong"
	if err = verifyAppliedMigration(baseline, appliedMigration{
		Name:     baseline.Name,
		Checksum: &wrongChecksum,
		Source:   "migration",
	}); err == nil {
		t.Fatal("baseline checksum drift was accepted")
	}
}
