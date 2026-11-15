package grpchandoff

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
)

// Compiler is the authenticated source use case, deliberately not implemented by
// the raw PG store, whose method cannot bypass original Identity/source admission.
type Compiler interface {
	Compile(context.Context, application.CompileFrozenInputRequest) (application.FrozenInputCompilation, error)
}

// NewCompilingServer exposes all three source capabilities under one private
// authenticated origin. Missing compile wiring cannot silently fall back locally.
func NewCompilingServer(store FrozenInputs, freezer Freezer, compiler Compiler, executionToken string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if freezer == nil || compiler == nil {
		return nil, domain.ErrFrozenInputInvalid
	}
	return newServer(store, freezer, compiler, executionToken, transport)
}

// CompileFrozenInput accepts no actor, credentials, inline draft or runtime
// overrides. All rights and frozen source facts come from the original key.
func (s *server) CompileFrozenInput(ctx context.Context, request *pb.CompileFrozenInputRequest) (*pb.FrozenInputCompilation, error) {
	if s.compiler == nil {
		return nil, status.Error(codes.Unimplemented, "frozen input compiler not configured")
	}
	if request == nil || request.CatalogContext == nil {
		return nil, rpcError(domain.ErrFrozenInputInvalid)
	}
	key, err := decodeKey(request.Key)
	if err != nil {
		return nil, rpcError(err)
	}
	catalog := request.CatalogContext
	modelID, err := wireUUID(catalog.ModelVersionId)
	if err != nil {
		return nil, rpcError(err)
	}
	local := application.CompileFrozenInputRequest{Key: key, CatalogContext: application.FrozenCatalogContext{ModelVersionID: modelID, ModelKey: catalog.ModelKey, Capability: catalog.Capability, Mode: catalog.Mode}}
	hash, err := local.CatalogContext.SHA256()
	if err != nil {
		return nil, rpcError(err)
	}
	result, err := s.compiler.Compile(ctx, local)
	if err != nil {
		return nil, rpcError(err)
	}
	if result.Key != local.Key || result.CatalogContext != local.CatalogContext || result.ContextSHA256 != hash || result.ModelVersionID != modelID || result.Mode != catalog.Mode || result.CompilerVersion != domain.CompilerVersion || !hashString(result.CompilationSHA256) {
		return nil, rpcError(domain.ErrFrozenInputConflict)
	}
	raw, err := result.Receipt()
	if err != nil {
		return nil, rpcError(err)
	}
	out := &pb.FrozenInputCompilation{Key: encodeKey(result.Key), CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: result.CatalogContext.ModelVersionID[:], ModelKey: result.CatalogContext.ModelKey, Capability: result.CatalogContext.Capability, Mode: result.CatalogContext.Mode}, ContextSha256: result.ContextSHA256, CompilationSha256: result.CompilationSHA256, ModelVersionId: result.ModelVersionID[:], Mode: result.Mode, OriginalPrompt: result.OriginalPrompt, FinalPrompt: result.FinalPrompt, ParamsJson: result.ParamsJSON, OutputCount: result.OutputCount, CompilerVersion: result.CompilerVersion, CanonicalCompilationJson: raw}
	for _, media := range result.Media {
		item := &pb.FrozenCompiledMedia{SeqNo: media.SeqNo, MediaAssetId: media.MediaAssetID[:], Role: media.Role, SubjectBindingSha256: media.SubjectBindingSHA256}
		if media.SubjectBindingID != nil {
			item.SubjectBindingId = media.SubjectBindingID[:]
		}
		out.Media = append(out.Media, item)
	}
	for _, mapping := range result.ReferenceMapping {
		item := &pb.FrozenReferenceMapping{ReferenceId: mapping.ReferenceID, Label: mapping.Label, Role: mapping.Role}
		for _, seq := range mapping.MediaSeqNos {
			item.MediaSeqNos = append(item.MediaSeqNos, int32(seq))
		}
		out.ReferenceMapping = append(out.ReferenceMapping, item)
	}
	if evidence := result.TemplateEvidence; evidence != nil {
		out.TemplateEvidence = &pb.FrozenTemplateEvidence{Version: int32(evidence.Version), Operation: evidence.Operation, Policy: evidence.Policy, TemplateVersion: int32(evidence.TemplateVersion), CustomizationRevision: evidence.CustomizationRevision, RequestSha256: evidence.RequestSHA256, UserPromptSha256: evidence.UserPromptSHA256, ContentSha256: evidence.ContentSHA256}
		if evidence.TemplateID != nil {
			out.TemplateEvidence.TemplateId = evidence.TemplateID[:]
		}
		if evidence.CustomizationID != nil {
			out.TemplateEvidence.CustomizationId = evidence.CustomizationID[:]
		}
	}
	return out, nil
}
