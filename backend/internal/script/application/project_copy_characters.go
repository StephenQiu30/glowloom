package application

import (
	"context"
	"slices"
	"strconv"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ProjectCopyCharacterReference preserves a historical identity and an optional immutable pin.
type ProjectCopyCharacterReference struct {
	CharacterID uuid.UUID
	VersionID   *uuid.UUID
}

// ProjectCopyCharacterMapping is an exact owning Bible source/target correspondence.
type ProjectCopyCharacterMapping struct{ Source, Target ProjectCopyCharacterReference }

// ProjectCopyCharacters freezes only the Bible owner's validated historical mappings in caller SQL.
type ProjectCopyCharacters interface {
	FreezeCharacters(context.Context, identityapp.Principal, ProjectCopyBinding, []ProjectCopyCharacterReference) ([]ProjectCopyCharacterMapping, error)
}

type characterCopyKey struct{ identity, version uuid.UUID }

func characterKey(c ProjectCopyCharacterReference) characterCopyKey {
	k := characterCopyKey{identity: c.CharacterID}
	if c.VersionID != nil {
		k.version = *c.VersionID
	}
	return k
}

// ProjectHistoryCharacterReferences enumerates complete immutable speaker bindings without refreshing pins.
func ProjectHistoryCharacterReferences(h ProjectCopyHistory) []ProjectCopyCharacterReference {
	seen := make(map[characterCopyKey]ProjectCopyCharacterReference)
	for _, s := range h.Structures {
		for _, scene := range s.Document.Scenes {
			for _, item := range scene.Items {
				if item.CharacterID != nil {
					ref := ProjectCopyCharacterReference{CharacterID: *item.CharacterID, VersionID: item.CharacterVersionID}
					seen[characterKey(ref)] = ref
				}
			}
		}
	}
	result := make([]ProjectCopyCharacterReference, 0, len(seen))
	for _, ref := range seen {
		result = append(result, ref)
	}
	slices.SortFunc(result, func(a, b ProjectCopyCharacterReference) int {
		ak, bk := characterKey(a), characterKey(b)
		if c := slices.Compare(ak.identity[:], bk.identity[:]); c != 0 {
			return c
		}
		return slices.Compare(ak.version[:], bk.version[:])
	})
	return result
}

func copyCharacterMap(h ProjectCopyHistory, mappings []ProjectCopyCharacterMapping) (map[characterCopyKey]ProjectCopyCharacterReference, error) {
	result := make(map[characterCopyKey]ProjectCopyCharacterReference, len(mappings))
	identities := make(map[uuid.UUID]uuid.UUID)
	targets := make(map[uuid.UUID]uuid.UUID)
	for _, m := range mappings {
		key := characterKey(m.Source)
		if m.Source.CharacterID == uuid.Nil || m.Target.CharacterID == uuid.Nil || m.Source.CharacterID == m.Target.CharacterID || (m.Source.VersionID == nil) != (m.Target.VersionID == nil) || m.Source.VersionID != nil && (key.version == uuid.Nil || *m.Target.VersionID == uuid.Nil || key.version == *m.Target.VersionID) {
			return nil, ErrContextUnavailable
		}
		if _, exists := result[key]; exists {
			return nil, ErrContextUnavailable
		}
		if prior := identities[m.Source.CharacterID]; prior != uuid.Nil && prior != m.Target.CharacterID {
			return nil, ErrContextUnavailable
		}
		if prior := targets[m.Target.CharacterID]; prior != uuid.Nil && prior != m.Source.CharacterID {
			return nil, ErrContextUnavailable
		}
		identities[m.Source.CharacterID] = m.Target.CharacterID
		targets[m.Target.CharacterID] = m.Source.CharacterID
		result[key] = m.Target
	}
	refs := ProjectHistoryCharacterReferences(h)
	if len(refs) != len(result) {
		return nil, ErrContextUnavailable
	}
	for _, ref := range refs {
		if _, exists := result[characterKey(ref)]; !exists {
			return nil, ErrContextUnavailable
		}
	}
	return result, nil
}

func remapStructureCharacter(id, version **uuid.UUID, mapping map[characterCopyKey]ProjectCopyCharacterReference) error {
	if *id == nil {
		if *version != nil {
			return ErrObjectMismatch
		}
		return nil
	}
	ref, exists := mapping[characterKey(ProjectCopyCharacterReference{CharacterID: **id, VersionID: *version})]
	if !exists {
		return ErrContextUnavailable
	}
	mapped := ref.CharacterID
	*id = &mapped
	*version = ref.VersionID
	return nil
}

func canonicalCharacterMapping(h ProjectCopyHistory) map[characterCopyKey]ProjectCopyCharacterReference {
	identities := make(map[uuid.UUID]uuid.UUID)
	versions := make(map[uuid.UUID]uuid.UUID)
	result := make(map[characterCopyKey]ProjectCopyCharacterReference)
	// Traversal follows frozen structure/item order, keeping relationships while preserving legacy nil bytes.
	for _, s := range h.Structures {
		for _, scene := range s.Document.Scenes {
			for _, item := range scene.Items {
				if item.CharacterID == nil {
					continue
				}
				id := *item.CharacterID
				if identities[id] == uuid.Nil {
					identities[id] = uuid.NewSHA1(uuid.Nil, []byte("script-character/"+strconv.Itoa(len(identities))))
				}
				ref := ProjectCopyCharacterReference{CharacterID: identities[id]}
				if item.CharacterVersionID != nil {
					version := *item.CharacterVersionID
					if versions[version] == uuid.Nil {
						versions[version] = uuid.NewSHA1(uuid.Nil, []byte("script-character-version/"+strconv.Itoa(len(versions))))
					}
					mapped := versions[version]
					ref.VersionID = &mapped
				}
				result[characterKey(ProjectCopyCharacterReference{CharacterID: id, VersionID: item.CharacterVersionID})] = ref
			}
		}
	}
	return result
}
