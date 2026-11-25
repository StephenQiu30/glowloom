package aigateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence/adapter/postgres"
	gatewaypg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// EmbedEvidence admits one paid project embedding through the original account,
// project budget, project embedding allowance, supplier route and usage receipt.
func (r *Runtime) EmbedEvidence(ctx context.Context, p evidencepg.Policy, actor identity.Principal, release uuid.UUID, purpose, text, key string, guard func(*gorm.DB) error) ([]float32, error) {
	if r == nil || r.Provider == nil || release == uuid.Nil || guard == nil {
		return nil, domain.ErrUnavailable
	}
	var row struct{ Document []byte }
	res := r.DB.WithContext(ctx).Raw("SELECT document FROM ai.available_model WHERE id=? AND capability=?", release, domain.EmbeddingCapability).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	var m domain.Model
	if res.RowsAffected != 1 || json.Unmarshal(row.Document, &m) != nil || m.ID != release {
		return nil, domain.ErrUnavailable
	}
	input, _ := json.Marshal(application.EmbeddingInput{Model: m.Name, Text: text, Purpose: purpose})
	call, err := application.Prepare(m, actor.ID, actor.ScopeID, p.ProjectID, p.BudgetID, key, domain.EmbeddingOrigin, input)
	if err != nil {
		return nil, err
	}
	limit := p.LimitCredits
	auth := func(ctx context.Context, tx *gorm.DB, a identity.Principal, scope, project uuid.UUID, admin bool) error {
		if admin || project != p.ProjectID || scope != p.ScopeID || actor.ScopeID != scope || a.ID != actor.ID {
			return domain.ErrForbidden
		}
		if err := guard(tx); err != nil {
			return err
		}
		access, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, a, project, purpose == "document")
		if err != nil {
			return err
		}
		if access.OwnerScopeID != scope {
			return domain.ErrForbidden
		}
		live, err := evidencepg.ReadPolicy(tx, project)
		if err != nil {
			return err
		}
		if live.Generation != p.Generation || live.BudgetID != p.BudgetID || (purpose == "document" && (live.ActorID != a.ID || live.ReleaseID != release)) || (purpose == "query" && (!live.QueryReady() || *live.ActiveReleaseID != release)) {
			return domain.ErrForbidden
		}
		limit = live.LimitCredits
		return nil
	}
	store := gatewaypg.New(r.DB, actor, auth).WithRunLimit(func(_ context.Context, _ *gorm.DB, id uuid.UUID) (int64, error) {
		if id != p.BudgetID {
			return 0, domain.ErrBudget
		}
		return limit, nil
	})
	record, err := application.New(store, r.Provider).Execute(ctx, call, nil)
	if err != nil {
		return nil, err
	}
	if record.State != "settled" || record.Receipt == nil {
		return nil, domain.ErrUnavailable
	}
	return domain.EmbeddingVector(record.Receipt.Body)
}

// IndexEvidence processes a bounded project batch and durably schedules the next sweep.
func (r *Runtime) IndexEvidence(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	var project uuid.UUID
	generation, err := uuid.Parse(job.BusinessID)
	if err != nil || json.Unmarshal(job.Reference, &project) != nil || project == uuid.Nil {
		return time.Time{}, domain.ErrInvalid
	}
	var p evidencepg.Policy
	var passages []evidencepg.Passage
	var archived bool
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := taskruntime.Fence(ctx, tx, job); err != nil {
			return err
		}
		var err error
		p, err = evidencepg.ReadPolicy(tx, project)
		if err != nil {
			return err
		}
		if p.Generation != generation {
			// Archived projects may be restored later. Keep their scheduled sweep, discard the index.
			res := tx.Raw(`SELECT s.enabled FROM workspace.evidence_semantic_policy s JOIN workspace.project p ON p.id=s.project_id
 WHERE s.project_id=? AND s.generation=? AND s.enabled AND (p.is_delete OR p.status<>'active') FOR SHARE OF s`, project, generation).Scan(&archived)
			if res.Error != nil || !archived {
				return res.Error
			}
			if err := tx.Exec("UPDATE workspace.evidence_semantic_policy SET active_release_id=NULL,active_profile='' WHERE project_id=? AND generation=?", project, generation).Error; err != nil {
				return err
			}
			return tx.Exec("DELETE FROM workspace.evidence_embedding WHERE project_id=?", project).Error
		}
		if _, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, policyActor(p), project, true); err != nil {
			return err
		}
		if err := evidencepg.PruneVectors(tx, p); err != nil {
			return err
		}
		pending, err := evidencepg.PendingScriptIndex(tx, project)
		if err != nil {
			return err
		}
		if pending != uuid.Nil {
			if err := evidencepg.ScheduleScriptIndex(ctx, tx, evidencepg.ScriptIndexRequest{ActorID: p.ActorID, OrgID: p.OrgID, ProjectID: project, VersionID: pending}, time.Now()); err != nil {
				return err
			}
		}
		if p.ReleaseID == uuid.Nil {
			return nil
		}
		passages, err = evidencepg.MissingPassages(tx, p)
		if err != nil || len(passages) > 0 || pending != uuid.Nil || p.QueryReady() {
			return err
		}
		return tx.Exec("UPDATE workspace.evidence_semantic_policy SET active_release_id=?,active_profile=? WHERE project_id=? AND generation=?", p.ReleaseID, evidencepg.Profile, project, generation).Error
	})
	if err == nil && archived {
		return time.Now().Add(time.Minute), nil
	}
	if err != nil || p.Generation != generation {
		return time.Time{}, err
	}
	if len(passages) == 0 {
		return time.Now().Add(time.Minute), nil
	}
	next := time.Now()
	for _, passage := range passages {
		check := func(tx *gorm.DB) error {
			if err := taskruntime.Fence(ctx, tx, job); err != nil {
				return err
			}
			res, err := evidencepg.Read(tx, project, passage.Reference)
			if err != nil {
				return err
			}
			if res.Status != "ok" {
				return evidencepg.ErrStale
			}
			return nil
		}
		// Re-enabling a project reuses the original purchase for this actor, source and release.
		key := "rag/doc/" + domain.Digest([]byte(p.BudgetID.String()+p.ActorID.String()+p.ReleaseID.String()+evidencepg.Profile+passage.Version))
		v, err := r.EmbedEvidence(ctx, p, policyActor(p), p.ReleaseID, "document", passage.Text, key, check)
		if err != nil {
			if ctx.Err() != nil {
				return time.Time{}, ctx.Err()
			}
			next = time.Now().Add(time.Minute)
			continue
		}
		err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := check(tx); err != nil {
				return err
			}
			if _, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, policyActor(p), project, true); err != nil {
				return err
			}
			live, err := evidencepg.ReadPolicy(tx, project)
			if err != nil || live.Generation != generation || live.ReleaseID != p.ReleaseID {
				return err
			}
			return evidencepg.PublishVector(tx, p, passage.Reference, v)
		})
		if err != nil {
			return time.Time{}, err
		}
	}
	// Advance past unavailable passages so they cannot starve later sources.
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := taskruntime.Fence(ctx, tx, job); err != nil {
			return err
		}
		return tx.Exec("UPDATE workspace.evidence_semantic_policy SET scan_cursor=? WHERE project_id=? AND generation=? AND enabled", passages[len(passages)-1].Cursor, project, generation).Error
	})
	return next, err
}

func policyActor(p evidencepg.Policy) identity.Principal {
	return identity.Principal{ID: p.ActorID, OrgID: p.OrgID, ScopeID: p.ScopeID}
}
