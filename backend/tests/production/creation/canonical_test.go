package creation_test

import (
	"encoding/json"
	"os"
	"testing"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

func TestCreationCommandUsesSharedProductionCanonicalHash(t *testing.T) {
	raw, err := os.ReadFile("testdata/command.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Command     domain.Command `json:"command"`
		PayloadHash string         `json:"payload_hash"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	actual, err := app.PayloadHash(fixture.Command)
	if err != nil {
		t.Fatal(err)
	}
	if actual != fixture.PayloadHash {
		t.Fatalf("Go/Python command hash mismatch: %s != %s", actual, fixture.PayloadHash)
	}
}
