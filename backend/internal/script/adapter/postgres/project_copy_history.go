package postgres

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
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
	if err := tx.Raw(`SELECT `+sourceColumns+` FROM script.script_source WHERE org_id=? AND project_id=? ORDER BY created_at,id`, org, project).Scan(&sources).Error; err != nil {
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
	if err := tx.Raw(`SELECT `+versionColumns+` FROM script.script_version WHERE org_id=? AND project_id=? ORDER BY version_no`, org, project).Scan(&versions).Error; err != nil {
		return result, err
	}
	for _, row := range versions {
		record, err := row.record()
		if err != nil {
			return result, err
		}
		result.Versions = append(result.Versions, record)
	}
	var characterVersionColumn bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='script' AND table_name='dialogue_line' AND column_name='character_version_id')`).Scan(&characterVersionColumn).Error; err != nil {
		return result, err
	}
	characterColumns := ""
	if characterVersionColumn {
		characterColumns = ",character_version_id"
	}
	for _, query := range []struct {
		sql string
		out any
	}{
		{`SELECT org_id,project_id,version_id,source_id,position FROM script.version_source WHERE org_id=? AND project_id=? ORDER BY version_id,position`, &result.VersionSources},
		{`SELECT version_id,split_revision,candidate_split_set_id AS candidate_set_id,confirmed_split_set_id AS confirmed_set_id FROM script.version_head WHERE org_id=? AND project_id=? ORDER BY version_id`, &result.VersionHeads},
		{`SELECT ` + episodeColumns + ` FROM script.episode WHERE org_id=? AND project_id=? ORDER BY script_version_id,seq_no,id`, &result.Episodes},
		{`SELECT id,org_id,project_id,episode_structure_id AS structure_id,scene_key,seq_no,heading,location_text,time_of_day,span_start AS start,span_end AS "end" FROM script.scene WHERE org_id=? AND project_id=? ORDER BY episode_structure_id,seq_no,id`, &result.Scenes},
		{`SELECT id,org_id,project_id,scene_id,line_key,seq_no,kind,speaker_text AS speaker,content,emotion,content_hash,character_id` + characterColumns + `,span_start AS start,span_end AS "end" FROM script.dialogue_line WHERE org_id=? AND project_id=? ORDER BY scene_id,seq_no,id`, &result.Dialogue},
		{`SELECT id,org_id,project_id,scene_id,line_key,seq_no,content,span_start AS start,span_end AS "end" FROM script.action_line WHERE org_id=? AND project_id=? ORDER BY scene_id,seq_no,id`, &result.Actions},
	} {
		if err := tx.Raw(query.sql, org, project).Scan(query.out).Error; err != nil {
			return result, err
		}
	}
	var setIDs []struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM script.split_set WHERE org_id=? AND project_id=? ORDER BY created_at,id`, org, project).Scan(&setIDs).Error; err != nil {
		return result, err
	}
	for _, row := range setIDs {
		record, err := readSplitSet(tx, org, project, row.ID)
		if err != nil {
			return result, err
		}
		result.SplitSets = append(result.SplitSets, record)
	}
	var structureIDs []struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM script.episode_structure WHERE org_id=? AND project_id=? ORDER BY episode_id,version_no`, org, project).Scan(&structureIDs).Error; err != nil {
		return result, err
	}
	for _, row := range structureIDs {
		record, err := readStructure(tx, org, project, row.ID)
		if err != nil {
			return result, err
		}
		result.Structures = append(result.Structures, record)
	}
	var confirmations []struct {
		ID, OrgID, ProjectID, VersionID, CandidateSetID, FormalSetID, ActorID uuid.UUID
		Revision                                                              int64
		Preface, Episodes                                                     string
		CreatedAt                                                             time.Time
	}
	if err := tx.Raw(`SELECT id,org_id,project_id,version_id,candidate_set_id,formal_set_id,actor_id,revision,preface::text,episodes::text,created_at FROM script.split_confirmation WHERE org_id=? AND project_id=? ORDER BY version_id,revision`, org, project).Scan(&confirmations).Error; err != nil {
		return result, err
	}
	for _, row := range confirmations {
		c := domain.SplitConfirmation{ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID, VersionID: row.VersionID, CandidateSetID: row.CandidateSetID, FormalSetID: row.FormalSetID, ActorID: row.ActorID, Revision: row.Revision, CreatedAt: row.CreatedAt.UTC()}
		if err := json.Unmarshal([]byte(row.Preface), &c.Preface); err != nil {
			return result, err
		}
		if err := json.Unmarshal([]byte(row.Episodes), &c.Episodes); err != nil {
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
