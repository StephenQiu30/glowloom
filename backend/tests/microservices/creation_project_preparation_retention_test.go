package microservices_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	canvaspg "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"

	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
)

// The original production retention worker clears payload and retains project /
// canvas tombstones. This checks that exact behavior, not a new root-row DELETE.
func assertProjectPreparationRetention(t *testing.T, f *sourcePolicyFixture, spec workspacesource.ProjectPreparationSpec) {
	t.Helper()
	type immutable struct {
		SourceSHA256        string `gorm:"column:source_sha256"`
		ProjectID, CanvasID uuid.UUID
		Wallet, Activation  string
	}
	readReceipt := func() immutable {
		var receipt immutable
		if err := f.creation.Raw(`SELECT source_sha256,project_id,canvas_id,wallet_receipt::text AS wallet,activation_receipt::text AS activation FROM workspace.project_preparation WHERE preparation_id=?`, spec.PreparationID).Scan(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	before := readReceipt()
	if before.Activation == "" || before.Wallet == "" {
		t.Fatal("retention fixture has no actual activated preparation")
	}
	pending, pendingSpec := projectPreparationFixture(t)
	if _, err := pgsource.New(pending.creation).PrepareProject(t.Context(), pendingSpec); err != nil {
		t.Fatal(err)
	}
	// Even a failed/cancelled hidden container with an expired deletion marker
	// cannot enter ordinary active/archived retention while preparation is unknown.
	for _, fixture := range []struct {
		db      *gorm.DB
		project uuid.UUID
	}{{pending.admin, pendingSpec.Project.ID}, {f.admin, spec.Project.ID}} {
		if err := fixture.db.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=now()-interval '31 days',purge_after=now()-interval '1 day',revision=revision+1 WHERE id=?`, fixture.project).Error; err != nil {
			t.Fatal(err)
		}
	}
	retention := workspacepg.NewProjectRetentionStore(f.creation, func(tx *gorm.DB) workspacepg.ProjectRetentionContent { return canvaspg.NewProjectRetentionStore(tx) })
	count, err := retention.EnqueueExpired(t.Context(), 10)
	if err != nil || count != 1 {
		t.Fatalf("pending preparation entered ordinary retention: count=%d %v", count, err)
	}
	processed, err := retention.ProcessNext(t.Context())
	if err != nil || !processed {
		t.Fatalf("actual original retention worker: %t %v", processed, err)
	}
	var canvas struct {
		Name, Viewport, Scope string
		IsDelete              bool
	}
	if err := f.creation.Raw(`SELECT name,viewport::text AS viewport,scope::text AS scope,is_delete FROM canvas.document WHERE id=?`, spec.Project.CanvasID).Scan(&canvas).Error; err != nil {
		t.Fatal(err)
	}
	if canvas.Name != "已删除画布" || !canvas.IsDelete || canvas.Viewport != "{}" || canvas.Scope != "{}" {
		t.Fatalf("original canvas payload not cleared: %+v", canvas)
	}
	if after := readReceipt(); after != before {
		t.Fatal("retention rewrote original immutable preparation IDs/ACK/receipt")
	}
	var pendingFacts struct {
		Name               string
		Jobs, Preparations int64
	}
	if err := pending.creation.Raw(`SELECT c.name,(SELECT count(*) FROM workspace.project_retention_job WHERE project_id=?) AS jobs,(SELECT count(*) FROM workspace.project_preparation WHERE preparation_id=? AND activated_at IS NULL AND wallet_receipt IS NULL) AS preparations FROM canvas.document c WHERE c.id=?`, pendingSpec.Project.ID, pendingSpec.PreparationID, pendingSpec.Project.CanvasID).Scan(&pendingFacts).Error; err != nil {
		t.Fatal(err)
	}
	if pendingFacts.Name != pendingSpec.Project.Name || pendingFacts.Jobs != 0 || pendingFacts.Preparations != 1 {
		t.Fatalf("unknown preparation lost content/responsibility: %+v", pendingFacts)
	}
	store := pgsource.New(f.creation)
	scope, err := store.ReadProjectCreationScope(t.Context(), spec.ActorID, spec.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LookupProjectCreation(t.Context(), workspacesource.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}, scope, spec.IdempotencyKey, spec.InputSHA256); !errors.Is(err, workspacesource.ErrProjectNotFound) {
		t.Fatalf("deleted project disclosed old creation receipt: %v", err)
	}
}
