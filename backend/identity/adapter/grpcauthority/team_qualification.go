package grpcauthority

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func (s *server) ReadAccountQualificationIntent(ctx context.Context, r *pb.ReadAccountQualificationIntentRequest) (*pb.ReadAccountQualificationIntentResponse, error) {
	if s.qualifications == nil {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	id, err := qualificationUUID(r.RequestId)
	if err != nil {
		return nil, rpcError(err)
	}
	intent, err := s.qualifications.Intent(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.ReadAccountQualificationIntentResponse{Intent: encodeQualificationIntent(intent)}, nil
}
func (s *server) IssueTeamAdministratorQualification(ctx context.Context, r *pb.IssueTeamAdministratorQualificationRequest) (*pb.TeamAdministratorQualificationResponse, error) {
	request, err := decodeQualificationRequest(r.RequestId, r.TeamId, r.CandidateId, r.SnapshotSha256, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	return s.issueQualification(ctx, request)
}
func (s *server) RefreshTeamAdministratorQualification(ctx context.Context, r *pb.RefreshTeamAdministratorQualificationRequest) (*pb.TeamAdministratorQualificationResponse, error) {
	if len(r.PreviousProofId) != 16 {
		return nil, rpcError(writefence.ErrInvalid)
	}
	request, err := decodeQualificationRequest(r.RequestId, r.TeamId, r.CandidateId, r.SnapshotSha256, r.PreviousProofId)
	if err != nil {
		return nil, rpcError(err)
	}
	return s.issueQualification(ctx, request)
}
func (s *server) issueQualification(ctx context.Context, request identity.TeamQualificationRequest) (*pb.TeamAdministratorQualificationResponse, error) {
	if s.qualifications == nil {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	proof, err := s.qualifications.Issue(ctx, request)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.TeamAdministratorQualificationResponse{Proof: encodeTeamQualification(proof)}, nil
}
func (s *server) ResolveTeamAdministratorQualification(ctx context.Context, r *pb.ResolveTeamAdministratorQualificationRequest) (*pb.TeamAdministratorQualificationResponse, error) {
	if s.qualifications == nil {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	id, err := qualificationUUID(r.ProofId)
	if err != nil {
		return nil, rpcError(err)
	}
	proof, err := s.qualifications.Resolve(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.TeamAdministratorQualificationResponse{Proof: encodeTeamQualification(proof)}, nil
}
func qualificationUUID(value []byte) (uuid.UUID, error) {
	id, err := uuid.FromBytes(value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, writefence.ErrInvalid
	}
	return id, nil
}
func decodeQualificationRequest(request, team, candidate []byte, snapshot string, previous []byte) (identity.TeamQualificationRequest, error) {
	out := identity.TeamQualificationRequest{SnapshotSHA256: snapshot}
	var err error
	out.RequestID, err = qualificationUUID(request)
	if err != nil {
		return out, err
	}
	out.TeamID, err = qualificationUUID(team)
	if err != nil {
		return out, err
	}
	out.CandidateID, err = qualificationUUID(candidate)
	if err != nil {
		return out, err
	}
	if len(previous) > 0 {
		out.PreviousProofID, err = qualificationUUID(previous)
		if err != nil {
			return out, err
		}
	}
	return out, out.Validate()
}
func encodeQualificationIntent(p identity.AccountQualificationIntent) *pb.AccountQualificationIntent {
	action := pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DISABLE
	if p.Action == "enable" {
		action = pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_ENABLE
	}
	if p.Action == "delete" {
		action = pb.AccountQualificationAction_ACCOUNT_QUALIFICATION_ACTION_DELETE
	}
	return &pb.AccountQualificationIntent{RequestId: append([]byte(nil), p.RequestID[:]...), AccountId: append([]byte(nil), p.AccountID[:]...), Action: action, InputSha256: p.InputSHA256, CloseControl: &coordination.Control{Authority: encodeSessionAuthority(p.CloseControl.Authority), Blocked: true, CoordinationId: append([]byte(nil), p.CloseControl.CoordinationID[:]...)}, EligibleAfter: p.EligibleAfter}
}
func encodeTeamQualification(p identity.TeamQualificationProof) *pb.TeamAdministratorQualification {
	var previous []byte
	if p.PreviousProofID != uuid.Nil {
		previous = append([]byte(nil), p.PreviousProofID[:]...)
	}
	return &pb.TeamAdministratorQualification{Id: append([]byte(nil), p.ID[:]...), PreviousProofId: previous, Intent: encodeQualificationIntent(p.Intent), TeamId: append([]byte(nil), p.TeamID[:]...), CandidateId: append([]byte(nil), p.CandidateID[:]...), SnapshotSha256: p.SnapshotSHA256, Eligible: p.Eligible, Control: &coordination.Control{Authority: encodeSessionAuthority(p.Control.Authority), Blocked: p.Control.Blocked, CoordinationId: append([]byte(nil), p.Control.CoordinationID[:]...)}, Command: encodeCommand(p.Command), Target: coordination.Writer_WRITER_CREATION}
}
