package asset_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
)

func TestProductionWorldAssetOwnerPublishesOneIdentityWithMultipleStatesInCallerTransaction(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Production World Asset owner journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedProductionWorldAssetOwner(t, database, now, workspaceID, projectID, userID)
	testgorm.RegisterOwnedFixtureCleanup(t, database, testgorm.OwnedFixture{
		UserID: userID.String(), WorkspaceID: workspaceID.String(), ProjectID: projectID.String(),
	})
	owner := assetapp.NewProductionWorldAssetOwner(func() time.Time { return now }, uuid.NewString)
	identities := []assetapp.ProductionWorldAssetIdentityInput{{
		IdentityKey: "character:linzhou", Kind: "character",
		States: []assetapp.ProductionWorldAssetStateInput{
			{StateKey: "state_character_linzhou_initial", Snapshot: json.RawMessage(`{"appearance":"plain","scene_scopes":["scene:first"]}`)},
			{StateKey: "state_character_linzhou_injured", Snapshot: json.RawMessage(`{"appearance":"injured","scene_scopes":["scene:second"]}`)},
		},
	}}
	command := assetapp.ApplyProductionWorldAssetsCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), ActorID: userID.String(),
		ExpectedBusinessKeyRoot: productionWorldAssetBusinessKeyRoot([]string{
			"asset_identity:character:linzhou",
			"asset_state:state_character_linzhou_initial",
			"asset_state:state_character_linzhou_injured",
		}),
		Identities: identities,
	}
	var result assetapp.ApplyProductionWorldAssetsResult
	err = platformdatabase.WithinTransaction(ctx, database, func(transaction *gorm.DB) error {
		var applyErr error
		result, applyErr = owner.ApplyProductionWorldAssets(
			ctx, assetgorm.NewProductionWorldRepository(transaction), command,
		)
		return applyErr
	})
	if err != nil {
		t.Fatalf("apply Production World Assets: %v", err)
	}
	if len(result.Assets) != 1 || len(result.States) != 2 || result.Assets[0].ID != result.States[0].AssetID ||
		result.Assets[0].ID != result.States[1].AssetID || result.Head.MemberCount != 2 ||
		result.Head.HeadRevision != 1 || len(result.Head.CollectionRootHash) != 64 {
		t.Fatalf("Production World Asset result = %#v", result)
	}
	assertProductionWorldAssetCounts(t, database, projectID, 1, 2, 2, 1)

	stale := command
	stale.Identities = []assetapp.ProductionWorldAssetIdentityInput{{
		IdentityKey: "prop:key", Kind: "prop",
		States: []assetapp.ProductionWorldAssetStateInput{{StateKey: "state_prop_key_initial", Snapshot: json.RawMessage(`{"holder":"character:linzhou"}`)}},
	}}
	stale.ExpectedBusinessKeyRoot = productionWorldAssetBusinessKeyRoot([]string{
		"asset_identity:prop:key", "asset_state:state_prop_key_initial",
	})
	err = platformdatabase.WithinTransaction(ctx, database, func(transaction *gorm.DB) error {
		_, applyErr := owner.ApplyProductionWorldAssets(ctx, assetgorm.NewProductionWorldRepository(transaction), stale)
		return applyErr
	})
	if !errors.Is(err, assetapp.ErrIdentityStateHeadConflict) {
		t.Fatalf("stale Asset Head error = %v", err)
	}
	assertProductionWorldAssetCounts(t, database, projectID, 1, 2, 2, 1)

	advanced := command
	advanced.ExpectedHeadRevision = result.Head.HeadRevision
	advanced.ExpectedHeadHash = result.Head.HeadContentHash
	advanced.Identities = append([]assetapp.ProductionWorldAssetIdentityInput(nil), identities...)
	advanced.Identities[0].States = append(
		append([]assetapp.ProductionWorldAssetStateInput(nil), identities[0].States...),
		assetapp.ProductionWorldAssetStateInput{
			StateKey: "state_character_linzhou_recovered",
			Snapshot: json.RawMessage(`{"appearance":"recovered","scene_scopes":["scene:third"]}`),
		},
	)
	advanced.ExpectedBusinessKeyRoot = productionWorldAssetBusinessKeyRoot([]string{
		"asset_identity:character:linzhou",
		"asset_state:state_character_linzhou_initial",
		"asset_state:state_character_linzhou_injured",
		"asset_state:state_character_linzhou_recovered",
	})
	var advancedResult assetapp.ApplyProductionWorldAssetsResult
	err = platformdatabase.WithinTransaction(ctx, database, func(transaction *gorm.DB) error {
		var applyErr error
		advancedResult, applyErr = owner.ApplyProductionWorldAssets(
			ctx, assetgorm.NewProductionWorldRepository(transaction), advanced,
		)
		return applyErr
	})
	if err != nil || advancedResult.Head.HeadRevision != 2 || advancedResult.Head.MemberCount != 3 ||
		len(advancedResult.Assets) != 1 || len(advancedResult.States) != 3 {
		t.Fatalf("advance Production World Assets: result=%#v err=%v", advancedResult, err)
	}
	assertProductionWorldAssetCounts(t, database, projectID, 1, 3, 5, 1)

	replayed := advanced
	replayed.ExpectedHeadRevision = advancedResult.Head.HeadRevision
	replayed.ExpectedHeadHash = advancedResult.Head.HeadContentHash
	var replayedResult assetapp.ApplyProductionWorldAssetsResult
	err = platformdatabase.WithinTransaction(ctx, database, func(transaction *gorm.DB) error {
		var applyErr error
		replayedResult, applyErr = owner.ApplyProductionWorldAssets(
			ctx, assetgorm.NewProductionWorldRepository(transaction), replayed,
		)
		return applyErr
	})
	if err != nil || replayedResult.Head.HeadRevision != advancedResult.Head.HeadRevision ||
		replayedResult.Head.HeadContentHash != advancedResult.Head.HeadContentHash {
		t.Fatalf("replay identical Asset collection: result=%#v err=%v", replayedResult, err)
	}
	assertProductionWorldAssetCounts(t, database, projectID, 1, 3, 5, 1)

	rollbackProjectID := uuid.New()
	if err = database.Create(&model.Project{
		ID: rollbackProjectID, WorkspaceID: workspaceID, Name: "Rollback Assets", AspectRatio: "9:16",
		Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed rollback project: %v", err)
	}
	rollback := command
	rollback.ProjectID = rollbackProjectID.String()
	rollback.ExpectedBusinessKeyRoot = command.ExpectedBusinessKeyRoot
	forcedRollback := errors.New("force coordinator rollback")
	err = platformdatabase.WithinTransaction(ctx, database, func(transaction *gorm.DB) error {
		if _, applyErr := owner.ApplyProductionWorldAssets(ctx, assetgorm.NewProductionWorldRepository(transaction), rollback); applyErr != nil {
			return applyErr
		}
		return forcedRollback
	})
	if !errors.Is(err, forcedRollback) {
		t.Fatalf("forced rollback error = %v", err)
	}
	assertProductionWorldAssetCounts(t, database, rollbackProjectID, 0, 0, 0, 0)
	if err = database.Unscoped().Delete(&model.Project{}, "id = ?", rollbackProjectID).Error; err != nil {
		t.Fatalf("delete rollback project: %v", err)
	}
}

func seedProductionWorldAssetOwner(
	t *testing.T,
	database *gorm.DB,
	now time.Time,
	workspaceID, projectID, userID uuid.UUID,
) {
	t.Helper()
	if err := database.Create(&model.UserAccount{
		ID: userID, EmailNormalized: "production-world-asset-" + userID.String() + "@example.test",
		PasswordHash: "test", TokenVersion: 1, DisplayName: "Production World Asset Owner",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := database.Create(&model.Workspace{
		ID: workspaceID, Name: "Production World Asset", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := database.Create(&model.Membership{
		ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "editor", Status: "active", JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if err := database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Production World Asset", AspectRatio: "9:16",
		Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func assertProductionWorldAssetCounts(
	t *testing.T,
	database *gorm.DB,
	projectID uuid.UUID,
	assets, states, memberships, heads int64,
) {
	t.Helper()
	for record, expected := range map[any]int64{
		&model.Asset{}:                        assets,
		&model.AssetState{}:                   states,
		&model.AssetIdentityStateMembership{}: memberships,
		&model.AssetIdentityStateScopeHead{}:  heads,
	} {
		var count int64
		if err := database.Model(record).Where("project_id = ?", projectID).Count(&count).Error; err != nil {
			t.Fatalf("count %T: %v", record, err)
		}
		if count != expected {
			t.Fatalf("%T count = %d, want %d", record, count, expected)
		}
	}
}

func productionWorldAssetBusinessKeyRoot(keys []string) string {
	encoded, _ := json.Marshal(keys)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
