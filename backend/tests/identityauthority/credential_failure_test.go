package identityauthority_test

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestCredentialFailurePreservesExactTypedPublicCause(t *testing.T) {
	for _, test := range []struct {
		name    string
		expired bool
		code    codes.Code
		reason  coordination.ErrorReason
		want    error
	}{
		{"csrf", false, codes.PermissionDenied, coordination.ErrorReason_ERROR_REASON_CSRF_INVALID, identity.ErrSessionCSRFInvalid},
		{"access expired", true, codes.Unauthenticated, coordination.ErrorReason_ERROR_REASON_ACCESS_EXPIRED, identity.ErrAccessExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth, policy, tokens := fixture(t)
			credential := authority.LegacyCredential(strings.Repeat("A", 43), strings.Repeat("B", 42)+"A")
			wire := &pb.Credential{Proof: &pb.Credential_LegacyToken{LegacyToken: strings.Repeat("A", 43)}, CsrfToken: strings.Repeat("B", 42) + "A"}
			if test.expired {
				auth.err = identity.ErrAccessExpired
				credential = authority.AccessCredential("synthetic-access-proof", "")
				wire = &pb.Credential{Proof: &pb.Credential_AccessToken{AccessToken: "synthetic-access-proof"}}
			}
			server, err := authority.NewServer(auth, policy, tokens, time.Now, insecure.NewCredentials())
			require.NoError(t, err)
			address := serve(t, server)
			_, err = rawClient(t, address).Verify(privateContext(t, tokens[writefence.WriterCreation]), &pb.VerifyRequest{Caller: coordination.Writer_WRITER_CREATION, Credential: wire})
			require.Equal(t, test.code, status.Code(err))
			var found bool
			for _, detail := range status.Convert(err).Details() {
				if failure, ok := detail.(*coordination.Failure); ok {
					found = failure.Reason == test.reason
				}
			}
			require.True(t, found, "credential cause was lost in transport")
			client, err := authority.NewClient(writefence.WriterCreation, authority.Target{Address: address, Token: tokens[writefence.WriterCreation], Transport: insecure.NewCredentials()})
			require.NoError(t, err)
			_, err = client.Verify(t.Context(), credential)
			require.True(t, errors.Is(err, test.want), "typed local cause was replaced")
		})
	}
}
