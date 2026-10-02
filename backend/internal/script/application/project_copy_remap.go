package application

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// RemapProjectHistory prepares every historical identity and independent object key.
// It never reads remote bytes or admits access; the owning store supplies authority.
func RemapProjectHistory(binding ProjectCopyBinding, history ProjectCopyHistory, mediaMapping map[uuid.UUID]uuid.UUID) (ProjectCopyManifest, error) {
	return RemapProjectHistoryWithCharacters(binding, history, mediaMapping, nil)
}

// RemapProjectHistoryWithCharacters consumes only exact owning historical Bible identity/version mappings.
func RemapProjectHistoryWithCharacters(binding ProjectCopyBinding, history ProjectCopyHistory, mediaMapping map[uuid.UUID]uuid.UUID, characters []ProjectCopyCharacterMapping) (ProjectCopyManifest, error) {
	if binding.JobID == uuid.Nil || binding.OrgID == uuid.Nil || binding.SourceProjectID == uuid.Nil || binding.TargetProjectID == uuid.Nil || binding.SourceProjectID == binding.TargetProjectID {
		return ProjectCopyManifest{}, domain.ErrInvalidSource
	}
	if err := validateCopyHistory(binding, history); err != nil {
		return ProjectCopyManifest{}, err
	}
	characterMapping, err := copyCharacterMap(history, characters)
	if err != nil {
		return ProjectCopyManifest{}, err
	}
	target, err := cloneCopyHistory(history)
	if err != nil {
		return ProjectCopyManifest{}, err
	}
	ids := copyHistoryIdentities(history)
	for old := range ids {
		ids[old] = projectCopyIdentity(binding.JobID, old)
	}
	objects := make([]ProjectCopyObject, 0)
	facts := make(map[string]domain.ObjectFact)
	replaceObject := func(fact domain.ObjectFact) domain.ObjectFact {
		if prior, ok := facts[fact.Key]; ok {
			return prior
		}
		mapped := fact
		mapped.Key = "projects/" + binding.TargetProjectID.String() + "/script/copy/" + binding.JobID.String() + "/" + domain.ContentSHA([]byte(fact.Key))
		facts[fact.Key] = mapped
		objects = append(objects, ProjectCopyObject{Source: fact, Target: mapped})
		return mapped
	}
	if err := transformCopyHistory(&target, binding.TargetProjectID, binding.OrgID, ids, mediaMapping, replaceObject, characterMapping); err != nil {
		return ProjectCopyManifest{}, err
	}
	records := make(map[uuid.UUID]domain.SourceRecord, len(target.Sources))
	for _, source := range target.Sources {
		records[source.ID] = source
	}
	for i := range target.Versions {
		version := &target.Versions[i]
		ordered := make([]domain.SourceRecord, 0, len(version.SourceIDs))
		for _, id := range version.SourceIDs {
			ordered = append(ordered, records[id])
		}
		data, err := json.Marshal(ordered)
		if err != nil {
			return ProjectCopyManifest{}, err
		}
		version.SourceManifestSHA256 = domain.ContentSHA(data)
	}
	content, err := ProjectHistoryContentSHA(history)
	if err != nil {
		return ProjectCopyManifest{}, err
	}
	counts := ScriptCopyCounts{Sources: len(history.Sources), Versions: len(history.Versions), VersionSources: len(history.VersionSources), VersionHeads: len(history.VersionHeads), SplitSets: len(history.SplitSets), SplitConfirmations: len(history.SplitConfirmations), Episodes: len(history.Episodes), Structures: len(history.Structures), Scenes: len(history.Scenes), DialogueLines: len(history.Dialogue), ActionLines: len(history.Actions), Objects: len(objects)}
	if history.State != nil {
		counts.ProjectStates = 1
	}
	return ProjectCopyManifest{Binding: binding, Source: history, Target: target, Objects: objects, Counts: counts, ContentSHA256: content, Characters: characters}, nil
}

func cloneCopyHistory(history ProjectCopyHistory) (ProjectCopyHistory, error) {
	data, err := json.Marshal(history)
	if err != nil {
		return ProjectCopyHistory{}, err
	}
	var result ProjectCopyHistory
	if err := json.Unmarshal(data, &result); err != nil {
		return ProjectCopyHistory{}, err
	}
	return result, nil
}

func copyHistoryIdentities(history ProjectCopyHistory) map[uuid.UUID]uuid.UUID {
	result := make(map[uuid.UUID]uuid.UUID)
	add := func(id uuid.UUID) {
		if id != uuid.Nil {
			if _, ok := result[id]; !ok {
				result[id] = uuid.NewSHA1(uuid.Nil, []byte("script-identity/"+strconv.Itoa(len(result))))
			}
		}
	}
	for _, s := range history.Sources {
		add(s.ID)
		add(s.LineageID)
	}
	for _, v := range history.Versions {
		add(v.ID)
	}
	for _, s := range history.SplitSets {
		add(s.ID)
	}
	for _, c := range history.SplitConfirmations {
		add(c.ID)
	}
	for _, e := range history.Episodes {
		add(e.ID)
	}
	for _, s := range history.Structures {
		add(s.ID)
		for _, scene := range s.Document.Scenes {
			add(scene.Key)
			for _, item := range scene.Items {
				add(item.Key)
			}
		}
		for _, line := range s.Document.Unassigned {
			add(line.Key)
		}
	}
	for _, s := range history.Scenes {
		add(s.ID)
		add(s.SceneKey)
	}
	for _, s := range history.Dialogue {
		add(s.ID)
		add(s.LineKey)
	}
	for _, s := range history.Actions {
		add(s.ID)
		add(s.LineKey)
	}
	return result
}

// ProjectHistoryContentSHA compares full semantic history across project/ID remapping.
// Actor/rights/provenance and object digests remain evidence; storage locations and
// own identifiers use deterministic ordinal identities, rather than being discarded.
func ProjectHistoryContentSHA(history ProjectCopyHistory) (string, error) {
	canonical, err := cloneCopyHistory(history)
	if err != nil {
		return "", err
	}
	mediaIDs := make(map[uuid.UUID]uuid.UUID)
	for _, source := range history.Sources {
		if source.MediaAssetID != nil {
			if _, ok := mediaIDs[*source.MediaAssetID]; !ok {
				mediaIDs[*source.MediaAssetID] = uuid.NewSHA1(uuid.Nil, []byte("script-media/"+strconv.Itoa(len(mediaIDs))))
			}
		}
	}
	keys := make(map[string]string)
	object := func(fact domain.ObjectFact) domain.ObjectFact {
		key, ok := keys[fact.Key]
		if !ok {
			key = "script-object/" + strconv.Itoa(len(keys))
			keys[fact.Key] = key
		}
		fact.Key = key
		return fact
	}
	if err := transformCopyHistory(&canonical, uuid.Nil, uuid.Nil, copyHistoryIdentities(history), mediaIDs, object, canonicalCharacterMapping(history)); err != nil {
		return "", err
	}
	for i := range canonical.Versions {
		canonical.Versions[i].SourceManifestSHA256 = ""
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return domain.ContentSHA(data), nil
}

func transformCopyHistory(h *ProjectCopyHistory, project, org uuid.UUID, ids, media map[uuid.UUID]uuid.UUID, object func(domain.ObjectFact) domain.ObjectFact, characters map[characterCopyKey]ProjectCopyCharacterReference) error {
	ptr := func(id **uuid.UUID) {
		if *id != nil {
			mapped := ids[**id]
			*id = &mapped
		}
	}
	episode := func(e *domain.Episode) {
		e.ID, e.OrgID, e.ProjectID, e.VersionID, e.SplitSetID = ids[e.ID], org, project, ids[e.VersionID], ids[e.SplitSetID]
		ptr(&e.CurrentStructureID)
		ptr(&e.ConfirmedStructureID)
		ptr(&e.PreviousEpisodeID)
	}
	if h.State != nil {
		h.State.ProjectID, h.State.OrgID = project, org
		ptr(&h.State.DraftVersionID)
		ptr(&h.State.AdoptedVersionID)
		h.State.UpdatedAt = h.State.UpdatedAt.UTC()
	}
	for i := range h.Sources {
		s := &h.Sources[i]
		s.ID, s.LineageID, s.OrgID, s.ProjectID = ids[s.ID], ids[s.LineageID], org, project
		ptr(&s.PreviousID)
		if s.MediaAssetID != nil {
			mapped, ok := media[*s.MediaAssetID]
			if !ok || mapped == uuid.Nil {
				return ErrContextUnavailable
			}
			s.MediaAssetID = &mapped
		}
		s.Original, s.Rich = object(s.Original), object(s.Rich)
		s.CreatedAt, s.RightsConfirmedAt = s.CreatedAt.UTC(), s.RightsConfirmedAt.UTC()
	}
	for i := range h.Versions {
		v := &h.Versions[i]
		v.ID, v.OrgID, v.ProjectID = ids[v.ID], org, project
		for j, id := range v.SourceIDs {
			v.SourceIDs[j] = ids[id]
		}
		for j := range v.Spans {
			v.Spans[j].SourceID = ids[v.Spans[j].SourceID]
			v.Spans[j].LineageID = ids[v.Spans[j].LineageID]
		}
		v.Text, v.Rich = object(v.Text), object(v.Rich)
		v.CreatedAt = v.CreatedAt.UTC()
	}
	for i := range h.VersionSources {
		v := &h.VersionSources[i]
		v.OrgID, v.ProjectID, v.VersionID, v.SourceID = org, project, ids[v.VersionID], ids[v.SourceID]
	}
	for i := range h.VersionHeads {
		v := &h.VersionHeads[i]
		v.VersionID, v.CandidateSetID = ids[v.VersionID], ids[v.CandidateSetID]
		ptr(&v.ConfirmedSetID)
	}
	for i := range h.SplitSets {
		s := &h.SplitSets[i]
		s.ID, s.OrgID, s.ProjectID, s.VersionID = ids[s.ID], org, project, ids[s.VersionID]
		s.CreatedAt = s.CreatedAt.UTC()
		for j := range s.Boundaries {
			ptr(&s.Boundaries[j].SourceLineageID)
		}
	}
	for i := range h.SplitConfirmations {
		c := &h.SplitConfirmations[i]
		c.ID, c.OrgID, c.ProjectID, c.VersionID, c.CandidateSetID, c.FormalSetID = ids[c.ID], org, project, ids[c.VersionID], ids[c.CandidateSetID], ids[c.FormalSetID]
		c.CreatedAt = c.CreatedAt.UTC()
		for j := range c.Episodes {
			episode(&c.Episodes[j])
		}
	}
	for i := range h.Episodes {
		episode(&h.Episodes[i])
	}
	for i := range h.Structures {
		s := &h.Structures[i]
		s.ID, s.OrgID, s.ProjectID, s.EpisodeID = ids[s.ID], org, project, ids[s.EpisodeID]
		s.CreatedAt = s.CreatedAt.UTC()
		for j := range s.Document.Scenes {
			scene := &s.Document.Scenes[j]
			scene.Key = ids[scene.Key]
			for k := range scene.Items {
				item := &scene.Items[k]
				item.Key = ids[item.Key]
				if err := remapStructureCharacter(&item.CharacterID, &item.CharacterVersionID, characters); err != nil {
					return err
				}
			}
		}
		for j := range s.Document.Unassigned {
			s.Document.Unassigned[j].Key = ids[s.Document.Unassigned[j].Key]
		}
	}
	for i := range h.Scenes {
		s := &h.Scenes[i]
		s.ID, s.OrgID, s.ProjectID, s.StructureID, s.SceneKey = ids[s.ID], org, project, ids[s.StructureID], ids[s.SceneKey]
	}
	for i := range h.Dialogue {
		s := &h.Dialogue[i]
		s.ID, s.OrgID, s.ProjectID, s.SceneID, s.LineKey = ids[s.ID], org, project, ids[s.SceneID], ids[s.LineKey]
		if err := remapStructureCharacter(&s.CharacterID, &s.CharacterVersionID, characters); err != nil {
			return err
		}
	}
	for i := range h.Actions {
		s := &h.Actions[i]
		s.ID, s.OrgID, s.ProjectID, s.SceneID, s.LineKey = ids[s.ID], org, project, ids[s.SceneID], ids[s.LineKey]
	}
	return nil
}

func validateCopyHistory(binding ProjectCopyBinding, h ProjectCopyHistory) error {
	scope := func(org, project uuid.UUID) bool { return org == binding.OrgID && project == binding.SourceProjectID }
	sources := make(map[uuid.UUID]domain.SourceRecord)
	versions := make(map[uuid.UUID]domain.ScriptVersion)
	sets := make(map[uuid.UUID]domain.SplitSet)
	episodes := make(map[uuid.UUID]domain.Episode)
	structures := make(map[uuid.UUID]domain.EpisodeStructure)
	scenes := make(map[uuid.UUID]ProjectCopyScene)
	objects := make(map[string]domain.ObjectFact)
	fact := func(f domain.ObjectFact) error {
		if err := f.Validate(); err != nil {
			return err
		}
		if prior, ok := objects[f.Key]; ok && prior != f {
			return ErrObjectMismatch
		}
		objects[f.Key] = f
		return nil
	}
	for _, s := range h.Sources {
		if s.ID == uuid.Nil || !scope(s.OrgID, s.ProjectID) || sources[s.ID].ID != uuid.Nil {
			return domain.ErrInvalidSource
		}
		sources[s.ID] = s
		if err := fact(s.Original); err != nil {
			return err
		}
		if err := fact(s.Rich); err != nil {
			return err
		}
	}
	for _, s := range h.Sources {
		if sources[s.LineageID].ID == uuid.Nil || s.PreviousID != nil && sources[*s.PreviousID].ID == uuid.Nil {
			return domain.ErrInvalidSource
		}
	}
	for _, v := range h.Versions {
		if v.Text.SHA256 != v.ContentHash || v.Rich.SHA256 != v.DocumentSHA256 {
			return ErrObjectMismatch
		}
		if v.ID == uuid.Nil || !scope(v.OrgID, v.ProjectID) || versions[v.ID].ID != uuid.Nil {
			return domain.ErrInvalidSource
		}
		versions[v.ID] = v
		for _, id := range v.SourceIDs {
			if sources[id].ID == uuid.Nil {
				return domain.ErrInvalidSource
			}
		}
		for _, span := range v.Spans {
			if sources[span.SourceID].ID == uuid.Nil || sources[span.SourceID].LineageID != span.LineageID {
				return domain.ErrInvalidSource
			}
		}
		if err := fact(v.Text); err != nil {
			return err
		}
		if err := fact(v.Rich); err != nil {
			return err
		}
	}
	membership := make(map[uuid.UUID]map[int]uuid.UUID)
	for _, v := range h.VersionSources {
		if !scope(v.OrgID, v.ProjectID) || versions[v.VersionID].ID == uuid.Nil || sources[v.SourceID].ID == uuid.Nil {
			return domain.ErrInvalidSource
		}
		if membership[v.VersionID] == nil {
			membership[v.VersionID] = make(map[int]uuid.UUID)
		}
		if _, ok := membership[v.VersionID][v.Position]; ok {
			return domain.ErrInvalidSource
		}
		membership[v.VersionID][v.Position] = v.SourceID
	}
	for _, v := range h.Versions {
		ordered := make([]domain.SourceRecord, 0, len(v.SourceIDs))
		for _, id := range v.SourceIDs {
			ordered = append(ordered, sources[id])
		}
		encoded, err := json.Marshal(ordered)
		if err != nil {
			return err
		}
		if domain.ContentSHA(encoded) != v.SourceManifestSHA256 {
			return ErrObjectMismatch
		}
		if len(membership[v.ID]) != len(v.SourceIDs) {
			return domain.ErrInvalidSource
		}
		for i, id := range v.SourceIDs {
			if membership[v.ID][i] != id {
				return domain.ErrInvalidSource
			}
		}
	}
	for _, s := range h.SplitSets {
		if s.ID == uuid.Nil || !scope(s.OrgID, s.ProjectID) || versions[s.VersionID].ID == uuid.Nil || sets[s.ID].ID != uuid.Nil {
			return domain.ErrInvalidSource
		}
		sets[s.ID] = s
		for _, b := range s.Boundaries {
			if b.SourceLineageID != nil && sources[*b.SourceLineageID].ID == uuid.Nil {
				return domain.ErrInvalidSource
			}
		}
	}
	for _, e := range h.Episodes {
		if e.ID == uuid.Nil || !scope(e.OrgID, e.ProjectID) || versions[e.VersionID].ID == uuid.Nil || sets[e.SplitSetID].ID == uuid.Nil || episodes[e.ID].ID != uuid.Nil {
			return domain.ErrInvalidSource
		}
		episodes[e.ID] = e
	}
	for _, s := range h.Structures {
		if s.ID == uuid.Nil || !scope(s.OrgID, s.ProjectID) || episodes[s.EpisodeID].ID == uuid.Nil || structures[s.ID].ID != uuid.Nil {
			return domain.ErrInvalidSource
		}
		structures[s.ID] = s
	}
	validPointer := func(id *uuid.UUID, contains func(uuid.UUID) bool) bool { return id == nil || contains(*id) }
	for _, e := range h.Episodes {
		if !validPointer(e.PreviousEpisodeID, func(id uuid.UUID) bool { return episodes[id].ID != uuid.Nil }) || !validPointer(e.CurrentStructureID, func(id uuid.UUID) bool { return structures[id].EpisodeID == e.ID }) || !validPointer(e.ConfirmedStructureID, func(id uuid.UUID) bool { return structures[id].EpisodeID == e.ID }) {
			return domain.ErrInvalidSource
		}
	}
	for _, head := range h.VersionHeads {
		if versions[head.VersionID].ID == uuid.Nil || sets[head.CandidateSetID].VersionID != head.VersionID || !validPointer(head.ConfirmedSetID, func(id uuid.UUID) bool { return sets[id].VersionID == head.VersionID }) {
			return domain.ErrInvalidSource
		}
	}
	if h.State != nil && (!scope(h.State.OrgID, h.State.ProjectID) || !validPointer(h.State.DraftVersionID, func(id uuid.UUID) bool { return versions[id].ID != uuid.Nil }) || !validPointer(h.State.AdoptedVersionID, func(id uuid.UUID) bool { return versions[id].ID != uuid.Nil })) {
		return domain.ErrInvalidSource
	}
	for _, c := range h.SplitConfirmations {
		if c.ID == uuid.Nil || !scope(c.OrgID, c.ProjectID) || versions[c.VersionID].ID == uuid.Nil || sets[c.CandidateSetID].VersionID != c.VersionID || sets[c.FormalSetID].VersionID != c.VersionID {
			return domain.ErrInvalidSource
		}
		for _, e := range c.Episodes {
			if !scope(e.OrgID, e.ProjectID) || episodes[e.ID].ID == uuid.Nil || versions[e.VersionID].ID == uuid.Nil || sets[e.SplitSetID].ID == uuid.Nil {
				return domain.ErrInvalidSource
			}
		}
	}
	for _, s := range h.Scenes {
		if s.ID == uuid.Nil || !scope(s.OrgID, s.ProjectID) || structures[s.StructureID].ID == uuid.Nil || scenes[s.ID].ID != uuid.Nil {
			return domain.ErrInvalidStructure
		}
		scenes[s.ID] = s
	}
	for _, d := range h.Dialogue {
		if d.ID == uuid.Nil || !scope(d.OrgID, d.ProjectID) || scenes[d.SceneID].ID == uuid.Nil {
			return domain.ErrInvalidStructure
		}
	}
	for _, a := range h.Actions {
		if a.ID == uuid.Nil || !scope(a.OrgID, a.ProjectID) || scenes[a.SceneID].ID == uuid.Nil {
			return domain.ErrInvalidStructure
		}
	}
	if h.State == nil && (len(h.Sources) != 0 || len(h.Versions) != 0 || len(h.Episodes) != 0) {
		return domain.ErrInvalidSource
	}
	if err := validateCopyStructures(h); err != nil {
		return err
	}
	if len(h.VersionHeads) != len(h.Versions) {
		return fmt.Errorf("script copy version heads: %w", ErrObjectMismatch)
	}
	return nil
}
