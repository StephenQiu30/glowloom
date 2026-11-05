package postgres

import (
	"sort"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

func readCopyHistory(tx *gorm.DB, org, project uuid.UUID) (application.ProjectCopyHistory, error) {
	result := application.ProjectCopyHistory{Sources: []domain.SourceRecord{}, Versions: []domain.ScriptVersion{}, VersionSources: []application.ProjectCopyVersionSource{}, VersionHeads: []application.VersionHead{}, SplitSets: []domain.SplitSet{}, SplitConfirmations: []domain.SplitConfirmation{}, Episodes: []domain.Episode{}, Structures: []domain.EpisodeStructure{}, Scenes: []application.ProjectCopyScene{}, Dialogue: []application.ProjectCopyDialogue{}, Actions: []application.ProjectCopyAction{}}
	state, err := readState(tx, org, project, false)
	if err != nil {
		return result, err
	}
	if state.Revision != 0 {
		state.UpdatedAt = state.UpdatedAt.UTC()
		result.State = &state
	}
	var sources []sourceRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("created_at").Order("id").Find(&sources).Error; err != nil {
		return result, err
	}
	for _, row := range sources {
		record, err := row.record()
		if err != nil {
			return result, err
		}
		result.Sources = append(result.Sources, record)
	}
	var versions []versionRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("version_no").Find(&versions).Error; err != nil {
		return result, err
	}
	for _, row := range versions {
		record, err := row.record()
		if err != nil {
			return result, err
		}
		result.Versions = append(result.Versions, record)
	}
	var versionSources []scriptVersionSourceRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("version_id").Order("position").Find(&versionSources).Error; err != nil {
		return result, err
	}
	for _, row := range versionSources {
		result.VersionSources = append(result.VersionSources, application.ProjectCopyVersionSource{
			OrgID: row.OrgID, ProjectID: row.ProjectID, VersionID: row.VersionID,
			SourceID: row.SourceID, Position: row.Position,
		})
	}
	var versionHeads []scriptVersionHeadRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("version_id").Find(&versionHeads).Error; err != nil {
		return result, err
	}
	for _, row := range versionHeads {
		result.VersionHeads = append(result.VersionHeads, application.VersionHead{
			VersionID: row.VersionID, SplitRevision: row.SplitRevision,
			CandidateSetID: row.CandidateSplitSetID, ConfirmedSetID: row.ConfirmedSplitSetID,
		})
	}
	var episodes []scriptEpisodeRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("script_version_id").Order("seq_no").Order("id").Find(&episodes).Error; err != nil {
		return result, err
	}
	for _, row := range episodes {
		result.Episodes = append(result.Episodes, row.episode())
	}
	var scenes []scriptSceneRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("episode_structure_id").Order("seq_no").Order("id").Find(&scenes).Error; err != nil {
		return result, err
	}
	for _, row := range scenes {
		result.Scenes = append(result.Scenes, row.projectCopyScene())
	}
	var dialogue []scriptDialogueLineRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("scene_id").Order("seq_no").Order("id").Find(&dialogue).Error; err != nil {
		return result, err
	}
	for _, row := range dialogue {
		result.Dialogue = append(result.Dialogue, row.projectCopyDialogue())
	}
	var actions []scriptActionLineRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("scene_id").Order("seq_no").Order("id").Find(&actions).Error; err != nil {
		return result, err
	}
	for _, row := range actions {
		result.Actions = append(result.Actions, row.projectCopyAction())
	}
	var splitSets []scriptSplitSetRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("created_at").Order("id").Find(&splitSets).Error; err != nil {
		return result, err
	}
	for _, row := range splitSets {
		record, err := row.splitSet()
		if err != nil {
			return result, err
		}
		result.SplitSets = append(result.SplitSets, record)
	}
	var structures []scriptEpisodeStructureRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("episode_id").Order("version_no").Find(&structures).Error; err != nil {
		return result, err
	}
	for _, row := range structures {
		record, err := row.structure()
		if err != nil {
			return result, err
		}
		result.Structures = append(result.Structures, record)
	}
	var confirmations []scriptSplitConfirmationRow
	if err := tx.Where("org_id = ? AND project_id = ?", org, project).Order("version_id").Order("revision").Find(&confirmations).Error; err != nil {
		return result, err
	}
	for _, row := range confirmations {
		c, err := row.confirmation()
		if err != nil {
			return result, err
		}
		result.SplitConfirmations = append(result.SplitConfirmations, c)
	}
	return result, nil
}

func orderCopyRows[T any](rows []T, expected []T, key func(T) uuid.UUID) {
	order := make(map[uuid.UUID]int, len(expected))
	for i, row := range expected {
		order[key(row)] = i
	}
	sort.SliceStable(rows, func(i, j int) bool { return order[key(rows[i])] < order[key(rows[j])] })
}

func alignCopyHistory(result *application.ProjectCopyHistory, expected application.ProjectCopyHistory) {
	orderCopyRows(result.Sources, expected.Sources, func(v domain.SourceRecord) uuid.UUID { return v.ID })
	orderCopyRows(result.Versions, expected.Versions, func(v domain.ScriptVersion) uuid.UUID { return v.ID })
	orderCopyRows(result.VersionHeads, expected.VersionHeads, func(v application.VersionHead) uuid.UUID { return v.VersionID })
	orderCopyRows(result.SplitSets, expected.SplitSets, func(v domain.SplitSet) uuid.UUID { return v.ID })
	orderCopyRows(result.SplitConfirmations, expected.SplitConfirmations, func(v domain.SplitConfirmation) uuid.UUID { return v.ID })
	orderCopyRows(result.Episodes, expected.Episodes, func(v domain.Episode) uuid.UUID { return v.ID })
	orderCopyRows(result.Structures, expected.Structures, func(v domain.EpisodeStructure) uuid.UUID { return v.ID })
	orderCopyRows(result.Scenes, expected.Scenes, func(v application.ProjectCopyScene) uuid.UUID { return v.ID })
	orderCopyRows(result.Dialogue, expected.Dialogue, func(v application.ProjectCopyDialogue) uuid.UUID { return v.ID })
	orderCopyRows(result.Actions, expected.Actions, func(v application.ProjectCopyAction) uuid.UUID { return v.ID })

	order := make(map[string]int, len(expected.VersionSources))
	key := func(v application.ProjectCopyVersionSource) string {
		return v.VersionID.String() + "/" + strconv.Itoa(v.Position)
	}
	for i, v := range expected.VersionSources {
		order[key(v)] = i
	}
	sort.SliceStable(result.VersionSources, func(i, j int) bool {
		return order[key(result.VersionSources[i])] < order[key(result.VersionSources[j])]
	})
}
