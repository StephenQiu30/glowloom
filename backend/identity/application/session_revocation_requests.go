package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

// SessionRevocationAction distinguishes an exact target from an accepted batch.
type SessionRevocationAction string

const (
	// RevokeSessionRequest closes one explicitly selected session.
	RevokeSessionRequest SessionRevocationAction = "revoke_session"
	// RevokeOtherSessionsRequest freezes the original set excluding its requester.
	RevokeOtherSessionsRequest SessionRevocationAction = "revoke_others"
)

// SessionCredentialKind identifies an existing credential's digest convention.
type SessionCredentialKind string

const (
	// SessionCredentialLegacy uses SHA-256 of the decoded opaque legacy secret.
	SessionCredentialLegacy SessionCredentialKind = "legacy"
	// SessionCredentialAccess uses SHA-256 of the exact signed access token string.
	SessionCredentialAccess SessionCredentialKind = "access"
	// SessionCredentialRefresh uses SHA-256 of the decoded opaque refresh secret.
	SessionCredentialRefresh SessionCredentialKind = "refresh"
)

// SessionRevocationCredentials are ephemeral transport evidence, never a public DTO.
// A legacy secret cannot be combined with either modern protocol credential.
type SessionRevocationCredentials struct {
	LegacyToken  string `json:"-"`
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
	CSRFToken    string `json:"-"`
}

// SessionRevocationProof contains only bounded digests accepted in a policy tx.
// Possessing a session UUID is not equivalent to possessing this original evidence.
type SessionRevocationProof struct {
	Kind        SessionCredentialKind
	TokenDigest []byte `json:"-"`
	CSRFHash    []byte `json:"-"`
}

// SessionRevocationInput binds the existing public UUID key to one immutable intent.
// Primary is set only by the trusted authenticator, never by an HTTP parameter.
type SessionRevocationInput struct {
	Key      uuid.UUID
	Action   SessionRevocationAction
	TargetID uuid.UUID
	Proofs   []SessionRevocationProof
	Primary  SessionCredentialKind
}

// Validate checks the intent and proof shape, not their authentication or ownership.
func (in SessionRevocationInput) Validate() error {
	if in.Key == uuid.Nil || len(in.Proofs) < 1 || len(in.Proofs) > 2 {
		return ErrSessionInvalid
	}
	switch in.Action {
	case RevokeSessionRequest:
		if in.TargetID == uuid.Nil {
			return ErrSessionInvalid
		}
	case RevokeOtherSessionsRequest:
		if in.TargetID != uuid.Nil {
			return ErrForbidden
		}
	default:
		return ErrSessionInvalid
	}
	seen := map[SessionCredentialKind]bool{}
	for _, proof := range in.Proofs {
		switch proof.Kind {
		case SessionCredentialLegacy, SessionCredentialAccess, SessionCredentialRefresh:
		default:
			return ErrSessionInvalid
		}
		if seen[proof.Kind] || len(proof.TokenDigest) != sha256.Size || len(proof.CSRFHash) != sha256.Size {
			return ErrSessionInvalid
		}
		seen[proof.Kind] = true
	}
	if seen[SessionCredentialLegacy] && len(seen) != 1 {
		return ErrSessionInvalid
	}
	if in.Primary != "" && !seen[in.Primary] {
		return ErrSessionInvalid
	}
	return nil
}

// SessionRevocationRequest fixes the first accepted batch and original source IDs.
// It survives ordinary credential cleanup; completion requires every stored target.
type SessionRevocationRequest struct {
	ID, Key, AccountID, RequestSessionID uuid.UUID
	Action                               SessionRevocationAction
	TargetID                             uuid.UUID
	Revocations                          []SessionRevocation
	RequestedAt                          time.Time
	CompletedAt                          *time.Time
}

// Validate rejects a retargeted or incomplete durable request before any writer RPC.
func (r SessionRevocationRequest) Validate() error {
	if r.ID == uuid.Nil || r.Key == uuid.Nil || r.AccountID == uuid.Nil || r.RequestSessionID == uuid.Nil || r.RequestedAt.IsZero() {
		return ErrSessionRevocationUnavailable
	}
	switch r.Action {
	case RevokeSessionRequest:
		if r.TargetID == uuid.Nil || len(r.Revocations) != 1 || r.Revocations[0].SessionID != r.TargetID {
			return ErrSessionRevocationUnavailable
		}
	case RevokeOtherSessionsRequest:
		if r.TargetID != uuid.Nil {
			return ErrSessionRevocationUnavailable
		}
	default:
		return ErrSessionRevocationUnavailable
	}
	seen := make(map[uuid.UUID]bool, len(r.Revocations))
	for _, revocation := range r.Revocations {
		if revocation.Validate() != nil || revocation.AccountID != r.AccountID || seen[revocation.SessionID] || (r.Action == RevokeOtherSessionsRequest && revocation.SessionID == r.RequestSessionID) {
			return ErrSessionRevocationUnavailable
		}
		if r.CompletedAt != nil && revocation.CompletedAt == nil {
			return ErrSessionRevocationUnavailable
		}
		seen[revocation.SessionID] = true
	}
	if r.CompletedAt != nil && r.CompletedAt.Before(r.RequestedAt) {
		return ErrSessionRevocationUnavailable
	}
	return nil
}

// SessionRevocationRequestStore authenticates old proofs only for their exact request.
// Prepare commits policy, target snapshot, all local closures and proofs atomically;
// Load must never create proof rows. Complete verifies all original source receipts.
type SessionRevocationRequestStore interface {
	PrepareSessionRevocationRequest(context.Context, Principal, SessionRevocationInput, func() time.Time) (SessionRevocationRequest, error)
	LoadSessionRevocationRequest(context.Context, SessionRevocationInput) (SessionRevocationRequest, error)
	CompleteSessionRevocationRequest(context.Context, SessionRevocationRequest, time.Time) error
}

// SessionRevocationAuthenticator proves a current identity, not a recovery identity.
// Refresh evidence can authorize only its own explicit logout, never a new batch.
type SessionRevocationAuthenticator interface {
	AuthenticateSessionRevocation(context.Context, SessionRevocationCredentials, SessionRevocationAction, uuid.UUID) (Principal, SessionCredentialKind, error)
}

func sessionRevocationInput(credentials SessionRevocationCredentials, key uuid.UUID, action SessionRevocationAction, target uuid.UUID) (SessionRevocationInput, error) {
	in := SessionRevocationInput{Key: key, Action: action, TargetID: target}
	if credentials.LegacyToken != "" && (credentials.AccessToken != "" || credentials.RefreshToken != "") {
		return in, ErrSessionInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(credentials.CSRFToken)
	if err != nil || len(raw) != 32 {
		return in, ErrSessionCSRFInvalid
	}
	csrf := sha256.Sum256([]byte(credentials.CSRFToken))
	add := func(kind SessionCredentialKind, digest []byte) {
		in.Proofs = append(in.Proofs, SessionRevocationProof{Kind: kind, TokenDigest: digest, CSRFHash: append([]byte(nil), csrf[:]...)})
	}
	if credentials.LegacyToken != "" {
		digest, err := refreshDigest(credentials.LegacyToken)
		if err != nil {
			return in, err
		}
		add(SessionCredentialLegacy, digest)
	} else {
		if credentials.AccessToken != "" {
			if len(credentials.AccessToken) > 16<<10 {
				return in, ErrSessionInvalid
			}
			digest := sha256.Sum256([]byte(credentials.AccessToken))
			add(SessionCredentialAccess, digest[:])
		}
		if credentials.RefreshToken != "" {
			digest, err := refreshDigest(credentials.RefreshToken)
			if err == nil {
				add(SessionCredentialRefresh, digest)
			} else if len(in.Proofs) == 0 {
				return in, err
			}
		}
	}
	return in, in.Validate()
}
