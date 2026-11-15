package identitysource

import (
	"context"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ workspacesource.SessionAuthorityReader = (*Client)(nil)

// ReadSourceSessionAuthorities validates the current credential plus required
// carried CSRF through Identity's real read boundary. It creates no head,
// command, admission or activity and never treats credentialEpoch as a head epoch.
func (c *Client) ReadSourceSessionAuthorities(ctx context.Context, credential workspacesource.Credential) (workspacesource.SessionAuthorities, error) {
	_, _, csrf := credential.IdentityProof()
	if !validProof(csrf) {
		return workspacesource.SessionAuthorities{}, workspacesource.ErrSourceCSRFInvalid
	}
	proof, err := encodeCredential(credential, true)
	if err != nil {
		return workspacesource.SessionAuthorities{}, workspacesource.ErrSourceSessionInvalid
	}
	bounded, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspacesource.SessionAuthorities{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAuthorityClient(connection).ReadSessionAuthorities(bounded, &pb.ReadSessionAuthoritiesRequest{Caller: coordination.Writer_WRITER_CREATION, Credential: proof})
	if err != nil {
		return workspacesource.SessionAuthorities{}, localError(err)
	}
	if !closed(response) {
		return workspacesource.SessionAuthorities{}, writefence.ErrUnavailable
	}
	identity, err := decodeIdentity(response.Identity)
	if err != nil {
		return workspacesource.SessionAuthorities{}, err
	}
	account, err := decodeSessionHead(response.Account, writefence.Account, identity.AccountID)
	if err != nil {
		return workspacesource.SessionAuthorities{}, err
	}
	session, err := decodeSessionHead(response.Session, writefence.Session, identity.SessionID)
	if err != nil {
		return workspacesource.SessionAuthorities{}, err
	}
	value := workspacesource.SessionAuthorities{Identity: identity, Account: account, Session: session}
	return value, value.Validate()
}

func decodeSessionHead(value *coordination.Authority, kind writefence.Kind, id uuid.UUID) (writefence.Authority, error) {
	if value == nil || value.Issuer != coordination.Issuer_ISSUER_IDENTITY {
		return writefence.Authority{}, writefence.ErrUnavailable
	}
	expected := coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT
	if kind == writefence.Session {
		expected = coordination.AuthorityKind_AUTHORITY_KIND_SESSION
	}
	parsed, err := uuid.FromBytes(value.Id)
	if err != nil || parsed != id || value.Kind != expected {
		return writefence.Authority{}, writefence.ErrUnavailable
	}
	out := writefence.Authority{Issuer: writefence.Identity, Kind: kind, ID: parsed, Epoch: value.Epoch}
	return out, out.Validate()
}
