package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

type barrierCoordinator struct {
	v2Coordinator
	entered, proceed chan struct{}
}

func (q *barrierCoordinator) Admit(ctx context.Context, _ uuid.UUID, _ string) (func(context.Context) error, error) {
	close(q.entered)
	select {
	case <-q.proceed:
		return func(context.Context) error { return nil }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Controlled transaction/admission barriers model authority order. This is NOT
// evidence of two PostgreSQL connections, Lua concurrency or deployed behavior.
func TestSessionV2RaceContracts(t *testing.T) {
	for _, kind := range []string{"logout", "disable", "password"} {
		t.Run(kind+" wins before refresh authority", func(t *testing.T) {
			_, m, now, a := sessionHTTPAuthFixture(t, false)
			r, err := a.Login(t.Context(), m.user.Email, "short", false)
			if err != nil {
				t.Fatal(err)
			}
			q := &barrierCoordinator{entered: make(chan struct{}), proceed: make(chan struct{})}
			a.WithSessionSecurity(&v2Codec{claims: map[string]identity.AccessClaims{}}, &v2Receipt{}, q)
			done := make(chan error, 1)
			go func() { _, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New()); done <- err }()
			<-q.entered
			if kind == "logout" {
				err = a.LogoutCurrentWithRefresh(t.Context(), r.RefreshToken, r.CSRFToken, r.Session.ID)
			} else {
				err = m.EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
					u, e := tx.AccountByID(m.user.ID)
					if e != nil {
						return e
					}
					if kind == "disable" {
						u.Status = domain.StatusDisabled
					} else {
						u.SessionEpoch++
					}
					if e = tx.UpdateAccount(u); e != nil {
						return e
					}
					return tx.RevokeAccountSessions(u.ID, *now)
				})
			}
			close(q.proceed)
			if err != nil {
				t.Fatal(err)
			}
			if err = <-done; !errors.Is(err, identity.ErrSessionInvalid) {
				t.Fatal("authority revived", err)
			}
			if len(m.refresh) != 1 || m.sessions[r.Session.ID].RevokedAt == nil {
				t.Fatal("revoked chain consumed")
			}
		})
	}
	t.Run("commit then cache failure and lost response", func(t *testing.T) {
		_, m, now, a := sessionHTTPAuthFixture(t, false)
		r, err := a.Login(t.Context(), m.user.Email, "short", false)
		if err != nil {
			t.Fatal(err)
		}
		child, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New())
		if err != nil || recovered.RefreshToken != child.RefreshToken || len(m.refresh) != 2 {
			t.Fatal("duplicate consumption", err)
		}
		// A higher committed generation fences an old receipt even inside its window.
		m.mu.Lock()
		s := m.sessions[r.Session.ID]
		s.RefreshGeneration = 2
		m.sessions[s.ID] = s
		m.mu.Unlock()
		late, err := a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New())
		if !errors.Is(err, identity.ErrRefreshRaced) || late.AccessToken != "" {
			t.Fatal("stale receipt", err)
		}
		*now = now.Add(10 * time.Second)
		if _, err = a.Refresh(t.Context(), r.RefreshToken, r.CSRFToken, "peer", uuid.New()); !errors.Is(err, identity.ErrSessionInvalid) || m.sessions[s.ID].RevokedAt == nil {
			t.Fatal("reuse revoke rolled back", err)
		}
	})
}
