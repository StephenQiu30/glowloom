package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ErrSessionActivation keeps credentials private until their initial writer heads
// are durably ready. It is not a new public session status.
var ErrSessionActivation = errors.New("session activation unavailable")

// ErrSessionActivationExists rolls back a concurrent duplicate login transaction
// before it can publish a second session. The caller then loads the original reply.
var ErrSessionActivationExists = errors.New("original session activation already accepted")

// SessionActivation holds one accepted login and its encrypted original response.
// Recovery never creates a second session or signs a replacement token.
type SessionActivation struct {
	RequestID, AccountID, SessionID uuid.UUID
	InputSHA256                     string
	Control                         writefence.Control
	ResponseKeyID                   string
	ResponseCiphertext              []byte
	AcceptedAt                      time.Time
	Completed                       bool
}

// SessionActivationTx runs only within the original successful password policy
// transaction, after the new exact session was inserted and before it is visible.
type SessionActivationTx interface {
	PrepareSessionActivation(SessionActivation) error
}

// SessionActivationStore owns original-proof lookup and the final live policy
// recheck. Completion must verify actual source heads and four persisted ACKs,
// append the success event once, and retain the immutable response after cleanup.
type SessionActivationStore interface {
	LoadSessionActivation(context.Context, uuid.UUID, []byte) (SessionActivation, bool, error)
	LoadAcceptedSessionActivation(context.Context, uuid.UUID) (SessionActivation, error)
	CompleteSessionActivation(context.Context, SessionActivation) error
}

// SessionResponseProtector keeps the response encryption inside Identity.
type SessionResponseProtector interface {
	ProofProtector
	Open(uuid.UUID, string, []byte) ([]byte, error)
}

// SessionActivations completes only the initial source control of an accepted login.
type SessionActivations struct {
	store     SessionActivationStore
	writers   SessionFenceCoordinator
	protector SessionResponseProtector
}

// WithSessionActivation is mandatory in an independent Identity composition root.
// Native login then requires the original HTTP operation key, including retries.
func (a *Auth) WithSessionActivation(store SessionActivationStore, writers SessionFenceCoordinator, protector SessionResponseProtector) error {
	if store == nil || writers == nil || protector == nil {
		return ErrSessionActivation
	}
	a.activation = &SessionActivations{store: store, writers: writers, protector: protector}
	return nil
}

// SessionActivationRecoveryTask identifies accepted original login responsibilities.
const SessionActivationRecoveryTask = "identity.session_activation"

type loginActivationResponse struct {
	Result                                            AuthResult
	LegacyToken, CSRFToken, AccessToken, RefreshToken string
}

// LoginFromCommand retains the existing public login result and policy while
// preparing its exact session on all writers before returning any credential.
func (a *Auth) LoginFromCommand(ctx context.Context, id uuid.UUID, email, password string, persistent bool, source string) (AuthResult, error) {
	if a.activation == nil {
		return a.LoginFrom(ctx, email, password, persistent, source)
	}
	if id == uuid.Nil {
		return AuthResult{}, ErrSessionActivation
	}
	encoded, err := json.Marshal(struct {
		Email, Password string
		Persistent      bool
	}{email, password, persistent})
	if err != nil {
		return AuthResult{}, err
	}
	proof := a.activation.protector.Digest("session_activation", id, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(proof) != 32 {
		return AuthResult{}, ErrSessionActivation
	}
	saved, found, err := a.activation.store.LoadSessionActivation(ctx, id, proof)
	if err != nil {
		return AuthResult{}, err
	}
	if found {
		return a.activation.finish(ctx, saved)
	}
	_, err = a.loginEmailPrepared(ctx, email, password, persistent, source, func(tx AuthTx, result AuthResult) error {
		preparation, ok := tx.(SessionActivationTx)
		if !ok {
			return ErrSessionActivation
		}
		response := loginActivationResponse{Result: result, LegacyToken: result.Token, CSRFToken: result.CSRFToken, AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}
		response.Result.User.PasswordHash = ""
		plain, err := json.Marshal(response)
		if err != nil {
			return err
		}
		defer clear(plain)
		if len(plain) > 60000 {
			return ErrSessionActivation
		}
		cipher, err := a.activation.protector.Seal(id, plain)
		if err != nil {
			return err
		}
		controlID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.login.session/"+id.String()+"/"+result.Session.ID.String()))
		p := SessionActivation{RequestID: id, AccountID: result.User.ID, SessionID: result.Session.ID, InputSHA256: hex.EncodeToString(proof), Control: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: result.Session.ID, Epoch: 1}, CoordinationID: controlID}, ResponseKeyID: a.activation.protector.KeyID(), ResponseCiphertext: cipher, AcceptedAt: result.ServerTime.UTC().Truncate(time.Microsecond)}
		return preparation.PrepareSessionActivation(p)
	})
	if err != nil && !errors.Is(err, ErrSessionActivationExists) {
		return AuthResult{}, err
	}
	saved, found, err = a.activation.store.LoadSessionActivation(ctx, id, proof)
	if err != nil {
		return AuthResult{}, err
	}
	if !found {
		return AuthResult{}, ErrSessionActivation
	}
	return a.activation.finish(ctx, saved)
}

// ResumeSessionActivation is the owner worker's original-request capability.
// Public transports never expose this method as an anonymous ID endpoint.
func (a *Auth) ResumeSessionActivation(ctx context.Context, id uuid.UUID) error {
	if a == nil || a.activation == nil {
		return ErrSessionActivation
	}
	return a.activation.ResumeAccepted(ctx, id)
}

// ResumeAccepted completes only an owner's persisted original login task.
func (s *SessionActivations) ResumeAccepted(ctx context.Context, id uuid.UUID) error {
	if s == nil || id == uuid.Nil {
		return ErrSessionActivation
	}
	p, err := s.store.LoadAcceptedSessionActivation(ctx, id)
	if err != nil {
		return err
	}
	if p.RequestID != id {
		return ErrSessionActivation
	}
	if p.Completed {
		return nil
	}
	_, err = s.finish(ctx, p)
	return err
}
func (s *SessionActivations) finish(ctx context.Context, p SessionActivation) (AuthResult, error) {
	if p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || p.SessionID == uuid.Nil || p.Control.Validate() != nil || p.Control.Issuer != writefence.Identity || p.Control.Kind != writefence.Session || p.Control.ID != p.SessionID || p.Control.Epoch != 1 || p.Control.Blocked {
		return AuthResult{}, ErrSessionActivation
	}
	expected := p.Control.Authority
	expected.Epoch = 0
	state, err := s.writers.Set(ctx, expected, p.Control.CoordinationID, false)
	if err != nil {
		return AuthResult{}, err
	}
	if !exactWriterCompletion(state, p.Control) {
		return AuthResult{}, ErrSessionActivation
	}
	if err := s.store.CompleteSessionActivation(ctx, p); err != nil {
		return AuthResult{}, err
	}
	plain, err := s.protector.Open(p.RequestID, p.ResponseKeyID, p.ResponseCiphertext)
	if err != nil {
		return AuthResult{}, ErrSessionActivation
	}
	defer clear(plain)
	var response loginActivationResponse
	if err := json.Unmarshal(plain, &response); err != nil {
		return AuthResult{}, ErrSessionActivation
	}
	result := response.Result
	if result.User.ID != p.AccountID || result.Session.AccountID != p.AccountID || result.Session.ID != p.SessionID || result.User.Status != domain.StatusActive || result.User.SessionEpoch != result.Session.CredentialRevision {
		return AuthResult{}, ErrSessionActivation
	}
	result.Token = response.LegacyToken
	result.CSRFToken = response.CSRFToken
	result.AccessToken = response.AccessToken
	result.RefreshToken = response.RefreshToken
	return result, nil
}
