package workspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyPlacementRetiredTargetCannotRestore(t *testing.T) {
	now := time.Now().UTC()
	purge := now.Add(30 * 24 * time.Hour)
	p := domain.Project{Status: "copying", IsDelete: true, DeleteTime: &now, PurgeAfter: &purge, Revision: 2}
	if err := p.Restore(now.Add(time.Second)); !errors.Is(err, domain.ErrProjectStateConflict) || !p.IsDelete {
		t.Fatalf("unpublished cleaned target resurrected %+v %v", p, err)
	}
}
func TestProjectCopyPlacementLegacyInputGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/project_copy_legacy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if json.Unmarshal(raw, &golden) != nil {
		t.Fatal("invalid pinned golden")
	}
	var input workspaceapp.ProjectCopyInput
	if json.Unmarshal([]byte(golden["input_bytes"]), &input) != nil {
		t.Fatal("invalid pinned legacy request")
	}
	input.IdempotencyKey = uuid.New()
	input.RequestID = uuid.NewString()
	if input.Validate() != nil || input.Placement != nil {
		t.Fatal("legacy input invalid")
	}
	actual, err := json.Marshal(input)
	if err != nil || string(actual) != golden["input_bytes"] {
		t.Fatalf("nil placement changed pinned29ce bytes %s %v", actual, err)
	}
	sum := sha256.Sum256(append([]byte("project-copy/v1\x00"), actual...))
	if hex.EncodeToString(sum[:]) != golden["input_sha256"] {
		t.Fatal("legacy fingerprint changed")
	}
}
