package projectcopy

import (
	"context"

	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/creation/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
	bibledomain "github.com/StephenQiu30/glowloom-server/backend/creation/bible/domain"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
)

type projectCopyBibleScopes struct{ store *pgscript.ProjectCopyStore }

func (s projectCopyBibleScopes) RemapLookScopes(ctx context.Context, actor bibleapp.Actor, b bibleapp.ProjectCopyBinding, scopes []bibledomain.LookScope) ([]bibleapp.ScopeMapping, error) {
	input := make([]scriptapp.BibleLookScope, len(scopes))
	for i, v := range scopes {
		input[i] = scriptapp.BibleLookScope{EpisodeID: v.EpisodeID, SceneKey: v.SceneKey}
	}
	mapped, err := s.store.RemapLookScopes(ctx, scriptwire.Actor(projectCopyIdentityActor(actor)), scriptapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, input)
	if err != nil {
		return nil, projectCopyScriptError(err)
	}
	result := make([]bibleapp.ScopeMapping, len(mapped))
	for i, v := range mapped {
		result[i] = bibleapp.ScopeMapping{Source: bibledomain.LookScope{EpisodeID: v.Source.EpisodeID, SceneKey: v.Source.SceneKey}, Target: bibledomain.LookScope{EpisodeID: v.Target.EpisodeID, SceneKey: v.Target.SceneKey}}
	}
	return result, nil
}

type projectCopyScriptCharacters struct {
	store    *pgbible.ProjectCopyStore
	snapshot *workspacedomain.ProjectCopyBibleSnapshot
}

func (c projectCopyScriptCharacters) FreezeCharacters(ctx context.Context, actor scriptapp.Actor, b scriptapp.ProjectCopyBinding, refs []scriptapp.ProjectCopyCharacterReference) ([]scriptapp.ProjectCopyCharacterMapping, error) {
	if c.snapshot == nil {
		return nil, scriptapp.ErrContextUnavailable
	}
	input := make([]bibleapp.CharacterCopyReference, len(refs))
	for i, v := range refs {
		input[i] = bibleapp.CharacterCopyReference{CharacterID: v.CharacterID, VersionID: v.VersionID}
	}
	mapped, err := c.store.ResolveCharacterMappings(ctx, projectCopyBibleActor(scriptwire.IdentityActor(actor)), bibleapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, bibleOwnerCopySnapshot(*c.snapshot), input)
	if err != nil {
		return nil, projectCopyBibleError(err)
	}
	result := make([]scriptapp.ProjectCopyCharacterMapping, len(mapped))
	for i, v := range mapped {
		result[i] = scriptapp.ProjectCopyCharacterMapping{Source: scriptapp.ProjectCopyCharacterReference{CharacterID: v.Source.CharacterID, VersionID: v.Source.VersionID}, Target: scriptapp.ProjectCopyCharacterReference{CharacterID: v.Target.CharacterID, VersionID: v.Target.VersionID}}
	}
	return result, nil
}

func provideProjectCopyCharacters(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) scriptapp.ProjectCopyCharacters {
	store := pgbible.NewProjectCopyStore(tx, func(bibleTx *gorm.DB) bibleapp.ProjectCopyAccess {
		return projectCopyBibleAccess{store: provideCopyAccess(bibleTx, a)}
	}, nil, nil)
	return projectCopyScriptCharacters{store: store, snapshot: a.Bible}
}
