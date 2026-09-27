// Package redis stores identity sessions under hashes of opaque bearer tokens.
package redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const tokenBytes = 32

var (
	// ErrInvalidSessionStore means the Redis client or session durations are unusable.
	ErrInvalidSessionStore = errors.New("invalid session store configuration")
	// ErrInvalidSession means the account identity is unusable for a new session.
	ErrInvalidSession = errors.New("invalid session identity")
	// ErrSessionNotFound covers malformed, revoked, idle-expired, and absolute-expired tokens.
	ErrSessionNotFound = domain.ErrSessionNotFound
	// ErrSessionCorrupt means stored session data failed validation.
	ErrSessionCorrupt = errors.New("invalid stored session")
)

// SessionStore owns Redis persistence for opaque, renewable login sessions.
type SessionStore struct {
	client      *redisclient.Client
	idleTTL     time.Duration
	absoluteTTL time.Duration
	now         func() time.Time
}

// NewSessionStore requires explicit dependencies and positive, ordered deadlines.
func NewSessionStore(client *redisclient.Client, idleTTL, absoluteTTL time.Duration, now func() time.Time) (*SessionStore, error) {
	if client == nil || now == nil || idleTTL < time.Millisecond || absoluteTTL < idleTTL {
		return nil, ErrInvalidSessionStore
	}
	return &SessionStore{client: client, idleTTL: idleTTL, absoluteTTL: absoluteTTL, now: now}, nil
}

// Create generates a fresh bearer token and persists only its digest as a key.
func (s *SessionStore) Create(ctx context.Context, orgID, userID uuid.UUID, epoch int64) (string, domain.Session, error) {
	if orgID == uuid.Nil || userID == uuid.Nil || epoch <= 0 {
		return "", domain.Session{}, ErrInvalidSession
	}
	now := s.now().UTC()
	session := domain.Session{
		OrgID: orgID, UserID: userID, SessionEpoch: epoch,
		CreateTime: now, LastSeen: now,
	}
	value, err := json.Marshal(session)
	if err != nil {
		return "", domain.Session{}, fmt.Errorf("encode session: %w", err)
	}
	for range 3 {
		secret := make([]byte, tokenBytes)
		if _, err := rand.Read(secret); err != nil {
			return "", domain.Session{}, fmt.Errorf("generate session token: %w", err)
		}
		token := base64.RawURLEncoding.EncodeToString(secret)
		created, err := s.client.SetNX(ctx, sessionKey(token), value, s.idleTTL).Result()
		if err != nil {
			return "", domain.Session{}, fmt.Errorf("create session: %w", err)
		}
		if created {
			return token, session, nil
		}
	}
	return "", domain.Session{}, errors.New("could not allocate a unique session token")
}

// Load reads a session without renewing it. The caller must check the current
// account status and epoch before calling Touch or authorizing a request.
func (s *SessionStore) Load(ctx context.Context, token string) (domain.Session, error) {
	if !validToken(token) {
		return domain.Session{}, ErrSessionNotFound
	}
	value, err := s.client.Get(ctx, sessionKey(token)).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return domain.Session{}, ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("load session: %w", err)
	}
	var session domain.Session
	if err := json.Unmarshal(value, &session); err != nil || !validSession(session) {
		return domain.Session{}, ErrSessionCorrupt
	}
	if s.expired(session, s.now().UTC()) {
		return domain.Session{}, ErrSessionNotFound
	}
	return session, nil
}

// Touch renews an existing session only after the caller has checked its epoch.
// SET XX prevents a concurrent logout or expiration from being undone.
func (s *SessionStore) Touch(ctx context.Context, token string, session domain.Session) error {
	if !validToken(token) {
		return ErrSessionNotFound
	}
	if !validSession(session) {
		return ErrSessionCorrupt
	}
	now := s.now().UTC()
	if s.expired(session, now) {
		return ErrSessionNotFound
	}
	remaining := session.CreateTime.Add(s.absoluteTTL).Sub(now)
	ttl := min(s.idleTTL, remaining)
	session.LastSeen = now
	value, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	updated, err := s.client.SetXX(ctx, sessionKey(token), value, ttl).Result()
	if err != nil {
		return fmt.Errorf("renew session: %w", err)
	}
	if !updated {
		return ErrSessionNotFound
	}
	return nil
}

// Destroy revokes the token and is safe to repeat.
func (s *SessionStore) Destroy(ctx context.Context, token string) error {
	if !validToken(token) {
		return nil
	}
	if err := s.client.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("destroy session: %w", err)
	}
	return nil
}

func (s *SessionStore) expired(session domain.Session, now time.Time) bool {
	return now.Before(session.CreateTime) ||
		!now.Before(session.LastSeen.Add(s.idleTTL)) ||
		!now.Before(session.CreateTime.Add(s.absoluteTTL))
}

func validSession(session domain.Session) bool {
	return session.UserID != uuid.Nil && session.OrgID != uuid.Nil && session.SessionEpoch > 0 &&
		!session.CreateTime.IsZero() && !session.LastSeen.Before(session.CreateTime)
}

func validToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == tokenBytes &&
		base64.RawURLEncoding.EncodeToString(decoded) == token
}

func sessionKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return "sess:" + hex.EncodeToString(digest[:])
}
