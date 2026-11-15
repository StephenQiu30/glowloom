package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// PublishOperationOutputRequest binds the original business digest to precise policy generations.
type PublishOperationOutputRequest = outputpublication.Request

// ValidateOutputPublication is Media's independent admission/archive policy.
func ValidateOutputPublication(r PublishOperationOutputRequest) error {
	if r.Validate() != nil || r.Command.Action != "media.operation_output.publish" || domain.ValidateOperationOutput(r.Output) != nil {
		return domain.ErrInvalidOutputPublication
	}
	var account, session, scope, project bool
	for _, a := range r.Command.Authorities {
		account = account || (a.Issuer == writefence.Identity && a.Kind == writefence.Account)
		session = session || (a.Issuer == writefence.Identity && a.Kind == writefence.Session)
		scope = scope || (a.Issuer == writefence.Creation && a.Kind == writefence.Scope && a.ID == r.Output.ScopeID)
		project = project || (a.Issuer == writefence.Creation && a.Kind == writefence.Project && a.ID == r.Output.ProjectID)
	}
	if !account || !session || !scope || !project {
		return domain.ErrInvalidOutputPublication
	}
	return nil
}

// CompleteOutputRenditions enforces Media's actual mandatory preview set.
func CompleteOutputRenditions(kind domain.Kind, renditions []domain.Rendition) bool {
	return completeRenditions(kind, renditions)
}

// OperationOutputWork is private Media state. Physical routes stay inside Media adapters.
type OperationOutputWork struct {
	// SourceVerified is set only by Media's durable prepare repository after
	// original Execution proof and local Guard; it is never accepted from the wire.
	SourceVerified bool
	PreparedAt     time.Time
	Request        PublishOperationOutputRequest
	AssetID        uuid.UUID
	Receipt        *domain.OperationOutputReceipt
}

// OutputReview is the Media review boundary's final decision for archived bytes.
type OutputReview struct {
	Status string
	Detail json.RawMessage
}

// OutputPublicationVerifier checks the original Execution command and worker evidence
// through a restricted receipt port, outside the Media SQL transaction.
type OutputPublicationVerifier interface {
	VerifyOutputPublication(context.Context, PublishOperationOutputRequest) error
}

// OutputPublicationRepository never holds an SQL transaction across external I/O.
type OutputPublicationRepository interface {
	PrepareOutput(context.Context, PublishOperationOutputRequest) (OperationOutputWork, error)
	ResolveOutput(context.Context, PublishOperationOutputRequest) (domain.OperationOutputReceipt, error)
	CommitOutput(context.Context, OperationOutputWork, domain.MediaAsset, []domain.Rendition, OutputReview) (domain.OperationOutputReceipt, error)
}

// OutputArchiver verifies and archives exact source bytes and required renditions.
type OutputArchiver interface {
	ArchiveOutput(context.Context, OperationOutputWork, *Downloaded) (domain.MediaAsset, []domain.Rendition, error)
}

// OutputReviewer owns the actual moderation decision; callers cannot self-assert readiness.
type OutputReviewer interface {
	ReviewOutput(context.Context, OperationOutputWork, domain.MediaAsset) (OutputReview, error)
}

// OutputPublisher coordinates durable Media responsibility, bounded bytes and local publication.
type OutputPublisher struct {
	repo    OutputPublicationRepository
	archive OutputArchiver
	review  OutputReviewer
}

// NewOutputPublisher uses explicit owner-local persistence and external object/review ports.
func NewOutputPublisher(repo OutputPublicationRepository, archive OutputArchiver, review OutputReviewer) *OutputPublisher {
	return &OutputPublisher{repo: repo, archive: archive, review: review}
}

// Resolve checks only the original command. Missing or prepared means unknown, never permission to delete.
func (s *OutputPublisher) Resolve(ctx context.Context, req PublishOperationOutputRequest) (domain.OperationOutputReceipt, error) {
	if s == nil || s.repo == nil {
		return domain.OperationOutputReceipt{}, ErrUnavailable
	}
	if err := ValidateOutputPublication(req); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	receipt, err := s.repo.ResolveOutput(ctx, req)
	if err != nil {
		return receipt, err
	}
	return receipt, validateOutputReceipt(req, receipt)
}
func validateOutputReceipt(req PublishOperationOutputRequest, r domain.OperationOutputReceipt) error {
	if domain.ValidateOperationOutputReceipt(r) != nil || r.CommandID != req.Command.ID || r.RequestSHA256 != req.Command.SHA256 || r.PayloadSHA256 != req.Output.Digest() {
		return domain.ErrOutputPublicationConflict
	}
	return nil
}

// Publish consumes EOF and verifies every byte before any object write. A transport
// failure after the last data frame is still failure. Replays return the original ACK.
func (s *OutputPublisher) Publish(ctx context.Context, req PublishOperationOutputRequest, source io.Reader) (domain.OperationOutputReceipt, error) {
	if s == nil || s.repo == nil || s.archive == nil || s.review == nil || source == nil {
		return domain.OperationOutputReceipt{}, ErrUnavailable
	}
	if err := ValidateOutputPublication(req); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	work, err := s.repo.PrepareOutput(ctx, req)
	if err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	if work.Receipt != nil {
		return *work.Receipt, validateOutputReceipt(req, *work.Receipt)
	}
	if work.AssetID == uuid.Nil || work.Request.Output.Digest() != req.Output.Digest() || work.Request.Command.ID != req.Command.ID {
		return domain.OperationOutputReceipt{}, domain.ErrOutputPublicationConflict
	}
	file, err := os.CreateTemp("", "glowloom-output-*")
	if err != nil {
		return domain.OperationOutputReceipt{}, fmt.Errorf("create output spool: %w", err)
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	hash := sha256.New()
	limited := io.LimitReader(source, req.Output.ByteSize+1)
	size, err := io.Copy(io.MultiWriter(file, hash), limited)
	if err != nil {
		return domain.OperationOutputReceipt{}, fmt.Errorf("receive output bytes: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	if size != req.Output.ByteSize || hex.EncodeToString(hash.Sum(nil)) != req.Output.SHA256 {
		return domain.OperationOutputReceipt{}, ErrObjectMismatch
	}
	// io.LimitReader reaches EOF only beyond the declared size, so a final upstream
	// status error is observed for a complete-sized source before this point.
	downloaded := &Downloaded{File: file, Size: size, SHA256: req.Output.SHA256, MIMEType: req.Output.MIMEType}
	asset, renditions, err := s.archive.ArchiveOutput(ctx, work, downloaded)
	if err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	review, err := s.review.ReviewOutput(ctx, work, asset)
	if err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	if (review.Status != "passed" && review.Status != "skipped" && review.Status != "rejected") || len(review.Detail) == 0 || len(review.Detail) > 16384 || !json.Valid(review.Detail) {
		return domain.OperationOutputReceipt{}, domain.ErrOutputPublicationConflict
	}
	receipt, err := s.repo.CommitOutput(ctx, work, asset, renditions, review)
	if err != nil {
		return receipt, err
	}
	return receipt, validateOutputReceipt(req, receipt)
}
