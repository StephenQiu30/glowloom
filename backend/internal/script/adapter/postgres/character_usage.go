package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

// CharacterUsage contains a digest of immutable lines and their current owning heads.
type CharacterUsage struct {
	SHA256          string
	CurrentLines    int64
	HistoricalLines int64
}

// ReadCharacterUsage runs under the caller's project content lock. Historical
// structures remain intact; deletion only prevents future character admission.
func (s *BibleScopes) ReadCharacterUsage(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) (result CharacterUsage, resultErr error) {
	if s == nil || s.tx == nil || s.access == nil || len(ids) == 0 {
		return result, application.ErrContextUnavailable
	}
	if _, ok := s.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return result, application.ErrContextUnavailable
	}
	facts, err := s.access.Authorize(ctx, actor, project, false)
	if err != nil {
		return result, err
	}
	if facts.ProjectID != project || facts.OrgID != actor.OrgID {
		return result, application.ErrContextUnavailable
	}
	tx := s.tx.WithContext(ctx)
	var sceneIDs []uuid.UUID
	if err := tx.Model(&scriptDialogueLineRow{}).Where("org_id = ? AND project_id = ? AND character_id IN ?", actor.OrgID, project, ids).
		Distinct("scene_id").Pluck("scene_id", &sceneIDs).Error; err != nil {
		return result, fmt.Errorf("read character scene references: %w", err)
	}
	var scenes []scriptSceneRow
	if len(sceneIDs) > 0 {
		if err := tx.Select("id", "episode_structure_id").Where("org_id = ? AND project_id = ? AND id IN ?", actor.OrgID, project, sceneIDs).Find(&scenes).Error; err != nil {
			return result, fmt.Errorf("read character scene owners: %w", err)
		}
	}
	sceneByID := make(map[uuid.UUID]scriptSceneRow, len(scenes))
	structureIDs := make([]uuid.UUID, 0, len(scenes))
	for _, scene := range scenes {
		sceneByID[scene.ID] = scene
		structureIDs = append(structureIDs, scene.StructureID)
	}
	var structures []scriptStructureSummaryRow
	if len(structureIDs) > 0 {
		if err := tx.Select("id", "episode_id").Where("org_id = ? AND project_id = ? AND id IN ?", actor.OrgID, project, structureIDs).Find(&structures).Error; err != nil {
			return result, fmt.Errorf("read character structure owners: %w", err)
		}
	}
	structureByID := make(map[uuid.UUID]scriptStructureSummaryRow, len(structures))
	episodeIDs := make([]uuid.UUID, 0, len(structures))
	for _, structure := range structures {
		structureByID[structure.ID] = structure
		episodeIDs = append(episodeIDs, structure.EpisodeID)
	}
	var episodes []scriptEpisodeRow
	if len(episodeIDs) > 0 {
		if err := tx.Select("id", "script_version_id", "split_set_id", "revision", "current_structure_id", "confirmed_structure_id", "is_delete").
			Where("org_id = ? AND project_id = ? AND id IN ?", actor.OrgID, project, episodeIDs).Find(&episodes).Error; err != nil {
			return result, fmt.Errorf("read character episode owners: %w", err)
		}
	}
	episodeByID := make(map[uuid.UUID]scriptEpisodeRow, len(episodes))
	versionIDs := make([]uuid.UUID, 0, len(episodes))
	for _, episode := range episodes {
		episodeByID[episode.ID] = episode
		versionIDs = append(versionIDs, episode.VersionID)
	}
	var heads []scriptVersionHeadRow
	if len(versionIDs) > 0 {
		if err := tx.Select("version_id", "confirmed_split_set_id").Where("org_id = ? AND project_id = ? AND version_id IN ?", actor.OrgID, project, versionIDs).Find(&heads).Error; err != nil {
			return result, fmt.Errorf("read character version heads: %w", err)
		}
	}
	headByVersion := make(map[uuid.UUID]scriptVersionHeadRow, len(heads))
	for _, head := range heads {
		headByVersion[head.VersionID] = head
	}
	state, err := readState(tx, actor.OrgID, project, false)
	if err != nil {
		return result, err
	}
	rows, err := tx.Model(&scriptDialogueLineRow{}).Select("id", "character_id", "character_version_id", "scene_id").
		Where("org_id = ? AND project_id = ? AND character_id IN ?", actor.OrgID, project, ids).
		Order("id").Rows()
	if err != nil {
		return result, fmt.Errorf("stream character dialogue lines: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, rows.Close())
	}()
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for rows.Next() {
		var line struct {
			ID, CharacterID uuid.UUID
			VersionID       *uuid.UUID
			SceneID         uuid.UUID
		}
		if err := rows.Scan(&line.ID, &line.CharacterID, &line.VersionID, &line.SceneID); err != nil {
			return result, err
		}
		scene, exists := sceneByID[line.SceneID]
		if !exists {
			continue
		}
		structure, exists := structureByID[scene.StructureID]
		if !exists {
			continue
		}
		episode, exists := episodeByID[structure.EpisodeID]
		if !exists {
			continue
		}
		head := headByVersion[episode.VersionID]
		currentVersion := state.DraftVersionID != nil && *state.DraftVersionID == episode.VersionID || state.AdoptedVersionID != nil && *state.AdoptedVersionID == episode.VersionID
		currentStructure := episode.CurrentStructureID != nil && *episode.CurrentStructureID == structure.ID || episode.ConfirmedStructureID != nil && *episode.ConfirmedStructureID == structure.ID
		current := !episode.IsDelete && currentStructure && head.ConfirmedSplitSetID != nil && *head.ConfirmedSplitSetID == episode.SplitSetID && currentVersion
		fact := struct {
			ID, CharacterID        uuid.UUID
			VersionID              *uuid.UUID
			StructureID, EpisodeID uuid.UUID
			Revision               int64
			Current                bool
		}{line.ID, line.CharacterID, line.VersionID, structure.ID, episode.ID, episode.Revision, current}
		if err := encoder.Encode(fact); err != nil {
			return result, err
		}
		if fact.Current {
			result.CurrentLines++
		} else {
			result.HistoricalLines++
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return result, nil
}
