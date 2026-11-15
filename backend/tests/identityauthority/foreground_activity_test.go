package identityauthority_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	authority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func (m *authModel) TouchSession(id uuid.UUID, now time.Time) error {
	session, found := m.sessions[id]
	if !found {
		return identity.ErrSessionInvalid
	}
	session.LastActiveAt = now
	m.sessions[id] = session
	return nil
}

func TestForegroundActivityIsExplicitAndRetainsStrictAccessCSRF(t *testing.T) {
	for _, test := range []struct {
		name                  string
		purpose               identity.ForegroundPurpose
		legacy, csrf, success bool
	}{
		{"access foreground read", identity.ForegroundProfileRead, false, false, true},
		{"access foreground write needs CSRF", identity.ForegroundPreferencesWrite, false, false, false},
		{"access foreground write", identity.ForegroundPreferencesWrite, false, true, true},
		{"legacy foreground read", identity.ForegroundProfileRead, true, false, true},
		{"legacy foreground write needs CSRF", identity.ForegroundProfileWrite, true, false, false},
		{"legacy foreground write", identity.ForegroundProfileWrite, true, true, true},
		{"unknown purpose", identity.ForegroundPurpose("polling"), false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authenticationFixture(t)
			credential := identity.SessionCredential{AccessToken: f.access}
			id := f.accessSession
			if test.legacy {
				id = f.legacySession
				credential = identity.SessionCredential{LegacyToken: f.legacy}
			}
			if test.csrf {
				credential.CSRFToken = f.csrfAccess
				if test.legacy {
					credential.CSRFToken = f.csrfLegacy
				}
			}
			before := f.model.sessions[id]
			principal, err := f.auth.RecordForegroundActivity(t.Context(), credential, test.purpose)
			after := f.model.sessions[id]
			if test.success {
				require.NoError(t, err)
				require.Equal(t, id, principal.SessionID)
				require.Equal(t, f.now, after.LastActiveAt)
			} else {
				require.Error(t, err)
				require.Equal(t, before.LastActiveAt, after.LastActiveAt)
			}
			require.Equal(t, before.AbsoluteExpiresAt, after.AbsoluteExpiresAt)
			_, err = f.auth.CurrentAccess(t.Context(), f.access, "", identity.NoActivity)
			require.Error(t, err, "explicit foreground reads cannot loosen CurrentAccess")
		})
	}
}

func TestForegroundRPCRejectsWrongOwnerAndUnknownPurposeBeforeTouch(t *testing.T) {
	for _, test := range []struct {
		name   string
		caller writefence.Writer
		change func(*pb.RecordForegroundActivityRequest)
		code   codes.Code
	}{
		{"owner", writefence.WriterMedia, func(*pb.RecordForegroundActivityRequest) {}, codes.PermissionDenied},
		{"caller", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) { r.Caller = coordination.Writer_WRITER_CREATION }, codes.PermissionDenied},
		{"unknown purpose", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) { r.Purpose = pb.ForegroundPurpose(999) }, codes.InvalidArgument},
		{"no purpose", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) {
			r.Purpose = pb.ForegroundPurpose_FOREGROUND_PURPOSE_UNSPECIFIED
		}, codes.InvalidArgument},
		{"header extension", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) { unknown(r) }, codes.InvalidArgument},
		{"credential extension", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) { unknown(r.Credential) }, codes.InvalidArgument},
		{"write proof", writefence.WriterIdentity, func(r *pb.RecordForegroundActivityRequest) {
			r.Purpose = pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROFILE_WRITE
		}, codes.PermissionDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authenticationFixture(t)
			_, policy, tokens := fixture(t)
			server, err := authority.NewServer(f.auth, policy, tokens, time.Now, insecure.NewCredentials())
			require.NoError(t, err)
			client := rawClient(t, serve(t, server))
			caller := coordination.Writer_WRITER_IDENTITY
			if test.caller == writefence.WriterMedia {
				caller = coordination.Writer_WRITER_MEDIA
			}
			request := &pb.RecordForegroundActivityRequest{Caller: caller, Credential: &pb.Credential{Proof: &pb.Credential_LegacyToken{LegacyToken: f.legacy}}, Purpose: pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROFILE_READ}
			test.change(request)
			before := f.model.sessions[f.legacySession]
			_, err = client.RecordForegroundActivity(privateContext(t, tokens[test.caller]), request)
			require.Equal(t, test.code, status.Code(err))
			require.Equal(t, before.LastActiveAt, f.model.sessions[f.legacySession].LastActiveAt)
		})
	}
}
