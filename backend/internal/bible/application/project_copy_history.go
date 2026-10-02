package application

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func historyClone(h CopyHistory) (CopyHistory, error) {
	data, err := json.Marshal(h)
	if err != nil {
		return CopyHistory{}, err
	}
	var result CopyHistory
	if err := json.Unmarshal(data, &result); err != nil {
		return CopyHistory{}, err
	}
	return result, nil
}

func copyTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// CanonicalCopyHistory orders own immutable facts and normalizes PostgreSQL timestamp representation.
// User text, optional content arrays, historic actors and media digests remain unchanged.
func CanonicalCopyHistory(h CopyHistory) (CopyHistory, error) {
	r, err := historyClone(h)
	if err != nil {
		return CopyHistory{}, err
	}
	if r.Heads == nil {
		r.Heads = []domain.Head{}
	}
	if r.Versions == nil {
		r.Versions = []domain.Version{}
	}
	if r.Looks == nil {
		r.Looks = []CopyLook{}
	}
	if r.Confirmations == nil {
		r.Confirmations = []CopyConfirmation{}
	}
	if r.Redirects == nil {
		r.Redirects = []domain.Redirect{}
	}
	if r.Splits == nil {
		r.Splits = []domain.Split{}
	}
	for i := range r.Heads {
		r.Heads[i].CreatedAt = copyTime(r.Heads[i].CreatedAt)
		r.Heads[i].UpdatedAt = copyTime(r.Heads[i].UpdatedAt)
	}
	for i := range r.Versions {
		r.Versions[i].CreatedAt = copyTime(r.Versions[i].CreatedAt)
	}
	for i := range r.Looks {
		r.Looks[i].CreatedAt = copyTime(r.Looks[i].CreatedAt)
	}
	for i := range r.Confirmations {
		r.Confirmations[i].Confirmation.CreatedAt = copyTime(r.Confirmations[i].Confirmation.CreatedAt)
	}
	for i := range r.Redirects {
		r.Redirects[i].CreatedAt = copyTime(r.Redirects[i].CreatedAt)
	}
	for i := range r.Splits {
		r.Splits[i].CreatedAt = copyTime(r.Splits[i].CreatedAt)
	}
	slices.SortFunc(r.Heads, func(a, b domain.Head) int { return compareCopyEntity(a.Kind, a.ID, b.Kind, b.ID) })
	slices.SortFunc(r.Versions, func(a, b domain.Version) int {
		if c := compareCopyEntity(a.Kind, a.EntryID, b.Kind, b.EntryID); c != 0 {
			return c
		}
		if a.Number < b.Number {
			return -1
		}
		if a.Number > b.Number {
			return 1
		}
		return compareCopyID(a.ID, b.ID)
	})
	slices.SortFunc(r.Looks, func(a, b CopyLook) int { return compareCopyID(a.ID, b.ID) })
	slices.SortFunc(r.Confirmations, func(a, b CopyConfirmation) int {
		return compareCopyEntity(a.Kind, a.Confirmation.ID, b.Kind, b.Confirmation.ID)
	})
	slices.SortFunc(r.Redirects, func(a, b domain.Redirect) int { return compareCopyID(a.ID, b.ID) })
	slices.SortFunc(r.Splits, func(a, b domain.Split) int { return compareCopyID(a.ID, b.ID) })
	return r, nil
}

func compareCopyEntity(ak domain.Kind, ai uuid.UUID, bk domain.Kind, bi uuid.UUID) int {
	if ak < bk {
		return -1
	}
	if ak > bk {
		return 1
	}
	return compareCopyID(ai, bi)
}

// ValidateCopyHistory checks all own historical closure and hashes before any identity remapping.
func ValidateCopyHistory(h CopyHistory, org, project uuid.UUID) (CopyCounts, error) {
	bad := func() (CopyCounts, error) { return CopyCounts{}, domain.ErrCorruptHistory }
	if org == uuid.Nil || project == uuid.Nil {
		return bad()
	}
	ids := make(map[uuid.UUID]bool)
	add := func(id uuid.UUID) bool {
		if id == uuid.Nil || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	heads := make(map[uuid.UUID]domain.Head, len(h.Heads))
	versions := make(map[uuid.UUID]domain.Version, len(h.Versions))
	byEntry := make(map[uuid.UUID][]domain.Version, len(h.Heads))
	counts := CopyCounts{Looks: len(h.Looks), Redirects: len(h.Redirects), Splits: len(h.Splits)}
	for _, head := range h.Heads {
		if !add(head.ID) || !head.Kind.Valid() || head.OrgID != org || head.ProjectID != project || head.Revision < 1 || head.CreatedAt.IsZero() || head.UpdatedAt.Before(head.CreatedAt) || head.Kind != domain.KindCharacter && head.RedirectID != nil {
			return bad()
		}
		heads[head.ID] = head
		switch head.Kind {
		case domain.KindCharacter:
			counts.Characters++
		case domain.KindLocation:
			counts.Locations++
		case domain.KindProp:
			counts.Props++
		}
	}
	looks := make(map[uuid.UUID]CopyLook, len(h.Looks))
	for _, look := range h.Looks {
		owner, ok := heads[look.CharacterID]
		if !add(look.ID) || !ok || owner.Kind != domain.KindCharacter || look.CreatedAt.IsZero() {
			return bad()
		}
		looks[look.ID] = look
	}
	usedLooks := make(map[uuid.UUID]bool, len(looks))
	for _, version := range h.Versions {
		head, ok := heads[version.EntryID]
		if !add(version.ID) || !ok || version.Kind != head.Kind || version.OrgID != org || version.ProjectID != project || version.Validate() != nil {
			return bad()
		}
		versions[version.ID] = version
		byEntry[version.EntryID] = append(byEntry[version.EntryID], version)
		switch version.Kind {
		case domain.KindCharacter:
			counts.CharacterVersions++
			for _, look := range version.Character.Looks {
				row, ok := looks[look.ID]
				if !ok || row.CharacterID != version.EntryID || row.CreatedAt.After(version.CreatedAt) {
					return bad()
				}
				usedLooks[look.ID] = true
				counts.LookVersions++
				counts.References += len(look.References)
			}
			if version.Character.Voice != nil {
				counts.Voices++
			}
		case domain.KindLocation:
			counts.LocationVersions++
		case domain.KindProp:
			counts.PropVersions++
		}
	}
	if len(usedLooks) != len(looks) {
		return bad()
	}
	for id, head := range heads {
		vs := byEntry[id]
		slices.SortFunc(vs, func(a, b domain.Version) int {
			if a.Number < b.Number {
				return -1
			}
			if a.Number > b.Number {
				return 1
			}
			return 0
		})
		if len(vs) == 0 || head.CurrentVersionID != vs[len(vs)-1].ID || head.Revision < int64(len(vs)) {
			return bad()
		}
		for i, v := range vs {
			if v.Number != int64(i+1) || i == 0 && v.PreviousID != nil || i > 0 && (v.PreviousID == nil || *v.PreviousID != vs[i-1].ID) {
				return bad()
			}
		}
	}
	confirmed := make(map[uuid.UUID]bool)
	confirmationKeys := make(map[string]bool)
	for _, row := range h.Confirmations {
		c := row.Confirmation
		head, ok := heads[c.EntryID]
		version, vok := versions[c.VersionID]
		key := fmt.Sprint(c.EntryID, "/", c.Revision)
		if !add(c.ID) || !ok || !vok || row.Kind != head.Kind || version.Kind != row.Kind || version.EntryID != head.ID || c.Revision < 1 || c.Revision > head.Revision || c.ActorID == uuid.Nil || c.CreatedAt.IsZero() || c.CreatedAt.Before(version.CreatedAt) || confirmationKeys[key] {
			return bad()
		}
		confirmationKeys[key] = true
		confirmed[c.VersionID] = true
		switch row.Kind {
		case domain.KindCharacter:
			counts.CharacterConfirmations++
		case domain.KindLocation:
			counts.LocationConfirmations++
		case domain.KindProp:
			counts.PropConfirmations++
		}
	}
	for _, head := range h.Heads {
		if head.ConfirmedVersionID != nil {
			v, ok := versions[*head.ConfirmedVersionID]
			if !ok || v.EntryID != head.ID || !confirmed[v.ID] {
				return bad()
			}
		}
	}
	redirected := make(map[uuid.UUID]uuid.UUID)
	for _, r := range h.Redirects {
		source, sok := heads[r.SourceID]
		target, tok := heads[r.TargetID]
		sv, svok := versions[r.SourceVersionID]
		tv, tvok := versions[r.TargetVersionID]
		if !add(r.ID) || !sok || !tok || source.Kind != domain.KindCharacter || target.Kind != domain.KindCharacter || r.SourceID == r.TargetID || !svok || !tvok || sv.EntryID != source.ID || tv.EntryID != target.ID || !confirmed[sv.ID] || !confirmed[tv.ID] || r.ActorID == uuid.Nil || r.CreatedAt.IsZero() || source.RedirectID == nil || *source.RedirectID != target.ID || redirected[source.ID] != uuid.Nil {
			return bad()
		}
		redirected[source.ID] = target.ID
	}
	for _, head := range h.Heads {
		if head.RedirectID != nil && redirected[head.ID] != *head.RedirectID {
			return bad()
		}
		seen := make(map[uuid.UUID]bool)
		current := head.ID
		for n := 0; redirected[current] != uuid.Nil; n++ {
			if n >= 32 || seen[current] {
				return bad()
			}
			seen[current] = true
			current = redirected[current]
		}
	}
	for _, r := range h.Splits {
		source, sok := heads[r.SourceID]
		target, tok := heads[r.TargetID]
		sv, svok := versions[r.SourceVersionID]
		tv, tvok := versions[r.TargetVersionID]
		if !add(r.ID) || !sok || !tok || source.Kind != domain.KindCharacter || target.Kind != domain.KindCharacter || r.SourceID == r.TargetID || !svok || !tvok || sv.EntryID != source.ID || tv.EntryID != target.ID || tv.Number != 1 || r.ActorID == uuid.Nil || r.CreatedAt.IsZero() {
			return bad()
		}
	}
	return counts, nil
}
