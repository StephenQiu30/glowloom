package workspace_test

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

// workspaceReferenceAllocation measures only the actual guard transaction. SQL
// fixture creation and the caller's GC happen outside the allocation interval.
func workspaceReferenceAllocation(t *testing.T, db *gorm.DB, actor identityapp.Principal, project uuid.UUID, wantError bool) uint64 {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var found bool
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var role string
		if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil {
			return err
		}
		if role != "lanverse_app" {
			t.Error("guard did not run with the actual nonowner role")
		}
		var err error
		found, err = pgworkspace.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, uuid.New())
		return err
	})
	runtime.ReadMemStats(&after)
	if found || (err != nil) != wantError {
		t.Fatalf("history budget rejection must remain unavailable, never an empty result: found=%t err=%v", found, err)
	}
	return after.TotalAlloc - before.TotalAlloc
}

func workspaceReferenceBudgetCase(t *testing.T, family string, rows, bodySize int) {
	t.Helper()
	ctx, db, actor, project := workspaceReferenceFixture(t)
	// Warm each SQL/driver path, then retain the largest of three empty-authority
	// baselines so one-time preparation and ordinary runtime noise cannot create
	// a false positive. Every measured rejection is repeated independently.
	_ = workspaceReferenceAllocation(t, db, actor, project, false)
	var baseline uint64
	for range 3 {
		baseline = max(baseline, workspaceReferenceAllocation(t, db, actor, project, false))
	}
	query := `INSERT INTO workspace.project_change_command(id,org_id,project_id,actor_id,idem_key,action,request_sha256,status_code,response_body) SELECT gen_random_uuid(),?,?,?,gen_random_uuid(),'patch',repeat('a',64),200,jsonb_build_object('padding',repeat('x',?)) FROM generate_series(1,?)`
	args := []any{actor.OrgID, project, actor.ID, bodySize, rows}
	if family == "folder" {
		query = `INSERT INTO workspace.project_folder_command(id,org_id,actor_id,idem_key,action,request_sha256,response_body) SELECT gen_random_uuid(),?,?,gen_random_uuid(),'patch',repeat('a',64),jsonb_build_object('padding',repeat('x',?)) FROM generate_series(1,?)`
		args = []any{actor.OrgID, actor.ID, bodySize, rows}
	}
	if err := db.WithContext(ctx).Exec(query, args...).Error; err != nil {
		t.Fatal("insert only this synthetic UUID owner's retained history", err)
	}
	// Existing contracts are 100000 rows per family, 64 MiB combined history
	// and 1 MiB per body. Rejecting before payload retrieval stays close to the
	// empty baseline; even one old oversized body requires several MiB to scan.
	for sample := range 3 {
		allocated := workspaceReferenceAllocation(t, db, actor, project, true)
		t.Logf("family=%s rows=%d body=%d sample=%d baseline=%d allocated=%d", family, rows, bodySize, sample, baseline, allocated)
		if allocated > baseline+1<<20 {
			t.Errorf("over-budget history was allocated before refusal: allocated=%d baseline=%d", allocated, baseline)
		}
	}
}

func TestWorkspaceMediaReferenceGuardRejectsOversizedBodiesBeforeAllocation(t *testing.T) {
	for _, family := range []string{"project", "folder"} {
		t.Run(family, func(t *testing.T) { workspaceReferenceBudgetCase(t, family, 1, 2<<20) })
	}
}

func TestWorkspaceMediaReferenceGuardRejectsAggregateHistoryBeforeAllocation(t *testing.T) {
	for _, family := range []string{"project", "folder"} {
		t.Run(family, func(t *testing.T) { workspaceReferenceBudgetCase(t, family, 257, 256<<10) })
	}
}

func TestWorkspaceMediaReferenceGuardRejectsExcessRowsBeforeAllocation(t *testing.T) {
	for _, family := range []string{"project", "folder"} {
		t.Run(family, func(t *testing.T) { workspaceReferenceBudgetCase(t, family, 100001, 0) })
	}
}
