package identitysource

import (
	"context"

	"github.com/google/uuid"

	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ReadAccountGovernanceIntent reads only Identity's exact original account close.
// It does not infer account qualification or team role from a directory response.
func (c *Client) ReadAccountGovernanceIntent(ctx context.Context, id uuid.UUID) (workspace.AccountGovernanceIntent, error) {
	if id == uuid.Nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAuthorityClient(connection).ReadAccountQualificationIntent(ctx, &pb.ReadAccountQualificationIntentRequest{Caller: coordination.Writer_WRITER_CREATION, RequestId: append([]byte(nil), id[:]...)})
	if err != nil {
		return workspace.AccountGovernanceIntent{}, localError(err)
	}
	if !closed(response) {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	intent, err := decodeGovernanceIntent(response.Intent)
	if err != nil || intent.RequestID != id {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	return intent, nil
}

// ReadGovernanceQualification uses an explicit successor RPC when prior evidence
// was fenced. The original business request and complete candidate snapshot remain
// unchanged; no hidden retry replaces a command or revives an old permit.
func (c *Client) ReadGovernanceQualification(ctx context.Context, r workspace.GovernanceQualificationRequest) (workspace.GovernanceQualification, error) {
	if r.RequestID == uuid.Nil || r.TeamID == uuid.Nil || r.CandidateID == uuid.Nil {
		return workspace.GovernanceQualification{}, writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspace.GovernanceQualification{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	var response *pb.TeamAdministratorQualificationResponse
	if r.PreviousProofID == uuid.Nil {
		response, err = pb.NewAuthorityClient(connection).IssueTeamAdministratorQualification(ctx, &pb.IssueTeamAdministratorQualificationRequest{Caller: coordination.Writer_WRITER_CREATION, RequestId: append([]byte(nil), r.RequestID[:]...), TeamId: append([]byte(nil), r.TeamID[:]...), CandidateId: append([]byte(nil), r.CandidateID[:]...), SnapshotSha256: r.SnapshotSHA256})
	} else {
		response, err = pb.NewAuthorityClient(connection).RefreshTeamAdministratorQualification(ctx, &pb.RefreshTeamAdministratorQualificationRequest{Caller: coordination.Writer_WRITER_CREATION, RequestId: append([]byte(nil), r.RequestID[:]...), TeamId: append([]byte(nil), r.TeamID[:]...), CandidateId: append([]byte(nil), r.CandidateID[:]...), SnapshotSha256: r.SnapshotSHA256, PreviousProofId: append([]byte(nil), r.PreviousProofID[:]...)})
	}
	if err != nil {
		return workspace.GovernanceQualification{}, localError(err)
	}
	if !closed(response) {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	proof, err := decodeGovernanceQualification(response.Proof)
	if err != nil || proof.Intent.RequestID != r.RequestID || proof.TeamID != r.TeamID || proof.CandidateID != r.CandidateID || proof.SnapshotSHA256 != r.SnapshotSHA256 {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	if r.PreviousProofID != uuid.Nil && proof.ID != r.PreviousProofID && proof.PreviousProofID != r.PreviousProofID {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	return proof, nil
}

func decodeGovernanceIntent(value *pb.AccountQualificationIntent) (workspace.AccountGovernanceIntent, error) {
	if value == nil || !closed(value) {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	request, err := uuid.FromBytes(value.RequestId)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	account, err := uuid.FromBytes(value.AccountId)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	action := ""
	switch value.Action {
	case pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DISABLE:
		action = "disable"
	case pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE:
		action = "enable"
	case pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE:
		action = "delete"
	default:
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	control, err := decodeGovernanceControl(value.CloseControl, account)
	if err != nil {
		return workspace.AccountGovernanceIntent{}, err
	}
	out := workspace.AccountGovernanceIntent{RequestID: request, AccountID: account, Action: action, InputSHA256: value.InputSha256, CloseControl: control, EligibleAfter: value.EligibleAfter}
	if out.Validate() != nil {
		return workspace.AccountGovernanceIntent{}, writefence.ErrUnavailable
	}
	return out, nil
}
func decodeGovernanceControl(value *coordination.Control, id uuid.UUID) (writefence.Control, error) {
	if value == nil || !closed(value) {
		return writefence.Control{}, writefence.ErrUnavailable
	}
	authority, err := decodeSessionHead(value.Authority, writefence.Account, id)
	if err != nil {
		return writefence.Control{}, err
	}
	coord, err := uuid.FromBytes(value.CoordinationId)
	if err != nil {
		return writefence.Control{}, writefence.ErrUnavailable
	}
	out := writefence.Control{Authority: authority, CoordinationID: coord, Blocked: value.Blocked}
	if out.Validate() != nil {
		return writefence.Control{}, writefence.ErrUnavailable
	}
	return out, nil
}
func decodeGovernanceQualification(value *pb.TeamAdministratorQualification) (workspace.GovernanceQualification, error) {
	if value == nil || !closed(value) || value.Target != coordination.Writer_WRITER_CREATION {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	intent, err := decodeGovernanceIntent(value.Intent)
	if err != nil {
		return workspace.GovernanceQualification{}, err
	}
	id, err := uuid.FromBytes(value.Id)
	if err != nil {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	team, err := uuid.FromBytes(value.TeamId)
	if err != nil {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	candidate, err := uuid.FromBytes(value.CandidateId)
	if err != nil {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	control, err := decodeGovernanceControl(value.Control, candidate)
	if err != nil {
		return workspace.GovernanceQualification{}, err
	}
	out := workspace.GovernanceQualification{ID: id, Intent: intent, TeamID: team, CandidateID: candidate, SnapshotSHA256: value.SnapshotSha256, Eligible: value.Eligible, Control: control}
	if len(value.PreviousProofId) > 0 {
		out.PreviousProofID, err = uuid.FromBytes(value.PreviousProofId)
		if err != nil {
			return out, writefence.ErrUnavailable
		}
	}
	if value.Command == nil || !closed(value.Command) || len(value.Command.Authorities) != 1 {
		return out, writefence.ErrUnavailable
	}
	commandID, err := uuid.FromBytes(value.Command.Id)
	if err != nil {
		return out, writefence.ErrUnavailable
	}
	authority, err := decodeSessionHead(value.Command.Authorities[0], writefence.Account, candidate)
	if err != nil {
		return out, err
	}
	out.Command = writefence.Command{ID: commandID, SHA256: value.Command.Sha256, Action: value.Command.Action, Authorities: []writefence.Authority{authority}}
	if out.Validate() != nil {
		return workspace.GovernanceQualification{}, writefence.ErrUnavailable
	}
	return out, nil
}

var _ workspace.GovernanceQualificationSource = (*Client)(nil)
