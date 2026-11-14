package application

import (
	"github.com/google/uuid"
)

// CopyLookScopeMapping is a script-owned historical applicability identity plan.
type CopyLookScopeMapping struct {
	Source BibleLookScope
	Target BibleLookScope
}

// PlanProjectCopyLookScopes preserves trusted frozen Bible scopes through exact formal script identities.
// Binding occurred under the confirmed structure contract; historical scene existence does not
// claim that a candidate is currently confirmed, and normal new-binding validation is unchanged.
func PlanProjectCopyLookScopes(b ProjectCopyBinding, h ProjectCopyHistory, scopes []BibleLookScope) ([]CopyLookScopeMapping, error) {
	if err := validateCopyHistory(b, h); err != nil {
		return nil, err
	}
	formal := make(map[uuid.UUID]bool)
	for _, confirmation := range h.SplitConfirmations {
		for _, episode := range confirmation.Episodes {
			formal[episode.ID] = true
		}
	}
	episodes := make(map[uuid.UUID]bool, len(h.Episodes))
	for _, e := range h.Episodes {
		episodes[e.ID] = formal[e.ID]
	}
	type key struct{ episode, scene uuid.UUID }
	scenes := make(map[key]bool)
	for _, s := range h.Structures {
		for _, scene := range s.Document.Scenes {
			scenes[key{s.EpisodeID, scene.Key}] = true
		}
	}
	result := make([]CopyLookScopeMapping, 0, len(scopes))
	seen := make(map[key]bool, len(scopes))
	for _, scope := range scopes {
		k := key{episode: scope.EpisodeID}
		if scope.SceneKey != nil {
			k.scene = *scope.SceneKey
		}
		if !episodes[scope.EpisodeID] || seen[k] || scope.SceneKey != nil && (k.scene == uuid.Nil || !scenes[k]) {
			return nil, ErrContextUnavailable
		}
		seen[k] = true
		target := BibleLookScope{EpisodeID: projectCopyIdentity(b.JobID, scope.EpisodeID)}
		if scope.SceneKey != nil {
			id := projectCopyIdentity(b.JobID, *scope.SceneKey)
			target.SceneKey = &id
		}
		result = append(result, CopyLookScopeMapping{Source: scope, Target: target})
	}
	return result, nil
}

func projectCopyIdentity(job, original uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(job, []byte("script/"+original.String()))
}
