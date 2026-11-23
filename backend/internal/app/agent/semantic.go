package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// searchEvidence keeps external embedding I/O outside authorization and receipt transactions.
func (b *Boundary) searchEvidence(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	var q evidence.Query
	if err := decode(in.Input, &q); err != nil {
		return domain.CallReceipt{}, err
	}
	q, err := q.Normalize()
	if err != nil {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	var p evidencepg.Policy
	guard := func(tx *gorm.DB) error { _, err := b.Store.ValidateGrantInTransaction(tx, in.Grant); return err }
	admit := func(tx *gorm.DB, s domain.Scope, _ uuid.UUID) error {
		if err := authorizeEvidence(ctx, tx, s); err != nil {
			return err
		}
		lexical, err := lexicalEvidence(ctx, tx, s, q)
		if err != nil || lexical.Exact || q.ShotID != nil || b.gateway == nil || b.gateway.Provider == nil {
			return err
		}
		if _, err := uuid.Parse(q.Text); err == nil {
			return nil
		}
		p, err = evidencepg.ReadPolicy(tx, s.ProjectID)
		return err
	}
	execute := func(ctx context.Context, s domain.Scope, key uuid.UUID) (domain.CallResult, error) {
		var vec []float32
		if p.QueryReady() {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			vec, _ = b.gateway.EmbedEvidence(callCtx, p, app.Principal(s), *p.ActiveReleaseID, "query", q.Text, "rag/query/"+key.String(), guard)
			cancel()
		}
		var result evidence.Result
		err := b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := guard(tx); err != nil {
				return err
			}
			if err := authorizeEvidence(ctx, tx, s); err != nil {
				return err
			}
			var err error
			if len(vec) > 0 {
				live, err := evidencepg.ReadPolicy(tx, s.ProjectID)
				if err != nil {
					return err
				}
				if live.Generation == p.Generation && live.QueryReady() && *live.ActiveReleaseID == *p.ActiveReleaseID {
					result, err = evidencepg.HybridSearch(tx, live, q, vec)
					if err != nil {
						return err
					}
					version, err := evidencepg.PendingScriptIndex(tx, s.ProjectID)
					result.ScriptIndexPending = version != uuid.Nil
					return err
				}
			}
			result, err = lexicalEvidence(ctx, tx, s, q)
			return err
		})
		if err != nil {
			return domain.CallResult{}, err
		}
		raw, err := json.Marshal(result)
		return domain.CallResult{Status: domain.CallStatusCompleted, Result: raw}, err
	}
	return b.Store.ExecuteToolIntent(ctx, evidence.SearchTool, in, admit, execute)
}

// verifyEvidenceReplay also protects the recovery lookup after an Agent process restart.
func verifyEvidenceReplay(tx *gorm.DB, a domain.Authority, receipt domain.CallReceipt) error {
	if receipt.Name != evidence.SearchTool && receipt.Name != evidence.ReadTool {
		return nil
	}
	if err := authorizeEvidence(tx.Statement.Context, tx, a.Scope); err != nil {
		return err
	}
	var original evidence.Result
	if err := json.Unmarshal(receipt.Result, &original); err != nil {
		return domain.ErrInvalid
	}
	if len(original.Items) == 0 {
		return nil
	}
	s := a.Scope
	err := evidencepg.VerifyModelReceipts(tx, s.ActorID, s.OrgID, s.ScopeID, s.ProjectID, a.SessionID, []uuid.UUID{receipt.ReceiptID})
	if errors.Is(err, evidencepg.ErrStale) {
		return domain.ErrConflict
	}
	return err
}
