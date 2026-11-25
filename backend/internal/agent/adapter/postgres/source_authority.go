package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// BindSourceAuthorization is called in the same transaction as the original
// call intent. It stores authority and an immutable owner command hash only.
func BindSourceAuthorization(tx *gorm.DB, id uuid.UUID, g domain.Grant, hash string) error {
	var unknown int64
	if err := tx.Raw(`SELECT count(*) FROM agent_business.source_authorization s JOIN agent_business.call_receipt c USING(receipt_id) WHERE c.root_run_id=? AND c.receipt_id<>? AND c.status IN ('in_flight','unknown')`, g.RootRunID, id).Scan(&unknown).Error; err != nil {
		return err
	}
	if unknown > 0 {
		return domain.ErrBlocked
	}
	return tx.Exec(`INSERT INTO agent_business.source_authorization(receipt_id,grant_id,source_hash)VALUES(?,?,?)`, id, g.GrantID, hash).Error
}

type sourceBinding struct {
	Status                            string
	RootRunID, GrantID, SubmissionID  uuid.UUID
	OwnerEpoch, AuthorizationRevision int64
	SourceHash                        string
}

func readSourceBinding(tx *gorm.DB, key uuid.UUID) (sourceBinding, bool, error) {
	var b sourceBinding
	r := tx.Raw(`SELECT c.root_run_id,s.grant_id,g.submission_id,g.owner_epoch,g.authorization_revision,s.source_hash,c.status FROM agent_business.source_authorization s JOIN agent_business.call_receipt c USING(receipt_id) JOIN agent_business.execution_grant g ON g.grant_id=s.grant_id WHERE s.receipt_id=? AND c.kind='tool' AND c.name='script_source_publish'`, key).Scan(&b)
	return b, r.RowsAffected == 1, r.Error
}

// AuthorizeSourcePublication is the persistent, transaction-local consumer port.
// An ordinary human command without an Agent binding retains its own authority.
func (s *Store) AuthorizeSourcePublication(ctx context.Context, actor identity.Principal, project, key uuid.UUID, hash string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		binding, found, err := readSourceBinding(tx, key)
		if err != nil || !found {
			return err
		}
		a, err := load(tx, binding.RootRunID)
		if err != nil {
			return err
		}
		// Completion and Stop also lock the root. Refresh status after acquiring it
		// so a terminal receipt cannot be reopened from a pre-lock observation.
		binding, found, err = readSourceBinding(tx, key)
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrNotFound
		}
		if binding.Status != "in_flight" && binding.Status != "unknown" {
			return domain.ErrBlocked
		}
		if binding.SourceHash != hash {
			return domain.ErrConflict
		}
		if a.Scope.ActorID != actor.ID || a.Scope.OrgID != actor.OrgID || a.Scope.ScopeID != actor.ScopeID || a.Scope.ProjectID != project {
			return domain.ErrNotFound
		}
		_, err = s.validate(tx, a, domain.Grant{GrantID: binding.GrantID, RootRunID: binding.RootRunID, SubmissionID: binding.SubmissionID, OwnerEpoch: binding.OwnerEpoch, AuthorizationRevision: binding.AuthorizationRevision})
		return err
	})
}

// RecoverSourceCall only reads the original formal command/receipt. It neither
// rebuilds a plan nor starts/resumes object I/O or publication.
func (s *Store) RecoverSourceCall(ctx context.Context, id uuid.UUID, read func(*gorm.DB, domain.Scope, uuid.UUID, string) (domain.CallResult, error)) (domain.CallReceipt, error) {
	var out domain.CallReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		binding, found, err := readSourceBinding(tx, id)
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrNotFound
		}
		a, err := load(tx, binding.RootRunID)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		result, err := read(tx, a.Scope, id, binding.SourceHash)
		if err != nil {
			return err
		}
		out, err = CompleteCall(tx, id, result)
		return err
	})
	return out, err
}
