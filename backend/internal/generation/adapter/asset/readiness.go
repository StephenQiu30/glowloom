package asset

import (
	"context"
	"errors"

	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
)

type Readiness struct{ service *assetapp.Service }

func NewReadiness(service *assetapp.Service) *Readiness { return &Readiness{service: service} }

func (readiness *Readiness) RequireReady(ctx context.Context, actor generationapp.Actor, artifactID string) (generationapp.ReadyArtifact, error) {
	if readiness == nil || readiness.service == nil {
		return generationapp.ReadyArtifact{}, errors.New("asset readiness service is unavailable")
	}
	artifact, err := readiness.service.RequireReady(ctx, assetapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, artifactID)
	if err != nil {
		var applicationErr *assetapp.Error
		if errors.As(err, &applicationErr) {
			return generationapp.ReadyArtifact{}, &generationapp.Error{
				Code: applicationErr.Code, Message: applicationErr.Message, NextAction: applicationErr.NextAction,
				Status: applicationErr.Status, Details: applicationErr.Details,
			}
		}
		return generationapp.ReadyArtifact{}, err
	}
	return generationapp.ReadyArtifact{
		ID: artifact.ID, WorkspaceID: artifact.WorkspaceID, ProjectID: artifact.ProjectID,
		SourceType: artifact.SourceType, SourceID: artifact.SourceID, OutputKey: artifact.OutputKey,
		MediaType: artifact.MediaType, SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes, Width: artifact.Width,
		Height: artifact.Height, Revision: artifact.Revision,
	}, nil
}
