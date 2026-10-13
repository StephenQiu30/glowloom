package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// CharacterUsage contains a digest of immutable lines and their current owning heads.
type CharacterUsage struct {
	SHA256          string
	CurrentLines    int64
	HistoricalLines int64
}

// ReadCharacterUsage runs under the caller's project content lock. Historical
// structures remain intact; deletion only prevents future character admission.
func (s *BibleScopes) ReadCharacterUsage(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) (CharacterUsage, error) {
	result := CharacterUsage{}
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
	rows, err := s.tx.WithContext(ctx).Raw(`SELECT l.id,l.character_id,l.character_version_id,st.id,e.id,e.revision,
 COALESCE(NOT e.is_delete AND (e.current_structure_id=st.id OR e.confirmed_structure_id=st.id)
 AND h.confirmed_split_set_id=e.split_set_id AND (p.draft_version_id=e.script_version_id OR p.adopted_version_id=e.script_version_id),false) AS current
 FROM script.dialogue_line l
 JOIN script.scene sc ON sc.id=l.scene_id AND sc.org_id=l.org_id AND sc.project_id=l.project_id
 JOIN script.episode_structure st ON st.id=sc.episode_structure_id AND st.org_id=l.org_id AND st.project_id=l.project_id
 JOIN script.episode e ON e.id=st.episode_id AND e.org_id=l.org_id AND e.project_id=l.project_id
 LEFT JOIN script.version_head h ON h.version_id=e.script_version_id AND h.org_id=e.org_id AND h.project_id=e.project_id
 LEFT JOIN script.project_state p ON p.org_id=e.org_id AND p.project_id=e.project_id
 WHERE l.org_id=? AND l.project_id=? AND l.character_id IN ? ORDER BY l.id`, actor.OrgID, project, ids).Rows()
	if err != nil {
		return result, fmt.Errorf("read character usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for rows.Next() {
		var line struct {
			ID, CharacterID        uuid.UUID
			VersionID              *uuid.UUID
			StructureID, EpisodeID uuid.UUID
			Revision               int64
			Current                bool
		}
		if err := rows.Scan(&line.ID, &line.CharacterID, &line.VersionID, &line.StructureID, &line.EpisodeID, &line.Revision, &line.Current); err != nil {
			return result, err
		}
		if err := encoder.Encode(line); err != nil {
			return result, err
		}
		if line.Current {
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
