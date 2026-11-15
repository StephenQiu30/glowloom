package identityauthority_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// authorityDatabase retains the existing microservices/authority_postgres_test.go
// loopback + explicit port + *_test guard and offline restricted LOGIN contract.
// No role, schema or data initialization runs in this package.
func authorityDatabase(t *testing.T, owner writefence.Writer) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_WRITE_FENCE_DB_DSN")
	if dsn == "" {
		t.Skip("real PostgreSQL not verified: requires isolated loopback *_test DSN and offline owner_b_*_test LOGINs")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") || u.Port() == "" || !strings.HasSuffix(strings.TrimPrefix(u.Path, "/"), "_test") {
		t.Fatal("authority tests require an explicit isolated loopback *_test PostgreSQL URL")
	}
	if owner != "" {
		require.NoError(t, owner.Validate())
		u.User = url.User("owner_b_" + string(owner) + "_test")
		query := u.Query()
		query.Del("options")
		u.RawQuery = query.Encode()
	}
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	if owner != "" {
		var safe bool
		err = db.Raw(`SELECT infra.runtime_owner()=? AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls) AND NOT pg_has_role(session_user,'glowloom_app','MEMBER') FROM pg_roles WHERE rolname=session_user`, string(owner)).Scan(&safe).Error
		require.NoError(t, err)
		require.True(t, safe, "fixture must be a restricted single owner LOGIN")
	}
	return db
}

type realWriters map[writefence.Writer]*pgfence.Store

func (w realWriters) Install(ctx context.Context, target writefence.Writer, control writefence.Control) error {
	return w[target].Install(ctx, control)
}

type pgFixture struct {
	root, restricted *gorm.DB
	auth             *actualAuth
	command          writefence.Command
	tokens           map[writefence.Writer]string
	client           *authority.Client
	address          string
	coordinator      *writefence.Coordinator
}

func postgresFixture(t *testing.T) *pgFixture {
	t.Helper()
	root := authorityDatabase(t, "")
	restricted := authorityDatabase(t, writefence.WriterIdentity)
	f := authenticationFixture(t)
	_, _, tokens := fixture(t)
	command := writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.create", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: f.model.user.ID, Epoch: 1}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: f.legacySession, Epoch: 1}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: uuid.New(), Epoch: 5}}}
	// Administrator writes only exact synthetic account/session records. The real
	// Auth and admission operations below use the restricted Identity LOGIN.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ids := []uuid.UUID{f.model.user.ID, f.legacySession}
		require.NoError(t, root.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			queries := []struct {
				sql  string
				args []any
			}{
				{`DELETE FROM identity.session_command_admission WHERE account_id=? AND session_id=?`, []any{f.model.user.ID, f.legacySession}},
				{`DELETE FROM infra.authority_admission WHERE service_owner='identity' AND authorities @> ?::jsonb`, []any{`[{"id":"` + f.model.user.ID.String() + `"}]`}},
				{`DELETE FROM infra.authority_acknowledgement WHERE service_owner='identity' AND coordination_id IN (SELECT coordination_id FROM infra.authority_coordination WHERE service_owner='identity' AND authority_id IN ?)`, []any{ids}},
				{`DELETE FROM infra.authority_coordination WHERE service_owner='identity' AND authority_id IN ?`, []any{ids}},
				{`DELETE FROM infra.authority_head WHERE service_owner='identity' AND authority_id IN ?`, []any{ids}},
				{`DELETE FROM infra.writer_fence WHERE issuer='identity' AND authority_id IN ?`, []any{ids}},
				{`DELETE FROM identity.user_session WHERE id=? AND account_id=?`, []any{f.legacySession, f.model.user.ID}},
				{`DELETE FROM identity."user" WHERE id=?`, []any{f.model.user.ID}},
			}
			for _, q := range queries {
				if err := tx.Exec(q.sql, q.args...).Error; err != nil {
					return err
				}
			}
			return nil
		}))
	})
	err := root.Transaction(func(tx *gorm.DB) error {
		u := f.model.user
		if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,status,password_hash,session_epoch,email_key,email_verified_at) VALUES (?,?,?,'authority fixture','producer','active','test-not-a-password',3,?,?)`, u.ID, uuid.New(), u.ID.String(), u.ID.String()+"@authority.invalid", u.EmailVerifiedAt).Error; err != nil {
			return err
		}
		s := f.model.sessions[f.legacySession]
		return tx.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,csrf_hash) VALUES (?,?,?,?,?,?,?,false,1,?)`, s.ID, s.AccountID, s.TokenHash, s.CredentialRevision, s.CreatedAt, s.LastActiveAt, s.AbsoluteExpiresAt, s.CSRFHash).Error
	})
	require.NoError(t, err)
	auth := identity.NewEmailAuth(pgidentity.NewStore(restricted), proofProtector{}, identity.EmailPolicy{}, func() time.Time { return f.now })
	s, err := authority.NewServer(auth, pgidentity.NewSessionAdmissionStore(restricted), tokens, func() time.Time { return f.now }, insecure.NewCredentials())
	require.NoError(t, err)
	address := serve(t, s)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: address, Token: tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	writers := realWriters{}
	for _, w := range writefence.Writers() {
		writers[w] = pgfence.New(authorityDatabase(t, w))
	}
	return &pgFixture{root: root, restricted: restricted, auth: f, command: command, tokens: tokens, client: c, address: address, coordinator: writefence.NewCoordinator(pgfence.NewAuthorityStore(restricted), writers)}
}
func (f *pgFixture) credential() authority.Credential {
	return authority.LegacyCredential(f.auth.legacy, f.auth.csrfLegacy)
}
func (f *pgFixture) open(t *testing.T) {
	t.Helper()
	for _, a := range f.command.Authorities[:2] {
		a.Epoch = 0
		state, err := f.coordinator.Set(t.Context(), a, uuid.New(), false)
		require.NoError(t, err)
		require.True(t, state.Complete)
		require.False(t, state.Control.Blocked)
	}
}
func (f *pgFixture) ledgerCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, f.root.Table("infra.authority_admission").Where("service_owner='identity' AND id=?", f.command.ID).Count(&count).Error)
	return count
}
func TestPostgresAuthorityRequiresOpenExactHeadsAndPersistsOriginalCommand(t *testing.T) {
	f := postgresFixture(t)
	_, err := f.client.Verify(t.Context(), f.credential())
	require.NoError(t, err)
	require.Zero(t, f.ledgerCount(t))
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrFenced)
	require.Zero(t, f.ledgerCount(t))
	var heads int64
	require.NoError(t, f.root.Table("infra.authority_head").Where("issuer='identity' AND authority_id IN ?", []uuid.UUID{f.auth.model.user.ID, f.auth.legacySession}).Count(&heads).Error)
	require.Zero(t, heads, "RPC must not initialize or reopen heads")
	f.open(t)
	readCredential := authority.LegacyCredential(f.auth.legacy, "")
	_, err = f.client.Verify(t.Context(), readCredential)
	require.NoError(t, err)
	_, err = f.client.AdmitSessionCommand(t.Context(), readCredential, f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrInvalid)
	a, policy, _ := fixture(t)
	withoutCSRF := requestFor(a, policy)
	withoutCSRF.Command = wireCommand(f.command)
	withoutCSRF.Credential = &pb.Credential{Proof: &pb.Credential_LegacyToken{LegacyToken: f.auth.legacy}}
	_, err = rawClient(t, f.address).AdmitSessionCommand(privateContext(t, f.tokens[writefence.WriterMedia]), withoutCSRF)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Zero(t, f.ledgerCount(t), "successful read verification cannot grant write admission")
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.NoError(t, err)
	require.EqualValues(t, 1, f.ledgerCount(t))
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.NoError(t, err)
	require.EqualValues(t, 1, f.ledgerCount(t))
	execution, err := authority.NewClient(writefence.WriterExecution, authority.Target{Address: f.address, Token: f.tokens[writefence.WriterExecution], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	_, err = execution.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterExecution)
	require.ErrorIs(t, err, writefence.ErrConflict, "another valid caller cannot change the original target")
	for _, mutate := range []func(*writefence.Command){func(c *writefence.Command) { c.Action = "media.delete" }, func(c *writefence.Command) { c.SHA256 = strings.Repeat("b", 64) }, func(c *writefence.Command) { c.Authorities[0].Epoch++ }} {
		changed := f.command
		changed.Authorities = append([]writefence.Authority(nil), changed.Authorities...)
		mutate(&changed)
		_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), changed, writefence.WriterMedia)
		require.Error(t, err)
	}
	state, err := f.coordinator.Set(t.Context(), f.command.Authorities[1], uuid.New(), true)
	require.NoError(t, err)
	require.True(t, state.Complete)
	blockedCommand := f.command
	blockedCommand.ID = uuid.New()
	blockedCommand.Authorities = append([]writefence.Authority(nil), f.command.Authorities...)
	blockedCommand.Authorities[1].Epoch = state.Control.Epoch
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), blockedCommand, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrFenced)
	_, err = f.client.Verify(t.Context(), f.credential())
	require.NoError(t, err)
}

type afterAuthentication struct {
	authority.Authenticator
	after func() error
}

func (a afterAuthentication) CurrentWithActivity(ctx context.Context, token string, activity identity.SessionActivity) (identity.AuthResult, error) {
	result, err := a.Authenticator.CurrentWithActivity(ctx, token, activity)
	if err == nil {
		err = a.after()
	}
	return result, err
}
func TestPostgresAdmissionRechecksCredentialEpochAfterAuthentication(t *testing.T) {
	f := postgresFixture(t)
	f.open(t)
	original := identity.NewEmailAuth(pgidentity.NewStore(f.restricted), proofProtector{}, identity.EmailPolicy{}, func() time.Time { return f.auth.now })
	auth := afterAuthentication{Authenticator: original, after: func() error {
		return f.root.WithContext(t.Context()).Exec(`UPDATE identity."user" SET session_epoch=session_epoch+1 WHERE id=?`, f.auth.model.user.ID).Error
	}}
	s, err := authority.NewServer(auth, pgidentity.NewSessionAdmissionStore(f.restricted), f.tokens, func() time.Time { return f.auth.now }, insecure.NewCredentials())
	require.NoError(t, err)
	c, err := authority.NewClient(writefence.WriterMedia, authority.Target{Address: serve(t, s), Token: f.tokens[writefence.WriterMedia], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	_, err = c.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, identity.ErrSessionInvalid)
	require.Zero(t, f.ledgerCount(t))
}
