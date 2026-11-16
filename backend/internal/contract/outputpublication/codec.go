package outputpublication

import (
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	fencewire "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

// Closed rejects unknown fields recursively, including nested streamed headers.
func Closed(value proto.Message) bool {
	if value == nil {
		return false
	}
	message := value.ProtoReflect()
	if !message.IsValid() || len(message.GetUnknown()) != 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return valid
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				valid = valid && Closed(list.Get(i).Message().Interface())
			}
		} else {
			valid = valid && Closed(value.Message().Interface())
		}
		return valid
	})
	return valid
}

// EncodeIdentity contains no physical route or unbounded supplier response.
func EncodeIdentity(o OperationOutputIdentity) *pb.OperationOutputIdentity {
	sequences := make([]int32, len(o.Sequences))
	for i, v := range o.Sequences {
		sequences[i] = int32(v)
	}
	return &pb.OperationOutputIdentity{OperationId: o.OperationID[:], OutputId: o.OutputID[:], ProjectId: o.ProjectID[:], ScopeId: o.ScopeID[:], Sequence: int32(o.Sequence), Sequences: sequences, ByteSize: o.ByteSize, Sha256: o.SHA256, MimeType: o.MIMEType, Kind: o.Kind, ProviderKey: o.ProviderKey, ModelKey: o.ModelKey, Region: o.Region, AspectRatio: o.AspectRatio, Provider: &pb.OutputProviderEvidence{RequestKey: o.Provider.RequestKey, Attempt: o.Provider.Attempt, ManifestSha256: o.Provider.ManifestSHA256, UsageSha256: o.Provider.UsageSHA256, SupplierUsageSha256: o.Provider.SupplierUsageSHA256, ActualCostMicros: o.Provider.ActualCostMicros}, Worker: &pb.OutputWorkerEvidence{TaskKind: o.Worker.Kind, TaskId: o.Worker.TaskID[:], Owner: o.Worker.Owner, Epoch: o.Worker.Epoch}}
}

// DecodeIdentity validates typed IDs and the complete bounded output manifest.
func DecodeIdentity(value *pb.OperationOutputIdentity) (OperationOutputIdentity, error) {
	var out OperationOutputIdentity
	if !Closed(value) || value.GetProvider() == nil || value.GetWorker() == nil || len(value.GetSequences()) > 128 {
		return out, ErrInvalidOutputPublication
	}
	ids := []*uuid.UUID{&out.OperationID, &out.OutputID, &out.ProjectID, &out.ScopeID, &out.Worker.TaskID}
	raws := [][]byte{value.GetOperationId(), value.GetOutputId(), value.GetProjectId(), value.GetScopeId(), value.GetWorker().GetTaskId()}
	for i, id := range ids {
		parsed, err := uuid.FromBytes(raws[i])
		if err != nil {
			return out, ErrInvalidOutputPublication
		}
		*id = parsed
	}
	out.Sequence = int(value.GetSequence())
	out.Sequences = make([]int, len(value.GetSequences()))
	for i, v := range value.GetSequences() {
		out.Sequences[i] = int(v)
	}
	out.ByteSize = value.GetByteSize()
	out.SHA256 = value.GetSha256()
	out.MIMEType = value.GetMimeType()
	out.Kind = value.GetKind()
	out.ProviderKey = value.GetProviderKey()
	out.ModelKey = value.GetModelKey()
	out.Region = value.GetRegion()
	out.AspectRatio = value.GetAspectRatio()
	p := value.GetProvider()
	out.Provider = OutputProviderEvidence{RequestKey: p.GetRequestKey(), Attempt: p.GetAttempt(), ManifestSHA256: p.GetManifestSha256(), UsageSHA256: p.GetUsageSha256(), SupplierUsageSHA256: p.GetSupplierUsageSha256(), ActualCostMicros: p.GetActualCostMicros()}
	out.Worker.Kind = value.GetWorker().GetTaskKind()
	out.Worker.Owner = value.GetWorker().GetOwner()
	out.Worker.Epoch = value.GetWorker().GetEpoch()
	return out, out.Validate()
}

// EncodeRequest preserves the original business digest and all authority generations.
func EncodeRequest(req Request) *pb.OutputPublication {
	return &pb.OutputPublication{Command: fencewire.EncodeCommand(req.Command), Output: EncodeIdentity(req.Output)}
}

// DecodeRequest rejects any extension or missing proof rather than forwarding it.
func DecodeRequest(value *pb.OutputPublication) (Request, error) {
	var req Request
	if !Closed(value) {
		return req, ErrInvalidOutputPublication
	}
	var err error
	req.Command, err = fencewire.DecodeCommand(value.GetCommand())
	if err != nil {
		return req, err
	}
	req.Output, err = DecodeIdentity(value.GetOutput())
	if err != nil {
		return req, err
	}
	return req, req.Validate()
}

// EncodeReceipt is a bounded acknowledgement with no physical object credentials.
func EncodeReceipt(r OperationOutputReceipt) *pb.OperationOutputReceipt {
	return &pb.OperationOutputReceipt{CommandId: r.CommandID[:], RequestSha256: r.RequestSHA256, PayloadSha256: r.PayloadSHA256, Output: EncodeIdentity(r.Output), AssetId: r.AssetID[:], AssetRevision: r.AssetRevision, ModerationStatus: r.ModerationStatus, ModerationDetailJson: r.ModerationDetail, State: r.State}
}

// DecodeReceipt checks the final asset/review identity before Execution adoption.
func DecodeReceipt(value *pb.OperationOutputReceipt) (OperationOutputReceipt, error) {
	var receipt OperationOutputReceipt
	if !Closed(value) {
		return receipt, ErrOutputPublicationConflict
	}
	var err error
	receipt.CommandID, err = uuid.FromBytes(value.GetCommandId())
	if err != nil {
		return receipt, ErrOutputPublicationConflict
	}
	receipt.AssetID, err = uuid.FromBytes(value.GetAssetId())
	if err != nil {
		return receipt, ErrOutputPublicationConflict
	}
	receipt.Output, err = DecodeIdentity(value.GetOutput())
	if err != nil {
		return receipt, err
	}
	receipt.RequestSHA256 = value.GetRequestSha256()
	receipt.PayloadSHA256 = value.GetPayloadSha256()
	receipt.AssetRevision = value.GetAssetRevision()
	receipt.ModerationStatus = value.GetModerationStatus()
	receipt.ModerationDetail = append([]byte(nil), value.GetModerationDetailJson()...)
	receipt.State = value.GetState()
	return receipt, receipt.Validate()
}
