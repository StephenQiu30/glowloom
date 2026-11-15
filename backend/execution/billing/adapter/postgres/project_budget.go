package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// ProjectBudgetStore owns only Execution's initial zero-credit wallet/budget facts.
// Creation proof is resolved by the application before this local transaction.
type ProjectBudgetStore struct{ db *gorm.DB }

// NewProjectBudgetStore injects the Execution-restricted database connection.
func NewProjectBudgetStore(db *gorm.DB) *ProjectBudgetStore { return &ProjectBudgetStore{db: db} }

var _ application.ProjectBudgetRepository = (*ProjectBudgetStore)(nil)

func (s *ProjectBudgetStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrProjectBudgetOwner
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterExecution {
		return application.ErrProjectBudgetOwner
	}
	return nil
}

func readProjectBudget(tx *gorm.DB, p application.ProjectBudgetPreparation) (application.ProjectBudgetReceipt, error) {
	var row struct {
		application.ProjectBudgetPreparation `gorm:"embedded"`
		RequestSHA256                        string `gorm:"column:request_sha256"`
		Receipt                              json.RawMessage
	}
	result := tx.Raw(`SELECT execution_command_id,preparation_id,source_command_id,source_sha256,input_sha256,request_sha256,project_id,organization_id,scope_id,receipt FROM billing.project_budget_preparation WHERE execution_command_id=?`, p.ExecutionCommandID).Scan(&row)
	if result.Error != nil {
		return application.ProjectBudgetReceipt{}, result.Error
	}
	if result.RowsAffected != 1 {
		return application.ProjectBudgetReceipt{}, application.ErrProjectBudgetUnknown
	}
	if row.ProjectBudgetPreparation != p || row.RequestSHA256 != p.Digest() {
		return application.ProjectBudgetReceipt{}, application.ErrProjectBudgetPreparation
	}
	var receipt application.ProjectBudgetReceipt
	if json.Unmarshal(row.Receipt, &receipt) != nil || receipt.Validate() != nil || receipt.Request != p {
		return receipt, application.ErrProjectBudgetPreparation
	}
	return receipt, nil
}

// ResolveProjectBudget reads only the original ACK and performs no source I/O or DML.
func (s *ProjectBudgetStore) ResolveProjectBudget(ctx context.Context, p application.ProjectBudgetPreparation) (application.ProjectBudgetReceipt, error) {
	if err := p.Validate(); err != nil {
		return application.ProjectBudgetReceipt{}, err
	}
	if err := s.ready(ctx); err != nil {
		return application.ProjectBudgetReceipt{}, err
	}
	return readProjectBudget(s.db.WithContext(ctx), p)
}

// CommitProjectBudget requires the exact previously verified source facts. Wallet
// account ensure, uncapped zero-use budget and immutable ACK commit in one Tx.
// Existing budgets are never silently adopted by a different preparation.
func (s *ProjectBudgetStore) CommitProjectBudget(ctx context.Context, p application.ProjectBudgetPreparation, proof application.ProjectPreparedProof) (application.ProjectBudgetReceipt, error) {
	var receipt application.ProjectBudgetReceipt
	if p.Validate() != nil || proof.Request != p || proof.Phase != "prepared" {
		return receipt, application.ErrProjectBudgetPreparation
	}
	if err := s.ready(ctx); err != nil {
		return receipt, err
	}
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69370,hashtext(?))`, p.PreparationID.String()).Error; err != nil {
			return err
		}
		original, err := readProjectBudget(tx, p)
		if err == nil {
			receipt = original
			return nil
		}
		if !errors.Is(err, application.ErrProjectBudgetUnknown) {
			return err
		}
		// Scope ownership was proven by Creation; this inserts no credit lot or grant,
		// and preserves a pre-existing personal/team wallet without touching its funds.
		if err := tx.Exec(`INSERT INTO billing.wallet_account(scope_id) VALUES(?) ON CONFLICT(scope_id) DO NOTHING`, p.ScopeID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO billing.project_wallet(project_id,scope_id) VALUES(?,?)`, p.ProjectID, p.ScopeID).Error; err != nil {
			return fmt.Errorf("project wallet resource conflict: %w: %w", application.ErrProjectBudgetPreparation, err)
		}
		receipt = application.NewProjectBudgetReceipt(p)
		if err := tx.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,?)`, receipt.BudgetID, p.ProjectID, receipt.LimitMicros).Error; err != nil {
			return fmt.Errorf("project budget resource conflict: %w: %w", application.ErrProjectBudgetPreparation, err)
		}
		raw, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		canonical, err := writefence.CanonicalReceipt(raw)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO billing.project_budget_preparation(execution_command_id,preparation_id,source_command_id,source_sha256,input_sha256,request_sha256,project_id,organization_id,scope_id,budget_id,receipt) VALUES(?,?,?,?,?,?,?,?,?,?,?::jsonb)`, p.ExecutionCommandID, p.PreparationID, p.SourceCommandID, p.SourceSHA256, p.InputSHA256, p.Digest(), p.ProjectID, p.OrganizationID, p.ScopeID, receipt.BudgetID, string(canonical)).Error; err != nil {
			return fmt.Errorf("project preparation identity conflict: %w: %w", application.ErrProjectBudgetPreparation, err)
		}
		saved, err := readProjectBudget(tx, p)
		if err != nil {
			return err
		}
		receipt = saved
		return nil
	})
	if err != nil {
		return application.ProjectBudgetReceipt{}, err
	}
	return receipt, nil
}
