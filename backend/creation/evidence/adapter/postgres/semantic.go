package postgres

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
)

// Profile identifies the pinned text representation, vector model, dimensions and instructions.
const Profile = "ark-251215-text-1024-asymmetric-v1"

// IndexTask is the durable provider queue task for one project policy generation.
const IndexTask = "project.evidence_embedding"

// Policy binds a project to its operator, cumulative allowance and embedding releases.
type Policy struct {
	ProjectID, BudgetID, Generation, ActorID, ReleaseID uuid.UUID
	ActiveReleaseID                                     *uuid.UUID
	ActiveProfile, ScanCursor                           string
	LimitCredits                                        int64
	Enabled                                             bool
	OrgID, ScopeID                                      uuid.UUID
}

// QueryReady reports whether this binary can encode queries in the active vector space.
func (p Policy) QueryReady() bool {
	return p.ActiveReleaseID != nil && *p.ActiveReleaseID == p.ReleaseID && p.ActiveProfile == Profile
}

// ReadPolicy locks the active project policy until admission or publication commits.
func ReadPolicy(tx *gorm.DB, project uuid.UUID) (Policy, error) {
	var p Policy
	err := tx.Raw(`SELECT s.*,p.org_id,p.owner_scope_id AS scope_id,m.id AS release_id
 FROM workspace.evidence_semantic_policy s JOIN workspace.project p ON p.id=s.project_id
 LEFT JOIN ai.available_model m ON m.capability='text.embed' AND m.name=p.default_models->>'text.embed'
 WHERE s.project_id=? AND s.enabled AND NOT p.is_delete AND p.status='active' FOR SHARE OF p,s`, project).Scan(&p).Error
	return p, err
}

// Passage contains the live evidence locator and the deterministic text to encode.
type Passage struct {
	evidence.Reference
	Text   string
	Cursor string
}

const passageText = "concat_ws(E'\\n',kind,nullif(title,''),nullif(aliases,''),nullif(excerpt,''))"
const eligible = "kind<>'shot_decision' AND (title<>'' OR excerpt<>'') AND (kind<>'script_source' OR excerpt<>'')"
const currentJoin = "e.project_id=v.project_id AND e.kind=v.kind AND e.id=v.id AND e.part=v.part AND e.version=v.version"

// MissingPassages enumerates a bounded batch. Only source locators survive the worker step.
func MissingPassages(tx *gorm.DB, p Policy) ([]Passage, error) {
	var out []Passage
	const cursor = "e.kind||'/'||e.id::text||'/'||lpad(e.part::text,5,'0')"
	err := tx.Raw(`SELECT e.kind,e.id,e.part,e.version,`+cursor+` AS cursor,`+passageText+` AS text FROM workspace.current_project_evidence e
 WHERE project_id=? AND `+eligible+` AND NOT EXISTS(SELECT 1 FROM workspace.evidence_embedding v WHERE `+currentJoin+` AND v.release_id=? AND v.profile=?)
 ORDER BY ((`+cursor+`) <= ?),cursor LIMIT 16`, p.ProjectID, p.ReleaseID, Profile, p.ScanCursor).Scan(&out).Error
	return out, err
}

// PublishVector rechecks the source fingerprint in the statement that writes its derived vector.
func PublishVector(tx *gorm.DB, p Policy, ref evidence.Reference, vector []float32) error {
	encoded, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO workspace.evidence_embedding(project_id,release_id,profile,kind,id,part,version,embedding)
 SELECT project_id,?,?,kind,id,part,version,?::public.vector FROM workspace.current_project_evidence
 WHERE project_id=? AND kind=? AND id=? AND part=? AND version=? AND `+eligible+` ON CONFLICT DO NOTHING`,
		p.ReleaseID, Profile, string(encoded), p.ProjectID, ref.Kind, ref.ID, ref.Part, ref.Version).Error
}

// PruneVectors removes dropped sources and generations outside an active cutover.
func PruneVectors(tx *gorm.DB, p Policy) error {
	return tx.Exec(`DELETE FROM workspace.evidence_embedding v WHERE project_id=? AND
 (((release_id,profile)<>(?,?) AND (release_id,profile) IS DISTINCT FROM (?,?)) OR NOT EXISTS(SELECT 1 FROM workspace.current_project_evidence e WHERE `+currentJoin+`))`,
		p.ProjectID, p.ReleaseID, Profile, p.ActiveReleaseID, p.ActiveProfile).Error
}

// HybridSearch returns current sources only. Exact names and ambiguity take precedence.
func HybridSearch(tx *gorm.DB, p Policy, q evidence.Query, vector []float32) (evidence.Result, error) {
	q, err := q.Normalize()
	if err != nil {
		return evidence.Result{}, err
	}
	lexical, err := search(tx, p.ProjectID, q, 20)
	original := lexical
	original.Items = original.Items[:min(q.Limit, len(original.Items))]
	if err != nil || lexical.Exact || !p.QueryReady() || len(vector) == 0 {
		return original, err
	}
	if _, err := uuid.Parse(q.Text); err == nil || q.ShotID != nil {
		return original, nil
	}
	data, err := json.Marshal(vector)
	if err != nil {
		return original, err
	}
	base := tx.Table("workspace.current_project_evidence AS e").Joins("JOIN workspace.evidence_embedding AS v ON "+currentJoin).
		Where("e.project_id=? AND v.release_id=? AND v.profile=? AND e.kind<>?", p.ProjectID, *p.ActiveReleaseID, Profile, "shot_decision")
	if len(q.Kinds) > 0 {
		base = base.Where("e.kind IN ?", q.Kinds)
	}
	var rows []row
	// The cosine floor is an initial abstention threshold, calibrated by the project eval.
	err = base.Select("e.*,v.embedding OPERATOR(public.<=>) ?::public.vector AS distance", string(data)).
		Where("v.embedding OPERATOR(public.<=>) ?::public.vector <= 0.60", string(data)).
		Order("distance,e.kind,e.id,e.part").Limit(20).Scan(&rows).Error
	if err == nil {
		original.RetrievalMode = "hybrid"
		lexical.RetrievalMode = "hybrid"
	}
	if err != nil || len(rows) == 0 {
		return original, err
	}
	scores := map[evidence.Reference]float64{}
	items := map[evidence.Reference]evidence.Item{}
	add := func(i int, item evidence.Item) {
		scores[item.Reference] += 1 / float64(61+i)
		items[item.Reference] = item
	}
	for i, item := range lexical.Items {
		add(i, item)
	}
	for i, r := range rows {
		add(i, r.item())
	}
	fused := make([]evidence.Item, 0, len(items))
	for _, item := range items {
		fused = append(fused, item)
	}
	sort.Slice(fused, func(i, j int) bool {
		a, b := fused[i], fused[j]
		if scores[a.Reference] != scores[b.Reference] {
			return scores[a.Reference] > scores[b.Reference]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.ID != b.ID {
			return a.ID.String() < b.ID.String()
		}
		return a.Part < b.Part
	})
	lexical.Status = "ok"
	lexical.Items = fused[:min(q.Limit, len(fused))]
	return lexical, nil
}
