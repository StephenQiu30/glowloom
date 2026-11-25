// Package postgres reads authorized project evidence and validates provenance.
package postgres

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence"
)

// ErrStale reports an evidence receipt that fails current model admission.
var ErrStale = errors.New("project evidence is no longer current")

type row struct {
	Kind         string
	ID           uuid.UUID
	Part         int
	Version      string
	Revision     int64
	Title        string
	Excerpt      string
	ParentID     *uuid.UUID
	StartPos     *int64
	EndPos       *int64
	PositionUnit string
	Draft        bool
	Rank         float64
}

func (r row) item() evidence.Item {
	return evidence.Item{Reference: evidence.Reference{Kind: r.Kind, ID: r.ID, Part: r.Part, Version: r.Version}, Revision: r.Revision,
		Title: r.Title, Excerpt: r.Excerpt, ParentID: r.ParentID, Start: r.StartPos, End: r.EndPos, PositionUnit: r.PositionUnit, Draft: r.Draft}
}

const columns = "kind,id,part,version,revision,title,excerpt,parent_id,start_pos,end_pos,position_unit,draft"

// Search serves only current evidence from a project already authorized by the caller.
func Search(tx *gorm.DB, project uuid.UUID, query evidence.Query) (evidence.Result, error) {
	return search(tx, project, query, 0)
}

func search(tx *gorm.DB, project uuid.UUID, query evidence.Query, candidates int) (evidence.Result, error) {
	result := evidence.Result{RetrievalMode: "lexical", Status: "no_match", Items: []evidence.Item{}}
	q, err := query.Normalize()
	if err != nil {
		return result, err
	}
	if candidates > q.Limit && q.ShotID == nil {
		q.Limit = candidates
	}
	base := tx.Table("workspace.current_project_evidence").Where("project_id=?", project)
	if len(q.Kinds) > 0 {
		base = base.Where("kind IN ?", q.Kinds)
	}
	var rows []row
	if q.ShotID != nil {
		err := base.Select(columns).Where("kind=? AND parent_id=?", "shot_decision", *q.ShotID).
			Order("title,id").Limit(q.Limit).Scan(&rows).Error
		if err != nil {
			return result, err
		}
	} else if id, err := uuid.Parse(q.Text); err == nil {
		err = base.Select(columns).Where("id=? AND kind<>?", id, "shot_decision").Order("kind,part").Limit(q.Limit).Scan(&rows).Error
		if err != nil {
			return result, err
		}
	} else {
		// Literal substring and exact aliases outrank approximate matches. Parameter
		// binding makes wildcard punctuation ordinary query text.
		const score = `CASE
		 WHEN part=1 AND lower(title)=lower(?) THEN 5
		 WHEN part=1 AND lower(?) IN (SELECT lower(a) FROM jsonb_array_elements_text(
		   CASE WHEN left(aliases,1)='[' AND right(aliases,1)=']' THEN aliases::jsonb ELSE '[]'::jsonb END) AS a) THEN 4
		 WHEN strpos(lower(title),lower(?))>0 THEN 3
		 WHEN strpos(lower(aliases),lower(?))>0 THEN 2.5
		 WHEN strpos(lower(excerpt),lower(?))>0 THEN 2
		 WHEN char_length(?)>=3 THEN greatest(word_similarity(lower(?),lower(title)),word_similarity(lower(?),lower(aliases)),word_similarity(lower(?),lower(excerpt)))
		 ELSE 0 END`
		scored := base.Where("kind<>?", "shot_decision").Select(columns+", "+score+" AS rank", q.Text, q.Text, q.Text, q.Text, q.Text, q.Text, q.Text, q.Text, q.Text)
		err = tx.Table("(?) AS scored", scored).Where("rank >= ?", 0.35).
			Order("rank DESC,part,kind,id").Limit(max(q.Limit, 2)).Scan(&rows).Error
		if err != nil {
			return result, err
		}
	}
	if len(rows) > 0 {
		result.Status = "ok"
		result.Exact = rows[0].Rank >= 4 || q.ShotID != nil
	}
	if len(rows) > 1 && rows[0].Rank >= 4 && rows[1].Rank >= 4 && (rows[0].ID != rows[1].ID || rows[0].Kind != rows[1].Kind) {
		result.Status = "ambiguous"
	}
	for _, r := range rows[:min(q.Limit, len(rows))] {
		result.Items = append(result.Items, r.item())
	}
	return result, nil
}

// Read resolves the live part and returns no content for a stale revision.
func Read(tx *gorm.DB, project uuid.UUID, raw evidence.Reference) (evidence.Result, error) {
	result := evidence.Result{Status: "unavailable", Items: []evidence.Item{}}
	ref, err := raw.Normalize()
	if err != nil {
		return result, err
	}
	var r row
	query := tx.Table("workspace.current_project_evidence").Select(columns).
		Where("project_id=? AND kind=? AND id=? AND part=?", project, ref.Kind, ref.ID, ref.Part).
		Limit(1).Scan(&r)
	if query.Error != nil || query.RowsAffected != 1 {
		return result, query.Error
	}
	if ref.Version != "" && ref.Version != r.Version {
		result.Status = "changed"
		return result, nil
	}
	result.Status = "ok"
	result.Items = append(result.Items, r.item())
	return result, nil
}

// VerifyModelReceipts proves that every original evidence item remains visible
// for the same project and private conversation at a new model admission.
func VerifyModelReceipts(tx *gorm.DB, actor, org, scope, project, session uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > evidence.MaxReceipts {
		return ErrStale
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return ErrStale
		}
		seen[id] = true
	}
	var valid int
	err := tx.Raw(`WITH authorized AS (
	  SELECT c.receipt_id,c.result FROM agent_business.call_receipt c
	  JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id
	  WHERE c.receipt_id IN ? AND c.kind='tool' AND c.status='completed' AND c.name IN ?
	    AND a.session_id=? AND a.scope->>'actor_id'=? AND a.scope->>'org_id'=? AND a.scope->>'scope_id'=? AND a.scope->>'project_id'=?
	)
	SELECT count(*) FROM authorized r
	WHERE jsonb_typeof(r.result->'items')='array' AND jsonb_array_length(r.result->'items')>0
	  AND NOT EXISTS (
	    SELECT 1 FROM jsonb_array_elements(r.result->'items') AS item
	    WHERE NOT EXISTS (
	      SELECT 1 FROM workspace.current_project_evidence e
	      WHERE e.project_id=? AND e.kind=item->>'kind' AND e.id::text=item->>'id'
	        AND e.part::text=item->>'part' AND e.version=item->>'version'
	    )
	  )`, ids, []string{evidence.SearchTool, evidence.ReadTool}, session, actor.String(), org.String(), scope.String(), project.String(), project).Scan(&valid).Error
	if err != nil {
		return err
	}
	if valid != len(ids) {
		return ErrStale
	}
	return nil
}
