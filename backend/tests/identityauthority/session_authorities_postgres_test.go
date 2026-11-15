package identityauthority_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPostgresReadSessionAuthoritiesNeverGuessesOrInitializesHeads(t *testing.T) {
	f := postgresFixture(t)
	_, err := f.client.ReadSessionAuthorities(t.Context(), f.credential())
	require.Error(t, err)
	require.Zero(t, f.ledgerCount(t))
	var heads int64
	require.NoError(t, f.restricted.Raw(`SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_id IN ?`, []uuid.UUID{f.auth.model.user.ID, f.auth.legacySession}).Scan(&heads).Error)
	require.Zero(t, heads, "read must not create missing heads")
	f.open(t)
	account := f.command.Authorities[0]
	for range 2 {
		closed, err := f.coordinator.Set(t.Context(), account, uuid.New(), true)
		require.NoError(t, err)
		_, err = f.client.ReadSessionAuthorities(t.Context(), f.credential())
		require.Error(t, err, "closed source must not provide usable generations")
		opened, err := f.coordinator.Set(t.Context(), closed.Control.Authority, uuid.New(), false)
		require.NoError(t, err)
		account = opened.Control.Authority
	}
	state, err := f.client.ReadSessionAuthorities(t.Context(), f.credential())
	require.NoError(t, err)
	require.Equal(t, int64(3), state.Identity.CredentialEpoch)
	require.Equal(t, int64(5), state.Account.Epoch)
	require.Equal(t, int64(1), state.Session.Epoch)
	require.Equal(t, f.auth.model.user.ID, state.Account.ID)
	require.Equal(t, f.auth.legacySession, state.Session.ID)
	require.Zero(t, f.ledgerCount(t), "reading exact heads must not admit a write")
	var activeAt struct{ LastActiveAt time.Time }
	require.NoError(t, f.restricted.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, f.auth.legacySession).Scan(&activeAt).Error)
	require.WithinDuration(t, f.auth.model.sessions[f.auth.legacySession].LastActiveAt, activeAt.LastActiveAt, time.Microsecond, "read must not extend idle activity")
}
