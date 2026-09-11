package application

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	"github.com/google/uuid"
)

type episodeOwnerMaterial struct {
	episode domain.EpisodeLifecycleEpisode
	script  domain.EpisodeLifecycleScriptVersion
}

func publishProjectEpisodeOwner(
	ctx context.Context,
	repository EpisodeLifecycleRepository,
	actor Actor,
	command ConfirmEpisodeLifecycleCommand,
	projectRevision int,
	active []episodeOwnerMaterial,
	archived []episodeOwnerMaterial,
	now time.Time,
	newID func() string,
) (domain.ProjectEpisodeCollectionReceipt, error) {
	previousHead, headExists, err := repository.GetProjectEpisodeScopeHead(ctx, command.ProjectID, true)
	if err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	if headExists {
		previousCollection, buildErr := ownercollection.Build(ownercollection.Scope{
			WorkspaceID: previousHead.WorkspaceID, ProjectID: previousHead.ProjectID,
			OwnerKind: "production/project", VersionFamily: domain.ProjectEpisodeCollectionFamily,
			ScopeKind: "project", ScopeKey: previousHead.ScopeKey, ScopeRevision: previousHead.ScopeRevision,
		}, previousHead.CurrentVersionRefs)
		rebuiltHead, headErr := domain.NewProjectEpisodeScopeHead(previousCollection, previousHead.HeadRevision, previousHead.UpdatedAt)
		if buildErr != nil || headErr != nil || !reflect.DeepEqual(rebuiltHead, previousHead) ||
			previousHead.WorkspaceID != command.WorkspaceID || previousHead.ProjectID != command.ProjectID ||
			previousHead.ScopeRevision != int64(command.ExpectedProjectRevision) {
			return domain.ProjectEpisodeCollectionReceipt{}, errors.New("Project Episode Scope Head has drifted")
		}
	}

	activeVersions := make([]domain.EpisodeOwnerVersion, len(active))
	for index, material := range active {
		version, ensureErr := ensureEpisodeOwnerVersion(ctx, repository, actor, material, now)
		if ensureErr != nil {
			return domain.ProjectEpisodeCollectionReceipt{}, ensureErr
		}
		activeVersions[index] = version
	}
	for _, material := range archived {
		if _, ensureErr := ensureEpisodeOwnerVersion(ctx, repository, actor, material, now); ensureErr != nil {
			return domain.ProjectEpisodeCollectionReceipt{}, ensureErr
		}
	}
	collection, err := domain.BuildProjectEpisodeCollection(
		command.WorkspaceID,
		command.ProjectID,
		int64(projectRevision),
		activeVersions,
	)
	if err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	memberships := make([]domain.ProjectEpisodeMembership, len(activeVersions))
	projectID := uuid.MustParse(command.ProjectID)
	for index, version := range activeVersions {
		memberships[index] = domain.ProjectEpisodeMembership{
			ID:          uuid.NewSHA1(projectID, []byte("project-episode-membership:"+strconv.FormatInt(collection.ScopeRevision, 10)+":"+version.EpisodeID)).String(),
			WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			ScopeRevision: collection.ScopeRevision, Position: version.Position,
			EpisodeID: version.EpisodeID, EpisodeVersionID: version.ID,
			VersionContentHash: version.ContentHash, CreatedAt: now,
		}
	}
	if err = repository.CreateProjectEpisodeMemberships(ctx, memberships); err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	headRevision := int64(1)
	if headExists {
		headRevision = previousHead.HeadRevision + 1
	}
	head, err := domain.NewProjectEpisodeScopeHead(collection, headRevision, now)
	if err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	if headExists {
		err = repository.AdvanceProjectEpisodeScopeHead(ctx, head, previousHead.HeadRevision, previousHead.HeadContentHash)
	} else {
		err = repository.CreateProjectEpisodeScopeHead(ctx, head)
	}
	if err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	receipt, err := domain.NewProjectEpisodeCollectionReceipt(
		newID(), newID(), command.IdempotencyKey, command.ReviewDecisionID, collection, now, actor.UserID,
	)
	if err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	if err = repository.CreateProjectEpisodeCollectionReceipt(ctx, receipt); err != nil {
		return domain.ProjectEpisodeCollectionReceipt{}, err
	}
	return receipt, nil
}

func ensureEpisodeOwnerVersion(
	ctx context.Context,
	repository EpisodeLifecycleRepository,
	actor Actor,
	material episodeOwnerMaterial,
	now time.Time,
) (domain.EpisodeOwnerVersion, error) {
	episode, script := material.episode, material.script
	current, exists, err := repository.FindEpisodeOwnerVersion(ctx, episode.ID)
	if err != nil {
		return domain.EpisodeOwnerVersion{}, err
	}
	value := domain.EpisodeOwnerVersion{
		WorkspaceID: episode.WorkspaceID, ProjectID: episode.ProjectID, EpisodeID: episode.ID,
		Revision: int64(episode.Revision), Status: episode.Status, Position: episode.Position,
		SequenceKey: domain.EpisodeSequenceKey(episode.Position), Name: episode.Name,
		TargetDurationMS: episode.TargetDurationMS, SourceVersionID: script.DocumentRevisionID,
		ScriptVersionID: script.ID, SourceStart: script.SourceStart, SourceEnd: script.SourceEnd,
		ScriptContentHash: script.ContentHash, CreatedBy: actor.UserID, CreatedAt: now,
	}
	if exists && current.Revision == value.Revision {
		value.ID, value.ParentVersionID, value.ParentContentHash = current.ID, current.ParentVersionID, current.ParentContentHash
		value.CreatedBy, value.CreatedAt = current.CreatedBy, current.CreatedAt
		rebuilt, rebuildErr := domain.NewEpisodeOwnerVersion(value)
		if rebuildErr != nil || rebuilt.ContentHash != current.ContentHash {
			return domain.EpisodeOwnerVersion{}, errors.New("Project Episode Version has drifted")
		}
		return current, nil
	}
	if exists {
		if current.Revision+1 != value.Revision {
			return domain.EpisodeOwnerVersion{}, errors.New("Project Episode Version chain has drifted")
		}
		parentID, parentHash := current.ID, current.ContentHash
		value.ParentVersionID, value.ParentContentHash = &parentID, &parentHash
	} else if value.Revision != 1 {
		return domain.EpisodeOwnerVersion{}, errors.New("Project Episode Version parent is missing")
	}
	value.ID = uuid.NewSHA1(
		uuid.MustParse(episode.ID),
		[]byte("project-episode-version:"+strconv.FormatInt(value.Revision, 10)+":"+script.ID),
	).String()
	value, err = domain.NewEpisodeOwnerVersion(value)
	if err != nil {
		return domain.EpisodeOwnerVersion{}, err
	}
	if err = repository.CreateEpisodeOwnerVersion(ctx, value); err != nil {
		return domain.EpisodeOwnerVersion{}, err
	}
	return value, nil
}
