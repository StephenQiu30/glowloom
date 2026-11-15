package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// SessionCredential contains only the original Identity credential and CSRF.
// It is never sent to Creation, Execution or a writer-control endpoint.
type SessionCredential struct{ LegacyToken, AccessToken, CSRFToken string }

// PasswordChangeInput is the explicit self-password business command.
type PasswordChangeInput struct {
	RequestID                                  uuid.UUID
	Credential                                 SessionCredential
	CredentialEpoch                            int64
	CurrentPassword, NewPassword, Confirmation string
}

// PasswordChange retains only an irreversible candidate and encrypted reply.
type PasswordChange struct {
	RequestID, AccountID, ActorSessionID, ReplacementSessionID uuid.UUID
	InputSHA256, ProofSHA256                                   string
	Candidate                                                  domain.User
	Persistent, IssueV2, Completed                             bool
	CloseControl, ReopenControl, SessionControl                writefence.Control
	ResponseKeyID                                              string
	ResponseCiphertext                                         []byte
	AcceptedAt                                                 time.Time
}

// PasswordChangeReply is minted once in the final local credential transaction.
type PasswordChangeReply struct {
	SessionID  uuid.UUID
	KeyID      string
	Ciphertext []byte
}

// PasswordChangeTx accepts one already verified self-password responsibility.
type PasswordChangeTx interface {
	PreparePasswordChange(PasswordChange) (PasswordChange, error)
}

// PasswordChangeStore keeps qualification, original reply and recovery atomic.
// Commit invokes only local Identity mutation/minting; no network call is allowed.
type PasswordChangeStore interface {
	LoadPasswordChange(context.Context, uuid.UUID, []byte, []byte) (PasswordChange, bool, error)
	LoadAcceptedPasswordChange(context.Context, uuid.UUID) (PasswordChange, error)
	CommitPasswordChange(context.Context, PasswordChange, func(AuthTx, domain.User, bool, time.Time) (PasswordChangeReply, error)) (PasswordChange, error)
	CompletePasswordChange(context.Context, PasswordChange) error
}

// PasswordChanges owns source closure and the original self-password result.
type PasswordChanges struct {
	store     PasswordChangeStore
	writers   SessionFenceCoordinator
	protector SessionResponseProtector
	auth      *Auth
}

var (
	// ErrPasswordChangeUnavailable retains an unresolved source responsibility.
	ErrPasswordChangeUnavailable = errors.New("password qualification unavailable")
	// ErrPasswordChangeExists rolls back a concurrent duplicate before mutation.
	ErrPasswordChangeExists = errors.New("password change already accepted")
)

// PasswordChangeRecoveryTask is the durable original-password wakeup kind.
const PasswordChangeRecoveryTask = "identity.password_change"

// WithPasswordChanges installs the complete source protocol, never a partial fallback.
func (a *Auth) WithPasswordChanges(store PasswordChangeStore, writers SessionFenceCoordinator, protector SessionResponseProtector) error {
	if store == nil || writers == nil || protector == nil {
		return ErrPasswordChangeUnavailable
	}
	a.passwordChanges = &PasswordChanges{store: store, writers: writers, protector: protector, auth: a}
	return nil
}

// PasswordChangesReady lets a trusted composition select the source-owned route.
func (a *Auth) PasswordChangesReady() bool { return a != nil && a.passwordChanges != nil }

// ChangePasswordCommand recovers an accepted proof before applying new policy or
// requiring the original browser session to remain live after its own revocation.
func (a *Auth) ChangePasswordCommand(ctx context.Context, input PasswordChangeInput) (AuthResult, error) {
	q := a.passwordChanges
	if q == nil || input.RequestID == uuid.Nil {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return AuthResult{}, err
	}
	digest := q.protector.Digest("password_change_input", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(digest)
	encoded, err = json.Marshal(input.Credential)
	if err != nil {
		return AuthResult{}, err
	}
	proof := q.protector.Digest("password_change_credential", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(digest) != 32 || len(proof) != 32 {
		return AuthResult{}, ErrProofInvalid
	}
	p, found, err := q.store.LoadPasswordChange(ctx, input.RequestID, digest, proof)
	if err != nil {
		return AuthResult{}, err
	}
	if found {
		return q.finish(ctx, p)
	}
	if input.NewPassword != input.Confirmation || input.CredentialEpoch < 1 {
		return AuthResult{}, domain.ErrInvalidProfile
	}
	if err := a.validateNewPassword(input.NewPassword); err != nil {
		return AuthResult{}, err
	}
	if (input.Credential.LegacyToken == "") == (input.Credential.AccessToken == "") || input.Credential.CSRFToken == "" {
		return AuthResult{}, ErrSessionCSRFInvalid
	}
	var verified AuthResult
	if input.Credential.AccessToken != "" {
		verified, err = a.CurrentAccess(ctx, input.Credential.AccessToken, input.Credential.CSRFToken, UserActivity)
	} else {
		verified, err = a.CurrentWithActivity(ctx, input.Credential.LegacyToken, UserActivity)
		if err == nil && !hmac.Equal([]byte(verified.CSRFToken), []byte(input.Credential.CSRFToken)) {
			err = ErrSessionCSRFInvalid
		}
	}
	if err != nil {
		return AuthResult{}, err
	}
	if a.sessionIssuanceEnabled() {
		if !a.sessionSecurityReady() {
			return AuthResult{}, ErrRefreshDependency
		}
		release, err := a.security.coordinator.Admit(ctx, verified.Session.ID, "password_change")
		if err != nil {
			return AuthResult{}, err
		}
		defer func() { _ = release(ctx) }()
	}
	var rejected error
	prepare := func(tx AuthTx, user domain.User, session domain.AuthSession, now time.Time) error {
		if user.SessionEpoch != input.CredentialEpoch {
			return domain.ErrRevisionConflict
		}
		valid, err := a.verifyCurrentPassword(tx, user, input.CurrentPassword, now)
		if err != nil {
			return err
		}
		if !valid {
			rejected = ErrCurrentPassword
			return nil
		}
		if user.SessionEpoch >= math.MaxInt32 || user.Revision >= math.MaxInt32 {
			return domain.ErrInvalidAccountState
		}
		hash, err := domain.HashPassword(input.NewPassword, user.PasswordHash)
		if err != nil {
			return err
		}
		user.PasswordHash = hash
		source, ok := tx.(PasswordChangeTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		p = PasswordChange{RequestID: input.RequestID, AccountID: user.ID, ActorSessionID: session.ID, InputSHA256: hex.EncodeToString(digest), ProofSHA256: hex.EncodeToString(proof), Candidate: user, Persistent: session.Persistent, IssueV2: a.sessionIssuanceEnabled(), AcceptedAt: now.UTC().Truncate(time.Microsecond)}
		p, err = source.PreparePasswordChange(p)
		return err
	}
	if input.Credential.LegacyToken != "" {
		err = a.withSession(ctx, input.Credential.LegacyToken, prepare)
	} else {
		actor := Principal{ID: verified.User.ID, SessionID: verified.Session.ID, CredentialEpoch: verified.User.SessionEpoch}
		err = a.sessionTransaction(ctx, func(tx AuthTx) error {
			u, s, now, err := a.lockedPrincipal(tx, actor)
			if err != nil {
				return err
			}
			return prepare(tx, u, s, now)
		})
	}
	if err != nil && !errors.Is(err, ErrPasswordChangeExists) {
		return AuthResult{}, err
	}
	if rejected != nil {
		return AuthResult{}, rejected
	}
	p, found, err = q.store.LoadPasswordChange(ctx, input.RequestID, digest, proof)
	if err != nil {
		return AuthResult{}, err
	}
	if !found {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	return q.finish(ctx, p)
}
func (q *PasswordChanges) finish(ctx context.Context, p PasswordChange) (AuthResult, error) {
	if p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || p.CloseControl.Validate() != nil || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.AccountID || !p.CloseControl.Blocked {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	expected := p.CloseControl.Authority
	expected.Epoch--
	state, err := q.writers.Set(ctx, expected, p.CloseControl.CoordinationID, true)
	if err != nil {
		return AuthResult{}, err
	}
	if !exactWriterCompletion(state, p.CloseControl) {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	p, err = q.store.CommitPasswordChange(ctx, p, func(tx AuthTx, current domain.User, persistent bool, accepted time.Time) (PasswordChangeReply, error) {
		if p.IssueV2 != q.auth.sessionIssuanceEnabled() {
			return PasswordChangeReply{}, ErrRefreshDependency
		}
		result, err := q.auth.replacePassword(tx, current, persistent, p.Candidate.PasswordHash, accepted)
		if err != nil {
			return PasswordChangeReply{}, err
		}
		reply := loginActivationResponse{Result: result, LegacyToken: result.Token, CSRFToken: result.CSRFToken, AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}
		reply.Result.User.PasswordHash = ""
		plain, err := json.Marshal(reply)
		if err != nil {
			return PasswordChangeReply{}, err
		}
		defer clear(plain)
		cipher, err := q.protector.Seal(p.RequestID, plain)
		if err != nil {
			return PasswordChangeReply{}, err
		}
		return PasswordChangeReply{SessionID: result.Session.ID, KeyID: q.protector.KeyID(), Ciphertext: cipher}, nil
	})
	if err != nil {
		return AuthResult{}, err
	}
	for _, control := range []writefence.Control{p.ReopenControl, p.SessionControl} {
		if control.Validate() != nil || control.Blocked {
			return AuthResult{}, ErrPasswordChangeUnavailable
		}
		expected := control.Authority
		expected.Epoch--
		state, err := q.writers.Set(ctx, expected, control.CoordinationID, false)
		if err != nil {
			return AuthResult{}, err
		}
		if !exactWriterCompletion(state, control) {
			return AuthResult{}, ErrPasswordChangeUnavailable
		}
	}
	if err := q.store.CompletePasswordChange(ctx, p); err != nil {
		return AuthResult{}, err
	}
	plain, err := q.protector.Open(p.RequestID, p.ResponseKeyID, p.ResponseCiphertext)
	if err != nil {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	defer clear(plain)
	var reply loginActivationResponse
	if json.Unmarshal(plain, &reply) != nil || reply.Result.User.ID != p.AccountID || reply.Result.Session.ID != p.ReplacementSessionID {
		return AuthResult{}, ErrPasswordChangeUnavailable
	}
	result := reply.Result
	result.Token = reply.LegacyToken
	result.CSRFToken = reply.CSRFToken
	result.AccessToken = reply.AccessToken
	result.RefreshToken = reply.RefreshToken
	return result, nil
}

// ResumePasswordChange is the trusted worker's original-request responsibility.
func (a *Auth) ResumePasswordChange(ctx context.Context, id uuid.UUID) error {
	if a == nil || a.passwordChanges == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	p, err := a.passwordChanges.store.LoadAcceptedPasswordChange(ctx, id)
	if err != nil {
		return err
	}
	if p.RequestID != id {
		return ErrPasswordChangeUnavailable
	}
	if p.Completed {
		return nil
	}
	_, err = a.passwordChanges.finish(ctx, p)
	return err
}
