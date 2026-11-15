package identityauthority_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func (f *pgFixture) proofReader(t *testing.T) *authority.Client {
	t.Helper()
	c, err := authority.NewClient(writefence.WriterCreation, authority.Target{Address: f.address, Token: f.tokens[writefence.WriterCreation], Transport: insecure.NewCredentials()})
	require.NoError(t, err)
	return c
}
func (f *pgFixture) proofCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, f.root.Table("identity.session_command_admission").Where("command_id=?", f.command.ID).Count(&count).Error)
	return count
}
func (f *pgFixture) admit(t *testing.T) {
	t.Helper()
	f.open(t)
	_, err := f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.NoError(t, err)
}
func TestPostgresResolveAdmissionRequiresOriginalExactProofAndCreationCaller(t *testing.T) {
	f := postgresFixture(t)
	reader := f.proofReader(t)
	_, err := reader.ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrNotFound)
	require.Zero(t, f.ledgerCount(t))
	require.Zero(t, f.proofCount(t))
	f.admit(t)
	proof, err := reader.ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
	require.NoError(t, err)
	require.Equal(t, authority.Identity{AccountID: f.auth.model.user.ID, SessionID: f.auth.legacySession, CredentialEpoch: 3}, proof.Identity)
	require.Equal(t, f.command, proof.Command)
	require.Equal(t, writefence.WriterMedia, proof.Target)
	// Credential generation 3 is the persisted original actor fact. The source
	// account/session authority heads are both generation 1, not credential epochs.
	require.NotEqual(t, f.command.Authorities[0].Epoch, proof.Identity.CredentialEpoch)
	for range 2 {
		_, err = reader.ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, f.ledgerCount(t))
	require.EqualValues(t, 1, f.proofCount(t))
	_, err = reader.ResolveAdmission(t.Context(), f.command, writefence.WriterExecution)
	require.ErrorIs(t, err, writefence.ErrNotFound)
	for _, change := range []func(*writefence.Command){func(c *writefence.Command) { c.SHA256 = strings.Repeat("b", 64) }, func(c *writefence.Command) { c.Action = "media.delete" }, func(c *writefence.Command) { c.Authorities[0].Epoch++ }, func(c *writefence.Command) { c.Authorities[1].ID = uuid.New() }} {
		changed := f.command
		changed.Authorities = append([]writefence.Authority(nil), changed.Authorities...)
		change(&changed)
		_, err = reader.ResolveAdmission(t.Context(), changed, writefence.WriterMedia)
		require.ErrorIs(t, err, writefence.ErrConflict)
	}
	wire := &pb.ResolveAdmissionRequest{Caller: coordination.Writer_WRITER_CREATION, Command: wireCommand(f.command), Target: coordination.Writer_WRITER_MEDIA}
	for _, caller := range []writefence.Writer{writefence.WriterIdentity, writefence.WriterMedia, writefence.WriterExecution} {
		_, err = rawClient(t, f.address).ResolveAdmission(privateContext(t, f.tokens[caller]), wire)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	unknown(wire)
	_, err = rawClient(t, f.address).ResolveAdmission(privateContext(t, f.tokens[writefence.WriterCreation]), wire)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.EqualValues(t, 1, f.ledgerCount(t))
	require.EqualValues(t, 1, f.proofCount(t))
}
func requirePermissionDenied(t *testing.T, err error) {
	t.Helper()
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, "42501", pgError.Code)
}

func TestPostgresAdmissionProofIsAppendOnlyAndPrivateToIdentity(t *testing.T) {
	f := postgresFixture(t)
	f.admit(t)
	requirePermissionDenied(t, f.restricted.WithContext(t.Context()).Exec(`UPDATE identity.session_command_admission SET credential_epoch = credential_epoch + 1 WHERE command_id=? AND target_writer='media'`, f.command.ID).Error)
	requirePermissionDenied(t, f.restricted.WithContext(t.Context()).Exec(`DELETE FROM identity.session_command_admission WHERE command_id=? AND target_writer='media'`, f.command.ID).Error)
	creation := authorityDatabase(t, writefence.WriterCreation)
	var count int64
	requirePermissionDenied(t, creation.WithContext(t.Context()).Table("identity.session_command_admission").Where("command_id=?", f.command.ID).Count(&count).Error)
	require.EqualValues(t, 1, f.proofCount(t))
	_, err := f.proofReader(t).ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
	require.NoError(t, err)
}
func TestPostgresResolveAdmissionRejectsRevokedStaleAndClosedIdentity(t *testing.T) {
	for _, failure := range []string{"session revoked", "account disabled", "credential epoch", "session expired", "head closed"} {
		t.Run(failure, func(t *testing.T) {
			f := postgresFixture(t)
			f.admit(t)
			reader := f.proofReader(t)
			var err error
			switch failure {
			case "session revoked":
				err = f.root.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=? AND account_id=?`, f.auth.now, f.auth.legacySession, f.auth.model.user.ID).Error
			case "account disabled":
				err = f.root.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.auth.model.user.ID).Error
			case "credential epoch":
				err = f.root.Exec(`UPDATE identity."user" SET session_epoch=session_epoch+1 WHERE id=?`, f.auth.model.user.ID).Error
			case "session expired":
				err = f.root.Exec(`UPDATE identity.user_session SET absolute_expires_at=? WHERE id=? AND account_id=?`, f.auth.now, f.auth.legacySession, f.auth.model.user.ID).Error
			case "head closed":
				_, err = f.coordinator.Set(t.Context(), f.command.Authorities[1], uuid.New(), true)
			}
			require.NoError(t, err)
			_, err = reader.ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
			if failure == "head closed" {
				require.ErrorIs(t, err, writefence.ErrFenced)
			} else {
				require.ErrorIs(t, err, identity.ErrSessionInvalid)
			}
			require.EqualValues(t, 1, f.proofCount(t))
			require.EqualValues(t, 1, f.ledgerCount(t))
		})
	}
}
func TestPostgresHistoricalAdmissionNeverAcquiresNewCredentialProof(t *testing.T) {
	f := postgresFixture(t)
	f.open(t)
	require.NoError(t, f.restricted.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return pgfence.NewAuthorityStore(tx).Register(t.Context(), tx, f.command, writefence.WriterMedia)
	}))
	require.EqualValues(t, 1, f.ledgerCount(t))
	require.Zero(t, f.proofCount(t))
	_, err := f.proofReader(t).ResolveAdmission(t.Context(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrNotFound)
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrNotFound)
	require.EqualValues(t, 1, f.ledgerCount(t))
	require.Zero(t, f.proofCount(t))
}
func TestPostgresProofTargetCannotReplaceOriginalAdmissionTarget(t *testing.T) {
	f := postgresFixture(t)
	f.admit(t)
	binding, err := f.command.AuthorityDigest()
	require.NoError(t, err)
	// Even an independently inserted proof row cannot substitute for the actual
	// original full admission of this same command ID to its immutable target.
	require.NoError(t, f.root.WithContext(t.Context()).Exec(`INSERT INTO identity.session_command_admission(command_id,target_writer,account_id,session_id,credential_epoch,request_sha256,authority_set_sha256) VALUES (?,'execution',?,?,3,?,?)`, f.command.ID, f.auth.model.user.ID, f.auth.legacySession, f.command.SHA256, binding).Error)
	_, err = f.proofReader(t).ResolveAdmission(t.Context(), f.command, writefence.WriterExecution)
	require.ErrorIs(t, err, writefence.ErrConflict)
	require.EqualValues(t, 1, f.ledgerCount(t))
	require.EqualValues(t, 2, f.proofCount(t))
}

func TestPostgresCredentialProofCollisionRollsBackFullAdmission(t *testing.T) {
	f := postgresFixture(t)
	f.open(t)
	binding, err := f.command.AuthorityDigest()
	require.NoError(t, err)
	require.NoError(t, f.root.Exec(`INSERT INTO identity.session_command_admission(command_id,target_writer,account_id,session_id,credential_epoch,request_sha256,authority_set_sha256) VALUES (?,'media',?,?,99,?,?)`, f.command.ID, f.auth.model.user.ID, f.auth.legacySession, f.command.SHA256, binding).Error)
	_, err = f.client.AdmitSessionCommand(t.Context(), f.credential(), f.command, writefence.WriterMedia)
	require.ErrorIs(t, err, writefence.ErrConflict)
	require.Zero(t, f.ledgerCount(t), "proof conflict must roll back the full original admission transaction")
	require.EqualValues(t, 1, f.proofCount(t))
}
