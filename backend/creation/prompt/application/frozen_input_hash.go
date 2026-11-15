package application

import (
	frozenwire "github.com/StephenQiu30/glowloom-server/backend/internal/contract/frozeninput"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func frozenInputSourceSHA256(request FreezeInputRequest) (string, error) {
	wire := &pb.FreezeFrozenInputRequest{
		HandoffId: request.HandoffID[:], RequestId: request.RequestID[:], Command: frozenHashCommand(request.Command), SourceCommand: frozenHashCommand(request.SourceCommand),
		ScopeId: request.ScopeID[:], ProjectId: request.ProjectID[:], SourceGeneration: request.SourceGeneration, ExpectedScopeRevision: request.ExpectedScopeRevision, ExpectedProjectRevision: request.ExpectedProjectRevision,
		Source: &pb.FrozenInputSource{Kind: request.Source.Kind, DraftId: request.Source.DraftID[:], Revision: request.Source.Revision, CanvasId: request.Source.CanvasID[:], NodeId: request.Source.NodeID[:], CanvasRevision: request.Source.CanvasRevision, InputRevision: request.Source.InputRevision},
	}
	if selection := request.TemplateSelection; selection != nil {
		if _, err := selection.Fingerprint(); err != nil {
			return "", err
		}
		wire.TemplateSelection = &pb.FrozenTemplateSelection{Operation: selection.Operation, ExpectedTemplateId: selection.ExpectedTemplateID[:], ExpectedCustomizationRevision: selection.ExpectedCustomizationRevision}
		if options := selection.Outline; options != nil {
			wire.TemplateSelection.Outline = &pb.FrozenOutlineOptions{ChapterCount: options.ChapterCount, Structure: options.Structure, WordCount: options.WordCount, Perspective: options.Perspective, Tone: options.Tone, CharacterScale: options.CharacterScale, ChapterLength: options.ChapterLength}
		}
	}
	for _, pin := range request.Pins {
		wire.Pins = append(wire.Pins, &pb.FrozenInputPin{ReferenceId: pin.ReferenceID, PinId: pin.PinID[:], AcquireCommandId: pin.AcquireCommandID[:], AssetId: pin.AssetID[:], AssetRevision: pin.AssetRevision, Kind: pin.Kind, SnapshotSha256: pin.SnapshotSHA256})
	}
	return frozenwire.SourceCommandSHA256(wire)
}
func frozenHashCommand(command writefence.Command) *coordination.Command {
	out := &coordination.Command{Id: command.ID[:], Sha256: command.SHA256, Action: command.Action}
	issuers := map[writefence.Issuer]coordination.Issuer{writefence.Identity: coordination.Issuer_ISSUER_IDENTITY, writefence.Creation: coordination.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]coordination.AuthorityKind{writefence.Account: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: coordination.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: coordination.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: coordination.AuthorityKind_AUTHORITY_KIND_PROJECT, writefence.AgentGrant: coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT}
	for _, authority := range command.Authorities {
		out.Authorities = append(out.Authorities, &coordination.Authority{Issuer: issuers[authority.Issuer], Kind: kinds[authority.Kind], Id: authority.ID[:], Epoch: authority.Epoch})
	}
	return out
}
