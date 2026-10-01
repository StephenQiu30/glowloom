package operation_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/staged"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/staging"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestM1StagedIngestPrivateImageCreatesOneCandidate(t *testing.T) {
	objects := receiptTestObjects(t)
	database := operationStoreDB(t)
	identity, store := seedDispatchCall(t, database, true)
	freezeReceiptFixtureModel(t, database, identity)
	receipts := application.NewProviderStageService(staging.NewObjects(objects), store)
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
	repo := &stagedFaultRepository{Repository: pgmedia.NewIngestRepository(database), failSave: true}
	service := mediaapp.NewStagedIngestService(repo, staged.NewReader(receipts), mediaflow.FFProber{}, mediaflow.FFRenderer{}, stagedMediaObjects{client: objects})
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
	if err != nil || first != second {
		t.Fatalf("candidate replay changed: %v", err)
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

type stagedMediaObjects struct{ client *objectstorage.Client }

func (s stagedMediaObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	err := s.client.PutIfAbsent(ctx, key, r, size, mime, digest)
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return mediaapp.ErrObjectAlreadyExists
	}
	return err
}
func (s stagedMediaObjects) Stat(ctx context.Context, key string) (mediaapp.ObjectInfo, error) {
	info, err := s.client.Stat(ctx, key)
	return mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, err
}
