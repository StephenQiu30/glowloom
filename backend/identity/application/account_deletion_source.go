package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
)

// DeletionMemberships is advisory Creation-owned membership metadata. It never
// authorizes final deletion or substitutes for retained-resource responsibility.
type DeletionMemberships struct {
	AccountID uuid.UUID
	Teams     []DeletionTeam
}

// DeletionTeam contains only administrator membership facts, never account status.
type DeletionTeam struct {
	ID                  uuid.UUID
	Revision            int64
	TargetAdministrator bool
	Administrators      []uuid.UUID
}

// Validate rejects ambiguous or unbound membership snapshots.
func (s DeletionMemberships) Validate(account uuid.UUID) bool {
	if account == uuid.Nil || s.AccountID != account {
		return false
	}
	seen := map[uuid.UUID]bool{}
	for _, team := range s.Teams {
		if team.ID == uuid.Nil || team.Revision < 1 || seen[team.ID] {
			return false
		}
		seen[team.ID] = true
		admins := map[uuid.UUID]bool{}
		for _, id := range team.Administrators {
			if id == uuid.Nil || id == account || admins[id] {
				return false
			}
			admins[id] = true
		}
	}
	return true
}

// DeletionMembershipReader is the fixed private Creation read-only audience.
type DeletionMembershipReader interface {
	ReadDeletionMemberships(context.Context, uuid.UUID) (DeletionMemberships, error)
}

// AccountDeletionSourceStore keeps local eligibility and all source writes within
// Identity. The Creation snapshot is fetched before its policy transaction.
type AccountDeletionSourceStore interface {
	ReadSelfDeletionAccount(context.Context, Principal, func() time.Time) error
	DeletionSourceTransaction(context.Context, Principal, func() time.Time, DeletionMemberships, func(AccountDeletionTx) error) error
}

// SelfDeletionSourceTx accepts only the original self-bound consumed email proof.
type SelfDeletionSourceTx interface {
	PrepareSelfDeletion(GovernanceResponsibility) error
}

type selfDeletionSource struct {
	store         AccountDeletionSourceStore
	memberships   DeletionMembershipReader
	qualification *Governance
	protector     ProofProtector
}

// SelfDeletionCommand retains the original email, session, revision and policy
// contract. Only keyed HMACs, not browser credentials or code, are persisted.
type SelfDeletionCommand struct {
	RequestID, OperationID uuid.UUID
	Revision               int64
	Code, PolicyVersion    string
	Credential             SessionCredential
}

// SelfDeletionRecoveryTask resumes only an already accepted self-deletion.
const SelfDeletionRecoveryTask = "identity.self_deletion"

// WithSource enables independent self-deletion acceptance while reusing the exact
// four-writer and Creation receipt phase machine. Platform authorization is never
// called to accept a self-deletion.
func (s *AccountDeletion) WithSource(store AccountDeletionSourceStore, memberships DeletionMembershipReader, qualification *Governance, protector ProofProtector) error {
	if s == nil || store == nil || memberships == nil || qualification == nil || qualification.source == nil || qualification.source.auth != s.auth || qualification.source.teams == nil || protector == nil {
		return ErrPasswordChangeUnavailable
	}
	s.source = &selfDeletionSource{store: store, memberships: memberships, qualification: qualification, protector: protector}
	return nil
}
func (s *AccountDeletion) preflightTransaction(ctx context.Context, actor Principal, run func(AccountDeletionTx) error) error {
	if s.source == nil {
		return s.store.DeletionTransaction(ctx, actor, s.now, run)
	}
	if err := s.source.store.ReadSelfDeletionAccount(ctx, actor, s.now); err != nil {
		return err
	}
	snapshot, err := s.source.memberships.ReadDeletionMemberships(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !snapshot.Validate(actor.ID) {
		return ErrPasswordChangeUnavailable
	}
	return s.source.store.DeletionSourceTransaction(ctx, actor, s.now, snapshot, run)
}

// ConfirmCommand first matches an accepted original proof before reading today's
// account/session state. A new request retains every original confirmation check.
func (s *AccountDeletion) ConfirmCommand(ctx context.Context, input SelfDeletionCommand) error {
	if s == nil || s.source == nil || input.RequestID == uuid.Nil || input.OperationID == uuid.Nil || input.Revision < 1 || input.PolicyVersion != SoftDeletionPolicy || len(input.Code) != 6 {
		return ErrProofInvalid
	}
	for _, r := range input.Code {
		if r < '0' || r > '9' {
			return ErrProofInvalid
		}
	}
	q := s.source.qualification
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	digest := s.source.protector.Digest("self_deletion_input", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(digest)
	encoded, err = json.Marshal(input.Credential)
	if err != nil {
		return err
	}
	proof := s.source.protector.Digest("self_deletion_credential", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(digest) != 32 || len(proof) != 32 {
		return ErrProofInvalid
	}
	saved, found, err := q.source.store.LoadGovernance(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if found {
		if saved.Action != "self_delete" || saved.ProofOperationID != input.OperationID {
			return ErrProofInvalid
		}
		return q.finishGovernance(ctx, saved)
	}
	credential := input.Credential
	if (credential.LegacyToken == "") == (credential.AccessToken == "") || credential.CSRFToken == "" {
		return ErrSessionCSRFInvalid
	}
	var current AuthResult
	if credential.AccessToken != "" {
		current, err = s.auth.CurrentAccess(ctx, credential.AccessToken, credential.CSRFToken, UserActivity)
	} else {
		current, err = s.auth.CurrentWithActivity(ctx, credential.LegacyToken, UserActivity)
		if err == nil && !hmac.Equal([]byte(current.CSRFToken), []byte(credential.CSRFToken)) {
			err = ErrSessionCSRFInvalid
		}
	}
	if err != nil {
		return err
	}
	actor := Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch, PlatformEpoch: current.User.PlatformEpoch}
	err = s.confirmPrepared(ctx, actor, input.Revision, input.OperationID, input.Code, func(tx AccountDeletionTx, user domain.User, now time.Time) error {
		source, ok := tx.(SelfDeletionSourceTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		user.PasswordHash = ""
		return source.PrepareSelfDeletion(GovernanceResponsibility{RequestID: input.RequestID, ActorSessionID: actor.SessionID, ProofOperationID: input.OperationID, Actor: user, Candidate: user, Action: "self_delete", Reason: "self account deletion", InputSHA256: hex.EncodeToString(digest), ProofSHA256: hex.EncodeToString(proof), AcceptedAt: now.UTC().Truncate(time.Microsecond)})
	})
	if err != nil {
		return err
	}
	saved, found, err = q.source.store.LoadGovernance(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if !found || saved.Action != "self_delete" {
		return ErrPasswordChangeUnavailable
	}
	return q.finishGovernance(ctx, saved)
}

// ResumeSelfDeletion never accepts a new proof or changes the original target.
func (s *AccountDeletion) ResumeSelfDeletion(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.source == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	p, err := s.source.qualification.source.store.LoadAcceptedGovernance(ctx, id)
	if err != nil {
		return err
	}
	if p.Action != "self_delete" || p.Actor.ID != p.Candidate.ID || p.ProofOperationID == uuid.Nil {
		return ErrProofInvalid
	}
	return s.source.qualification.ResumeGovernance(ctx, id)
}
