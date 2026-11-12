// Package postgres persists business authority independently of Pi Storage.
package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Store persists Go-owned Agent authority, grants and permanent business receipts.
type Store struct {
	db        *gorm.DB
	access    func(*gorm.DB) app.ControlAccess
	now       func() time.Time
	validator func(*gorm.DB) app.DecisionValidator
}

// NewStore injects the database and current access owner for Agent business facts.
func NewStore(db *gorm.DB, access func(*gorm.DB) app.ControlAccess, now func() time.Time) *Store {
	return &Store{db: db, access: access, now: now}
}

// WithProposalValidator injects current business checks for proposed changes.
func (s *Store) WithProposalValidator(v func(*gorm.DB) app.DecisionValidator) *Store {
	s.validator = v
	return s
}
func (s *Store) request(tx *gorm.DB, actor identity.Principal, pid uuid.UUID, write bool) error {
	if s.access == nil || actor.ID == uuid.Nil || pid == uuid.Nil {
		return identity.ErrForbidden
	}
	return s.access(tx).AuthorizeRequest(tx.Statement.Context, actor, pid, write)
}
func (s *Store) worker(tx *gorm.DB, a domain.Authority) error {
	if s.access == nil {
		return identity.ErrForbidden
	}
	return s.access(tx).AuthorizeWorker(tx.Statement.Context, a.Scope.Principal(), a.Scope.ProjectID)
}

// AuthorizeProject checks current request access to the project.
func (s *Store) AuthorizeProject(ctx context.Context, actor identity.Principal, pid uuid.UUID, write bool) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.request(tx, actor, pid, write) })
}

type authorityRow struct {
	RootRunID, SessionID, SubmissionID, RequestID                   uuid.UUID
	ContentHash                                                     string
	SkillPins                                                       []byte
	ModelSelection                                                  []byte
	Scope                                                           []byte
	BudgetMicros, ReservedMicros, AuthorizationRevision, OwnerEpoch int64
	Barrier, Terminal                                               bool
	CurrentDecisionID                                               *uuid.UUID
}

func load(tx *gorm.DB, rid uuid.UUID) (domain.Authority, error) {
	var r authorityRow
	q := tx.Raw(`SELECT * FROM agent_business.run_authorization WHERE root_run_id=? FOR UPDATE`, rid).Scan(&r)
	if q.Error != nil {
		return domain.Authority{}, q.Error
	}
	if q.RowsAffected != 1 {
		return domain.Authority{}, domain.ErrNotFound
	}
	a := domain.Authority{Admission: domain.Admission{RootRunID: r.RootRunID, SessionID: r.SessionID, SubmissionID: r.SubmissionID, RequestID: r.RequestID, ContentHash: r.ContentHash, BudgetMicros: r.BudgetMicros}, ReservedMicros: r.ReservedMicros, AuthorizationRevision: r.AuthorizationRevision, OwnerEpoch: r.OwnerEpoch, Barrier: r.Barrier, Terminal: r.Terminal}
	if json.Unmarshal(r.Scope, &a.Scope) != nil {
		return a, domain.ErrInvalid
	}
	a.ModelSelection = append(json.RawMessage(nil), r.ModelSelection...)
	if len(r.SkillPins) > 0 && json.Unmarshal(r.SkillPins, &a.SkillPins) != nil {
		return a, domain.ErrInvalid
	}
	if r.CurrentDecisionID != nil {
		var row struct{ Body []byte }
		q = tx.Raw(`SELECT body FROM agent_business.control_decision WHERE decision_id=?`, *r.CurrentDecisionID).Scan(&row)
		if q.Error != nil {
			return a, q.Error
		}
		if q.RowsAffected != 1 {
			return a, domain.ErrInvalid
		}
		var d domain.Decision
		if json.Unmarshal(row.Body, &d) != nil {
			return a, domain.ErrInvalid
		}
		a.Current = &d
	}
	return a, nil
}

// AdmitRun records accepted creative intent and its business authorization.
func (s *Store) AdmitRun(ctx context.Context, in domain.Admission) error {
	if len(in.ModelSelection) == 0 {
		in.ModelSelection = json.RawMessage(`{}`)
	}
	if len(in.ModelSelection) > 65536 || !json.Valid(in.ModelSelection) {
		return domain.ErrInvalid
	}
	if len(in.SkillPins) > 64 {
		return domain.ErrInvalid
	}
	if !in.Scope.Valid() || in.RootRunID == uuid.Nil || in.SessionID == uuid.Nil || in.SubmissionID == uuid.Nil || in.RequestID == uuid.Nil || len(in.ContentHash) != 64 || in.BudgetMicros < 0 {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.worker(tx, domain.Authority{Admission: in}); err != nil {
			return err
		}
		// Serialize new admissions for this private session. Replays of the
		// original admission must remain readable while its call is unresolved.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text,0))`, in.SessionID.String()).Error; err != nil {
			return err
		}
		var existing int64
		if err := tx.Raw(`SELECT count(*) FROM agent_business.run_authorization WHERE root_run_id=?`, in.RootRunID).Scan(&existing).Error; err != nil {
			return err
		}
		if existing == 0 {
			var unresolved int64
			if err := tx.Raw(`SELECT count(*) FROM agent_business.call_receipt c JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id WHERE a.session_id=? AND c.status IN ('in_flight','unknown')`, in.SessionID).Scan(&unresolved).Error; err != nil {
				return err
			}
			if unresolved > 0 {
				return domain.ErrBlocked
			}
		}
		for _, pin := range in.SkillPins {
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM agent_business.skill_version WHERE skill_id=? AND version=? AND resource_hash=? AND owner_actor_id=? AND org_id=? AND scope_id=? AND enabled`, pin.SkillID, pin.Version, pin.ResourceHash, in.Scope.ActorID, in.Scope.OrgID, in.Scope.ScopeID).Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return domain.ErrNotFound
			}
		}
		pins, err := json.Marshal(in.SkillPins)
		if err != nil {
			return err
		}
		if string(pins) == "null" {
			pins = []byte(`[]`)
		}
		raw, err := json.Marshal(in.Scope)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,skill_pins,model_selection) VALUES(?,?,?,?,?,?::jsonb,?,?::jsonb,?::jsonb) ON CONFLICT DO NOTHING`, in.RootRunID, in.SessionID, in.SubmissionID, in.RequestID, in.ContentHash, string(raw), in.BudgetMicros, string(pins), string(in.ModelSelection)).Error; err != nil {
			return err
		}
		a, err := load(tx, in.RootRunID)
		if err != nil {
			return err
		}
		if a.ContentHash != in.ContentHash || a.SessionID != in.SessionID || a.RequestID != in.RequestID || domain.ContentHash(a.Scope) != domain.ContentHash(in.Scope) || a.BudgetMicros != in.BudgetMicros || domain.Hash(a.ModelSelection) != domain.Hash(in.ModelSelection) || domain.ContentHash(a.SkillPins) != domain.ContentHash(in.SkillPins) && (len(a.SkillPins) > 0 || len(in.SkillPins) > 0) {
			return domain.ErrConflict
		}
		return nil
	})
}

// ReadAuthority returns run authorization under the caller's current project access.
func (s *Store) ReadAuthority(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID) (domain.Authority, error) {
	var a domain.Authority
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		a, err = load(tx, rid)
		if err != nil {
			return err
		}
		if a.Scope.ActorID != actor.ID || a.Scope.ProjectID != pid || a.Scope.ScopeID != actor.ScopeID || a.Scope.OrgID != actor.OrgID {
			return domain.ErrNotFound
		}
		return s.request(tx, actor, pid, false)
	})
	return a, err
}

// CurrentAuthority returns the durable business authorization for an admitted run.
func (s *Store) CurrentAuthority(ctx context.Context, rid uuid.UUID) (domain.Authority, error) {
	var a domain.Authority
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		a, err = load(tx, rid)
		if err != nil {
			return err
		}
		return s.worker(tx, a)
	})
	return a, err
}
func grantExpiry(now, lease time.Time) (time.Time, error) {
	if !lease.After(now) {
		return time.Time{}, domain.ErrGrant
	}
	expiry := now.Add(30 * time.Second)
	if lease.Before(expiry) {
		expiry = lease
	}
	return expiry, nil
}
func spendableMicros(a domain.Authority) int64 {
	return max(int64(0), a.BudgetMicros-a.ReservedMicros)
}

func issue(tx *gorm.DB, a domain.Authority, segment uuid.UUID, epoch int64, expiry time.Time) (domain.Grant, error) {
	var r struct {
		GrantID   uuid.UUID
		ExpiresAt time.Time
	}
	id := uuid.New()
	q := tx.Raw(`INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(root_run_id,submission_id,owner_epoch,authorization_revision) DO UPDATE SET expires_at=EXCLUDED.expires_at RETURNING grant_id,expires_at`, id, a.RootRunID, segment, epoch, a.AuthorizationRevision, expiry).Scan(&r)
	return domain.Grant{AvailableMicros: spendableMicros(a), GrantID: r.GrantID, RootRunID: a.RootRunID, SubmissionID: segment, OwnerEpoch: epoch, AuthorizationRevision: a.AuthorizationRevision, ExpiresAt: r.ExpiresAt, Scope: a.Scope}, q.Error
}

// RegisterGrant issues a capability for the current runtime owner and authorization revision.
func (s *Store) RegisterGrant(ctx context.Context, in domain.GrantRequest) (domain.Grant, error) {
	if in.RootRunID == uuid.Nil || in.SubmissionID == uuid.Nil || in.OwnerEpoch < 1 || in.OwnerEpoch > 9007199254740991 {
		return domain.Grant{}, domain.ErrInvalid
	}
	expiry, err := grantExpiry(s.now(), in.LeaseUntil)
	if err != nil {
		return domain.Grant{}, err
	}
	var g domain.Grant
	blocked := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, in.RootRunID)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		if a.Terminal {
			return domain.ErrTerminal
		}
		if in.OwnerEpoch < a.OwnerEpoch || in.AuthorizationRevision != a.AuthorizationRevision {
			return domain.ErrGrant
		}
		if in.OwnerEpoch > a.OwnerEpoch {
			if err = tx.Exec(`UPDATE agent_business.run_authorization SET owner_epoch=? WHERE root_run_id=?`, in.OwnerEpoch, a.RootRunID).Error; err != nil {
				return err
			}
			a.OwnerEpoch = in.OwnerEpoch
		}
		if a.Barrier {
			blocked = true
			return nil
		}
		if in.SubmissionID != a.SubmissionID {
			return domain.ErrGrant
		}
		g, err = issue(tx, a, in.SubmissionID, in.OwnerEpoch, expiry)
		return err
	})
	if err == nil && blocked {
		err = domain.ErrBlocked
	}
	return g, err
}
func (s *Store) validate(tx *gorm.DB, a domain.Authority, in domain.Grant) (domain.Grant, error) {
	if a.Terminal {
		return domain.Grant{}, domain.ErrTerminal
	}
	if a.Barrier {
		return domain.Grant{}, domain.ErrBlocked
	}
	if in.GrantID == uuid.Nil || in.RootRunID != a.RootRunID || in.SubmissionID != a.SubmissionID || in.OwnerEpoch != a.OwnerEpoch || in.AuthorizationRevision != a.AuthorizationRevision {
		return domain.Grant{}, domain.ErrGrant
	}
	var row struct{ ExpiresAt time.Time }
	q := tx.Raw(`SELECT expires_at FROM agent_business.execution_grant WHERE grant_id=? AND root_run_id=? AND submission_id=? AND owner_epoch=? AND authorization_revision=?`, in.GrantID, in.RootRunID, in.SubmissionID, in.OwnerEpoch, in.AuthorizationRevision).Scan(&row)
	if q.Error != nil {
		return domain.Grant{}, q.Error
	}
	if q.RowsAffected != 1 || !row.ExpiresAt.After(s.now()) {
		return domain.Grant{}, domain.ErrGrant
	}
	if err := s.worker(tx, a); err != nil {
		return domain.Grant{}, err
	}
	if err := validatePins(tx, a.Scope, a.SkillPins); err != nil {
		return domain.Grant{}, err
	}
	in.AvailableMicros = spendableMicros(a)
	in.ExpiresAt = row.ExpiresAt
	in.Scope = a.Scope
	return in, nil
}

// ValidateGrant checks the capability against current durable business authority.
func (s *Store) ValidateGrant(ctx context.Context, in domain.Grant) (domain.Grant, error) {
	var g domain.Grant
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, in.RootRunID)
		if err != nil {
			return err
		}
		g, err = s.validate(tx, a, in)
		return err
	})
	return g, err
}

// Decide records an authorized control decision with its permanent request identity.
func (s *Store) Decide(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID, in domain.DecisionInput) (domain.Decision, error) {
	if in.RequestID == uuid.Nil || in.ExpectedAuthorizationRevision < 1 {
		return domain.Decision{}, domain.ErrInvalid
	}
	switch in.Action {
	case "approve", "reject", "pause", "resume", "stop":
	default:
		return domain.Decision{}, domain.ErrInvalid
	}
	var d domain.Decision
	hash := domain.ContentHash(in)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		if a.Scope.ActorID != actor.ID || a.Scope.ProjectID != pid || a.Scope.ScopeID != actor.ScopeID || a.Scope.OrgID != actor.OrgID {
			return domain.ErrNotFound
		}
		if err = s.request(tx, actor, pid, in.Action == "approve" || in.Action == "resume"); err != nil {
			return err
		}
		var old struct {
			InputHash string
			Body      []byte
		}
		q := tx.Raw(`SELECT input_hash,body FROM agent_business.control_decision WHERE root_run_id=? AND request_id=?`, rid, in.RequestID).Scan(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if old.InputHash != hash {
				return domain.ErrConflict
			}
			return json.Unmarshal(old.Body, &d)
		}
		if a.Terminal {
			return domain.ErrTerminal
		}
		if a.AuthorizationRevision != in.ExpectedAuthorizationRevision {
			return domain.ErrConflict
		}
		if in.Action == "approve" {
			if err = s.approve(tx, a, in); err != nil {
				return err
			}
		}
		if in.Action == "resume" && (!a.Barrier || a.Current == nil || a.Current.Action != "pause") {
			return domain.ErrBlocked
		}
		d = domain.Decision{DecisionID: uuid.New(), RootRunID: rid, SessionID: a.SessionID, AuthorizationRevision: a.AuthorizationRevision + 1, Action: in.Action, ContentHash: hash, Scope: a.Scope}
		if in.Action == "approve" {
			proposalID := in.ProposalID
			d.ProposalID = &proposalID
			d.CandidateHash = in.CandidateHash
		}
		body, err := json.Marshal(d)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO agent_business.control_decision(decision_id,root_run_id,request_id,input_hash,authorization_revision,action,body) VALUES(?,?,?,?,?,?,?::jsonb)`, d.DecisionID, rid, in.RequestID, hash, d.AuthorizationRevision, in.Action, string(body)).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE agent_business.run_authorization SET authorization_revision=?,barrier=true,terminal=?,current_decision_id=? WHERE root_run_id=?`, d.AuthorizationRevision, in.Action == "stop", d.DecisionID, rid).Error
	})
	return d, err
}
func (s *Store) approve(tx *gorm.DB, a domain.Authority, in domain.DecisionInput) error {
	if in.ProposalID == uuid.Nil || in.CandidateHash == "" || s.validator == nil {
		return domain.ErrUnavailable
	}
	var row struct {
		Body          []byte
		CandidateHash string
	}
	q := tx.Raw(`SELECT body,candidate_hash FROM agent_business.proposal WHERE proposal_id=? AND root_run_id=? AND proposal_id=(SELECT current_proposal_id FROM agent_business.run_authorization WHERE root_run_id=?)`, in.ProposalID, a.RootRunID, a.RootRunID).Scan(&row)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 || row.CandidateHash != in.CandidateHash {
		return domain.ErrConflict
	}
	var p domain.ProposalInput
	if json.Unmarshal(row.Body, &p) != nil {
		return domain.ErrInvalid
	}
	return s.validator(tx).ValidateProposal(tx.Statement.Context, a.Scope, p)
}

// Quiesce records stopped execution before admitting an authorized continuation.
func (s *Store) Quiesce(ctx context.Context, rid uuid.UUID, in domain.QuiescedReceipt) (domain.Grant, error) {
	if in.ReceiptID == uuid.Nil || in.DecisionID == uuid.Nil || in.OwnerEpoch < 1 || in.OwnerEpoch > 9007199254740991 || in.ContinuationID != domain.ContinuationID(rid, in.DecisionID) {
		return domain.Grant{}, domain.ErrInvalid
	}
	expiry, err := grantExpiry(s.now(), in.LeaseUntil)
	if err != nil {
		return domain.Grant{}, err
	}
	var g domain.Grant
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		if a.Terminal {
			return domain.ErrTerminal
		}
		if a.Current == nil || a.Current.DecisionID != in.DecisionID || a.AuthorizationRevision != in.AuthorizationRevision || a.OwnerEpoch != in.OwnerEpoch || (a.Current.Action != "approve" && a.Current.Action != "resume") {
			return domain.ErrConflict
		}
		var old struct {
			ReceiptID, ContinuationID, GrantID uuid.UUID
			OwnerEpoch, AuthorizationRevision  int64
		}
		q := tx.Raw(`SELECT receipt_id,continuation_id,grant_id,owner_epoch,authorization_revision FROM agent_business.quiesced_receipt WHERE decision_id=?`, in.DecisionID).Scan(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if old.ReceiptID != in.ReceiptID || old.ContinuationID != in.ContinuationID || old.AuthorizationRevision != in.AuthorizationRevision || old.OwnerEpoch > in.OwnerEpoch {
				return domain.ErrConflict
			}
			if a.Barrier || a.SubmissionID != in.ContinuationID {
				return domain.ErrConflict
			}
			// The original durable proof already opened this one continuation. A new
			// registered owner recovers that grant after a lost response; it cannot
			// invent another proof, segment or approval. Earlier owners stay fenced.
			g, err = issue(tx, a, in.ContinuationID, in.OwnerEpoch, expiry)
			if err != nil {
				return err
			}
			if old.OwnerEpoch < in.OwnerEpoch {
				updated := tx.Exec(`UPDATE agent_business.quiesced_receipt SET owner_epoch=?,grant_id=? WHERE decision_id=? AND receipt_id=? AND continuation_id=? AND owner_epoch=? AND authorization_revision=?`, in.OwnerEpoch, g.GrantID, in.DecisionID, in.ReceiptID, in.ContinuationID, old.OwnerEpoch, in.AuthorizationRevision)
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return domain.ErrConflict
				}
			}
			return nil
		}
		var active int64
		if err = tx.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=? AND status='in_flight'`, rid).Scan(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return domain.ErrBlocked
		}
		if a.ReservedMicros > a.BudgetMicros {
			return domain.ErrBudget
		}
		g, err = issue(tx, a, in.ContinuationID, in.OwnerEpoch, expiry)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO agent_business.quiesced_receipt(decision_id,receipt_id,root_run_id,continuation_id,owner_epoch,authorization_revision,grant_id) VALUES(?,?,?,?,?,?,?)`, in.DecisionID, in.ReceiptID, rid, in.ContinuationID, in.OwnerEpoch, in.AuthorizationRevision, g.GrantID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE agent_business.run_authorization SET submission_id=?,barrier=false WHERE root_run_id=?`, in.ContinuationID, rid).Error
	})
	return g, err
}

// Acknowledge records the runtime's application of a durable control decision.
func (s *Store) Acknowledge(ctx context.Context, rid uuid.UUID, in domain.AppliedReceipt) error {
	if in.ReceiptID == uuid.Nil || in.DecisionID == uuid.Nil || in.ExecutionRevision < 0 || (in.Status != "applied" && in.Status != "superseded") {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		var row struct {
			AuthorizationRevision int64
			Action, DeliveryState string
			AppliedReceipt        []byte
		}
		q := tx.Raw(`SELECT authorization_revision,action,delivery_state,applied_receipt FROM agent_business.control_decision WHERE root_run_id=? AND decision_id=? FOR UPDATE`, rid, in.DecisionID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || row.AuthorizationRevision != in.AuthorizationRevision {
			return domain.ErrConflict
		}
		if len(row.AppliedReceipt) > 0 {
			var old domain.AppliedReceipt
			if json.Unmarshal(row.AppliedReceipt, &old) != nil {
				return domain.ErrInvalid
			}
			if old != in {
				return domain.ErrConflict
			}
			return nil
		}
		if in.Status == "applied" {
			if a.AuthorizationRevision != in.AuthorizationRevision {
				return domain.ErrConflict
			}
			if row.Action == "approve" || row.Action == "resume" {
				var count int64
				if err = tx.Raw(`SELECT count(*) FROM agent_business.quiesced_receipt WHERE decision_id=?`, in.DecisionID).Scan(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return domain.ErrBlocked
				}
			}
		} else if in.AuthorizationRevision >= a.AuthorizationRevision {
			return domain.ErrConflict
		}
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return tx.Exec(`UPDATE agent_business.control_decision SET delivery_state=?,applied_receipt=?::jsonb,lease_until=NULL WHERE decision_id=?`, in.Status, string(raw), in.DecisionID).Error
	})
}

// ClaimDecision claims one due control decision for delivery.
func (s *Store) ClaimDecision(ctx context.Context, now time.Time) (domain.Decision, bool, error) {
	var d domain.Decision
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			DecisionID uuid.UUID
			Body       []byte
		}
		q := tx.Raw(`SELECT decision_id,body FROM agent_business.control_decision WHERE (delivery_state='pending' AND next_attempt_at<=?) OR (delivery_state='leased' AND lease_until<=?) ORDER BY next_attempt_at,decision_id LIMIT 1 FOR UPDATE SKIP LOCKED`, now, now).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return nil
		}
		if json.Unmarshal(row.Body, &d) != nil {
			return domain.ErrInvalid
		}
		if err := tx.Exec(`UPDATE agent_business.control_decision SET delivery_state='leased',lease_until=?,attempts=attempts+1 WHERE decision_id=?`, now.Add(30*time.Second), row.DecisionID).Error; err != nil {
			return err
		}
		found = true
		return nil
	})
	return d, found, err
}

// RetryDecision schedules another delivery of the same durable decision.
func (s *Store) RetryDecision(ctx context.Context, id uuid.UUID, next time.Time) error {
	return s.db.WithContext(ctx).Exec(`UPDATE agent_business.control_decision SET delivery_state='pending',next_attempt_at=?,lease_until=NULL WHERE decision_id=? AND delivery_state='leased'`, next, id).Error
}

// Propose records a candidate under the current grant and business validator.
func (s *Store) Propose(ctx context.Context, in domain.ProposalInput) (domain.Proposal, error) {
	if in.RequestID == uuid.Nil || !json.Valid(in.Candidate) || in.CandidateHash != domain.Hash(in.Candidate) || in.BudgetMicros < 0 {
		return domain.Proposal{}, domain.ErrInvalid
	}
	hash := domain.ContentHash(struct {
		RunID, SubmissionID, RequestID uuid.UUID
		CandidateHash                  string
		CanvasRevision                 int64
		PriceVersion                   string
		BudgetMicros                   int64
		Candidate                      json.RawMessage
	}{in.Grant.RootRunID, in.Grant.SubmissionID, in.RequestID, in.CandidateHash, in.CanvasRevision, in.PriceVersion, in.BudgetMicros, in.Candidate})
	var p domain.Proposal
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, in.Grant.RootRunID)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		var old struct {
			Body                  []byte
			ProposalID            uuid.UUID
			InputHash             string
			AuthorizationRevision int64
		}
		q := tx.Raw(`SELECT * FROM agent_business.proposal WHERE root_run_id=? AND request_id=?`, a.RootRunID, in.RequestID).Scan(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if old.InputHash != hash {
				return domain.ErrConflict
			}
			p = domain.Proposal{ProposalID: old.ProposalID, RootRunID: a.RootRunID, AuthorizationRevision: old.AuthorizationRevision}
			if json.Unmarshal(old.Body, &p.ProposalInput) != nil {
				return domain.ErrInvalid
			}
			p.Control.Terminate = true
			return nil
		}
		if _, err = s.validate(tx, a, in.Grant); err != nil {
			return err
		}
		if in.BudgetMicros > spendableMicros(a) {
			return domain.ErrBudget
		}
		p = domain.Proposal{ProposalID: uuid.New(), RootRunID: a.RootRunID, AuthorizationRevision: a.AuthorizationRevision + 1, ProposalInput: in}
		p.Control.Terminate = true
		body, err := json.Marshal(in)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO agent_business.proposal(proposal_id,root_run_id,request_id,input_hash,candidate_hash,body,authorization_revision) VALUES(?,?,?,?,?,?::jsonb,?)`, p.ProposalID, a.RootRunID, in.RequestID, hash, in.CandidateHash, string(body), p.AuthorizationRevision).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE agent_business.run_authorization SET authorization_revision=?,barrier=true,current_proposal_id=? WHERE root_run_id=?`, p.AuthorizationRevision, p.ProposalID, a.RootRunID).Error
	})
	return p, err
}

var _ app.BusinessStore = (*Store)(nil)

// ApprovedProposalInTransaction proves that the current candidate was explicitly
// approved. It is consumed by a formally bound paid command, never by the model.
func (s *Store) ApprovedProposalInTransaction(tx *gorm.DB, g domain.Grant) (domain.ProposalInput, error) {
	a, err := load(tx, g.RootRunID)
	if err != nil {
		return domain.ProposalInput{}, err
	}
	if _, err = s.validate(tx, a, g); err != nil {
		return domain.ProposalInput{}, err
	}
	var row struct {
		Body       []byte
		ProposalID uuid.UUID
	}
	q := tx.Raw(`SELECT p.proposal_id,p.body FROM agent_business.proposal p JOIN agent_business.run_authorization a ON a.current_proposal_id=p.proposal_id WHERE a.root_run_id=? AND EXISTS(SELECT 1 FROM agent_business.control_decision d WHERE d.root_run_id=a.root_run_id AND d.action='approve' AND d.body->>'proposal_id'=p.proposal_id::text AND d.authorization_revision<=a.authorization_revision)`, a.RootRunID).Scan(&row)
	if q.Error != nil {
		return domain.ProposalInput{}, q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ProposalInput{}, domain.ErrBlocked
	}
	var p domain.ProposalInput
	if json.Unmarshal(row.Body, &p) != nil {
		return p, domain.ErrInvalid
	}
	return p, nil
}

// ProposeInTransaction joins a formal tool command and its approval barrier in
// the caller's transaction. Nested GORM transactions are savepoints, not RPCs.
func (s *Store) ProposeInTransaction(tx *gorm.DB, in domain.ProposalInput) (domain.Proposal, error) {
	bound := *s
	bound.db = tx
	return bound.Propose(tx.Statement.Context, in)
}
