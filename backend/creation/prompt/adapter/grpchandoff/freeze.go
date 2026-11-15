package grpchandoff

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FreezeFrozenInput delegates only to the source-admission use case. An execution
// service token authenticates the caller but cannot supply account, session, role
// or a permit. The use case resolves the original Identity proof before any effect.
func (s *server) FreezeFrozenInput(ctx context.Context, request *pb.FreezeFrozenInputRequest) (*pb.FrozenInput, error) {
	if s.freezer == nil {
		return nil, status.Error(codes.Unimplemented, "frozen input commands not configured")
	}
	local, err := decodeFreezeRequest(request)
	if err != nil {
		return nil, rpcError(err)
	}
	envelope, err := s.freezer.Freeze(ctx, local)
	if err != nil {
		return nil, rpcError(err)
	}
	if !application.SameFrozenCommand(envelope.Input.Command, local.Command) || !application.SameFrozenCommand(envelope.Input.SourceCommand, local.SourceCommand) || envelope.Input.HandoffID != local.HandoffID || envelope.Input.RequestID != local.RequestID || envelope.Input.ActorID != local.ActorID || envelope.Input.SessionID != local.SessionID || envelope.Input.ScopeID != local.ScopeID || envelope.Input.ProjectID != local.ProjectID || envelope.Input.Source != local.Source || envelope.Input.SourceGeneration != local.SourceGeneration || envelope.Input.ScopeRevision != local.ExpectedScopeRevision || envelope.Input.ProjectRevision != local.ExpectedProjectRevision {
		return nil, rpcError(domain.ErrFrozenInputConflict)
	}
	out, err := encodeInput(envelope)
	if err != nil {
		return nil, rpcError(err)
	}
	return out, nil
}

func decodeFreezeRequest(request *pb.FreezeFrozenInputRequest) (application.FreezeInputRequest, error) {
	var local application.FreezeInputRequest
	if request == nil || request.Source == nil {
		return local, domain.ErrFrozenInputInvalid
	}
	var err error
	local.HandoffID, err = wireUUID(request.HandoffId)
	if err != nil {
		return local, err
	}
	local.RequestID, err = wireUUID(request.RequestId)
	if err != nil {
		return local, err
	}
	local.ScopeID, err = wireUUID(request.ScopeId)
	if err != nil {
		return local, err
	}
	local.ProjectID, err = wireUUID(request.ProjectId)
	if err != nil {
		return local, err
	}
	local.Command, err = decodeCommand(request.Command)
	if err != nil {
		return local, err
	}
	local.SourceCommand, err = decodeCommand(request.SourceCommand)
	if err != nil {
		return local, err
	}
	// These are command references, not an identity assertion: SourceAdmission must
	// bind both to Identity's authenticated original admission before proceeding.
	for _, authority := range local.Command.Authorities {
		if authority.Issuer != writefence.Identity {
			continue
		}
		switch authority.Kind {
		case writefence.Account:
			local.ActorID = authority.ID
		case writefence.Session:
			local.SessionID = authority.ID
		}
	}
	local.Source.Kind = request.Source.Kind
	local.Source.DraftID, err = optionalUUID(request.Source.DraftId)
	if err != nil {
		return local, err
	}
	local.Source.CanvasID, err = optionalUUID(request.Source.CanvasId)
	if err != nil {
		return local, err
	}
	local.Source.NodeID, err = optionalUUID(request.Source.NodeId)
	if err != nil {
		return local, err
	}
	local.Source.Revision = request.Source.Revision
	local.Source.CanvasRevision = request.Source.CanvasRevision
	local.Source.InputRevision = request.Source.InputRevision
	local.SourceGeneration = request.SourceGeneration
	local.ExpectedScopeRevision = request.ExpectedScopeRevision
	local.ExpectedProjectRevision = request.ExpectedProjectRevision
	if selection := request.TemplateSelection; selection != nil {
		id, err := wireUUID(selection.ExpectedTemplateId)
		if err != nil {
			return local, err
		}
		local.TemplateSelection = &application.TemplateRequest{Operation: selection.Operation, ExpectedTemplateID: id, ExpectedCustomizationRevision: selection.ExpectedCustomizationRevision}
		if options := selection.Outline; options != nil {
			local.TemplateSelection.Outline = &application.OutlineOptions{ChapterCount: options.ChapterCount, Structure: options.Structure, WordCount: options.WordCount, Perspective: options.Perspective, Tone: options.Tone, CharacterScale: options.CharacterScale, ChapterLength: options.ChapterLength}
		}
		if _, err := local.TemplateSelection.Fingerprint(); err != nil {
			return local, err
		}
	}
	local.Pins = make([]domain.FrozenPin, 0, len(request.Pins))
	for _, pin := range request.Pins {
		if pin == nil {
			return local, domain.ErrFrozenInputInvalid
		}
		value := domain.FrozenPin{ReferenceID: pin.ReferenceId, AssetRevision: pin.AssetRevision, Kind: pin.Kind, SnapshotSHA256: pin.SnapshotSha256}
		value.PinID, err = wireUUID(pin.PinId)
		if err != nil {
			return local, err
		}
		value.AcquireCommandID, err = wireUUID(pin.AcquireCommandId)
		if err != nil {
			return local, err
		}
		value.AssetID, err = wireUUID(pin.AssetId)
		if err != nil {
			return local, err
		}
		local.Pins = append(local.Pins, value)
	}
	if local.Command.Action != "execution.frozen_input.accept" || local.SourceCommand.Action != "creation.frozen_input.freeze" || !local.Source.Valid() || local.SourceGeneration < 1 || local.ExpectedScopeRevision < 1 || local.ExpectedProjectRevision < 1 || domain.ValidateFrozenAuthorities(local.Command, local.ActorID, local.SessionID, local.ScopeID, local.ProjectID) != nil || domain.ValidateFrozenAuthorities(local.SourceCommand, local.ActorID, local.SessionID, local.ScopeID, local.ProjectID) != nil {
		return local, domain.ErrFrozenInputInvalid
	}
	return local, nil
}
func optionalUUID(raw []byte) (uuid.UUID, error) {
	if len(raw) == 0 {
		return uuid.Nil, nil
	}
	id, err := uuid.FromBytes(raw)
	if err != nil {
		return uuid.Nil, domain.ErrFrozenInputInvalid
	}
	return id, nil
}
