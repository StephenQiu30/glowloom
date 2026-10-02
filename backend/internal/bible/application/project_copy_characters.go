package application

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

// CharacterCopyReference preserves a historical identity and optional immutable version pin.
type CharacterCopyReference struct {
	CharacterID uuid.UUID
	VersionID   *uuid.UUID
}

// CharacterCopyMapping is a Bible-owner-proven historical identity/version correspondence.
type CharacterCopyMapping struct{ Source, Target CharacterCopyReference }

// ResolveCopyCharacters reads only the complete frozen owner history, never a current public projection.
// Historical pins must have an actual confirmation; a legacy nil pin remains nil.
func ResolveCopyCharacters(m CopyManifest, requested []CharacterCopyReference) ([]CharacterCopyMapping, error) {
	if _, err := ValidateCopyHistory(m.Source, m.Binding.OrgID, m.Binding.SourceProjectID); err != nil {
		return nil, err
	}
	identities := make(map[uuid.UUID]uuid.UUID)
	versions := make(map[uuid.UUID]uuid.UUID)
	for _, mapping := range m.Identities {
		if mapping.Kind == domain.KindCharacter {
			identities[mapping.SourceID] = mapping.TargetID
		}
	}
	for _, mapping := range m.Versions {
		if mapping.Kind == domain.KindCharacter {
			versions[mapping.SourceID] = mapping.TargetID
		}
	}
	versionOwners := make(map[uuid.UUID]uuid.UUID)
	confirmed := make(map[uuid.UUID]bool)
	for _, v := range m.Source.Versions {
		if v.Kind == domain.KindCharacter {
			versionOwners[v.ID] = v.EntryID
		}
	}
	for _, c := range m.Source.Confirmations {
		if c.Kind == domain.KindCharacter {
			confirmed[c.Confirmation.VersionID] = true
		}
	}
	type key struct{ identity, version uuid.UUID }
	seen := make(map[key]bool, len(requested))
	result := make([]CharacterCopyMapping, 0, len(requested))
	for _, ref := range requested {
		k := key{identity: ref.CharacterID}
		if ref.VersionID != nil {
			k.version = *ref.VersionID
		}
		if ref.CharacterID == uuid.Nil || identities[ref.CharacterID] == uuid.Nil || seen[k] {
			return nil, ErrUnavailable
		}
		seen[k] = true
		target := CharacterCopyReference{CharacterID: identities[ref.CharacterID]}
		if ref.VersionID != nil {
			if *ref.VersionID == uuid.Nil || !confirmed[*ref.VersionID] || versionOwners[*ref.VersionID] != ref.CharacterID || versions[*ref.VersionID] == uuid.Nil {
				return nil, ErrUnavailable
			}
			id := versions[*ref.VersionID]
			target.VersionID = &id
		}
		result = append(result, CharacterCopyMapping{Source: ref, Target: target})
	}
	return result, nil
}
