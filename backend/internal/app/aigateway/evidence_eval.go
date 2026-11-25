package aigateway

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence"
	ep "github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// EvidenceCase pins independently annotated source versions. An empty expected list denotes no answer.
type EvidenceCase struct {
	ID       string               `json:"id"`
	Query    evidence.Query       `json:"query"`
	Expected []evidence.Reference `json:"expected"`
}

// EvidenceMetrics reports macro averages over the appropriate labelled populations.
type EvidenceMetrics struct {
	RecallAt6                 *float64 `json:"recall_at_6"`
	MRR                       *float64 `json:"mrr"`
	NoAnswerFalsePositiveRate *float64 `json:"no_answer_false_positive_rate"`
	P95MS                     int64    `json:"p95_ms"`
}

type caseScore struct {
	recall, rr         float64
	expected, returned int
	latency            time.Duration
}

func scoreCase(items []evidence.Item, labels []evidence.Reference, elapsed time.Duration) caseScore {
	s := caseScore{expected: len(labels), returned: len(items), latency: elapsed}
	for rank, item := range items {
		for _, label := range labels {
			if item.Reference == label {
				s.recall += 1 / float64(len(labels))
				if s.rr == 0 {
					s.rr = 1 / float64(rank+1)
				}
				break
			}
		}
	}
	return s
}

func aggregateEvidence(scores []caseScore) EvidenceMetrics {
	var m EvidenceMetrics
	if len(scores) == 0 {
		return m
	}
	var recall, rr, fp float64
	var positive, negative int
	dur := make([]time.Duration, 0, len(scores))
	for _, s := range scores {
		dur = append(dur, s.latency)
		if s.expected > 0 {
			positive++
			recall += s.recall
			rr += s.rr
		} else {
			negative++
			if s.returned > 0 {
				fp++
			}
		}
	}
	if positive > 0 {
		recall /= float64(positive)
		rr /= float64(positive)
		m.RecallAt6 = &recall
		m.MRR = &rr
	}
	if negative > 0 {
		fp /= float64(negative)
		m.NoAnswerFalsePositiveRate = &fp
	}
	sort.Slice(dur, func(i, j int) bool { return dur[i] < dur[j] })
	m.P95MS = dur[(95*len(dur)+99)/100-1].Milliseconds()
	return m
}

// EvidenceTrial reports retrieval quality, observed latency and settled query credits.
type EvidenceTrial struct {
	Trial   uuid.UUID       `json:"trial"`
	Release uuid.UUID       `json:"release"`
	Corpus  string          `json:"corpus_sha256"`
	Profile string          `json:"profile"`
	Cases   int             `json:"completed_cases"`
	Charged int64           `json:"query_credits"`
	Lexical EvidenceMetrics `json:"lexical"`
	Hybrid  EvidenceMetrics `json:"hybrid"`
}

func validateLabels(cases []EvidenceCase) error {
	if len(cases) == 0 || len(cases) > 200 {
		return domain.ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range cases {
		q, err := c.Query.Normalize()
		if c.ID == "" || len(c.ID) > 80 || seen[c.ID] || err != nil || q.ShotID != nil || q.Limit != 6 || c.Expected == nil || len(c.Expected) > 32 {
			return domain.ErrInvalid
		}
		seen[c.ID] = true
		refs := map[evidence.Reference]bool{}
		for _, ref := range c.Expected {
			normalized, err := ref.Normalize()
			if err != nil || normalized != ref || ref.Version == "" || refs[ref] {
				return domain.ErrInvalid
			}
			refs[ref] = true
		}
	}
	return nil
}

// EvaluateEvidence runs paired lexical and hybrid queries for an enabled, authorized project.
// A repeated trial recovers the original purchase; use a fresh trial to measure live latency.
func (r *Runtime) EvaluateEvidence(ctx context.Context, project, actor, trial uuid.UUID, cases []EvidenceCase) (EvidenceTrial, error) {
	out := EvidenceTrial{Trial: trial, Profile: ep.Profile}
	if trial == uuid.Nil || project == uuid.Nil || actor == uuid.Nil {
		return out, domain.ErrInvalid
	}
	if err := validateLabels(cases); err != nil {
		return out, err
	}
	var p ep.Policy
	session := r.DB.WithContext(ctx)
	check := func(tx *gorm.DB) error {
		live, err := ep.ReadPolicy(tx, project)
		if err != nil {
			return err
		}
		if live.ActorID != actor || !live.QueryReady() {
			return domain.ErrForbidden
		}
		if p.Generation != uuid.Nil && (live.Generation != p.Generation || live.ReleaseID != p.ReleaseID) {
			return domain.ErrConflict
		}
		if _, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, policyActor(live), project, true); err != nil {
			return err
		}
		p = live
		return nil
	}
	corpus := func(tx *gorm.DB) (string, error) {
		var hash string
		err := tx.Raw(`SELECT encode(sha256(convert_to(coalesce(string_agg(version,'' ORDER BY kind,id,part),''),'UTF8')),'hex') FROM workspace.current_project_evidence WHERE project_id=?`, project).Row().Scan(&hash)
		return hash, err
	}
	err := session.Transaction(func(tx *gorm.DB) error {
		if err := check(tx); err != nil {
			return err
		}
		out.Release = p.ReleaseID
		var err error
		out.Corpus, err = corpus(tx)
		if err != nil {
			return err
		}
		for _, c := range cases {
			for _, ref := range c.Expected {
				result, err := ep.Read(tx, project, ref)
				if err != nil {
					return err
				}
				if result.Status != "ok" {
					return fmt.Errorf("case %s: expected source is unavailable", c.ID)
				}
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	var lexicalScores, hybridScores []caseScore
	for _, c := range cases {
		q, _ := c.Query.Normalize()
		var lexical, hybrid evidence.Result
		start := time.Now()
		err = session.Transaction(func(tx *gorm.DB) error {
			if err := check(tx); err != nil {
				return err
			}
			var err error
			lexical, err = ep.Search(tx, project, q)
			return err
		})
		if err != nil {
			return out, err
		}
		lexTime := time.Since(start)
		hybrid = lexical
		_, parseErr := uuid.Parse(q.Text)
		if !lexical.Exact && parseErr != nil {
			key := "rag/eval/" + trial.String() + "/" + domain.Digest([]byte(c.ID))
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			v, err := r.EmbedEvidence(callCtx, p, policyActor(p), p.ReleaseID, "query", q.Text, key, check)
			cancel()
			if err != nil {
				return out, fmt.Errorf("case %s: query embedding unverified: %w", c.ID, err)
			}
			err = session.Transaction(func(tx *gorm.DB) error {
				if err := check(tx); err != nil {
					return err
				}
				var searchErr error
				hybrid, searchErr = ep.HybridSearch(tx, p, q, v)
				return searchErr
			})
			if err != nil {
				return out, err
			}
			duration := time.Since(start)
			var charged int64
			err = session.Raw(`SELECT h.charged FROM ai.request r JOIN ai.hold h ON h.request_id=r.id WHERE r.scope_id=? AND r.project_id=? AND r.run_id=? AND r.idempotency_key=? AND r.state='settled' AND h.closed`, p.ScopeID, project, p.BudgetID, key).Row().Scan(&charged)
			if err != nil {
				return out, err
			}
			out.Charged += charged
			hybridScores = append(hybridScores, scoreCase(hybrid.Items, c.Expected, duration))
		} else {
			hybridScores = append(hybridScores, scoreCase(hybrid.Items, c.Expected, lexTime))
		}
		lexicalScores = append(lexicalScores, scoreCase(lexical.Items, c.Expected, lexTime))
		out.Cases++
	}
	err = session.Transaction(func(tx *gorm.DB) error {
		if err := check(tx); err != nil {
			return err
		}
		current, err := corpus(tx)
		if err != nil {
			return err
		}
		if current != out.Corpus {
			return errors.New("evaluation corpus changed during trial")
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out.Lexical = aggregateEvidence(lexicalScores)
	out.Hybrid = aggregateEvidence(hybridScores)
	return out, nil
}
