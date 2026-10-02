package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func copySHA(data []byte) string       { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func compareCopyID(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }

type scopeKey struct {
	episode, scene uuid.UUID
	hasScene       bool
}

func keyScope(s domain.LookScope) scopeKey {
	k := scopeKey{episode: s.EpisodeID}
	if s.SceneKey != nil {
		k.scene = *s.SceneKey
		k.hasScene = true
	}
	return k
}

// HistoryMediaFacts returns all distinct historical image and sample-audio bindings in deterministic order.
func HistoryMediaFacts(h CopyHistory) []domain.MediaFact {
	seen := make(map[domain.MediaFact]bool)
	for _, v := range h.Versions {
		if v.Character != nil {
			for _, l := range v.Character.Looks {
				for _, r := range l.References {
					seen[r.Media] = true
				}
			}
			if voice := v.Character.Voice; voice != nil && voice.Sample != nil {
				seen[voice.Sample.Media] = true
			}
		}
	}
	result := make([]domain.MediaFact, 0, len(seen))
	for fact := range seen {
		result = append(result, fact)
	}
	slices.SortFunc(result, func(a, b domain.MediaFact) int {
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		return bytes.Compare(aj, bj)
	})
	return result
}

// HistoryLookScopes returns all distinct historical formal applicability bindings in deterministic order.
func HistoryLookScopes(h CopyHistory) []domain.LookScope {
	seen := make(map[scopeKey]domain.LookScope)
	for _, v := range h.Versions {
		if v.Character != nil {
			for _, l := range v.Character.Looks {
				for _, s := range l.AppliesTo {
					seen[keyScope(s)] = s
				}
			}
		}
	}
	result := make([]domain.LookScope, 0, len(seen))
	for _, scope := range seen {
		result = append(result, scope)
	}
	slices.SortFunc(result, func(a, b domain.LookScope) int {
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		return bytes.Compare(aj, bj)
	})
	return result
}

// RemapProjectHistory validates complete source evidence before applying own and foreign owner mappings.
// Generation provenance remains original evidence; no operations, commands or budget rows are copied.
func RemapProjectHistory(b ProjectCopyBinding, history CopyHistory, refs []ReferenceMapping, scopes []ScopeMapping) (CopyManifest, error) {
	if b.JobID == uuid.Nil || b.OrgID == uuid.Nil || b.SourceProjectID == uuid.Nil || b.TargetProjectID == uuid.Nil || b.SourceProjectID == b.TargetProjectID {
		return CopyManifest{}, ErrUnavailable
	}
	source, err := CanonicalCopyHistory(history)
	if err != nil {
		return CopyManifest{}, err
	}
	counts, err := ValidateCopyHistory(source, b.OrgID, b.SourceProjectID)
	if err != nil {
		return CopyManifest{}, err
	}
	references, err := copyReferenceMap(source, refs)
	if err != nil {
		return CopyManifest{}, err
	}
	scopeMappings, err := copyScopeMap(source, scopes)
	if err != nil {
		return CopyManifest{}, err
	}
	target, err := historyClone(source)
	if err != nil {
		return CopyManifest{}, err
	}
	owned := func(id uuid.UUID) uuid.UUID { return uuid.NewSHA1(b.JobID, []byte("bible/"+id.String())) }
	optional := func(id *uuid.UUID) *uuid.UUID {
		if id == nil {
			return nil
		}
		mapped := owned(*id)
		return &mapped
	}
	identities := make([]IdentityMapping, 0, len(source.Heads))
	versionIDs := make([]VersionMapping, 0, len(source.Versions))
	for i := range target.Heads {
		h := &target.Heads[i]
		identities = append(identities, IdentityMapping{Kind: h.Kind, SourceID: h.ID, TargetID: owned(h.ID)})
		h.ID = owned(h.ID)
		h.ProjectID = b.TargetProjectID
		h.CurrentVersionID = owned(h.CurrentVersionID)
		h.ConfirmedVersionID = optional(h.ConfirmedVersionID)
		h.RedirectID = optional(h.RedirectID)
	}
	for i := range target.Versions {
		v := &target.Versions[i]
		versionIDs = append(versionIDs, VersionMapping{Kind: v.Kind, SourceID: v.ID, TargetID: owned(v.ID)})
		v.ID = owned(v.ID)
		v.EntryID = owned(v.EntryID)
		v.ProjectID = b.TargetProjectID
		v.PreviousID = optional(v.PreviousID)
		if v.Character != nil {
			for j := range v.Character.Looks {
				look := &v.Character.Looks[j]
				look.ID = owned(look.ID)
				for k := range look.References {
					look.References[k].Media = references[look.References[k].Media]
				}
				for k := range look.AppliesTo {
					look.AppliesTo[k] = scopeMappings[keyScope(look.AppliesTo[k])]
				}
			}
			if voice := v.Character.Voice; voice != nil && voice.Sample != nil {
				voice.Sample.Media = references[voice.Sample.Media]
			}
		}
		_, v.ContentSHA256, err = v.Content()
		if err != nil {
			return CopyManifest{}, err
		}
	}
	for i := range target.Looks {
		l := &target.Looks[i]
		l.ID = owned(l.ID)
		l.CharacterID = owned(l.CharacterID)
	}
	for i := range target.Confirmations {
		c := &target.Confirmations[i].Confirmation
		c.ID = owned(c.ID)
		c.EntryID = owned(c.EntryID)
		c.VersionID = owned(c.VersionID)
	}
	for i := range target.Redirects {
		r := &target.Redirects[i]
		r.ID = owned(r.ID)
		r.SourceID = owned(r.SourceID)
		r.TargetID = owned(r.TargetID)
		r.SourceVersionID = owned(r.SourceVersionID)
		r.TargetVersionID = owned(r.TargetVersionID)
	}
	for i := range target.Splits {
		r := &target.Splits[i]
		r.ID = owned(r.ID)
		r.SourceID = owned(r.SourceID)
		r.TargetID = owned(r.TargetID)
		r.SourceVersionID = owned(r.SourceVersionID)
		r.TargetVersionID = owned(r.TargetVersionID)
	}
	target, err = CanonicalCopyHistory(target)
	if err != nil {
		return CopyManifest{}, err
	}
	targetCounts, err := ValidateCopyHistory(target, b.OrgID, b.TargetProjectID)
	if err != nil || targetCounts != counts {
		return CopyManifest{}, domain.ErrCorruptHistory
	}
	data, err := json.Marshal(source)
	if err != nil {
		return CopyManifest{}, err
	}
	orderedRefs := make([]ReferenceMapping, 0, len(references))
	for _, s := range HistoryMediaFacts(source) {
		orderedRefs = append(orderedRefs, ReferenceMapping{Source: s, Target: references[s]})
	}
	orderedScopes := make([]ScopeMapping, 0, len(scopeMappings))
	for _, s := range HistoryLookScopes(source) {
		orderedScopes = append(orderedScopes, ScopeMapping{Source: s, Target: scopeMappings[keyScope(s)]})
	}
	return CopyManifest{Binding: b, Source: source, Target: target, References: orderedRefs, Scopes: orderedScopes, Counts: counts, ContentSHA256: copySHA(data), Identities: identities, Versions: versionIDs}, nil
}

func copyReferenceMap(h CopyHistory, mappings []ReferenceMapping) (map[domain.MediaFact]domain.MediaFact, error) {
	result := make(map[domain.MediaFact]domain.MediaFact, len(mappings))
	for _, m := range mappings {
		if m.Source.Validate(m.Source.Kind) != nil || m.Target.Validate(m.Source.Kind) != nil || m.Source.AssetID == m.Target.AssetID || m.Source.SHA256 != m.Target.SHA256 || m.Source.ByteSize != m.Target.ByteSize || m.Source.RenditionSHA256 != m.Target.RenditionSHA256 || m.Source.Kind == "image" && m.Source.RenditionID == m.Target.RenditionID {
			return nil, ErrUnavailable
		}
		if _, exists := result[m.Source]; exists {
			return nil, ErrUnavailable
		}
		result[m.Source] = m.Target
	}
	facts := HistoryMediaFacts(h)
	if len(facts) != len(result) {
		return nil, ErrUnavailable
	}
	for _, fact := range facts {
		if _, exists := result[fact]; !exists {
			return nil, ErrUnavailable
		}
	}
	return result, nil
}

func copyScopeMap(h CopyHistory, mappings []ScopeMapping) (map[scopeKey]domain.LookScope, error) {
	result := make(map[scopeKey]domain.LookScope, len(mappings))
	for _, m := range mappings {
		k := keyScope(m.Source)
		if m.Source.EpisodeID == uuid.Nil || m.Target.EpisodeID == uuid.Nil || m.Source.EpisodeID == m.Target.EpisodeID || k.hasScene != (m.Target.SceneKey != nil) || k.hasScene && (k.scene == uuid.Nil || *m.Target.SceneKey == uuid.Nil || k.scene == *m.Target.SceneKey) {
			return nil, ErrUnavailable
		}
		if _, exists := result[k]; exists {
			return nil, ErrUnavailable
		}
		result[k] = m.Target
	}
	all := HistoryLookScopes(h)
	if len(all) != len(result) {
		return nil, ErrUnavailable
	}
	for _, scope := range all {
		if _, exists := result[keyScope(scope)]; !exists {
			return nil, ErrUnavailable
		}
	}
	return result, nil
}
