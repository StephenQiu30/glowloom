package domain

import (
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const SourceCollectionFamily = "script_source_set"

func BuildSourceCollectionRef(
	workspaceID string,
	projectID string,
	scopeRevision int64,
	source SourceVersionIdentity,
	index SourceSpanIndex,
) (ownercollection.Ref, error) {
	if source.OwnerKind != "production/script" || index.WorkspaceID != workspaceID || index.ProjectID != projectID ||
		index.DocumentRevisionID != source.VersionID || index.SourceHash != source.ContentHash {
		return ownercollection.Ref{}, errors.New("invalid Script Source Collection material")
	}
	member := func(logicalID, versionID string, revision int64, contentHash string) ownercollection.VersionRef {
		return ownercollection.VersionRef{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/script", VersionFamily: SourceCollectionFamily,
			OwnerLogicalID: logicalID, OwnerVersionID: versionID,
			OwnerRevision: revision, OwnerContentHash: contentHash,
		}
	}
	return ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/script", VersionFamily: SourceCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + projectID, ScopeRevision: scopeRevision,
	}, []ownercollection.VersionRef{
		member(source.LogicalID, source.VersionID, source.Revision, source.ContentHash),
		member(source.LogicalID+":span-index", index.ID, source.Revision, index.ContentHash),
	})
}
