package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type StructureIdentityReader interface {
	ReadCurrentStructureIdentity(
		context.Context,
		string,
		string,
	) (domain.StructureIdentitySetVersion, domain.StructureIdentityCollectionReceipt, error)
}

type StructureIdentityProjectReader interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type StructureIdentitySnapshot struct {
	Version domain.StructureIdentitySetVersion        `json:"version"`
	Receipt domain.StructureIdentityCollectionReceipt `json:"receipt"`
}

type StructureIdentityQuery struct {
	versions StructureIdentityReader
	projects StructureIdentityProjectReader
}

func NewStructureIdentityQuery(
	versions StructureIdentityReader,
	projects StructureIdentityProjectReader,
) *StructureIdentityQuery {
	return &StructureIdentityQuery{versions: versions, projects: projects}
}

func (query *StructureIdentityQuery) GetCurrent(
	ctx context.Context,
	actor Actor,
	projectID string,
) (StructureIdentitySnapshot, error) {
	if _, err := uuid.Parse(projectID); err != nil {
		return StructureIdentitySnapshot{}, &Error{
			Code: "validation_failed", Message: "Invalid Project identity", Status: 422,
		}
	}
	project, err := query.projects.Get(
		ctx,
		projectapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion},
		projectID,
	)
	if err != nil {
		var problem *projectapp.Error
		if errors.As(err, &problem) {
			return StructureIdentitySnapshot{}, fmt.Errorf("project access: %w: %w", &Error{
				Code: problem.Code, Message: problem.Message, Status: problem.Status,
			}, err)
		}
		return StructureIdentitySnapshot{}, err
	}
	version, receipt, err := query.versions.ReadCurrentStructureIdentity(
		ctx,
		project.WorkspaceID,
		projectID,
	)
	if errors.Is(err, ErrNotFound) {
		return StructureIdentitySnapshot{}, &Error{
			Code: "not_found", Message: "Current Structure Identity result not found", Status: 404,
		}
	}
	if err != nil {
		return StructureIdentitySnapshot{}, err
	}
	if version.SchemaVersion != domain.StructureIdentitySetSchemaVersion || version.ID == "" ||
		version.WorkspaceID != project.WorkspaceID || version.ProjectID != projectID || version.Version < 1 ||
		!validStructureIdentityHash(version.ContentHash) || version.CreatedAt.IsZero() ||
		receipt.ID == "" || receipt.CheckpointKey != domain.StructureIdentityCheckpointKey ||
		receipt.CollectionFamily != domain.StructureIdentityCollectionFamily ||
		receipt.VersionID != version.ID || receipt.VersionContentHash != version.ContentHash ||
		!validStructureIdentityHash(receipt.CollectionRootHash) ||
		!validStructureIdentityHash(receipt.ReceiptContentHash) {
		return StructureIdentitySnapshot{}, &Error{
			Code: "formal_version_drift", Message: "Stored Structure Identity result does not match its proof", Status: 409,
		}
	}
	return StructureIdentitySnapshot{Version: version, Receipt: receipt}, nil
}
