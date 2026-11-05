package operation_test

import (
	"context"
	"errors"
	"testing"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/staged"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestM1StagedIngestPrivateImageCreatesOneCandidate(t *testing.T) {
	receiptTestObjects(t)
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	freezeReceiptFixtureModel(t, database, identity)
	businessPinOperation(t, database, identity)
	receipts := application.NewRoutedProviderStageService(staging.NewRoutedFactory(businessStorageRegistry(t), pgmedia.NewStorageRoutes(database)), store)
	launch := &codexTestLauncher{t: t, behavior: "success"}
	adapter, err := codex.New(store, launch, receipts, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Submit(t.Context(), application.ProviderImageInput{Identity: identity, Model: "fixture-model", Prompt: "生成测试图片"})
	if err != nil || result.Receipt == nil {
		t.Fatalf("stage image: %v", err)
	}
	// The fixture enters ingesting directly to exercise media ingestion. This does
	// not validate synchronous cost gating or authorize a real paid model call.
	if err := database.Exec(`UPDATE operation.operation SET status='ingesting' WHERE id=?::uuid`, identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	repo := &stagedFaultRepository{Repository: pgmedia.NewScopedIngestRepository(database, app.NewMediaWorkerScopeAccess), failSave: true}
	businessPinOperation(t, database, identity)
	service := mediaapp.NewRoutedStagedIngestService(repo, staged.NewReader(receipts), mediaflow.FFProber{}, mediaflow.FFRenderer{}, mediaobjects.NewGeneratedObjects(businessStorageRegistry(t), pgmedia.NewStorageRoutes(database)), pgmedia.NewStorageRoutes(database))
	input := mediaapp.IngestInput{OperationID: identity.OperationID.String(), SeqNo: 1, Receipt: result.Receipt}
	if _, err := service.Ingest(t.Context(), input); err == nil {
		t.Fatal("injected DB failure lost")
	}
	repo.failSave = false
	first, err := service.Ingest(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Ingest(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObjectLocation == nil || second.ObjectLocation == nil || first.ObjectLocation.ID != second.ObjectLocation.ID || first.ObjectLocation.Ref != second.ObjectLocation.Ref {
		t.Fatal("candidate replay changed physical identity", first, second)
	}
	// Ledger state and fact pointers are refreshed during readback. Replay must
	// preserve the complete immutable output and the same saved location identity.
	first.ObjectLocation, second.ObjectLocation = nil, nil
	if first != second {
		t.Fatal("candidate replay changed immutable output", first, second)
	}
	var count int64
	if err := database.Raw(`SELECT COUNT(*) FROM operation.operation_output WHERE operation_id=?::uuid`, identity.OperationID.String()).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("duplicate candidate created")
	}
	var asset struct {
		ProjectID                uuid.UUID
		Status, ModerationStatus string
		Count                    int64
	}
	if err := database.Raw(`SELECT project_id,status,moderation_status,COUNT(*) OVER() AS count FROM media.media_asset WHERE source_operation_id=?::uuid`, identity.OperationID.String()).Scan(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if asset.ProjectID != identity.ProjectID || asset.Count != 1 || asset.Status != "processing" || asset.ModerationStatus != "pending" {
		t.Fatalf("staging bypassed review or project ownership: %+v", asset)
	}
	changed := *result.Receipt
	changed.Identity.ProjectID = uuid.New()
	input.Receipt = &changed
	if _, err := service.Ingest(t.Context(), input); err == nil {
		t.Fatal("cross-project receipt accepted")
	}
	changed = *result.Receipt
	changed.ManifestSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	input.Receipt = &changed
	if _, err := service.Ingest(t.Context(), input); err == nil {
		t.Fatal("wrong manifest accepted on replay")
	}
	input.Receipt = result.Receipt
	input.URL = "https://invalid.example/arbitrary"
	if _, err := service.Ingest(t.Context(), input); err == nil {
		t.Fatal("both URL and staged receipt accepted")
	}
	if launch.starts != 1 {
		t.Fatal("media retry generated another image")
	}
}

type stagedFaultRepository struct {
	mediaapp.Repository
	failSave bool
}

func (r *stagedFaultRepository) SaveOutput(ctx context.Context, asset mediadomain.MediaAsset, renditions []mediadomain.Rendition, seq int, id uuid.UUID) (mediaapp.IngestOutput, error) {
	if r.failSave {
		return mediaapp.IngestOutput{}, errors.New("fixture media database interruption")
	}
	return r.Repository.SaveOutput(ctx, asset, renditions, seq, id)
}
