package providerartifact

import (
	"time"

	"github.com/google/uuid"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediaartifact"
	fencewire "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

// EncodeRequest preserves the original call timestamp and exact authority set.
func EncodeRequest(r Request) *pb.ProviderArtifactRequest {
	return &pb.ProviderArtifactRequest{Command: fencewire.EncodeCommand(r.Command), DeliveryId: r.DeliveryID[:], ProviderCallId: r.ProviderCallID[:], ProviderCallCreatedAt: r.ProviderCallCreatedAt.UTC().Format(time.RFC3339Nano), OperationId: r.OperationID[:], ProjectId: r.ProjectID[:], CallAction: r.CallAction, CallAttempt: r.CallAttempt, RequestKey: r.RequestKey, Sequence: int32(r.Sequence), ByteSize: r.ByteSize, Sha256: r.SHA256, ManifestSha256: r.ManifestSHA256, Mime: r.MIME, Processing: r.Processing}
}

// DecodeRequest rejects unknown fields, malformed IDs and altered digests.
func DecodeRequest(v *pb.ProviderArtifactRequest) (Request, error) {
	var r Request
	if v == nil || len(v.ProtoReflect().GetUnknown()) != 0 {
		return r, ErrInvalid
	}
	if v.GetCommand() == nil || len(v.GetCommand().ProtoReflect().GetUnknown()) != 0 {
		return r, ErrInvalid
	}
	for _, authority := range v.GetCommand().GetAuthorities() {
		if authority == nil || len(authority.ProtoReflect().GetUnknown()) != 0 {
			return r, ErrInvalid
		}
	}
	command, err := fencewire.DecodeCommand(v.GetCommand())
	if err != nil {
		return r, ErrInvalid
	}
	r.Command = command
	values := [][]byte{v.GetDeliveryId(), v.GetProviderCallId(), v.GetOperationId(), v.GetProjectId()}
	targets := []*uuid.UUID{&r.DeliveryID, &r.ProviderCallID, &r.OperationID, &r.ProjectID}
	for index, raw := range values {
		id, err := uuid.FromBytes(raw)
		if err != nil {
			return r, ErrInvalid
		}
		*targets[index] = id
	}
	r.ProviderCallCreatedAt, err = time.Parse(time.RFC3339Nano, v.GetProviderCallCreatedAt())
	if err != nil {
		return r, ErrInvalid
	}
	r.CallAction, r.CallAttempt, r.RequestKey, r.Sequence = v.GetCallAction(), v.GetCallAttempt(), v.GetRequestKey(), int(v.GetSequence())
	r.ByteSize, r.SHA256, r.ManifestSHA256, r.MIME = v.GetByteSize(), v.GetSha256(), v.GetManifestSha256(), v.GetMime()
	r.Processing = v.GetProcessing()
	return r, r.Validate()
}

// EncodeReceipt contains only the exact request and opaque present responsibility.
func EncodeReceipt(r Receipt) *pb.ProviderArtifactReceipt {
	out := &pb.ProviderArtifactReceipt{Request: EncodeRequest(r.Request), ArtifactId: r.ArtifactID[:], State: r.State}
	if r.Canonical != nil {
		d := r.Canonical
		out.Canonical = &pb.ProviderArtifactDescriptor{ByteSize: d.ByteSize, Sha256: d.SHA256, Mime: d.MIME, DurationMs: d.DurationMS}
	}
	return out
}

// DecodeReceipt validates the complete echoed original identity.
func DecodeReceipt(v *pb.ProviderArtifactReceipt) (Receipt, error) {
	var r Receipt
	if v == nil || len(v.ProtoReflect().GetUnknown()) != 0 {
		return r, ErrInvalid
	}
	req, err := DecodeRequest(v.GetRequest())
	if err != nil {
		return r, err
	}
	id, err := uuid.FromBytes(v.GetArtifactId())
	if err != nil {
		return r, ErrInvalid
	}
	r = Receipt{Request: req, ArtifactID: id, State: v.GetState()}
	if d := v.GetCanonical(); d != nil {
		if len(d.ProtoReflect().GetUnknown()) != 0 {
			return r, ErrInvalid
		}
		r.Canonical = &Descriptor{ByteSize: d.GetByteSize(), SHA256: d.GetSha256(), MIME: d.GetMime(), DurationMS: d.GetDurationMs()}
	}
	if !r.Matches(req) {
		return r, ErrConflict
	}
	return r, nil
}
