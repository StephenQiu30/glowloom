package asset

import (
	"context"
	"errors"

	"github.com/google/uuid"

	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
)

type ProviderOutputReadiness struct{ service *assetapp.Service }

func NewProviderOutputReadiness(service *assetapp.Service) *ProviderOutputReadiness {
	return &ProviderOutputReadiness{service: service}
}

func (readiness *ProviderOutputReadiness) EnsureProviderOutputReady(
	ctx context.Context,
	actor generationapp.Actor,
	command generationapp.ProviderOutputAssetCommand,
) (generationapp.ReadyArtifact, error) {
	if readiness == nil || readiness.service == nil {
		return generationapp.ReadyArtifact{}, errors.New("asset Provider output readiness service is unavailable")
	}
	if _, err := uuid.Parse(command.ProviderJobID); err != nil {
		return generationapp.ReadyArtifact{}, &generationapp.Error{
			Code: "invalid_request", Message: "Invalid Provider output Job", Status: 422,
		}
	}
	if _, err := uuid.Parse(command.ProviderCallID); err != nil {
		return generationapp.ReadyArtifact{}, &generationapp.Error{
			Code: "invalid_request", Message: "Invalid Provider output Call", Status: 422,
		}
	}
	if _, err := uuid.Parse(command.ProviderReceiptID); err != nil {
		return generationapp.ReadyArtifact{}, &generationapp.Error{
			Code: "invalid_request", Message: "Invalid Provider output receipt", Status: 422,
		}
	}
	if command.ExpectedWidth < 1 || command.ExpectedHeight < 1 {
		return generationapp.ReadyArtifact{}, &generationapp.Error{
			Code: "invalid_request", Message: "Invalid Provider output target dimensions", Status: 422,
		}
	}
	assetActor := assetapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	registered, err := readiness.service.RegisterStaged(ctx, assetActor, assetapp.RegisterStagedCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		SourceType: "generation_provider_receipt", SourceID: command.ProviderReceiptID, OutputKey: command.Output.OutputKey,
		ProviderJobID: command.ProviderJobID, ProviderCallID: command.ProviderCallID,
		ObjectKey: command.Output.StagingObjectKey, MediaType: command.Output.MediaType,
		SHA256: command.Output.SHA256, SizeBytes: command.Output.Bytes,
		IdempotencyKey: command.RegisterIdempotencyKey,
	})
	if err != nil {
		return generationapp.ReadyArtifact{}, providerOutputError(err)
	}
	validated, err := readiness.service.ValidateReady(ctx, assetActor, assetapp.ValidateReadyCommand{
		ArtifactID: registered.Artifact.ID, ExpectedRevision: 1,
		ExpectedWidth: command.ExpectedWidth, ExpectedHeight: command.ExpectedHeight,
		IdempotencyKey: command.ValidateIdempotencyKey,
	})
	if err != nil {
		return generationapp.ReadyArtifact{}, providerOutputError(err)
	}
	if validated.Artifact.Status != assetdomain.ReadinessReady || validated.Location.Status != assetdomain.LocationPrimary {
		return generationapp.ReadyArtifact{}, &generationapp.Error{
			Code: "artifact_not_ready", Message: "Provider output Artifact is not ready",
			Status: 409, NextAction: "wait_or_replace", Details: map[string]any{
				"artifact_id": validated.Artifact.ID, "failure_code": validated.Artifact.FailureCode,
			},
		}
	}
	return generationapp.ReadyArtifact{
		ID: validated.Artifact.ID, WorkspaceID: validated.Artifact.WorkspaceID,
		ProjectID: validated.Artifact.ProjectID, SourceType: validated.Artifact.SourceType,
		SourceID: validated.Artifact.SourceID, OutputKey: validated.Artifact.OutputKey,
		MediaType: validated.Artifact.MediaType, SHA256: validated.Artifact.SHA256,
		SizeBytes: validated.Artifact.SizeBytes, Width: validated.Artifact.Width,
		Height: validated.Artifact.Height, Revision: validated.Artifact.Revision,
	}, nil
}

func providerOutputError(err error) error {
	var applicationErr *assetapp.Error
	if !errors.As(err, &applicationErr) {
		return err
	}
	return &generationapp.Error{
		Code: applicationErr.Code, Message: applicationErr.Message, NextAction: applicationErr.NextAction,
		Status: applicationErr.Status, Details: applicationErr.Details,
	}
}
