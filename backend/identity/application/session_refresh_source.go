package application

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// RefreshReuseRequestProof identifies only a stored expired-parent responsibility.
// It conveys no fresh identity and contains no raw credential or caller target.
type RefreshReuseRequestProof struct {
	Key   uuid.UUID
	Proof SessionRevocationProof
}

// Validate bounds the only proof kind accepted by reuse-only historical lookup.
func (p RefreshReuseRequestProof) Validate() error {
	if p.Key == uuid.Nil || p.Proof.Kind != SessionCredentialRefresh || len(p.Proof.TokenDigest) != sha256.Size || len(p.Proof.CSRFHash) != sha256.Size {
		return ErrSessionInvalid
	}
	return nil
}

// RefreshReuseRequestReader cannot enroll a new proof or select a new target.
// The accepted request already fixes its session and this internal action.
type RefreshReuseRequestReader interface {
	LoadRefreshReuseRequest(context.Context, RefreshReuseRequestProof) (SessionRevocationRequest, error)
}

type currentSessionSourceTx interface {
	CheckCurrentSessionSources(Principal) error
}

// WithSessionRevocationRequests requires the existing proof-backed closure store.
// Native refresh cannot fall back to a local-only revocation when this is absent.
func (a *Auth) WithSessionRevocationRequests(requests *PublicSessionRevocations) error {
	if requests == nil || requests.authenticator == nil || requests.store == nil || requests.closures == nil || !requests.closures.ready() || requests.now == nil {
		return ErrSessionRevocationUnavailable
	}
	if _, ok := requests.store.(RefreshReuseRequestReader); !ok {
		return ErrSessionRevocationUnavailable
	}
	a.sessionRevocations = requests
	return nil
}

func (a *Auth) checkCurrentSessionSources(tx AuthTx, user domain.User, session domain.AuthSession) error {
	if a.activation == nil {
		return nil
	}
	source, ok := tx.(currentSessionSourceTx)
	if !ok {
		return ErrSessionRevocationUnavailable
	}
	return source.CheckCurrentSessionSources(Principal{ID: user.ID, SessionID: session.ID, CredentialEpoch: user.SessionEpoch})
}

func refreshReuseProof(credentials SessionRevocationCredentials, key uuid.UUID) (RefreshReuseRequestProof, error) {
	out := RefreshReuseRequestProof{Key: key}
	if credentials.LegacyToken != "" || credentials.AccessToken != "" || credentials.RefreshToken == "" {
		return out, ErrSessionInvalid
	}
	proofs, err := sessionRevocationProofs(credentials)
	if err != nil {
		return out, err
	}
	if len(proofs) != 1 {
		return out, ErrSessionInvalid
	}
	out.Proof = proofs[0]
	return out, out.Validate()
}

// ResumeRefreshReuse recognizes only a previously accepted original proof/key.
// A miss never authorizes or revokes anything, even after session-row collection.
func (s *PublicSessionRevocations) ResumeRefreshReuse(ctx context.Context, credentials SessionRevocationCredentials, key uuid.UUID) (bool, error) {
	if s == nil || s.store == nil || s.closures == nil || !s.closures.ready() || s.now == nil {
		return false, ErrSessionRevocationUnavailable
	}
	proof, err := refreshReuseProof(credentials, key)
	if err != nil {
		return false, err
	}
	reader, ok := s.store.(RefreshReuseRequestReader)
	if !ok {
		return false, ErrSessionRevocationUnavailable
	}
	request, err := reader.LoadRefreshReuseRequest(ctx, proof)
	if errors.Is(err, writefence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if request.Validate() != nil || request.Action != RevokeRefreshReuseRequest || request.Key != key || request.TargetID != request.RequestSessionID {
		return false, ErrSessionRevocationUnavailable
	}
	if err := finishSessionRevocationRequest(ctx, request, s.store, s.closures, s.now); err != nil {
		return true, err
	}
	return true, nil
}

// RevokeReusedRefresh has no public route. Its authenticator and prepare store
// both recheck the expired consumed refresh parent and CSRF under original locks.
func (s *PublicSessionRevocations) RevokeReusedRefresh(ctx context.Context, credentials SessionRevocationCredentials, key, target uuid.UUID) error {
	if _, err := refreshReuseProof(credentials, key); err != nil {
		return err
	}
	_, err := s.execute(ctx, credentials, key, RevokeRefreshReuseRequest, target)
	if errors.Is(err, ErrSessionInvalid) {
		// Resolve a concurrent same-key acceptance using immutable proof, never a
		// second fresh authorization or a replacement target.
		if found, recovery := s.ResumeRefreshReuse(ctx, credentials, key); found || recovery != nil {
			return recovery
		}
	}
	return err
}
