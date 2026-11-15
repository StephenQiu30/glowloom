package identitysource

import (
	"context"

	"google.golang.org/protobuf/proto"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ workspacesource.AdmissionProof = (*Client)(nil)

// ResolveSourceAdmission uses only Creation's dedicated proof-reader capability.
// The effect owner must already have admitted this exact command for itself; this
// read never forwards raw credentials or registers Identity on another's behalf.
func (c *Client) ResolveSourceAdmission(ctx context.Context, command writefence.Command, target writefence.Writer) (workspacesource.Identity, error) {
	if _, err := command.AuthorityDigest(); err != nil {
		return workspacesource.Identity{}, err
	}
	var audience coordination.Writer
	switch target {
	case writefence.WriterExecution:
		audience = coordination.Writer_WRITER_EXECUTION
	case writefence.WriterMedia:
		audience = coordination.Writer_WRITER_MEDIA
	default:
		return workspacesource.Identity{}, writefence.ErrInvalid
	}
	encoded := encodeCommand(command)
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAuthorityClient(connection).ResolveAdmission(ctx, &pb.ResolveAdmissionRequest{Caller: coordination.Writer_WRITER_CREATION, Command: encoded, Target: audience})
	if err != nil {
		return workspacesource.Identity{}, localError(err)
	}
	if !closed(response) || response.Proof == nil || response.Proof.Target != audience || !proto.Equal(encoded, response.Proof.Command) {
		return workspacesource.Identity{}, writefence.ErrConflict
	}
	identity, err := decodeIdentity(response.Proof.Identity)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	if !identityMatchesCommand(identity, command) {
		return workspacesource.Identity{}, writefence.ErrConflict
	}
	return identity, nil
}

func identityMatchesCommand(identity workspacesource.Identity, command writefence.Command) bool {
	var account, session bool
	for _, authority := range command.Authorities {
		if authority.Issuer != writefence.Identity {
			continue
		}
		switch {
		case authority.Kind == writefence.Account && authority.ID == identity.AccountID:
			account = true
		case authority.Kind == writefence.Session && authority.ID == identity.SessionID:
			session = true
		default:
			return false
		}
	}
	return account && session
}
