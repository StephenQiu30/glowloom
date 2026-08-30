package asset_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

type dimensionContractTransactions struct {
	repository *dimensionContractRepository
}

func (transactions *dimensionContractTransactions) WithinTransaction(
	ctx context.Context,
	operation func(assetapp.Repository) error,
) error {
	return operation(transactions.repository)
}

type dimensionContractRepository struct {
	bundle       assetdomain.ArtifactWithLocation
	receipt      platformcommand.Receipt
	receiptCount int
}

func (*dimensionContractRepository) AuthorizeProject(
	context.Context,
	assetapp.Actor,
	string,
	string,
	bool,
) error {
	return nil
}

func (repository *dimensionContractRepository) FindReceipt(
	_ context.Context,
	workspaceID, operation, key string,
) (platformcommand.Receipt, error) {
	if repository.receipt.ID == "" || repository.receipt.WorkspaceID != workspaceID ||
		repository.receipt.Operation != operation || repository.receipt.IdempotencyKey != key {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return repository.receipt, nil
}

func (repository *dimensionContractRepository) EnsureReceipt(
	_ context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	if repository.receipt.ID != "" {
		return repository.receipt, nil
	}
	repository.receipt = receipt
	repository.receiptCount++
	return receipt, nil
}

func (*dimensionContractRepository) EnsureStaged(
	context.Context,
	assetdomain.ArtifactWithLocation,
) (assetdomain.ArtifactWithLocation, error) {
	return assetdomain.ArtifactWithLocation{}, errors.New("unexpected artifact registration")
}

func (repository *dimensionContractRepository) Get(
	context.Context,
	string,
	bool,
) (assetdomain.ArtifactWithLocation, error) {
	return repository.bundle, nil
}

func (repository *dimensionContractRepository) SaveReadiness(
	_ context.Context,
	bundle assetdomain.ArtifactWithLocation,
	expectedRevision int,
) error {
	if repository.bundle.Artifact.Status != assetdomain.ReadinessPendingValidation ||
		repository.bundle.Artifact.Revision != expectedRevision {
		return errors.New("artifact readiness changed")
	}
	repository.bundle = bundle
	return nil
}

type dimensionContractObjectReader struct {
	contents []byte
}

func (reader dimensionContractObjectReader) ReadVerified(
	context.Context,
	string,
	int64,
	string,
	int64,
) ([]byte, error) {
	return append([]byte(nil), reader.contents...), nil
}

func TestValidateReadyQuarantinesDeclaredImageDimensionDrift(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	artifactID, locationID, providerReceiptID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	contents := testPNG(t, 4, 3)
	repository := &dimensionContractRepository{bundle: assetdomain.ArtifactWithLocation{
		Artifact: assetdomain.Artifact{
			ID: artifactID, WorkspaceID: workspaceID, ProjectID: projectID,
			SourceType: "generation_provider_receipt", SourceID: providerReceiptID, OutputKey: "image-1",
			MediaType: "image/png", SHA256: sha256Hex(contents), SizeBytes: int64(len(contents)),
			Status: assetdomain.ReadinessPendingValidation, Revision: 1, CreatedAt: now, UpdatedAt: now,
		},
		Location: assetdomain.Location{
			ID: locationID, WorkspaceID: workspaceID, ArtifactID: artifactID, LocationNo: 1,
			StorageProfile: "private-primary", Bucket: "test", ObjectKey: "staging/test/image-1.png",
			Region: "us-east-1", Checksum: sha256Hex(contents), Status: assetdomain.LocationStaging,
			CreatedAt: now, UpdatedAt: now,
		},
	}}
	service := assetapp.NewService(
		&dimensionContractTransactions{repository: repository},
		dimensionContractObjectReader{contents: contents},
		assetapp.Config{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			Bucket: "test", StorageProfile: "private-primary", Region: "us-east-1", MaxImageBytes: 20 << 20,
		},
	)
	actor := assetapp.Actor{UserID: userID, TokenVersion: 1}

	if _, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: artifactID, ExpectedRevision: 1, ExpectedHeight: 3, IdempotencyKey: "dimension-required",
	}); assetErrorCode(err) != "invalid_request" {
		t.Fatalf("validation accepted an incomplete expected image dimension pair: %T %v", err, err)
	}

	command := assetapp.ValidateReadyCommand{
		ArtifactID: artifactID, ExpectedRevision: 1, ExpectedWidth: 5, ExpectedHeight: 3,
		IdempotencyKey: "dimension-drift",
	}
	result, err := service.ValidateReady(ctx, actor, command)
	if err != nil {
		t.Fatalf("validate declared image dimension drift: %v", err)
	}
	if result.Artifact.Status != assetdomain.ReadinessQuarantined ||
		result.Artifact.FailureCode != "image_dimensions_mismatch" ||
		result.Artifact.Width != 4 || result.Artifact.Height != 3 || result.Artifact.Revision != 2 ||
		result.Location.Status != assetdomain.LocationStaging || repository.receiptCount != 1 {
		t.Fatalf("dimension quarantine result = %#v, receipt count = %d", result, repository.receiptCount)
	}
	if _, err = service.RequireReady(ctx, actor, artifactID); assetErrorCode(err) != "artifact_not_ready" {
		t.Fatalf("dimension-quarantined Artifact passed RequireReady: %T %v", err, err)
	}

	replayed, err := service.ValidateReady(ctx, actor, command)
	if err != nil || replayed.Receipt.ID != result.Receipt.ID || repository.receiptCount != 1 {
		t.Fatalf("replay dimension quarantine: result=%#v receipt_count=%d err=%v", replayed, repository.receiptCount, err)
	}

	drifted := command
	drifted.ExpectedWidth = 4
	if _, err = service.ValidateReady(ctx, actor, drifted); assetErrorCode(err) != "state_conflict" {
		t.Fatalf("validation receipt accepted drifted expected dimensions: %T %v", err, err)
	}
}

func assetErrorCode(err error) string {
	var applicationErr *assetapp.Error
	if errors.As(err, &applicationErr) {
		return applicationErr.Code
	}
	return ""
}
