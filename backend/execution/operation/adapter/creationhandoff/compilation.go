package creationhandoff

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// CompileFrozenInput consumes original Creation compilation, with no actor,
// inline draft, reselected template or source credentials crossing this port.
func (c *SourceClient) CompileFrozenInput(ctx context.Context, request application.CompileFrozenInputRequest) (application.FrozenInputCompilation, error) {
	var out application.FrozenInputCompilation
	if c == nil {
		return out, application.ErrFrozenInputInvalid
	}
	if _, err := decodeKey(encodeKey(request.Key)); err != nil {
		return out, err
	}
	if _, err := request.CatalogContext.SHA256(); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			md.Set(key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(16384), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return out, writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	catalog := request.CatalogContext
	value, err := pb.NewFrozenInputsClient(conn).CompileFrozenInput(ctx, &pb.CompileFrozenInputRequest{Key: encodeKey(request.Key), CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: catalog.ModelVersionID[:], ModelKey: catalog.ModelKey, Capability: catalog.Capability, Mode: catalog.Mode}})
	if err != nil {
		return out, compilationError(err)
	}
	if value == nil || !closed(value) || len(value.CanonicalCompilationJson) == 0 || len(value.CanonicalCompilationJson) > writefence.MaxReceiptBytes {
		return out, application.ErrFrozenInputInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(value.CanonicalCompilationJson))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return application.FrozenInputCompilation{}, application.ErrFrozenInputInvalid
	}
	var extra json.RawMessage
	if decoder.Decode(&extra) != io.EOF {
		return application.FrozenInputCompilation{}, application.ErrFrozenInputInvalid
	}
	if err := out.Validate(request); err != nil {
		return application.FrozenInputCompilation{}, err
	}
	expected := encodeCompilation(out, value.CanonicalCompilationJson)
	params, paramsErr := writefence.CanonicalReceipt(value.ParamsJson)
	canonicalParams, canonicalErr := writefence.CanonicalReceipt(out.ParamsJSON)
	if paramsErr != nil || canonicalErr != nil || !bytes.Equal(params, canonicalParams) {
		return application.FrozenInputCompilation{}, application.ErrFrozenInputConflict
	}
	// JSONB may add whitespace; compare exact normalized numbers and key values.
	expected.ParamsJson = value.ParamsJson
	if !proto.Equal(expected, value) {
		return application.FrozenInputCompilation{}, application.ErrFrozenInputConflict
	}
	return out, nil
}

func encodeCompilation(value application.FrozenInputCompilation, canonical []byte) *pb.FrozenInputCompilation {
	catalog := value.CatalogContext
	out := &pb.FrozenInputCompilation{Key: encodeKey(value.Key), CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: catalog.ModelVersionID[:], ModelKey: catalog.ModelKey, Capability: catalog.Capability, Mode: catalog.Mode}, ContextSha256: value.ContextSHA256, CompilationSha256: value.CompilationSHA256, ModelVersionId: value.ModelVersionID[:], Mode: value.Mode, OriginalPrompt: value.OriginalPrompt, FinalPrompt: value.FinalPrompt, ParamsJson: value.ParamsJSON, OutputCount: value.OutputCount, CompilerVersion: value.CompilerVersion, CanonicalCompilationJson: canonical}
	for _, media := range value.Media {
		item := &pb.FrozenCompiledMedia{SeqNo: media.SeqNo, MediaAssetId: media.MediaAssetID[:], Role: media.Role, SubjectBindingSha256: media.SubjectBindingSHA256}
		if media.SubjectBindingID != nil {
			item.SubjectBindingId = media.SubjectBindingID[:]
		}
		out.Media = append(out.Media, item)
	}
	for _, mapping := range value.ReferenceMapping {
		item := &pb.FrozenReferenceMapping{ReferenceId: mapping.ReferenceID, Label: mapping.Label, Role: mapping.Role}
		for _, seq := range mapping.MediaSeqNos {
			item.MediaSeqNos = append(item.MediaSeqNos, int32(seq))
		}
		out.ReferenceMapping = append(out.ReferenceMapping, item)
	}
	if evidence := value.TemplateEvidence; evidence != nil {
		out.TemplateEvidence = &pb.FrozenTemplateEvidence{Version: int32(evidence.Version), Operation: evidence.Operation, Policy: evidence.Policy, TemplateVersion: int32(evidence.TemplateVersion), CustomizationRevision: evidence.CustomizationRevision, RequestSha256: evidence.RequestSHA256, UserPromptSha256: evidence.UserPromptSHA256, ContentSha256: evidence.ContentSHA256}
		if evidence.TemplateID != nil {
			out.TemplateEvidence.TemplateId = evidence.TemplateID[:]
		}
		if evidence.CustomizationID != nil {
			out.TemplateEvidence.CustomizationId = evidence.CustomizationID[:]
		}
	}
	return out
}

func compilationError(err error) error {
	for _, detail := range status.Convert(err).Details() {
		if failure, ok := detail.(*pb.FrozenInputFailure); ok && closed(failure) {
			switch failure.Reason {
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_INVALID:
				return application.ErrFrozenTemplateInvalid
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CHANGED:
				return application.ErrFrozenTemplateChanged
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CONTEXT_UNAVAILABLE:
				return application.ErrFrozenTemplateContextUnavailable
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNSUPPORTED_COMPILER:
				return application.ErrFrozenCompilerUnsupported
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_SOURCE_STALE:
				return application.ErrQuoteSourceStale
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_REFERENCE_UNAVAILABLE:
				return application.ErrFrozenReferenceUnavailable
			}
		}
	}
	return localError(err)
}

var _ application.FrozenInputCompiler = (*SourceClient)(nil)
